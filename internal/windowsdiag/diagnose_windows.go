//go:build windows

package windowsdiag

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

type Result struct {
	LogPath string
}

type logger struct {
	mu       sync.Mutex
	file     *os.File
	secrets  []string
	hex64    *regexp.Regexp
}

func Run(profile windowsruntime.Profile, logPath string) (result Result, retErr error) {
	if strings.TrimSpace(logPath) == "" {
		logPath = filepath.Join(os.TempDir(), "WBD", "support-"+time.Now().Format("20060102-150405")+".jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(logPath), 0o700); err != nil {
		return result, err
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return result, err
	}
	l := &logger{
		file:    f,
		secrets: []string{profile.Password, profile.RouteKey},
		hex64:   regexp.MustCompile(`(?i)\b[0-9a-f]{64}\b`),
	}
	result.LogPath = logPath
	defer func() {
		_ = f.Sync()
		_ = f.Close()
	}()
	l.event("self_test_start", map[string]any{
		"route_mode": profile.RouteMode,
		"dns_mode":   profile.DNSMode,
		"fec":        profile.FEC,
		"mtu":        profile.MTU,
		"server":     shortHash(profile.ServerRaw),
	})

	if err := profile.Validate(); err != nil {
		l.event("profile_fail", map[string]any{"error": l.redact(err.Error())})
		return result, err
	}
	l.event("profile_pass", nil)
	if err := windowsruntime.ValidateRoutingAssets(profile); err != nil {
		l.event("routing_assets_fail", map[string]any{"error": l.redact(err.Error())})
		return result, err
	}
	l.event("routing_assets_pass", nil)

	// Use the same Controller startup path as the product GUI. The wrapper only
	// adds redacted diagnostic events; it does not pre-discover the underlay. This
	// is important because Controller first performs idempotent stale route/IPv6
	// cleanup, then discovers the physical underlay, then opens the sole public
	// FakeTCP flow. Diagnostics must not observe a different startup order.
	runner := &loggingRunner{log: l}
	discoverer := &loggingDiscoverer{log: l, inner: windowsruntime.PowerShellUnderlayDiscoverer{}}
	controller := windowsruntime.NewController(runner, discoverer, nil)
	started := time.Now()
	if err := controller.Connect(profile); err != nil {
		l.event("connect_fail", map[string]any{"elapsed_ms": time.Since(started).Milliseconds(), "error": l.redact(err.Error())})
		return result, err
	}
	connected := true
	defer func() {
		if !connected {
			return
		}
		if err := controller.Disconnect(); err != nil {
			l.event("cleanup_fail", map[string]any{"error": l.redact(err.Error())})
			retErr = errors.Join(retErr, fmt.Errorf("automatic self-test cleanup: %w", err))
		} else {
			l.event("cleanup_pass", map[string]any{"routes_removed": true, "runtime_stopped": true})
		}
	}()
	l.event("connect_pass", map[string]any{"elapsed_ms": time.Since(started).Milliseconds()})
	logRouteState(l, profile.RouteState)

	var probeErrs []error
	if err := probeSystemDNS(); err != nil {
		probeErrs = append(probeErrs, fmt.Errorf("system DNS: %w", err))
		l.event("probe_system_dns_fail", map[string]any{"error": l.redact(err.Error())})
	} else {
		l.event("probe_system_dns_pass", nil)
	}
	udpServer, tcpTarget := "1.1.1.1:53", "1.1.1.1:443"
	if profile.RouteMode == windowsruntime.RouteChina {
		udpServer, tcpTarget = "223.5.5.5:53", "www.baidu.com:443"
	}
	if err := probeUDPDNS(udpServer, "cloudflare.com"); err != nil {
		probeErrs = append(probeErrs, fmt.Errorf("UDP DNS: %w", err))
		l.event("probe_udp_fail", map[string]any{"target": udpServer, "error": l.redact(err.Error())})
	} else {
		l.event("probe_udp_pass", map[string]any{"target": udpServer})
	}
	if err := probeTCP(tcpTarget); err != nil {
		probeErrs = append(probeErrs, fmt.Errorf("TCP: %w", err))
		l.event("probe_tcp_fail", map[string]any{"target": tcpTarget, "error": l.redact(err.Error())})
	} else {
		l.event("probe_tcp_pass", map[string]any{"target": tcpTarget})
	}

	if err := controller.Disconnect(); err != nil {
		l.event("cleanup_fail", map[string]any{"error": l.redact(err.Error())})
		return result, errors.Join(errors.Join(probeErrs...), fmt.Errorf("automatic self-test cleanup: %w", err))
	}
	connected = false
	l.event("cleanup_pass", map[string]any{"routes_removed": true, "runtime_stopped": true})
	if err := errors.Join(probeErrs...); err != nil {
		l.event("self_test_fail", map[string]any{"error": l.redact(err.Error())})
		return result, err
	}
	l.event("self_test_pass", map[string]any{"dns": true, "udp": true, "tcp": true, "cleanup": true})
	return result, nil
}

type loggingDiscoverer struct {
	log   *logger
	inner windowsruntime.PowerShellUnderlayDiscoverer
}

func (d *loggingDiscoverer) Preflight(profile windowsruntime.Profile) error {
	if err := d.inner.Preflight(profile); err != nil {
		d.log.event("dependency_preflight_fail", map[string]any{"error": d.log.redact(err.Error())})
		return err
	}
	d.log.event("dependency_preflight_pass", map[string]any{"npcap": "ready"})
	return nil
}

func (d *loggingDiscoverer) Discover(profile windowsruntime.Profile) (windowsruntime.Underlay, error) {
	underlay, err := d.inner.Discover(profile)
	if err != nil {
		d.log.event("underlay_fail", map[string]any{"error": d.log.redact(err.Error())})
		return windowsruntime.Underlay{}, err
	}
	d.log.event("underlay_pass", map[string]any{
		"source_ip":       underlay.SourceIP,
		"packet_device":   shortHash(underlay.PacketDevice),
		"source_mac":      shortHash(underlay.SourceMAC),
		"next_hop_mac":    shortHash(underlay.NextHopMAC),
	})
	return underlay, nil
}

type loggingRunner struct{ log *logger }

func (r *loggingRunner) Run(command windowsruntime.Command) error {
	start := time.Now()
	r.log.event("command_run", map[string]any{"name": command.Name})
	cmd := exec.Command(command.Path, command.Args...)
	out := newLineWriter(r.log, command.Name, "stdout")
	errOut := newLineWriter(r.log, command.Name, "stderr")
	cmd.Stdout, cmd.Stderr = out, errOut
	err := cmd.Run()
	out.Flush()
	errOut.Flush()
	fields := map[string]any{"name": command.Name, "elapsed_ms": time.Since(start).Milliseconds()}
	if err != nil {
		fields["error"] = r.log.redact(err.Error())
		r.log.event("command_fail", fields)
	} else {
		r.log.event("command_pass", fields)
	}
	return err
}

func (r *loggingRunner) Start(command windowsruntime.Command) (windowsruntime.Process, error) {
	r.log.event("command_start", map[string]any{"name": command.Name})
	cmd := exec.Command(command.Path, command.Args...)
	out := newLineWriter(r.log, command.Name, "stdout")
	errOut := newLineWriter(r.log, command.Name, "stderr")
	cmd.Stdout, cmd.Stderr = out, errOut
	if err := cmd.Start(); err != nil {
		r.log.event("command_start_fail", map[string]any{"name": command.Name, "error": r.log.redact(err.Error())})
		return nil, err
	}
	return &loggingProcess{name: command.Name, cmd: cmd, log: r.log, stdout: out, stderr: errOut}, nil
}

type loggingProcess struct {
	name           string
	cmd            *exec.Cmd
	log            *logger
	stdout, stderr *lineWriter
}

func (p *loggingProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	cmd := p.cmd
	killErr := cmd.Process.Kill()
	waitErr := cmd.Wait()
	// A successfully attempted Wait consumes exec.Cmd's one-shot wait state.
	// Clear our reference so any later cleanup call is a true idempotent no-op
	// instead of issuing a second Wait and reporting "Wait was already called".
	p.cmd = nil
	p.stdout.Flush()
	p.stderr.Flush()
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	if _, ok := waitErr.(*exec.ExitError); ok && killErr == nil {
		waitErr = nil
	}
	err := errors.Join(killErr, waitErr)
	if err != nil {
		p.log.event("command_stop_fail", map[string]any{"name": p.name, "error": p.log.redact(err.Error())})
	} else {
		p.log.event("command_stop_pass", map[string]any{"name": p.name})
	}
	return err
}

type lineWriter struct {
	mu      sync.Mutex
	log     *logger
	command string
	stream  string
	buf     []byte
}

func newLineWriter(l *logger, command, stream string) *lineWriter {
	return &lineWriter{log: l, command: command, stream: stream}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := strings.IndexByte(string(w.buf), '\n')
		if i < 0 {
			break
		}
		w.emit(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
	}
	return len(p), nil
}

func (w *lineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.emit(strings.TrimRight(string(w.buf), "\r\n"))
		w.buf = nil
	}
}

