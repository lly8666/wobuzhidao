//go:build linux

package main

import (
	"context"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

const (
	defaultTransitPrefix = "198.18.240.0/24"
	defaultInnerServer   = "10.66.0.1/30"
	defaultIdleTimeout   = 2 * time.Minute
	defaultMTU           = 1400
	defaultMaxSessions   = 64
	vrfTableBase         = 12000
	conntrackZoneBase    = 2000
)

type config struct {
	listen        string
	firewall      string
	backend       string
	firewallState string
	nftForward    string
	transitPrefix string
	innerServer   string
	idleTimeout   time.Duration
	mtu           int
	maxSessions   int
}

type gatewaySession struct {
	key       string
	peer      *net.UDPAddr
	slot      int
	tun       *tunnel.TUN
	tunIf     string
	vrfIf     string
	hostIf    string
	vrfName   string
	hostIP    netip.Addr
	transitIP netip.Addr
	table     int
	zone      int

	activityMu sync.Mutex
	last       time.Time
}

func (s *gatewaySession) touch(now time.Time) {
	s.activityMu.Lock()
	s.last = now
	s.activityMu.Unlock()
}

func (s *gatewaySession) idleFor(now time.Time) time.Duration {
	s.activityMu.Lock()
	last := s.last
	s.activityMu.Unlock()
	if last.IsZero() || now.Before(last) {
		return 0
	}
	return now.Sub(last)
}

type gateway struct {
	cfg         config
	conn        *net.UDPConn
	transit     netip.Prefix
	inner       netip.Prefix
	innerServer netip.Addr

	mu       sync.Mutex
	sessions map[string]*gatewaySession
	slots    []bool
}

func main() {
	var c config
	flag.StringVar(&c.listen, "listen", "127.0.0.1:49100", "UDP service address receiving M6A raw-IP frames from LINK mux")
	flag.StringVar(&c.firewall, "firewall-helper", "", "path to linux_ip_gateway_firewall.sh")
	flag.StringVar(&c.backend, "backend", "auto", "netfilter backend: auto, nft or iptables")
	flag.StringVar(&c.firewallState, "firewall-state", "/run/wbd/ip-gateway-firewall.state", "WBD-owned netfilter state path")
	flag.StringVar(&c.nftForward, "nft-forward", "", "optional existing nft forward chain FAMILY:TABLE:CHAIN")
	flag.StringVar(&c.transitPrefix, "transit-prefix", defaultTransitPrefix, "host<->VRF transit prefix; four addresses reserved per session")
	flag.StringVar(&c.innerServer, "inner-server", defaultInnerServer, "server TUN IPv4 CIDR; clients use the other /30 host address")
	flag.DurationVar(&c.idleTimeout, "idle-timeout", defaultIdleTimeout, "raw-IP backend idle cleanup timeout")
	flag.IntVar(&c.mtu, "mtu", defaultMTU, "TUN MTU")
	flag.IntVar(&c.maxSessions, "max-sessions", defaultMaxSessions, "maximum simultaneous raw-IP sessions")
	flag.Parse()

	g, err := newGateway(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_IP_GATEWAY_FAIL", err)
		os.Exit(1)
	}
	defer g.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_IP_GATEWAY_READY listen=%s max_sessions=%d inner=%s transit=%s isolation=vrf+conntrack-zone\n", g.conn.LocalAddr(), c.maxSessions, g.inner, g.transit)
	if err := g.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(os.Stderr, "WBD_IP_GATEWAY_FAIL", err)
		os.Exit(1)
	}
}

