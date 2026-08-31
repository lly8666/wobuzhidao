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
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

const (
	defaultLeasePrefix = "10.66.0.0/16"
	defaultIdleTimeout = 2 * time.Minute
	defaultMTU         = 1400
	defaultMaxSessions = 64
	defaultTunIf       = "wbdg0"
)

type config struct {
	listen        string
	firewall      string
	backend       string
	firewallState string
	nftForward    string
	leasePrefix   string
	tunIf         string
	idleTimeout   time.Duration
	mtu           int
	maxSessions   int
}

type sessionCounters struct {
	rawRx atomic.Uint64
	rawTx atomic.Uint64
	tunTx atomic.Uint64
	tunRx atomic.Uint64
	drop  atomic.Uint64
}

type sharedSession struct {
	key      string
	tunnelID logicaltunnel.TunnelID
	lease    netip.Addr
	peer     *net.UDPAddr

	activityMu sync.Mutex
	last       time.Time
	counters   sessionCounters
	rawRxFirst sync.Once
	rawTxFirst sync.Once
}

func (s *sharedSession) touch(now time.Time) {
	s.activityMu.Lock()
	s.last = now
	s.activityMu.Unlock()
}

func (s *sharedSession) idleFor(now time.Time) time.Duration {
	s.activityMu.Lock()
	last := s.last
	s.activityMu.Unlock()
	if last.IsZero() || now.Before(last) {
		return 0
	}
	return now.Sub(last)
}

func tunnelIDPrefix(id logicaltunnel.TunnelID) string {
	raw := string(id)
	if len(raw) > 8 {
		return raw[:8]
	}
	return raw
}

type sharedGateway struct {
	cfg    config
	conn   *net.UDPConn
	tun    *tunnel.TUN
	leases netip.Prefix

	mu       sync.Mutex
	byPeer   map[string]*sharedSession
	byLease  map[netip.Addr]*sharedSession
	byTunnel map[logicaltunnel.TunnelID]*sharedSession
}

func main() {
	var c config
	flag.StringVar(&c.listen, "listen", "127.0.0.1:49100", "UDP service address receiving M6A raw-IP frames from LINK mux")
	flag.StringVar(&c.firewall, "firewall-helper", "", "path to linux_shared_tun_firewall.sh")
	flag.StringVar(&c.backend, "backend", "auto", "netfilter backend: auto, nft or iptables")
	flag.StringVar(&c.firewallState, "firewall-state", "/run/wbd/shared-tun-firewall.state", "WBD-owned shared-TUN netfilter state path")
	flag.StringVar(&c.nftForward, "nft-forward", "", "optional existing nft forward chain FAMILY:TABLE:CHAIN")
	flag.StringVar(&c.leasePrefix, "lease-prefix", defaultLeasePrefix, "Logical Tunnel IPv4 lease pool routed through the shared TUN")
	flag.StringVar(&c.tunIf, "tun-if", defaultTunIf, "shared root-namespace TUN interface")
	flag.DurationVar(&c.idleTimeout, "idle-timeout", defaultIdleTimeout, "backend peer idle cleanup timeout")
	flag.IntVar(&c.mtu, "mtu", defaultMTU, "shared TUN MTU")
	flag.IntVar(&c.maxSessions, "max-sessions", defaultMaxSessions, "maximum simultaneous Logical Tunnel sessions")
	flag.Parse()

	g, err := newSharedGateway(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_SHARED_TUN_GATEWAY_FAIL", err)
		os.Exit(1)
	}
	defer g.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_SHARED_TUN_GATEWAY_READY listen=%s tun=%s lease_prefix=%s max_sessions=%d isolation=shared_tun nat=host logical_tunnel=v2\n", g.conn.LocalAddr(), g.tun.Name(), g.leases, c.maxSessions)
	if err := g.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(os.Stderr, "WBD_SHARED_TUN_GATEWAY_FAIL", err)
		os.Exit(1)
	}
}

