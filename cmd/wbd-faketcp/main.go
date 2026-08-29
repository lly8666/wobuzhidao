package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

var errRawTimeout = errors.New("raw packet read timeout")

type rawPacketIO interface {
	ReadPacket([]byte) (int, error)
	WritePacket([]byte, [4]byte) error
	SetReadTimeout(time.Duration) error
	ClearReadTimeout() error
	Close() error
}

type config struct {
	role      string
	localUDP  string
	targetUDP string
	source    string
	listen    string
	remote    string
	recovery  string

	packetDevice string
	sourceMAC    string
	nextHopMAC   string

	realityServerName string
	realityRouteKey   string
	realityUsername   string
	realityPassword   string
	realityTicketOut  string
	realityVerify     bool
	realityTimeout    time.Duration
}

type endpoint struct {
	cfg              config
	raw              rawPacketIO
	srcIP, dstIP     [4]byte
	srcPort, dstPort uint16
	udp              *net.UDPConn
	innerMu          sync.RWMutex
	inner            *net.UDPAddr
	senderMu         sync.Mutex
	sender           *faketcp.Sender
	receiverMu       sync.Mutex
	receiver         *faketcp.Receiver
	sendMu           sync.Mutex
	sendBuf          []byte
	ipID             uint32
	rawTx, rawRx     uint64
	ackTx, dataTx    uint64
	dataRx           uint64
	stop             chan struct{}
	stopOnce         sync.Once

	bootstrapMu  sync.RWMutex
	bootstrap    *faketcp.BootstrapStream
	bootstrapAck chan struct{}
}