func newGateway(c config) (*gateway, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("raw-IP gateway requires root/CAP_NET_ADMIN")
	}
	if c.firewall == "" {
		return nil, errors.New("-firewall-helper is required")
	}
	if c.backend != "auto" && c.backend != "nft" && c.backend != "iptables" {
		return nil, fmt.Errorf("invalid backend %q", c.backend)
	}
	if c.idleTimeout <= 0 || c.mtu < 576 || c.mtu > dataplane.MaxPacketLen || c.maxSessions <= 0 {
		return nil, errors.New("positive idle/max-sessions and MTU 576..9000 are required")
	}
	transit, err := netip.ParsePrefix(c.transitPrefix)
	if err != nil || !transit.Addr().Is4() {
		return nil, errors.New("transit-prefix must be IPv4 CIDR")
	}
	transit = transit.Masked()
	if capacityForPrefix(transit) < c.maxSessions {
		return nil, fmt.Errorf("transit prefix %s has capacity %d sessions, need %d", transit, capacityForPrefix(transit), c.maxSessions)
	}
	innerServer, err := netip.ParsePrefix(c.innerServer)
	if err != nil || !innerServer.Addr().Is4() || innerServer.Bits() != 30 {
		return nil, errors.New("inner-server must be an IPv4 /30")
	}
	inner := innerServer.Masked()
	listenAddr, err := net.ResolveUDPAddr("udp4", c.listen)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)
	g := &gateway{
		cfg: c, conn: conn, transit: transit, inner: inner, innerServer: innerServer.Addr(),
		sessions: make(map[string]*gatewaySession), slots: make([]bool, c.maxSessions),
	}
	if err := g.resetOwnedState(); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if err := g.runFirewall("apply", nil); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return g, nil
}

func (g *gateway) Run(ctx context.Context) error {
	buf := make([]byte, 65535)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		if err := g.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			return err
		}
		n, peer, err := g.conn.ReadFromUDP(buf)
		now := time.Now()
		if err == nil {
			if err := g.handleFrame(peer, buf[:n], now); err != nil {
				fmt.Fprintf(os.Stderr, "WBD_IP_GATEWAY_DROP peer=%s err=%v\n", peer, err)
			}
		} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			g.expire(now)
		default:
		}
	}
}

func (g *gateway) handleFrame(peer *net.UDPAddr, frame []byte, now time.Time) error {
	packet, err := dataplane.UnmarshalIP(frame)
	if err != nil {
		return err
	}
	if packet[0]>>4 != 4 {
		return errors.New("IPv6 raw-IP gateway is disabled while Windows IPv6 kill-switch is active")
	}
	key := peer.String()
	s, err := g.getOrCreateSession(key, peer, now)
	if err != nil {
		return err
	}
	n, err := s.tun.WritePacket(packet)
	if err != nil {
		g.dropSession(key, "tun-write-error")
		return err
	}
	if n != len(packet) {
		return fmt.Errorf("short TUN write %d/%d", n, len(packet))
	}
	s.touch(now)
	return nil
}

func (g *gateway) getOrCreateSession(key string, peer *net.UDPAddr, now time.Time) (*gatewaySession, error) {
	g.mu.Lock()
	if s := g.sessions[key]; s != nil {
		g.mu.Unlock()
		return s, nil
	}
	slot := -1
	for i, used := range g.slots {
		if !used {
			slot = i
			g.slots[i] = true
			break
		}
	}
	g.mu.Unlock()
	if slot < 0 {
		return nil, errors.New("raw-IP gateway session capacity reached")
	}

	s, err := g.createSession(key, peer, slot, now)
	if err != nil {
		g.mu.Lock()
		g.slots[slot] = false
		g.mu.Unlock()
		return nil, err
	}
	g.mu.Lock()
	if existing := g.sessions[key]; existing != nil {
		g.slots[slot] = false
		g.mu.Unlock()
		g.destroySession(s)
		return existing, nil
	}
	g.sessions[key] = s
	g.mu.Unlock()
	go g.tunReadLoop(s)
	fmt.Printf("WBD_IP_GATEWAY_SESSION_READY peer=%s slot=%d tun=%s vrf=%s zone=%d transit=%s inner=%s\n", key, slot, s.tunIf, s.vrfName, s.zone, s.transitIP, g.innerServer)
	return s, nil
}

