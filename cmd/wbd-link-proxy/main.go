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
	maxBlocks       = 64
	setupRetryAfter = 200 * time.Millisecond
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
	demoRealityWitness    string
	demoRealityWitnessDir string
	demoRealityServerName string
	demoRealityTicket     string
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
	flag.StringVar(&o.demoRealityWitness, "demo-reality-witness", "", "legacy mirror demo: 64-hex ClientHello witness")
	flag.StringVar(&o.demoRealityWitnessDir, "demo-reality-witness-dir", "", "legacy mirror demo server: local witness directory")
	flag.StringVar(&o.demoRealityServerName, "demo-reality-server-name", "", "legacy mirror demo server: target SNI bound to witness")
	flag.StringVar(&o.demoRealityTicket, "demo-reality-ticket", "", "same-entry Reality front demo: one-time 64-hex authenticated ticket")
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
			return errors.New("client demo mode accepts only -demo-reality-witness or -demo-reality-ticket")
		}
		if strings.TrimSpace(o.demoRealityWitness) != "" && strings.TrimSpace(o.demoRealityTicket) != "" {
			return errors.New("choose exactly one Reality demo binding: witness or ticket")
		}
	} else {
		if strings.TrimSpace(o.demoRealityWitness) != "" || strings.TrimSpace(o.demoRealityTicket) != "" {
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
	init := control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}
	var startup clientStartupSession
	demoKind := "off"
	if raw := strings.TrimSpace(o.demoRealityTicket); raw != "" {
		ticket, err := realityfront.ParseTicketHex(raw)
		if err != nil {
			return err
		}
		var bind [control.DemoWitnessLen]byte
		copy(bind[:], ticket[:])
		startup, err = control.NewDemoTicketLinkClientSession(init, bind)
		if err != nil {
			return err
		}
		demoKind = "ticket"
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
	fmt.Printf("WBD_LINK_READY role=client fec=%s mtu=%d lanes=%d immutable=1 auth=%t demo_reality=%t demo_kind=%s\n", o.fec, cfg.MTU, cfg.LaneCount, startupAcceptAuth(startup), demoKind != "off", demoKind)
	return clientDataLoop(conn, dtlsAddr, path, startup, stop)
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
				continue
			}
			return err
		}
		select {
		case <-stop:
			return nil
		default:
		}
		if !sameUDPAddr(from, dtlsAddr) {
			continue
		}
		next, err := startup.HandleWire(buf[:n])
		if err != nil {
			return err
		}
		if len(next) != 0 {
			if _, err := conn.WriteToUDP(next, dtlsAddr); err != nil {
				return err
			}
			nextSend = time.Now().Add(setupRetryAfter)
		}
	}
	return nil
}

func runServer(conn *net.UDPConn, o options, stop <-chan os.Signal) error {
	if o.service == "" {
		return errors.New("server requires -service")
	}
	serviceAddr, err := net.ResolveUDPAddr("udp4", o.service)
	if err != nil {
		return err
	}
	var startup serverStartupSession
	demoKind := "off"
	if ticketDir := strings.TrimSpace(o.demoRealityTicketDir); ticketDir != "" {
		verify := func(bind [control.DemoWitnessLen]byte) error {
			var ticket realityfront.Ticket
			copy(ticket[:], bind[:])
			return realityfront.ConsumeTicket(ticketDir, ticket, time.Now(), o.demoRealityTTL)
		}
		startup, err = control.NewDemoTicketReliableLinkServerSession(1, 1, control.CurrentLinkPolicy(), verify)
		demoKind = "ticket"
	} else if witnessDir := strings.TrimSpace(o.demoRealityWitnessDir); witnessDir != "" {
		verify := func(witness [control.DemoWitnessLen]byte) error {
			return persona.ConsumeWitness(witnessDir, persona.WitnessID(witness), o.demoRealityServerName, time.Now(), o.demoRealityTTL)
		}
		startup, err = control.NewDemoReliableLinkServerSession(1, 1, []byte(o.expectedToken), control.CurrentLinkPolicy(), verify)
		demoKind = "witness"
	} else {
		startup, err = control.NewReliableLinkServerSession(1, 1, []byte(o.expectedToken), control.CurrentLinkPolicy())
	}
	if err != nil {
		return err
	}
	dtlsPeer, err := serverStartup(conn, serviceAddr, startup, o.setupTimeout, stop)
	if err != nil {
		return err
	}
	cfg := startup.Stats().Config
	path, err := linkdata.New(cfg, maxBlocks)
	if err != nil {
		return err
	}
	fmt.Printf("WBD_LINK_READY role=server fec_mode=%d fec=%d:%d mtu=%d lanes=%d immutable=1 auth=%t demo_reality=%t demo_kind=%s\n",
		cfg.FECMode, cfg.DataShards, cfg.ParityShards, cfg.MTU, cfg.LaneCount, startup.Stats().AuthRequired, demoKind != "off", demoKind)
	return serverDataLoop(conn, serviceAddr, dtlsPeer, path, startup, stop)
}

