package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/singleflow"
)

const (
	phaseBootstrap = iota
	phaseDatagram
)

type bridgeServer struct {
	conn *net.UDPConn
	cfg  realityfront.ServerConfig

	mu        sync.RWMutex
	peer      *net.UDPAddr
	assoc     *faketcp.ServerAssociation
	flow      faketcp.ServerFlow
	phase     int
	assembler *singleflow.OrderedAssembler
	inbound   chan []byte

	startOnce sync.Once
	sendMu    sync.Mutex
	ipID      uint32
	done      chan struct{}
}

func main() {
	listen := flag.String("listen", "127.0.0.1:48188", "localhost UDP bridge listen address")
	serverName := flag.String("server-name", "www.speedtest.net", "Reality-like TLS SNI")
	routeKey := flag.String("route-key", "0123456789abcdef0123456789abcdef", "Reality-like route key")
	username := flag.String("username", "abi-user", "bootstrap username")
	password := flag.String("password", "abi-password", "bootstrap password")
	flag.Parse()

	cert, err := ephemeralServerCert(*serverName)
	if err != nil {
		fatalf("certificate: %v", err)
	}
	ticketDir, err := os.MkdirTemp("", "wbd-npcap-full-ticket-")
	if err != nil {
		fatalf("ticket dir: %v", err)
	}
	defer os.RemoveAll(ticketDir)

	addr, err := net.ResolveUDPAddr("udp4", *listen)
	if err != nil {
		fatalf("resolve listen: %v", err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		fatalf("listen: %v", err)
	}
	defer conn.Close()

	s := &bridgeServer{
		conn: conn,
		cfg: realityfront.ServerConfig{
			RouteKey:         []byte(*routeKey),
			ServerName:       *serverName,
			TLSConfig:        &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
			ExpectedUsername: *username,
			ExpectedPassword: *password,
			TicketDir:        ticketDir,
			HelloTimeout:     6 * time.Second,
		},
		phase:   phaseBootstrap,
		inbound: make(chan []byte, 256),
		done:    make(chan struct{}),
	}
	fmt.Printf("WBD_NPCAP_FULL_SERVER_READY listen=%s\n", conn.LocalAddr())
	go s.retransmitLoop()
	if err := s.readLoop(); err != nil && !errors.Is(err, net.ErrClosed) {
		fatalf("read loop: %v", err)
	}
}

func (s *bridgeServer) readLoop() error {
	buf := make([]byte, 65535)
	for {
		n, peer, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return err
		}
		seg, err := faketcp.ParseIPv4TCP(buf[:n])
		if err != nil {
			continue
		}
		s.mu.Lock()
		s.peer = cloneUDPAddr(peer)
		assoc := s.assoc
		s.mu.Unlock()

		if assoc == nil {
			if !faketcp.IsWBDHandshakeSegment(seg) {
				continue
			}
			assoc, err = faketcp.NewServerAssociation(seg, 0x6a09e667, faketcp.RecoveryLegacy, time.Second)
			if err != nil {
				continue
			}
			s.mu.Lock()
			if s.assoc == nil {
				s.assoc = assoc
				s.flow = faketcp.ServerFlowFromSegment(seg)
				s.assembler = singleflow.NewOrderedAssembler(assoc.ReceiverNext())
			} else {
				assoc = s.assoc
			}
			s.mu.Unlock()
			seq, ack, err := assoc.SYNACK()
			if err == nil {
				_ = s.sendRaw(seq, ack, faketcp.FlagSYN|faketcp.FlagACK, nil, nil)
				fmt.Printf("WBD_NPCAP_FULL_SERVER_SYNACK client_port=%d\n", seg.SrcPort)
			}
			continue
		}

		flow := assoc.Flow()
		if !flow.Matches(seg) {
			continue
		}
		if seg.Flags&faketcp.FlagSYN != 0 {
			if seq, ack, err := assoc.SYNACK(); err == nil {
				_ = s.sendRaw(seq, ack, faketcp.FlagSYN|faketcp.FlagACK, nil, nil)
			}
			continue
		}
		if assoc.State() == faketcp.ServerAssociationAwaitACK {
			if err := assoc.HandleHandshakeACK(seg); err != nil {
				continue
			}
			s.startOnce.Do(func() { go s.runBootstrap() })
		}

		res, err := assoc.HandleSegment(seg, time.Now())
		if err != nil {
			continue
		}
		if res.FastRetransmit != nil {
			_ = s.sendPending(res.FastRetransmit)
		}
		if res.AckNeeded {
			_ = s.sendRaw(assoc.SenderNext(), res.Ack, faketcp.FlagACK, res.SACK[:res.SACKN], nil)
		}
		if len(res.Deliver) != 0 {
			if err := s.handleDelivered(seg); err != nil {
				return err
			}
		}
	}
}