func (g *gateway) createSession(key string, peer *net.UDPAddr, slot int, now time.Time) (*gatewaySession, error) {
	hostIP, transitIP, err := transitPair(g.transit, slot)
	if err != nil {
		return nil, err
	}
	tunIf := fmt.Sprintf("wt%02d", slot)
	vrfName := fmt.Sprintf("wv%02d", slot)
	hostIf := fmt.Sprintf("wh%02d", slot)
	vrfIf := fmt.Sprintf("we%02d", slot)
	table := vrfTableBase + slot
	zone := conntrackZoneBase + slot

	tunDev, err := tunnel.OpenTUN(tunIf)
	if err != nil {
		return nil, err
	}
	s := &gatewaySession{
		key: key, peer: cloneUDPAddr(peer), slot: slot, tun: tunDev,
		tunIf: tunIf, vrfIf: vrfIf, hostIf: hostIf, vrfName: vrfName,
		hostIP: hostIP, transitIP: transitIP, table: table, zone: zone, last: now,
	}
	failed := true
	defer func() {
		if failed {
			g.destroySession(s)
		}
	}()

	commands := [][]string{
		{"ip", "link", "add", vrfName, "type", "vrf", "table", strconv.Itoa(table)},
		{"ip", "link", "set", vrfName, "up"},
		{"ip", "link", "set", tunIf, "master", vrfName},
		{"ip", "link", "set", tunIf, "mtu", strconv.Itoa(g.cfg.mtu), "up"},
		{"ip", "addr", "add", g.cfg.innerServer, "dev", tunIf},
		{"ip", "link", "add", hostIf, "type", "veth", "peer", "name", vrfIf},
		{"ip", "link", "set", vrfIf, "master", vrfName},
		{"ip", "addr", "add", hostIP.String() + "/30", "dev", hostIf},
		{"ip", "addr", "add", transitIP.String() + "/30", "dev", vrfIf},
		{"ip", "link", "set", hostIf, "up"},
		{"ip", "link", "set", vrfIf, "up"},
		{"ip", "route", "replace", "table", strconv.Itoa(table), "default", "via", hostIP.String(), "dev", vrfIf},
	}
	for _, cmd := range commands {
		if err := run(cmd[0], cmd[1:]...); err != nil {
			return nil, err
		}
	}
	for _, ifname := range []string{tunIf, hostIf, vrfIf} {
		_ = run("sysctl", "-q", "-w", "net.ipv4.conf."+ifname+".rp_filter=0")
		_ = run("sysctl", "-q", "-w", "net.ipv4.conf."+ifname+".forwarding=1")
	}
	if err := g.runFirewall("session-add", s); err != nil {
		return nil, err
	}
	failed = false
	return s, nil
}

func (g *gateway) tunReadLoop(s *gatewaySession) {
	buf := make([]byte, dataplane.MaxPacketLen)
	for {
		n, err := s.tun.ReadPacket(buf)
		if err != nil {
			return
		}
		wire, err := dataplane.MarshalIP(buf[:n])
		if err != nil {
			fmt.Fprintf(os.Stderr, "WBD_IP_GATEWAY_TUN_DROP peer=%s err=%v\n", s.key, err)
			continue
		}
		if _, err := g.conn.WriteToUDP(wire, s.peer); err != nil {
			return
		}
		s.touch(time.Now())
	}
}

func (g *gateway) expire(now time.Time) {
	var stale []string
	g.mu.Lock()
	for key, s := range g.sessions {
		if s.idleFor(now) >= g.cfg.idleTimeout {
			stale = append(stale, key)
		}
	}
	g.mu.Unlock()
	for _, key := range stale {
		g.dropSession(key, "idle")
	}
}

func (g *gateway) dropSession(key, reason string) {
	g.mu.Lock()
	s := g.sessions[key]
	if s != nil {
		delete(g.sessions, key)
		g.slots[s.slot] = false
	}
	g.mu.Unlock()
	if s == nil {
		return
	}
	g.destroySession(s)
	fmt.Printf("WBD_IP_GATEWAY_SESSION_CLEAN peer=%s slot=%d reason=%s\n", key, s.slot, reason)
}