func (w *lineWriter) emit(text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	w.log.event("child_log", map[string]any{"command": w.command, "stream": w.stream, "text": w.log.redact(text)})
}

func (l *logger) redact(s string) string {
	for _, secret := range l.secrets {
		if strings.TrimSpace(secret) != "" {
			s = strings.ReplaceAll(s, secret, "<redacted>")
		}
	}
	return l.hex64.ReplaceAllString(s, "<redacted-64hex>")
}

func (l *logger) event(kind string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	row := map[string]any{"time": time.Now().UTC().Format(time.RFC3339Nano), "event": kind}
	for k, v := range fields {
		row[k] = v
	}
	b, _ := json.Marshal(row)
	_, _ = l.file.Write(append(b, '\n'))
}

func logRouteState(l *logger, path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		l.event("route_state_missing", map[string]any{"error": err.Error()})
		return
	}
	var state map[string]any
	if err := json.Unmarshal(b, &state); err != nil {
		l.event("route_state_invalid", map[string]any{"error": err.Error()})
		return
	}
	count := func(name string) int {
		if a, ok := state[name].([]any); ok { return len(a) }
		return 0
	}
	l.event("route_state", map[string]any{
		"schema": state["Schema"],
		"capture_routes": count("CaptureRoutes"),
		"direct_routes": count("DirectRoutes"),
		"underlay_routes": count("UnderlayRoutes"),
		"dns_configured": state["DNSConfigured"],
	})
}

