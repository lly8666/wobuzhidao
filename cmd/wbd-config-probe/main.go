package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
)

func main() {
	mode := flag.String("mode", "", "server or client")
	addr := flag.String("addr", "", "UDP listen/destination address")
	token := flag.String("token", "", "client bearer token")
	expected := flag.String("expected-token", "", "server expected bearer token")
	fecMode := flag.String("fec", "weak-2x", "off/normal or weak-2x/20:20; other live profiles are rejected")
	mtu := flag.Int("mtu", 1400, "immutable link MTU")
	flushMS := flag.Int("fec-flush-ms", 8, "fixed FEC flush in milliseconds")
	lanes := flag.Int("lanes", 1, "immutable raw lane count")
	flag.Parse()

	var err error
	switch *mode {
	case "server":
		err = runServer(*addr, []byte(*expected))
	case "client":
		cfg, parseErr := parseLinkConfig(*fecMode, *mtu, *flushMS, *lanes)
		if parseErr != nil {
			err = parseErr
		} else {
			err = runClient(*addr, []byte(*token), cfg)
		}
	default:
		err = fmt.Errorf("-mode must be server or client")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseLinkConfig(mode string, mtu, flushMS, lanes int) (control.LinkConfig, error) {
	if mtu <= 0 || mtu > 65535 || flushMS < 0 || flushMS > 65535 || lanes <= 0 || lanes > 255 {
		return control.LinkConfig{}, fmt.Errorf("invalid link numeric parameters")
	}
	base := control.LinkConfig{LaneCount: uint8(lanes), MTU: uint16(mtu)}
	switch mode {
	case "off", "normal":
		base.FECMode = control.FECOff
		base.Scheduler = control.FECSchedulerNone
		return base, nil
	case "weak-2x", "20:20":
		if flushMS == 0 {
			return control.LinkConfig{}, fmt.Errorf("fixed FEC requires positive -fec-flush-ms")
		}
		base.FECMode = control.FECFixed
		base.Scheduler = control.FECSchedulerTailRS
		base.DataShards = 20
		base.ParityShards = 20
		base.FlushMillis = uint16(flushMS)
		return base, nil
	case "weak-1.5x", "20:10":
		return control.LinkConfig{}, fmt.Errorf("20:10 is a reference profile but the current live WBD codec is still 20:20; reconnect cannot enable an unimplemented codec")
	case "auto":
		return control.LinkConfig{}, fmt.Errorf("auto FEC is deferred advanced research")
	default:
		return control.LinkConfig{}, fmt.Errorf("unsupported FEC profile %q", mode)
	}
}

func runServer(addr string, expected []byte) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer pc.Close()
	s, err := control.NewLinkServerSession(1, 1, expected, control.CurrentLinkPolicy())
	if err != nil {
		return err
	}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	_ = pc.SetDeadline(time.Now().Add(10 * time.Second))
	for s.State() != control.StateEstablished {
		n, peer, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		wire, err := s.HandleWire(buf[:n], uint64(time.Now().UnixNano()))
		if err != nil {
			return err
		}
		if _, err = pc.WriteTo(wire, peer); err != nil {
			return err
		}
	}
	st := s.Stats()
	fmt.Printf("SERVER state=%d auth_required=%t authenticated=%t configured=%t fec_mode=%d scheduler=%d fec=%d:%d flush_ms=%d mtu=%d lanes=%d rx=%d tx=%d\n",
		st.State, st.AuthRequired, st.Authenticated, st.Configured,
		st.Config.FECMode, st.Config.Scheduler, st.Config.DataShards, st.Config.ParityShards,
		st.Config.FlushMillis, st.Config.MTU, st.Config.LaneCount, st.ControlRX, st.ControlTX)
	return nil
}

func runClient(addr string, token []byte, cfg control.LinkConfig) error {
	peer, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	c, err := net.DialUDP("udp", nil, peer)
	if err != nil {
		return err
	}
	defer c.Close()
	_ = c.SetDeadline(time.Now().Add(10 * time.Second))

	init := control.LinkInit{MinProtocol: 1, MaxProtocol: 1, Config: cfg}
	if err = exchange(c, init, func(v any) error {
		accept, ok := v.(control.LinkAccept)
		if !ok {
			return fmt.Errorf("LINK_INIT reply %T", v)
		}
		return control.ValidateLinkAccept(init, accept)
	}); err != nil {
		return err
	}

	if len(token) != 0 {
		if err = exchange(c, control.Auth{Token: token}, func(v any) error {
			_, ok := v.(control.AuthOK)
			if !ok {
				return fmt.Errorf("AUTH reply %T", v)
			}
			return nil
		}); err != nil {
			return err
		}
	}
	fmt.Printf("CLIENT established=true fec_mode=%d scheduler=%d fec=%d:%d flush_ms=%d mtu=%d lanes=%d immutable=true\n",
		cfg.FECMode, cfg.Scheduler, cfg.DataShards, cfg.ParityShards, cfg.FlushMillis, cfg.MTU, cfg.LaneCount)
	return nil
}

func exchange(c *net.UDPConn, frame any, check func(any) error) error {
	wire, err := control.MarshalLink(frame)
	if err != nil {
		return err
	}
	if _, err = c.Write(wire); err != nil {
		return err
	}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	n, err := c.Read(buf)
	if err != nil {
		return err
	}
	v, err := control.UnmarshalLink(buf[:n])
	if err != nil {
		return err
	}
	if e, ok := v.(control.Error); ok {
		return fmt.Errorf("server error code=%d message=%q", e.Code, e.Message)
	}
	return check(v)
}