func (g *gateway) destroySession(s *gatewaySession) {
	if s == nil {
		return
	}
	_ = g.runFirewall("session-del", s)
	if s.tun != nil {
		_ = s.tun.Close()
	}
	_ = runIgnore("ip", "link", "del", s.hostIf)
	_ = runIgnore("ip", "link", "del", s.vrfName)
}

func (g *gateway) resetOwnedState() error {
	for slot := 0; slot < g.cfg.maxSessions; slot++ {
		hostIP, transitIP, err := transitPair(g.transit, slot)
		if err != nil {
			return err
		}
		s := &gatewaySession{
			slot: slot, tunIf: fmt.Sprintf("wt%02d", slot), vrfIf: fmt.Sprintf("we%02d", slot),
			hostIf: fmt.Sprintf("wh%02d", slot), vrfName: fmt.Sprintf("wv%02d", slot),
			hostIP: hostIP, transitIP: transitIP, table: vrfTableBase + slot, zone: conntrackZoneBase + slot,
		}
		_ = g.runFirewall("session-del", s)
		_ = runIgnore("ip", "link", "del", s.hostIf)
		_ = runIgnore("ip", "link", "del", s.vrfName)
		_ = runIgnore("ip", "link", "del", s.tunIf)
	}
	_ = g.runFirewall("cleanup", nil)
	return nil
}

func (g *gateway) runFirewall(action string, s *gatewaySession) error {
	args := []string{action, "--backend", g.cfg.backend, "--state", g.cfg.firewallState,
		"--transit-prefix", g.transit.String(), "--inner-prefix", g.inner.String()}
	if g.cfg.nftForward != "" {
		args = append(args, "--nft-forward", g.cfg.nftForward)
	}
	if s != nil {
		args = append(args,
			"--slot", strconv.Itoa(s.slot), "--tun-if", s.tunIf, "--vrf-if", s.vrfIf,
			"--transit-ip", s.transitIP.String(), "--zone", strconv.Itoa(s.zone))
	}
	cmd := exec.Command(g.cfg.firewall, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("firewall helper %s: %w: %s", action, err, string(out))
	}
	return nil
}

func (g *gateway) Close() {
	g.mu.Lock()
	keys := make([]string, 0, len(g.sessions))
	for key := range g.sessions {
		keys = append(keys, key)
	}
	g.mu.Unlock()
	for _, key := range keys {
		g.dropSession(key, "shutdown")
	}
	_ = g.runFirewall("cleanup", nil)
	if g.conn != nil {
		_ = g.conn.Close()
	}
}

func capacityForPrefix(prefix netip.Prefix) int {
	if !prefix.Addr().Is4() || prefix.Bits() > 30 {
		return 0
	}
	addresses := uint64(1) << uint(32-prefix.Bits())
	return int(addresses / 4)
}

func transitPair(prefix netip.Prefix, slot int) (netip.Addr, netip.Addr, error) {
	if slot < 0 || slot >= capacityForPrefix(prefix) {
		return netip.Addr{}, netip.Addr{}, errors.New("transit slot out of range")
	}
	baseBytes := prefix.Masked().Addr().As4()
	base := binary.BigEndian.Uint32(baseBytes[:])
	start := base + uint32(slot*4)
	var hostBytes, edgeBytes [4]byte
	binary.BigEndian.PutUint32(hostBytes[:], start+1)
	binary.BigEndian.PutUint32(edgeBytes[:], start+2)
	return netip.AddrFrom4(hostBytes), netip.AddrFrom4(edgeBytes), nil
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v: %w: %s", name, args, err, string(out))
	}
	return nil
}

func runIgnore(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	_, err := cmd.CombinedOutput()
	return err
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone}
}
