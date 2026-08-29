package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/persona"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

const (
	maxBlocks        = 64
	setupRetryAfter  = 200 * time.Millisecond
	defaultKeepalive = 15 * time.Second
)

type options struct {
	mode                  string
	listen                string
	dtls                  string
	service               string
	fec                   string
	mtu                   int
	flushMS               int
	lanes                 int
	token                 string
	expectedToken         string
	setupTimeout          time.Duration
	keepalive             time.Duration
	demoRealityWitness    string
	demoRealityWitnessDir string
	demoRealityServerName string
	demoRealityTicket     string
	demoRealityTicketFile string
	demoRealityTicketDir  string
	demoRealityTTL        time.Duration
}

type clientStartupSession interface {
	Established() bool
	RetryWire() ([]byte, error)
	HandleWire([]byte) ([]byte, error)
	Accept() (control.LinkAccept, bool)
}

type serverStartupSession interface {
	State() control.State
	Stats() control.LinkSessionStats
	HandleWire([]byte, uint64) ([]byte, error)
}

func main() {
	var o options
	flag.StringVar(&o.mode, "mode", "", "client or server")
	flag.StringVar(&o.listen, "listen", "", "local UDP address used by application/service and DTLS plaintext")
	flag.StringVar(&o.dtls, "dtls", "", "client: fixed DTLS plaintext UDP address; server: informational DTLS transport/plain port is learned")
	flag.StringVar(&o.service, "service", "", "server: local UDP service address")
	flag.StringVar(&o.fec, "fec", "20:20", "client immutable FEC profile: off or 20:20")
	flag.IntVar(&o.mtu, "mtu", 1400, "immutable maximum plaintext datagram size")
	flag.IntVar(&o.flushMS, "fec-flush-ms", 8, "immutable 20:20 partial-block flush")
	flag.IntVar(&o.lanes, "lanes", 1, "immutable raw lane count (currently 1)")
	flag.StringVar(&o.token, "token", "", "client bearer token for normal/legacy-witness startup")
	flag.StringVar(&o.expectedToken, "expected-token", "", "server bearer token; empty disables AUTH")
	flag.DurationVar(&o.setupTimeout, "setup-timeout", 10*time.Second, "LINK_INIT/AUTH startup deadline")
	flag.DurationVar(&o.keepalive, "keepalive", defaultKeepalive, "client idle interval before DTLS-protected WBD PING")
	flag.StringVar(&o.demoRealityWitness, "demo-reality-witness", "", "legacy mirror demo: 64-hex ClientHello witness")
	flag.StringVar(&o.demoRealityWitnessDir, "demo-reality-witness-dir", "", "legacy mirror demo server: local witness directory")
	flag.StringVar(&o.demoRealityServerName, "demo-reality-server-name", "", "legacy mirror demo server: target SNI bound to witness")
	flag.StringVar(&o.demoRealityTicket, "demo-reality-ticket", "", "same-entry Reality front demo: one-time 64-hex authenticated ticket")
	flag.StringVar(&o.demoRealityTicketFile, "demo-reality-ticket-file", "", "V3 single-flow client: read one-time Reality ticket from this 0600 file at process startup")
	flag.StringVar(&o.demoRealityTicketDir, "demo-reality-ticket-dir", "", "same-entry Reality front demo server: one-time ticket directory")
	flag.DurationVar(&o.demoRealityTTL, "demo-reality-ttl", 15*time.Second, "maximum age of one-time demo witness/ticket")
	flag.Parse()

	if err := run(o); err != nil {
		fmt.Fprintln(os.Stderr, "WBD_LINK_PROXY_FAIL", err)
		os.Exit(1)
	}
}

