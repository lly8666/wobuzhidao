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
	configMode := flag.String("config-mode", "weak-1.5x", "normal, weak-1.5x, or weak-2x")
	flag.Parse()
	var err error
	switch *mode {
	case "server":
		err = runServer(*addr, []byte(*expected))
	case "client":
		err = runClient(*addr, []byte(*token), *configMode)
	default:
		err = fmt.Errorf("-mode must be server or client")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func parseMode(s string) (control.ProtectionMode, error) {
	switch s {
	case "normal":
		return control.ProtectionNormal, nil
	case "weak-1.5x":
		return control.ProtectionWeak15, nil
	case "weak-2x":
		return control.ProtectionWeak2, nil
	case "auto":
		return 0, fmt.Errorf("auto protection mode is not admitted")
	default:
		return 0, fmt.Errorf("unsupported protection mode %q", s)
	}
}

func runServer(addr string, expected []byte) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer pc.Close()
	s, err := control.NewConfigServerSession(1, 1, expected)
	if err != nil {
		return err
	}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	_ = pc.SetDeadline(time.Now().Add(10 * time.Second))
	for !s.Stats().Configured {
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
	fmt.Printf("SERVER state=%d auth_required=%t authenticated=%t configured=%t protection_mode=%s rx=%d tx=%d rx_bytes=%d tx_bytes=%d\n",
		st.State, st.AuthRequired, st.Authenticated, st.Configured, st.ProtectionMode, st.ControlRX, st.ControlTX, st.ControlRXBytes, st.ControlTXBytes)
	return nil
}

func runClient(addr string, token []byte, modeText string) error {
	mode, err := parseMode(modeText)
	if err != nil {
		return err
	} // fail before socket
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
	if err = exchange(c, control.Hello{MinProtocol: 1, MaxProtocol: 1}, func(v any) error {
		_, ok := v.(control.Accept)
		if !ok {
			return fmt.Errorf("HELLO reply %T", v)
		}
		return nil
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
	if err = exchange(c, control.Config{Mode: mode}, func(v any) error {
		ok, yes := v.(control.ConfigOK)
		if !yes || ok.Mode != mode {
			return fmt.Errorf("CONFIG reply %#v", v)
		}
		return nil
	}); err != nil {
		return err
	}
	fmt.Printf("CLIENT configured=true protection_mode=%s\n", mode)
	return nil
}

func exchange(c *net.UDPConn, frame any, check func(any) error) error {
	wire, err := control.MarshalExtended(frame)
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
	v, err := control.UnmarshalExtended(buf[:n])
	if err != nil {
		return err
	}
	if e, ok := v.(control.Error); ok {
		return fmt.Errorf("server error code=%d message=%q", e.Code, e.Message)
	}
	return check(v)
}