type finalStats struct {
	Role     string                `json:"role"`
	Recovery string                `json:"recovery"`
	RawTx    uint64                `json:"raw_tx"`
	RawRx    uint64                `json:"raw_rx"`
	AckTx    uint64                `json:"ack_tx"`
	DataTx   uint64                `json:"data_tx"`
	DataRx   uint64                `json:"data_rx"`
	Sender   faketcp.SenderStats   `json:"sender"`
	Receiver faketcp.ReceiverStats `json:"receiver"`
	RTOms    float64               `json:"rto_ms"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	role := os.Args[1]
	fs := flag.NewFlagSet(role, flag.ExitOnError)
	var c config
	c.role = role
	fs.StringVar(&c.localUDP, "local-udp", "", "client UDP listen address")
	fs.StringVar(&c.targetUDP, "target-udp", "", "server downstream UDP address")
	fs.StringVar(&c.source, "source", "", "client raw source ip:port")
	fs.StringVar(&c.listen, "listen", "", "server raw listen ip:port")
	fs.StringVar(&c.remote, "remote", "", "client raw remote ip:port")
	fs.StringVar(&c.recovery, "shadow-recovery", "legacy", "TCP-like shadow recovery: legacy (default) or sack-rack experimental")
	fs.StringVar(&c.packetDevice, "packet-device", "", "Windows/Npcap capture device, for example \\Device\\NPF_{GUID}")
	fs.StringVar(&c.sourceMAC, "source-mac", "", "Windows/Npcap physical source MAC")
	fs.StringVar(&c.nextHopMAC, "next-hop-mac", "", "Windows/Npcap routed next-hop MAC")
	fs.StringVar(&c.realityServerName, "reality-server-name", "", "single-flow TLS bootstrap SNI/server name")
	fs.StringVar(&c.realityRouteKey, "reality-route-key", "", "single-flow Reality-like classifier secret")
	fs.StringVar(&c.realityUsername, "reality-username", "", "single-flow shared account username")
	fs.StringVar(&c.realityPassword, "reality-password", "", "single-flow shared account password")
	fs.StringVar(&c.realityTicketOut, "reality-ticket-out", "", "0600 local file receiving the same-flow one-time ticket")
	fs.BoolVar(&c.realityVerify, "reality-verify-server", false, "verify TLS bootstrap certificate/hostname")
	fs.DurationVar(&c.realityTimeout, "reality-timeout", 12*time.Second, "single-flow TLS/bootstrap deadline")
	_ = fs.Parse(os.Args[2:])
	if role != "client" && role != "server" {
		usage()
		os.Exit(2)
	}
	if _, err := parseRecovery(c.recovery); err != nil {
		fmt.Fprintln(os.Stderr, "wbd-faketcp:", err)
		os.Exit(2)
	}
	if err := c.validateSingleFlow(); err != nil {
		fmt.Fprintln(os.Stderr, "wbd-faketcp:", err)
		os.Exit(2)
	}

	e, err := newEndpoint(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wbd-faketcp:", err)
		os.Exit(1)
	}
	defer e.close()
	if err := e.handshake(); err != nil {
		fmt.Fprintln(os.Stderr, "wbd-faketcp handshake:", err)
		os.Exit(1)
	}

	sig := make(chan os.Signal, 1)
	notifySignals(sig)
	errCh := make(chan error, 3)
	rawStarted := false
	if c.singleFlowEnabled() {
		stream, err := e.newBootstrapStream()
		if err != nil {
			fmt.Fprintln(os.Stderr, "WBD_SINGLE_FLOW_BOOTSTRAP_FAIL", err)
			os.Exit(1)
		}
		e.setBootstrap(stream)
		go func() { errCh <- e.rawLoop() }()
		rawStarted = true
		ticket, tlsState, err := realityfront.BootstrapClientSingleFlow(context.Background(), stream, realityfront.SingleFlowClientConfig{
			ServerName: c.realityServerName, RouteKey: []byte(c.realityRouteKey), Username: c.realityUsername,
			Password: c.realityPassword, VerifyServer: c.realityVerify, Timeout: c.realityTimeout,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "WBD_SINGLE_FLOW_BOOTSTRAP_FAIL", err)
			os.Exit(1)
		}
		if err := os.WriteFile(c.realityTicketOut, []byte(ticket.Hex()+"\n"), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "WBD_SINGLE_FLOW_BOOTSTRAP_FAIL write ticket:", err)
			os.Exit(1)
		}
		e.clearBootstrap(stream)
		_ = stream.Close()
		fmt.Printf("WBD_SINGLE_FLOW_BOOTSTRAP_READY tls=%x server_name=%s same_flow=1\n", tlsState.Version, c.realityServerName)
	}

	e.senderMu.Lock()
	startupRTO := e.sender.RTO()
	e.senderMu.Unlock()
	fmt.Printf("READY role=%s rto_ms=%.3f recovery=%s single_flow_bootstrap=%t\n", role, float64(startupRTO)/float64(time.Millisecond), c.recovery, c.singleFlowEnabled())

	if !rawStarted {
		go func() { errCh <- e.rawLoop() }()
	}
	go func() { errCh <- e.udpLoop() }()
	go func() { errCh <- e.retransmitLoop() }()
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil && !errors.Is(err, os.ErrClosed) {
			fmt.Fprintln(os.Stderr, "wbd-faketcp:", err)
		}
	}
	e.close()
	e.printStats()
}

func (c config) singleFlowEnabled() bool {
	return strings.TrimSpace(c.realityServerName) != "" || strings.TrimSpace(c.realityRouteKey) != "" ||
		strings.TrimSpace(c.realityUsername) != "" || strings.TrimSpace(c.realityPassword) != "" || strings.TrimSpace(c.realityTicketOut) != ""
}

func (c config) validateSingleFlow() error {
	if !c.singleFlowEnabled() {
		return nil
	}
	if c.role != "client" {
		return errors.New("single-flow Reality bootstrap is currently a client option; use wbd-faketcp-mux on the product server")
	}
	if strings.TrimSpace(c.realityServerName) == "" || len(c.realityRouteKey) < 16 || c.realityUsername == "" || c.realityPassword == "" || strings.TrimSpace(c.realityTicketOut) == "" || c.realityTimeout <= 0 {
		return errors.New("single-flow bootstrap requires reality-server-name, route-key >=16 bytes, username/password, ticket-out and positive timeout")
	}
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  wbd-faketcp client --local-udp 127.0.0.1:PORT --source IP:PORT --remote IP:PORT [--shadow-recovery legacy|sack-rack]")
	fmt.Fprintln(os.Stderr, "  wbd-faketcp server --listen IP:PORT --target-udp 127.0.0.1:PORT [--shadow-recovery legacy|sack-rack]")
	fmt.Fprintln(os.Stderr, "  product client single-flow bootstrap additionally uses --reality-server-name --reality-route-key --reality-username --reality-password --reality-ticket-out")
	fmt.Fprintln(os.Stderr, "  Windows client additionally requires --packet-device --source-mac --next-hop-mac")
}

func parseRecovery(s string) (faketcp.RecoveryMode, error) {
	switch s {
	case "legacy":
		return faketcp.RecoveryLegacy, nil
	case "sack-rack", "advanced":
		return faketcp.RecoverySACKRACK, nil
	default:
		return faketcp.RecoveryLegacy, fmt.Errorf("unknown --shadow-recovery %q", s)
	}
}

func newEndpoint(c config) (*endpoint, error) {
	e := &endpoint{cfg: c, stop: make(chan struct{}), sendBuf: make([]byte, 65535), bootstrapAck: make(chan struct{}, 1)}
	var rawLocal, rawRemote *net.UDPAddr
	var err error
	if c.role == "client" {
		if c.localUDP == "" || c.source == "" || c.remote == "" {
			return nil, errors.New("client requires --local-udp --source --remote")
		}
		rawLocal, err = net.ResolveUDPAddr("udp4", c.source)
		if err != nil { return nil, err }
		rawRemote, err = net.ResolveUDPAddr("udp4", c.remote)
		if err != nil { return nil, err }
		la, err := net.ResolveUDPAddr("udp4", c.localUDP)
		if err != nil { return nil, err }
		e.udp, err = net.ListenUDP("udp4", la)
		if err != nil { return nil, err }
	} else {
		if c.listen == "" || c.targetUDP == "" {
			return nil, errors.New("server requires --listen --target-udp")
		}
		rawLocal, err = net.ResolveUDPAddr("udp4", c.listen)
		if err != nil { return nil, err }
		e.inner, err = net.ResolveUDPAddr("udp4", c.targetUDP)
		if err != nil { return nil, err }
		la, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
		if err != nil { return nil, err }
		e.udp, err = net.ListenUDP("udp4", la)
		if err != nil { return nil, err }
	}
	if rawLocal.Port <= 0 || rawLocal.Port > 65535 {
		e.close()
		return nil, errors.New("bad raw local port")
	}
	e.srcPort = uint16(rawLocal.Port)
	e.srcIP, _ = faketcp.IPv4(rawLocal.IP)
	if e.srcIP == [4]byte{} {
		e.close()
		return nil, errors.New("raw local address must be IPv4")
	}
	if rawRemote != nil {
		e.dstPort = uint16(rawRemote.Port)
		e.dstIP, _ = faketcp.IPv4(rawRemote.IP)
		if e.dstIP == [4]byte{} {
			e.close()
			return nil, errors.New("raw remote address must be IPv4")
		}
	}
	e.raw, err = openRawPacketIO(c, e.srcIP)
	if err != nil {
		e.close()
		return nil, err
	}
	return e, nil
}

func randomSeq() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil { return binary.BigEndian.Uint32(b[:]) }
	return uint32(time.Now().UnixNano())
}

func (e *endpoint) newSender(seq uint32, rto time.Duration) *faketcp.Sender {
	mode, _ := parseRecovery(e.cfg.recovery)
	return faketcp.NewSenderWithRecovery(seq, rto, mode)
}

func (e *endpoint) handshake() error {
	if e.cfg.role == "client" { return e.handshakeClient() }
	return e.handshakeServer()
}

func (e *endpoint) handshakeClient() error {
	isn := randomSeq()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		sent := time.Now()
		if err := e.send(isn, 0, faketcp.FlagSYN, nil, nil); err != nil { return err }
		_ = e.raw.SetReadTimeout(300 * time.Millisecond)
		for time.Now().Before(sent.Add(300 * time.Millisecond)) {
			seg, err := e.recvOne()
			if err != nil {
				if errors.Is(err, errRawTimeout) { break }
				return err
			}
			if seg.SrcIP != e.dstIP || seg.DstIP != e.srcIP || seg.SrcPort != e.dstPort || seg.DstPort != e.srcPort { continue }
			if !faketcp.IsWBDHandshakeSegment(seg) || seg.Flags&(faketcp.FlagSYN|faketcp.FlagACK) != faketcp.FlagSYN|faketcp.FlagACK || seg.Ack != isn+1 { continue }
			peerNext := seg.Seq + 1
			if err := e.send(isn+1, peerNext, faketcp.FlagACK, nil, nil); err != nil { return err }
			rtt := time.Since(sent)
			e.sender = e.newSender(isn+1, maxDuration(40*time.Millisecond, 2*rtt))
			e.receiver = faketcp.NewReceiver(peerNext)
			_ = e.raw.ClearReadTimeout()
			return nil
		}
	}
	return errors.New("client SYN timeout")
}

func (e *endpoint) handshakeServer() error {
	_ = e.raw.ClearReadTimeout()
	for {
		seg, err := e.recvOne()
		if err != nil {
			if errors.Is(err, errRawTimeout) { continue }
			return err
		}
		if seg.DstIP != e.srcIP || seg.DstPort != e.srcPort || seg.Flags&faketcp.FlagSYN == 0 || !faketcp.IsWBDHandshakeSegment(seg) { continue }
		e.dstIP, e.dstPort = seg.SrcIP, seg.SrcPort
		peerNext := seg.Seq + 1
		isn := randomSeq()
		for attempts := 0; attempts < 60; attempts++ {
			sent := time.Now()
			if err := e.send(isn, peerNext, faketcp.FlagSYN|faketcp.FlagACK, nil, nil); err != nil { return err }
			_ = e.raw.SetReadTimeout(300 * time.Millisecond)
			for time.Now().Before(sent.Add(300 * time.Millisecond)) {
				a, err := e.recvOne()
				if err != nil {
					if errors.Is(err, errRawTimeout) { break }
					return err
				}
				if a.SrcIP != e.dstIP || a.DstIP != e.srcIP || a.SrcPort != e.dstPort || a.DstPort != e.srcPort || a.Flags&faketcp.FlagACK == 0 || a.Ack != isn+1 { continue }
				rtt := time.Since(sent)
				e.sender = e.newSender(isn+1, maxDuration(40*time.Millisecond, 2*rtt))
				e.receiver = faketcp.NewReceiver(peerNext)
				_ = e.raw.ClearReadTimeout()
				return nil
			}
		}
		return errors.New("server SYN-ACK timeout")
	}
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b { return a }
	return b
}

func (e *endpoint) recvOne() (faketcp.Segment, error) {
	var zero faketcp.Segment
	buf := make([]byte, 65535)
	n, err := e.raw.ReadPacket(buf)
	if err != nil { return zero, err }
	seg, err := faketcp.ParseIPv4TCP(buf[:n])
	if err != nil { return zero, err }
	atomic.AddUint64(&e.rawRx, 1)
	return seg, nil
}

func (e *endpoint) newBootstrapStream() (*faketcp.BootstrapStream, error) {
	local := &net.TCPAddr{IP: net.IPv4(e.srcIP[0], e.srcIP[1], e.srcIP[2], e.srcIP[3]), Port: int(e.srcPort)}
	remote := &net.TCPAddr{IP: net.IPv4(e.dstIP[0], e.dstIP[1], e.dstIP[2], e.dstIP[3]), Port: int(e.dstPort)}
	return faketcp.NewBootstrapStream(e.receiverNext(), func(payload []byte) (uint32, error) {
		e.senderMu.Lock()
		p := e.sender.Enqueue(payload, time.Now())
		end := p.End
		err := e.sendDataPending(p)
		e.senderMu.Unlock()
		return end, err
	}, e.waitBootstrapAck, local, remote)
}

func (e *endpoint) waitBootstrapAck(end uint32, deadline time.Time) error {
	for {
		e.senderMu.Lock()
		acked := e.sender.LastAck() == end
		e.senderMu.Unlock()
		if acked { return nil }
		var timer <-chan time.Time
		if !deadline.IsZero() {
			d := time.Until(deadline)
			if d <= 0 { return faketcp.ErrBootstrapTimeout }
			t := time.NewTimer(d)
			defer t.Stop()
			timer = t.C
		}
		select {
		case <-e.bootstrapAck:
		case <-e.stop:
			return faketcp.ErrBootstrapClosed
		case <-timer:
			return faketcp.ErrBootstrapTimeout
		}
	}
}

func (e *endpoint) signalBootstrapAck() {
	select { case e.bootstrapAck <- struct{}{}: default: }
}

func (e *endpoint) setBootstrap(s *faketcp.BootstrapStream) {
	e.bootstrapMu.Lock(); e.bootstrap = s; e.bootstrapMu.Unlock()
}

func (e *endpoint) clearBootstrap(expected *faketcp.BootstrapStream) {
	e.bootstrapMu.Lock()
	if e.bootstrap == expected { e.bootstrap = nil }
	e.bootstrapMu.Unlock()
}

func (e *endpoint) bootstrapStream() *faketcp.BootstrapStream {
	e.bootstrapMu.RLock(); s := e.bootstrap; e.bootstrapMu.RUnlock(); return s
}

func (e *endpoint) rawLoop() error {
	buf := make([]byte, 65535)
	for {
		n, err := e.raw.ReadPacket(buf)
		if err != nil {
			if errors.Is(err, errRawTimeout) {
				select { case <-e.stop: return nil; default: continue }
			}
			select { case <-e.stop: return nil; default: return err }
		}
		seg, err := faketcp.ParseIPv4TCP(buf[:n])
		if err != nil { continue }
		if seg.SrcIP != e.dstIP || seg.DstIP != e.srcIP || seg.SrcPort != e.dstPort || seg.DstPort != e.srcPort { continue }
		atomic.AddUint64(&e.rawRx, 1)

		if e.cfg.role == "client" && faketcp.IsWBDHandshakeSegment(seg) && seg.Flags&(faketcp.FlagSYN|faketcp.FlagACK) == faketcp.FlagSYN|faketcp.FlagACK {
			snd := e.senderNext(); rcv := e.receiverNext()
			if seg.Ack == snd && seg.Seq+1 == rcv {
				if err := e.send(snd, rcv, faketcp.FlagACK, nil, nil); err != nil { return err }
				atomic.AddUint64(&e.ackTx, 1)
				continue
			}
		}

		now := time.Now()
		if seg.Flags&faketcp.FlagACK != 0 {
			e.senderMu.Lock()
			p := e.sender.AckSelective(seg.Ack, seg.SACK[:seg.SACKN], now)
			if p != nil { err = e.sendDataPending(p) }
			e.senderMu.Unlock()
			e.signalBootstrapAck()
			if err != nil { return err }
		}
		if len(seg.Payload) == 0 { continue }
		var sackBuf [4]faketcp.SACKBlock
		e.receiverMu.Lock()
		deliver, oo := e.receiver.Accept(seg.Seq, len(seg.Payload))
		ack := e.receiver.Next()
		sackN := 0
		if oo { sackN = e.receiver.SACKBlocks(&sackBuf) }
		e.receiverMu.Unlock()
		atomic.AddUint64(&e.dataRx, 1)
		var sacks []faketcp.SACKBlock
		if sackN != 0 { sacks = sackBuf[:sackN] }
		if err := e.send(e.senderNext(), ack, faketcp.FlagACK, sacks, nil); err != nil { return err }
		atomic.AddUint64(&e.ackTx, 1)
		if deliver {
			if stream := e.bootstrapStream(); stream != nil {
				stream.Feed(seg.Seq, seg.Payload)
				continue
			}
			peer := e.innerPeer()
			if peer != nil { _, _ = e.udp.WriteToUDP(seg.Payload, peer) }
		}
	}
}

func (e *endpoint) udpLoop() error {
	buf := make([]byte, 65535)
	for {
		n, from, err := e.udp.ReadFromUDP(buf)
		if err != nil {
			select { case <-e.stop: return nil; default: return err }
		}
		if e.cfg.role == "client" {
			e.innerMu.Lock()
			if e.inner == nil { cp := *from; e.inner = &cp }
			known := e.inner
			e.innerMu.Unlock()
			if !udpEqual(from, known) { continue }
		} else if !udpEqual(from, e.innerPeer()) { continue }
		if n == 0 { continue }
		now := time.Now()
		e.senderMu.Lock()
		p := e.sender.Enqueue(buf[:n], now)
		err = e.sendDataPending(p)
		e.senderMu.Unlock()
		if err != nil { return err }
	}
}

func udpEqual(a, b *net.UDPAddr) bool {
	if a == nil || b == nil { return false }
	return a.Port == b.Port && a.IP.Equal(b.IP)
}

func (e *endpoint) retransmitLoop() error {
	t := time.NewTicker(2 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-e.stop:
			return nil
		case now := <-t.C:
			e.senderMu.Lock()
			p := e.sender.RetransmitDue(now)
			var err error
			if p != nil { err = e.sendDataPending(p) }
			e.senderMu.Unlock()
			if err != nil { return err }
		}
	}
}

func (e *endpoint) senderNext() uint32 { e.senderMu.Lock(); defer e.senderMu.Unlock(); return e.sender.NextSeq() }
func (e *endpoint) receiverNext() uint32 { e.receiverMu.Lock(); defer e.receiverMu.Unlock(); return e.receiver.Next() }

func (e *endpoint) innerPeer() *net.UDPAddr {
	e.innerMu.RLock(); defer e.innerMu.RUnlock()
	if e.inner == nil { return nil }
	cp := *e.inner
	return &cp
}

func (e *endpoint) sendDataPending(p *faketcp.Pending) error {
	if p == nil || len(p.Payload) == 0 { return nil }
	if err := e.send(p.Seq, e.receiverNext(), faketcp.FlagACK|faketcp.FlagPSH, nil, p.Payload); err != nil { return err }
	atomic.AddUint64(&e.dataTx, 1)
	return nil
}

func (e *endpoint) send(seq, ack uint32, flags uint8, sacks []faketcp.SACKBlock, payload []byte) error {
	e.sendMu.Lock(); defer e.sendMu.Unlock()
	id := uint16(atomic.AddUint32(&e.ipID, 1))
	pkt := faketcp.MarshalIPv4TCPSACKInto(e.sendBuf, e.srcIP, e.dstIP, e.srcPort, e.dstPort, seq, ack, flags, 65535, sacks, payload, id)
	if err := e.raw.WritePacket(pkt, e.dstIP); err != nil { return err }
	atomic.AddUint64(&e.rawTx, 1)
	return nil
}

func (e *endpoint) close() {
	e.stopOnce.Do(func() {
		close(e.stop)
		if s := e.bootstrapStream(); s != nil { _ = s.Close() }
		if e.udp != nil { _ = e.udp.Close() }
		if e.raw != nil { _ = e.raw.Close() }
	})
}

func (e *endpoint) printStats() {
	if e.sender == nil || e.receiver == nil { return }
	e.senderMu.Lock(); ss := e.sender.Stats(); rto := e.sender.RTO(); e.senderMu.Unlock()
	e.receiverMu.Lock(); rs := e.receiver.Stats(); e.receiverMu.Unlock()
	st := finalStats{
		Role: e.cfg.role, Recovery: e.cfg.recovery, RawTx: atomic.LoadUint64(&e.rawTx), RawRx: atomic.LoadUint64(&e.rawRx),
		AckTx: atomic.LoadUint64(&e.ackTx), DataTx: atomic.LoadUint64(&e.dataTx), DataRx: atomic.LoadUint64(&e.dataRx),
		Sender: ss, Receiver: rs, RTOms: float64(rto) / float64(time.Millisecond),
	}
	b, _ := json.Marshal(st)
	fmt.Printf("WBD_FAKETCP_STATS %s\n", b)
}
