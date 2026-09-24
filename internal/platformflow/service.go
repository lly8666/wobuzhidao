package platformflow

import (
	"net"
	"net/netip"
	"time"
)

type ClientConfig struct {
	UDPIdle     time.Duration
	MaxUDPFlows int
	TCP         TCPConfig
}

func DefaultClientConfig() ClientConfig {
	return ClientConfig{
		UDPIdle: DefaultUDPIdleTimeout, MaxUDPFlows: DefaultMaxUDPFlows,
		TCP: DefaultTCPConfig(),
	}
}

type Client struct {
	channel *TunnelChannel
	udp     *UDPClient
	tcp     *TCPClient
}

func NewClient(channel *TunnelChannel, cfg ClientConfig, reply UDPReply) (*Client, error) {
	udp, err := NewUDPClient(channel, cfg.UDPIdle, cfg.MaxUDPFlows, reply)
	if err != nil {
		return nil, err
	}
	tcp, err := NewTCPClient(channel, cfg.TCP)
	if err != nil {
		udp.Close()
		return nil, err
	}
	return &Client{channel: channel, udp: udp, tcp: tcp}, nil
}

func (c *Client) ForwardUDP(client, peer netip.AddrPort, payload []byte, now time.Time) error {
	if c == nil {
		return ErrClosed
	}
	return c.udp.Forward(client, peer, payload, now)
}

func (c *Client) AddTCP(conn net.Conn, peer netip.AddrPort, now time.Time) (uint64, error) {
	if c == nil {
		return 0, ErrClosed
	}
	return c.tcp.Add(conn, peer, now)
}

func (c *Client) HandleServicePacket(packet []byte, now time.Time) (bool, error) {
	if c == nil {
		return false, ErrClosed
	}
	frame, handled, err := c.channel.Decode(packet)
	if err != nil || !handled {
		return handled, err
	}
	switch frame.Kind {
	case KindUDPDatagram:
		return true, c.udp.Handle(frame, now)
	case KindTCPAck, KindTCPData, KindTCPClose:
		return true, c.tcp.Handle(frame, now)
	default:
		return true, ErrUnsupported
	}
}

func (c *Client) Tick(now time.Time) {
	if c == nil {
		return
	}
	c.udp.Tick(now)
	c.tcp.Tick(now)
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.tcp.Close()
	c.udp.Close()
}

func (c *Client) UDPFlows() int {
	if c == nil { return 0 }
	return c.udp.Len()
}

func (c *Client) TCPFlows() int {
	if c == nil { return 0 }
	return c.tcp.Len()
}

type ServerConfig struct {
	UDPIdle     time.Duration
	MaxUDPFlows int
	TCP         TCPConfig
}

func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		UDPIdle: DefaultUDPIdleTimeout, MaxUDPFlows: DefaultMaxUDPFlows,
		TCP: DefaultTCPConfig(),
	}
}

type Server struct {
	channel *TunnelChannel
	udp     *UDPServer
	tcp     *TCPServer
}

func NewServer(channel *TunnelChannel, cfg ServerConfig) (*Server, error) {
	udp, err := NewUDPServer(channel, cfg.UDPIdle, cfg.MaxUDPFlows)
	if err != nil {
		return nil, err
	}
	tcp, err := NewTCPServer(channel, cfg.TCP)
	if err != nil {
		udp.Close()
		return nil, err
	}
	return &Server{channel: channel, udp: udp, tcp: tcp}, nil
}

func (s *Server) HandleServicePacket(packet []byte, now time.Time) (bool, error) {
	if s == nil {
		return false, ErrClosed
	}
	frame, handled, err := s.channel.Decode(packet)
	if err != nil || !handled {
		return handled, err
	}
	switch frame.Kind {
	case KindUDPDatagram:
		return true, s.udp.Handle(frame, now)
	case KindTCPOpen, KindTCPData, KindTCPAck, KindTCPClose:
		return true, s.tcp.Handle(frame, now)
	default:
		return true, ErrUnsupported
	}
}

func (s *Server) Tick(now time.Time) {
	if s == nil {
		return
	}
	s.udp.Tick(now)
	s.tcp.Tick(now)
}

func (s *Server) Close() {
	if s == nil {
		return
	}
	s.tcp.Close()
	s.udp.Close()
}

func (s *Server) UDPFlows() int {
	if s == nil { return 0 }
	return s.udp.Len()
}

func (s *Server) UDPDiagnostic() UDPServerDiagnostic {
	if s == nil || s.udp == nil {
		return UDPServerDiagnostic{}
	}
	return s.udp.Diagnostic()
}

func (s *Server) TCPFlows() int {
	if s == nil { return 0 }
	return s.tcp.Len()
}
