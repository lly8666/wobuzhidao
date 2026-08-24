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
	addr := flag.String("addr", "", "UDP listen address (server) or destination address (client)")
	min := flag.Uint("min", 1, "client minimum protocol version")
	max := flag.Uint("max", 1, "client maximum protocol version")
	token := flag.String("token", "", "client bearer token")
	expectedToken := flag.String("expected-token", "", "server expected bearer token; empty disables auth")
	flag.Parse()
	var err error
	switch *mode {
	case "server":
		err = runServer(*addr, []byte(*expectedToken))
	case "client":
		err = runClient(*addr, uint16(*min), uint16(*max), []byte(*token))
	default:
		err = fmt.Errorf("-mode must be server or client")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(addr string, expectedToken []byte) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer pc.Close()
	session, err := control.NewServerSession(control.ProtocolVersion1, control.ProtocolVersion1, expectedToken)
	if err != nil {
		return err
	}
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	_ = pc.SetDeadline(time.Now().Add(10 * time.Second))
	for session.State() != control.StateEstablished && session.State() != control.StateFailed {
		n, peer, err := pc.ReadFrom(buf)
		if err != nil {
			return err
		}
		msg, err := control.Unmarshal(buf[:n])
		if err != nil {
			return err
		}
		reply := session.Handle(msg)
		wire, err := control.Marshal(reply)
		if err != nil {
			return err
		}
		if _, err = pc.WriteTo(wire, peer); err != nil {
			return err
		}
		fmt.Printf("SERVER received=%T reply=%T state=%d\n", msg, reply, session.State())
	}
	return nil
}

func runClient(addr string, min, max uint16, token []byte) error {
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
	if err := send(c, control.Hello{MinProtocol: min, MaxProtocol: max}); err != nil {
		return err
	}
	msg, err := recv(c)
	if err != nil {
		return err
	}
	switch m := msg.(type) {
	case control.Accept:
		fmt.Printf("CLIENT reply=ACCEPT protocol=%d\n", m.Protocol)
	case control.Error:
		fmt.Printf("CLIENT reply=ERROR code=%d message=%q\n", m.Code, m.Message)
		return nil
	default:
		return fmt.Errorf("unexpected reply %T", msg)
	}
	if len(token) == 0 {
		fmt.Println("CLIENT state=ESTABLISHED auth=disabled")
		return nil
	}
	if err := send(c, control.Auth{Token: token}); err != nil {
		return err
	}
	msg, err = recv(c)
	if err != nil {
		return err
	}
	switch m := msg.(type) {
	case control.AuthOK:
		fmt.Println("CLIENT reply=AUTH_OK state=ESTABLISHED")
	case control.Error:
		fmt.Printf("CLIENT reply=ERROR code=%d message=%q\n", m.Code, m.Message)
	default:
		return fmt.Errorf("unexpected auth reply %T", msg)
	}
	return nil
}

func send(c *net.UDPConn, frame any) error {
	wire, err := control.Marshal(frame)
	if err != nil {
		return err
	}
	_, err = c.Write(wire)
	return err
}

func recv(c *net.UDPConn) (any, error) {
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	n, err := c.Read(buf)
	if err != nil {
		return nil, err
	}
	return control.Unmarshal(buf[:n])
}