func newSharedGateway(c config) (*sharedGateway, error) {
	if os.Geteuid() != 0 {
		return nil, errors.New("shared-TUN gateway requires root/CAP_NET_ADMIN")
	}
	if strings.TrimSpace(c.firewall) == "" {
		return nil, errors.New("-firewall-helper is required")
	}
	if c.backend != "auto" && c.backend != "nft" && c.backend != "iptables" {
		return nil, fmt.Errorf("invalid backend %q", c.backend)
	}
	if c.idleTimeout <= 0 || c.mtu < 576 || c.mtu > dataplane.MaxPacketLen || c.maxSessions <= 0 {
		return nil, errors.New("positive idle/max-sessions and MTU 576..9000 are required")
	}
	leases, err := netip.ParsePrefix(c.leasePrefix)
	if err != nil || !leases.Addr().Is4() || leases.Bits() > 30 {
		return nil, errors.New("lease-prefix must be an IPv4 CIDR with at least four addresses")
	}
	leases = leases.Masked()
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

	_ = runIgnore("ip", "link", "del", c.tunIf)
	tunDev, err := tunnel.OpenTUN(c.tunIf)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	failed := true
	defer func() {
		if failed {
			_ = tunDev.Close()
			_ = conn.Close()
		}
	}()
	if err := run("ip", "link", "set", tunDev.Name(), "mtu", strconv.Itoa(c.mtu), "up"); err != nil {
		return nil, err
	}
	if err := run("ip", "route", "replace", leases.String(), "dev", tunDev.Name()); err != nil {
		return nil, err
	}
	_ = run("sysctl", "-q", "-w", "net.ipv4.conf."+tunDev.Name()+".rp_filter=0")
	g := &sharedGateway{
		cfg: c, conn: conn, tun: tunDev, leases: leases,
		byPeer: make(map[string]*sharedSession), byLease: make(map[netip.Addr]*sharedSession), byTunnel: make(map[logicaltunnel.TunnelID]*sharedSession),
	}
	_ = g.runFirewall("cleanup")
	if err := g.runFirewall("apply"); err != nil {
		return nil, err
	}
	failed = false
	return g, nil
}