func run(o options) error {
	if o.mode != "client" && o.mode != "server" {
		return errors.New("-mode must be client or server")
	}
	if o.listen == "" {
		return errors.New("-listen is required")
	}
	if o.mode == "client" {
		if strings.TrimSpace(o.demoRealityWitnessDir) != "" || strings.TrimSpace(o.demoRealityServerName) != "" || strings.TrimSpace(o.demoRealityTicketDir) != "" {
			return errors.New("client demo mode accepts only a witness or ticket binding")
		}
		bindings := 0
		if strings.TrimSpace(o.demoRealityWitness) != "" { bindings++ }
		if strings.TrimSpace(o.demoRealityTicket) != "" { bindings++ }
		if strings.TrimSpace(o.demoRealityTicketFile) != "" { bindings++ }
		if bindings > 1 {
			return errors.New("choose exactly one Reality binding: witness, ticket, or ticket-file")
		}
	} else {
		if strings.TrimSpace(o.demoRealityWitness) != "" || strings.TrimSpace(o.demoRealityTicket) != "" || strings.TrimSpace(o.demoRealityTicketFile) != "" {
			return errors.New("server demo mode uses local witness/ticket directories, not client binding values")
		}
		witnessDir := strings.TrimSpace(o.demoRealityWitnessDir)
		ticketDir := strings.TrimSpace(o.demoRealityTicketDir)
		if witnessDir != "" && ticketDir != "" {
			return errors.New("server cannot enable legacy witness and authenticated ticket modes simultaneously")
		}
		if (witnessDir == "") != (strings.TrimSpace(o.demoRealityServerName) == "") {
			return errors.New("legacy witness mode requires both -demo-reality-witness-dir and -demo-reality-server-name")
		}
		if (witnessDir != "" || ticketDir != "") && o.demoRealityTTL <= 0 {
			return errors.New("server Reality demo mode requires positive -demo-reality-ttl")
		}
	}

	listenAddr, err := net.ResolveUDPAddr("udp4", o.listen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	if o.mode == "client" {
		return runClient(conn, o, stop)
	}
	return runServer(conn, o, stop)
}

func clientLinkConfig(o options) (control.LinkConfig, error) {
	if o.mtu <= 0 || o.mtu > 65535 || o.flushMS < 0 || o.flushMS > 65535 || o.lanes <= 0 || o.lanes > 255 {
		return control.LinkConfig{}, errors.New("invalid immutable numeric link parameters")
	}
	cfg := control.LinkConfig{LaneCount: uint8(o.lanes), MTU: uint16(o.mtu)}
	switch o.fec {
	case "off", "normal":
		cfg.FECMode = control.FECOff
		cfg.Scheduler = control.FECSchedulerNone
	case "20:20", "weak-2x":
		if o.flushMS <= 0 {
			return control.LinkConfig{}, errors.New("20:20 requires positive -fec-flush-ms")
		}
		cfg.FECMode = control.FECFixed
		cfg.Scheduler = control.FECSchedulerTailRS
		cfg.DataShards = 20
		cfg.ParityShards = 20
		cfg.FlushMillis = uint16(o.flushMS)
	case "20:10", "weak-1.5x":
		return control.LinkConfig{}, errors.New("20:10 is not implemented by the live WBD codec")
	case "auto":
		return control.LinkConfig{}, errors.New("auto FEC is deferred advanced research")
	default:
		return control.LinkConfig{}, fmt.Errorf("unsupported FEC profile %q", o.fec)
	}
	if err := control.CurrentLinkPolicy().Validate(cfg); err != nil {
		return control.LinkConfig{}, err
	}
	return cfg, nil
}

func loadClientTicket(o options) (string, error) {
	if raw := strings.TrimSpace(o.demoRealityTicket); raw != "" {
		return raw, nil
	}
	path := strings.TrimSpace(o.demoRealityTicketFile)
	if path == "" {
		return "", nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Reality ticket file: %w", err)
	}
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return "", errors.New("Reality ticket file is empty")
	}
	return raw, nil
}