func serverStartup(conn *net.UDPConn, serviceAddr *net.UDPAddr, startup serverStartupSession, timeout time.Duration, stop <-chan os.Signal) (*net.UDPAddr, error) {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	var peer *net.UDPAddr
	for startup.State() != control.StateEstablished {
		if startup.State() == control.StateFailed || startup.State() == control.StateClosed {
			return nil, errors.New("immutable link startup failed; reconnect required")
		}
		_ = conn.SetReadDeadline(deadline)
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return nil, err
		}
		select {
		case <-stop:
			return nil, nil
		default:
		}
		if sameUDPAddr(from, serviceAddr) {
			continue
		}
		if peer == nil {
			peer = cloneUDPAddr(from)
		} else if !sameUDPAddr(from, peer) {
			continue
		}
		reply, err := startup.HandleWire(buf[:n], uint64(time.Now().UnixNano()))
		if err != nil {
			return nil, err
		}
		if _, err := conn.WriteToUDP(reply, peer); err != nil {
			return nil, err
		}
	}
	if peer == nil {
		return nil, errors.New("server established without DTLS plaintext peer")
	}
	return peer, nil
}

func clientDataLoop(conn *net.UDPConn, dtlsAddr *net.UDPAddr, path *linkdata.Path, startup clientStartupSession, stop <-chan os.Signal) error {
	buf := make([]byte, 65535)
	var appPeer *net.UDPAddr
	for {
		select {
		case <-stop:
			return flushPath(conn, dtlsAddr, path)
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, from, err := conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				return err
			}
		} else if sameUDPAddr(from, dtlsAddr) {
			if isStartupControl(buf[:n]) {
				if next, err := startup.HandleWire(buf[:n]); err != nil {
					return err
				} else if len(next) != 0 {
					if _, err := conn.WriteToUDP(next, dtlsAddr); err != nil {
						return err
					}
				}
				continue
			}
			packets, err := path.Decode(buf[:n])
			if err != nil {
				if !errors.Is(err, fec.ErrDecoderFull) {
					return err
				}
			} else if appPeer != nil {
				for _, packet := range packets {
					if _, err := conn.WriteToUDP(packet, appPeer); err != nil {
						return err
					}
				}
			}
		} else {
			appPeer = cloneUDPAddr(from)
			wire, err := path.Encode(buf[:n], now)
			if err != nil {
				return err
			}
			if err := sendWire(conn, dtlsAddr, wire); err != nil {
				return err
			}
		}
		wire, err := path.FlushDue(now)
		if err != nil {
			return err
		}
		if err := sendWire(conn, dtlsAddr, wire); err != nil {
			return err
		}
	}
}

func serverDataLoop(conn *net.UDPConn, serviceAddr, dtlsPeer *net.UDPAddr, path *linkdata.Path, startup serverStartupSession, stop <-chan os.Signal) error {
	buf := make([]byte, 65535)
	for {
		select {
		case <-stop:
			return flushPath(conn, dtlsPeer, path)
		default:
		}
		_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, from, err := conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				return err
			}
		} else if sameUDPAddr(from, serviceAddr) {
			wire, err := path.Encode(buf[:n], now)
			if err != nil {
				return err
			}
			if err := sendWire(conn, dtlsPeer, wire); err != nil {
				return err
			}
		} else if sameUDPAddr(from, dtlsPeer) {
			if isStartupControl(buf[:n]) {
				reply, err := startup.HandleWire(buf[:n], uint64(now.UnixNano()))
				if err != nil {
					return err
				}
				if _, err := conn.WriteToUDP(reply, dtlsPeer); err != nil {
					return err
				}
				if startup.State() == control.StateFailed {
					return errors.New("post-establishment link change requires reconnect")
				}
				continue
			}
			packets, err := path.Decode(buf[:n])
			if err != nil {
				if !errors.Is(err, fec.ErrDecoderFull) {
					return err
				}
			} else {
				for _, packet := range packets {
					if _, err := conn.WriteToUDP(packet, serviceAddr); err != nil {
						return err
					}
				}
			}
		}
		wire, err := path.FlushDue(now)
		if err != nil {
			return err
		}
		if err := sendWire(conn, dtlsPeer, wire); err != nil {
			return err
		}
	}
}

func flushPath(conn *net.UDPConn, dst *net.UDPAddr, path *linkdata.Path) error {
	wire, err := path.Flush()
	if err != nil {
		return err
	}
	return sendWire(conn, dst, wire)
}

func sendWire(conn *net.UDPConn, dst *net.UDPAddr, wire [][]byte) error {
	for _, packet := range wire {
		if _, err := conn.WriteToUDP(packet, dst); err != nil {
			return err
		}
	}
	return nil
}

func isStartupControl(packet []byte) bool {
	if len(packet) < control.HeaderLen || string(packet[:4]) != string(control.Magic[:]) || packet[4] != control.FrameVersion1 {
		return false
	}
	switch control.Type(packet[5]) {
	case control.TypeDemoBind, control.TypeDemoBindOK, control.TypeLinkInit, control.TypeLinkAccept, control.TypeError, control.TypeAuth, control.TypeAuthOK:
		return true
	default:
		return false
	}
}

func sameUDPAddr(a, b *net.UDPAddr) bool {
	if a == nil || b == nil || a.Port != b.Port || a.Zone != b.Zone {
		return false
	}
	return a.IP.Equal(b.IP)
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil {
		return nil
	}
	return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone}
}