func (g *sharedGateway) Run(ctx context.Context) error {
	go g.tunReadLoop()
	buf := make([]byte, 65535)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := g.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			return err
		}
		n, peer, err := g.conn.ReadFromUDP(buf)
		now := time.Now()
		if err == nil {
			if err := g.handleFrame(peer, append([]byte(nil), buf[:n]...), now); err != nil {
				fmt.Fprintf(os.Stderr, "WBD_SHARED_TUN_DROP peer=%s err=%v\n", peer, err)
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

func (g *sharedGateway) handleFrame(peer *net.UDPAddr, frame []byte, now time.Time) error {
	key := peer.String()
	if meta, ok := rawipbackend.UnmarshalTunnelMeta(frame); ok {
		return g.register(key, peer, meta, now)
	}
	if _, ok := rawipbackend.UnmarshalSessionMeta(frame); ok {
		return errors.New("legacy v1 raw-IP metadata is not supported by shared-TUN v2 gateway")
	}
	packet, err := dataplane.UnmarshalIP(frame)
	if err != nil {
		return err
	}
	src, _, err := ipv4SourceDest(packet)
	if err != nil {
		return err
	}
	g.mu.Lock()
	s := g.byPeer[key]
	g.mu.Unlock()
	if s == nil {
		return errors.New("Logical Tunnel metadata is required before raw-IP data")
	}
	if src != s.lease {
		s.counters.drop.Add(1)
		return fmt.Errorf("raw-IP source %s does not match Logical Tunnel lease %s", src, s.lease)
	}
	s.counters.rawRx.Add(1)
	s.rawRxFirst.Do(func() {
		fmt.Printf("WBD_SHARED_RAWIP_RX_FIRST tunnel_id_prefix=%s address4=%s bytes=%d\n", tunnelIDPrefix(s.tunnelID), s.lease, len(packet))
	})
	n, err := g.tun.WritePacket(packet)
	if err != nil {
		s.counters.drop.Add(1)
		return err
	}
	if n != len(packet) {
		s.counters.drop.Add(1)
		return fmt.Errorf("short shared TUN write %d/%d", n, len(packet))
	}
	s.counters.tunTx.Add(1)
	s.touch(now)
	return nil
}

func (g *sharedGateway) register(key string, peer *net.UDPAddr, meta rawipbackend.TunnelMeta, now time.Time) error {
	if !meta.Address4.Is4() || !g.leases.Contains(meta.Address4) {
		return fmt.Errorf("Logical Tunnel lease %s is outside shared lease prefix %s", meta.Address4, g.leases)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing := g.byPeer[key]; existing != nil {
		if existing.tunnelID != meta.TunnelID || existing.lease != meta.Address4 {
			return errors.New("Logical Tunnel metadata changed on an active backend peer")
		}
		existing.touch(now)
		return nil
	}
	if len(g.byPeer) >= g.cfg.maxSessions {
		return errors.New("shared-TUN gateway session capacity reached")
	}
	if existing := g.byLease[meta.Address4]; existing != nil {
		return fmt.Errorf("Logical Tunnel lease %s already belongs to tunnel %s", meta.Address4, tunnelIDPrefix(existing.tunnelID))
	}
	if existing := g.byTunnel[meta.TunnelID]; existing != nil {
		return fmt.Errorf("Logical Tunnel %s already registered by peer %s", tunnelIDPrefix(meta.TunnelID), existing.peer)
	}
	s := &sharedSession{key: key, tunnelID: meta.TunnelID, lease: meta.Address4, peer: cloneUDPAddr(peer), last: now}
	g.byPeer[key] = s
	g.byLease[meta.Address4] = s
	g.byTunnel[meta.TunnelID] = s
	fmt.Printf("WBD_SHARED_TUN_SESSION_READY tunnel_id_prefix=%s address4=%s peer=%s tun=%s nat=host\n", tunnelIDPrefix(s.tunnelID), s.lease, s.peer, g.tun.Name())
	return nil
}

func (g *sharedGateway) tunReadLoop() {
	buf := make([]byte, dataplane.MaxPacketLen)
	for {
		n, err := g.tun.ReadPacket(buf)
		if err != nil {
			return
		}
		packet := append([]byte(nil), buf[:n]...)
		_, dst, err := ipv4SourceDest(packet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "WBD_SHARED_TUN_RX_DROP err=%v\n", err)
			continue
		}
		g.mu.Lock()
		s := g.byLease[dst]
		g.mu.Unlock()
		if s == nil {
			fmt.Fprintf(os.Stderr, "WBD_SHARED_TUN_RX_DROP dst=%s err=no-live-lease\n", dst)
			continue
		}
		wire, err := dataplane.MarshalIP(packet)
		if err != nil {
			s.counters.drop.Add(1)
			continue
		}
		if _, err := g.conn.WriteToUDP(wire, s.peer); err != nil {
			s.counters.drop.Add(1)
			continue
		}
		s.counters.tunRx.Add(1)
		s.counters.rawTx.Add(1)
		s.rawTxFirst.Do(func() {
			fmt.Printf("WBD_SHARED_RAWIP_TX_FIRST tunnel_id_prefix=%s address4=%s bytes=%d\n", tunnelIDPrefix(s.tunnelID), s.lease, n)
		})
		s.touch(time.Now())
	}
}

func (g *sharedGateway) expire(now time.Time) {
	var stale []string
	g.mu.Lock()
	for key, s := range g.byPeer {
		if s.idleFor(now) >= g.cfg.idleTimeout {
			stale = append(stale, key)
		}
	}
	g.mu.Unlock()
	for _, key := range stale {
		g.dropSession(key, "idle")
	}
}

func (g *sharedGateway) dropSession(key, reason string) {
	g.mu.Lock()
	s := g.byPeer[key]
	if s != nil {
		delete(g.byPeer, key)
		delete(g.byLease, s.lease)
		delete(g.byTunnel, s.tunnelID)
	}
	g.mu.Unlock()
	if s == nil {
		return
	}
	fmt.Printf("WBD_SHARED_TUN_SESSION_COUNTERS tunnel_id_prefix=%s address4=%s rawip_tx=%d rawip_rx=%d tun_tx=%d tun_rx=%d drop=%d\n", tunnelIDPrefix(s.tunnelID), s.lease, s.counters.rawTx.Load(), s.counters.rawRx.Load(), s.counters.tunTx.Load(), s.counters.tunRx.Load(), s.counters.drop.Load())
	fmt.Printf("WBD_SHARED_TUN_SESSION_CLEAN tunnel_id_prefix=%s address4=%s reason=%s shared_tun=kept nat=kept\n", tunnelIDPrefix(s.tunnelID), s.lease, reason)
}

func (g *sharedGateway) runFirewall(action string) error {
	args := []string{action, "--backend", g.cfg.backend, "--state", g.cfg.firewallState, "--lease-prefix", g.leases.String(), "--tun-if", g.tun.Name()}
	if g.cfg.nftForward != "" {
		args = append(args, "--nft-forward", g.cfg.nftForward)
	}
	cmd := exec.Command(g.cfg.firewall, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("shared-TUN firewall helper %s: %w: %s", action, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (g *sharedGateway) Close() {
	if g == nil {
		return
	}
	g.mu.Lock()
	keys := make([]string, 0, len(g.byPeer))
	for key := range g.byPeer {
		keys = append(keys, key)
	}
	g.mu.Unlock()
	for _, key := range keys {
		g.dropSession(key, "shutdown")
	}
	_ = g.runFirewall("cleanup")
	if g.tun != nil {
		_ = g.tun.Close()
	}
	if g.conn != nil {
		_ = g.conn.Close()
	}
	_ = runIgnore("ip", "link", "del", g.cfg.tunIf)
}

func ipv4SourceDest(packet []byte) (netip.Addr, netip.Addr, error) {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return netip.Addr{}, netip.Addr{}, errors.New("raw-IP packet is not IPv4")
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return netip.Addr{}, netip.Addr{}, errors.New("invalid IPv4 header length")
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total < ihl || total > len(packet) {
		return netip.Addr{}, netip.Addr{}, errors.New("invalid IPv4 total length")
	}
	var srcRaw, dstRaw [4]byte
	copy(srcRaw[:], packet[12:16])
	copy(dstRaw[:], packet[16:20])
	return netip.AddrFrom4(srcRaw), netip.AddrFrom4(dstRaw), nil
}

func cloneUDPAddr(in *net.UDPAddr) *net.UDPAddr {
	if in == nil {
		return nil
	}
	out := *in
	out.IP = append(net.IP(nil), in.IP...)
	return &out
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runIgnore(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	_ = cmd.Run()
	return nil
}