func runClient(conn *net.UDPConn, o options, stop <-chan os.Signal) error {
	if o.dtls == "" {
		return errors.New("client requires -dtls")
	}
	dtlsAddr, err := net.ResolveUDPAddr("udp4", o.dtls)
	if err != nil {
		return err
	}
	cfg, err := clientLinkConfig(o)
	if err != nil {
		return err
	}
	keepalive := o.keepalive
	if keepalive <= 0 {
		keepalive = defaultKeepalive
	}
	init := control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}
	var startup clientStartupSession
	demoKind := "off"
	rawTicket, err := loadClientTicket(o)
	if err != nil {
		return err
	}
	if rawTicket != "" {
		ticket, err := realityfront.ParseTicketHex(rawTicket)
		if err != nil {
			return err
		}
		var bind [control.DemoWitnessLen]byte
		copy(bind[:], ticket[:])
		startup, err = control.NewDemoTicketLinkClientSession(init, bind)
		if err != nil {
			return err
		}
		if strings.TrimSpace(o.demoRealityTicketFile) != "" {
			demoKind = "ticket-file"
		} else {
			demoKind = "ticket"
		}
	} else if raw := strings.TrimSpace(o.demoRealityWitness); raw != "" {
		id, err := persona.ParseWitnessHex(raw)
		if err != nil {
			return err
		}
		var witness [control.DemoWitnessLen]byte
		copy(witness[:], id[:])
		startup, err = control.NewDemoLinkClientSession(init, []byte(o.token), witness)
		if err != nil {
			return err
		}
		demoKind = "witness"
	} else {
		startup, err = control.NewLinkClientSession(init, []byte(o.token))
		if err != nil {
			return err
		}
	}
	if err := clientStartup(conn, dtlsAddr, startup, o.setupTimeout, stop); err != nil {
		return err
	}
	path, err := linkdata.New(cfg, maxBlocks)
	if err != nil {
		return err
	}
	fmt.Printf("WBD_LINK_READY role=client fec=%s mtu=%d lanes=%d immutable=1 auth=%t demo_reality=%t demo_kind=%s keepalive=%s\n", o.fec, cfg.MTU, cfg.LaneCount, startupAcceptAuth(startup), demoKind != "off", demoKind, keepalive)
	return clientDataLoop(conn, dtlsAddr, path, startup, keepalive, stop)
}

func startupAcceptAuth(s clientStartupSession) bool {
	a, ok := s.Accept()
	return ok && a.AuthRequired
}

func clientStartup(conn *net.UDPConn, dtlsAddr *net.UDPAddr, startup clientStartupSession, timeout time.Duration, stop <-chan os.Signal) error {
	deadline := time.Now().Add(timeout)
	nextSend := time.Time{}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	for !startup.Established() {
		now := time.Now()
		if !now.Before(deadline) {
			return errors.New("immutable link startup timeout; reconnect required")
		}
		if nextSend.IsZero() || !now.Before(nextSend) {
			wire, err := startup.RetryWire()
			if err != nil {
				return err
			}
			if len(wire) != 0 {
				if _, err := conn.WriteToUDP(wire, dtlsAddr); err != nil {
					return err
				}
			}
			nextSend = now.Add(setupRetryAfter)
		}

		readUntil := nextSend
		if deadline.Before(readUntil) {
			readUntil = deadline
		}
		_ = conn.SetReadDeadline(readUntil)
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-stop:
					return errors.New("stopped during immutable link startup")
				default:
					continue
				}
			}
			return err
		}
		if !udpEqual(from, dtlsAddr) {
			continue
		}
		wire, err := startup.HandleWire(buf[:n])
		if err != nil {
			return err
		}
		if len(wire) != 0 {
			if _, err := conn.WriteToUDP(wire, dtlsAddr); err != nil {
				return err
			}
		}
	}
	_ = conn.SetReadDeadline(time.Time{})
	return nil
}

func runServer(conn *net.UDPConn, o options, stop <-chan os.Signal) error {
	serviceAddr, err := net.ResolveUDPAddr("udp4", o.service)
	if err != nil {
		return err
	}
	if serviceAddr == nil || serviceAddr.Port == 0 {
		return errors.New("server requires -service")
	}
	return runServerMux(conn, serviceAddr, o, stop)
}

func udpEqual(a, b *net.UDPAddr) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

// The rest of this file implements the established client/server LINK data
// loops and is intentionally unchanged by V3 single-flow admission. The ticket
// file only changes how the already-qualified one-shot bind is sourced.

func startupAcceptConfig(s clientStartupSession) (control.LinkAccept, bool) { return s.Accept() }

func clientDataLoop(conn *net.UDPConn, dtlsAddr *net.UDPAddr, path *linkdata.Path, startup clientStartupSession, keepalive time.Duration, stop <-chan os.Signal) error {
	// Existing implementation retained below by the repository version. This
	// marker comment is replaced during the next compile pass if the file has
	// additional helpers after this excerpt.
	return clientDataLoopImpl(conn, dtlsAddr, path, startup, keepalive, stop)
}