func probeSystemDNS() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	answers, err := net.DefaultResolver.LookupHost(ctx, "cloudflare.com")
	if err != nil { return err }
	if len(answers) == 0 { return errors.New("no DNS answers") }
	return nil
}

func probeTCP(target string) error {
	conn, err := net.DialTimeout("tcp4", target, 5*time.Second)
	if err != nil { return err }
	return conn.Close()
}

func probeUDPDNS(server, name string) error {
	addr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil { return err }
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil { return err }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	query, id, err := dnsQuery(name)
	if err != nil { return err }
	if _, err := conn.Write(query); err != nil { return err }
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil { return err }
	if n < 12 || binary.BigEndian.Uint16(buf[:2]) != id || binary.BigEndian.Uint16(buf[6:8]) == 0 {
		return errors.New("invalid or empty DNS response")
	}
	return nil
}

func dnsQuery(name string) ([]byte, uint16, error) {
	var idBytes [2]byte
	if _, err := io.ReadFull(rand.Reader, idBytes[:]); err != nil { return nil, 0, err }
	id := binary.BigEndian.Uint16(idBytes[:])
	buf := make([]byte, 12)
	binary.BigEndian.PutUint16(buf[0:2], id)
	binary.BigEndian.PutUint16(buf[2:4], 0x0100)
	binary.BigEndian.PutUint16(buf[4:6], 1)
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 { return nil, 0, errors.New("invalid DNS name") }
		buf = append(buf, byte(len(label)))
		buf = append(buf, label...)
	}
	buf = append(buf, 0, 0, 1, 0, 1)
	return buf, id, nil
}

func shortHash(s string) string {
	if s == "" { return "" }
	// A deterministic non-secret identifier is enough to correlate two support
	// logs without exposing a local adapter GUID/MAC/server endpoint.
	var h uint64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return fmt.Sprintf("fnv64:%016x", h)
}

var _ io.Writer = (*lineWriter)(nil)
var _ = bufio.ErrInvalidUnreadByte
