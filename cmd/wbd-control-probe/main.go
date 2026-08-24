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
	flag.Parse()
	var err error
	switch *mode {
	case "server":
		err = runServer(*addr)
	case "client":
		err = runClient(*addr, uint16(*min), uint16(*max))
	default:
		err = fmt.Errorf("-mode must be server or client")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runServer(addr string) error {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return err
	}
	defer pc.Close()
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	_ = pc.SetDeadline(time.Now().Add(10 * time.Second))
	n, peer, err := pc.ReadFrom(buf)
	if err != nil {
		return err
	}
	msg, err := control.Unmarshal(buf[:n])
	if err != nil {
		return err
	}
	h, ok := msg.(control.Hello)
	if !ok {
		return fmt.Errorf("expected HELLO, got %T", msg)
	}
	reply := control.Negotiate(h, control.ProtocolVersion1, control.ProtocolVersion1)
	wire, err := control.Marshal(reply)
	if err != nil {
		return err
	}
	if _, err = pc.WriteTo(wire, peer); err != nil {
		return err
	}
	fmt.Printf("SERVER received=HELLO min=%d max=%d reply=%T\n", h.MinProtocol, h.MaxProtocol, reply)
	return nil
}

func runClient(addr string, min, max uint16) error {
	peer, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	c, err := net.DialUDP("udp", nil, peer)
	if err != nil {
		return err
	}
	defer c.Close()
	wire, err := control.Marshal(control.Hello{MinProtocol: min, MaxProtocol: max})
	if err != nil {
		return err
	}
	if _, err = c.Write(wire); err != nil {
		return err
	}
	_ = c.SetReadDeadline(time.Now().Add(10 * time.Second))
	buf := make([]byte, control.HeaderLen+control.MaxBodyLen)
	n, err := c.Read(buf)
	if err != nil {
		return err
	}
	msg, err := control.Unmarshal(buf[:n])
	if err != nil {
		return err
	}
	switch m := msg.(type) {
	case control.Accept:
		fmt.Printf("CLIENT reply=ACCEPT protocol=%d\n", m.Protocol)
	case control.Error:
		fmt.Printf("CLIENT reply=ERROR code=%d message=%q\n", m.Code, m.Message)
	default:
		return fmt.Errorf("unexpected reply %T", msg)
	}
	return nil
}