func (s *bridgeServer) runBootstrap() {
	app, carrier := net.Pipe()
	defer app.Close()
	defer carrier.Close()
	go s.carrierWriter(carrier)
	go s.carrierReader(carrier)

	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	res, tlsConn, err := realityfront.HandleServerConnSimpleSingleFlow(ctx, app, s.cfg)
	if err != nil {
		fmt.Printf("WBD_NPCAP_FULL_SERVER_TLS_FAIL err=%q\n", err)
		return
	}
	fmt.Printf("WBD_NPCAP_FULL_SERVER_AUTH_OK tls=1.3 ticket_prefix=%s\n", res.Ticket.Hex()[:8])

	_ = tlsConn.SetDeadline(time.Now().Add(8 * time.Second))
	req := make([]byte, singleflow.SwitchFrameLen)
	if _, err := io.ReadFull(tlsConn, req); err != nil {
		fmt.Printf("WBD_NPCAP_FULL_SERVER_SWITCH_READ_FAIL err=%q\n", err)
		return
	}
	if !singleflow.IsSwitchRequest(req, res.Ticket[:]) {
		fmt.Println("WBD_NPCAP_FULL_SERVER_SWITCH_BAD")
		return
	}
	fmt.Println("WBD_NPCAP_FULL_SERVER_SWITCH_REQUEST_OK")
	if err := s.waitSenderDrained(8 * time.Second); err != nil {
		fmt.Printf("WBD_NPCAP_FULL_SERVER_DRAIN_FAIL err=%q\n", err)
		return
	}
	if _, err := tlsConn.Write(singleflow.SwitchAck(res.Ticket[:])); err != nil {
		fmt.Printf("WBD_NPCAP_FULL_SERVER_SWITCH_ACK_FAIL err=%q\n", err)
		return
	}
	fmt.Println("WBD_NPCAP_FULL_SERVER_SWITCH_ACK_SENT")

	s.mu.Lock()
	s.phase = phaseDatagram
	s.assembler = nil
	s.mu.Unlock()
	_ = tlsConn.SetDeadline(time.Time{})
	fmt.Println("WBD_NPCAP_FULL_SERVER_DATAGRAM_READY hol=bootstrap-only")
}

func (s *bridgeServer) carrierWriter(carrier net.Conn) {
	for {
		select {
		case p := <-s.inbound:
			if len(p) == 0 {
				continue
			}
			if _, err := carrier.Write(p); err != nil {
				return
			}
		case <-s.done:
			return
		}
	}
}

func (s *bridgeServer) carrierReader(carrier net.Conn) {
	buf := make([]byte, singleflow.BootstrapMaxPayload)
	for {
		n, err := carrier.Read(buf)
		if n > 0 {
			if e := s.sendPayload(buf[:n]); e != nil {
				fmt.Printf("WBD_NPCAP_FULL_SERVER_SEND_FAIL err=%q\n", e)
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *bridgeServer) handleDelivered(seg faketcp.Segment) error {
	s.mu.RLock()
	phase := s.phase
	assembler := s.assembler
	s.mu.RUnlock()
	if phase == phaseDatagram {
		if err := s.sendPayload(seg.Payload); err != nil {
			return err
		}
		fmt.Printf("WBD_NPCAP_FULL_SERVER_DATAGRAM_ECHO bytes=%d\n", len(seg.Payload))
		return nil
	}
	if assembler == nil {
		return errors.New("bootstrap assembler missing")
	}
	contiguous := assembler.Push(seg.Seq, seg.Payload)
	if len(contiguous) == 0 {
		return nil
	}
	select {
	case s.inbound <- contiguous:
		return nil
	case <-time.After(2 * time.Second):
		return errors.New("bootstrap inbound queue timeout")
	}
}

func (s *bridgeServer) sendPayload(payload []byte) error {
	for len(payload) != 0 {
		n := len(payload)
		if n > singleflow.BootstrapMaxPayload {
			n = singleflow.BootstrapMaxPayload
		}
		s.mu.RLock()
		assoc := s.assoc
		s.mu.RUnlock()
		if assoc == nil {
			return errors.New("association missing")
		}
		p, err := assoc.Enqueue(payload[:n], time.Now())
		if err != nil {
			return err
		}
		if err := s.sendPending(p); err != nil {
			return err
		}
		payload = payload[n:]
	}
	return nil
}

func (s *bridgeServer) sendPending(p *faketcp.Pending) error {
	if p == nil || len(p.Payload) == 0 {
		return nil
	}
	s.mu.RLock()
	assoc := s.assoc
	s.mu.RUnlock()
	if assoc == nil {
		return errors.New("association missing")
	}
	return s.sendRaw(p.Seq, assoc.ReceiverNext(), faketcp.FlagACK|faketcp.FlagPSH, nil, p.Payload)
}

func (s *bridgeServer) sendRaw(seq, ack uint32, flags uint8, sacks []faketcp.SACKBlock, payload []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	s.mu.RLock()
	flow := s.flow
	peer := cloneUDPAddr(s.peer)
	s.mu.RUnlock()
	if peer == nil {
		return errors.New("bridge peer missing")
	}
	id := uint16(atomic.AddUint32(&s.ipID, 1))
	pkt := make([]byte, faketcp.PacketLenSACK(flags, len(payload), sacks))
	pkt = faketcp.MarshalIPv4TCPSACKInto(pkt, flow.ServerIP, flow.ClientIP, flow.ServerPort, flow.ClientPort, seq, ack, flags, 65535, sacks, payload, id)
	_, err := s.conn.WriteToUDP(pkt, peer)
	return err
}

func (s *bridgeServer) waitSenderDrained(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		s.mu.RLock()
		assoc := s.assoc
		s.mu.RUnlock()
		if assoc == nil {
			return errors.New("association missing")
		}
		if assoc.SenderPending() == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout with %d pending segments", assoc.SenderPending())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (s *bridgeServer) retransmitLoop() {
	t := time.NewTicker(2 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case now := <-t.C:
			s.mu.RLock()
			assoc := s.assoc
			s.mu.RUnlock()
			if assoc != nil {
				if p := assoc.RetransmitDue(now); p != nil {
					_ = s.sendPending(p)
				}
			}
		case <-s.done:
			return
		}
	}
}

func ephemeralServerCert(serverName string) (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serverName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{serverName},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, nil
}

func cloneUDPAddr(in *net.UDPAddr) *net.UDPAddr {
	if in == nil {
		return nil
	}
	out := *in
	out.IP = append(net.IP(nil), in.IP...)
	return &out
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "WBD_NPCAP_FULL_SERVER_FATAL "+format+"\n", args...)
	os.Exit(1)
}
