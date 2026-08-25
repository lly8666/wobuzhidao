package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

const (
	maxPacketSize = 1400
	flushAfter    = 8 * time.Millisecond
	maxBlocks     = 64
)

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wbd-fec-proxy client LISTEN_PORT DTLS_PORT | server LISTEN_PORT DTLS_PORT SERVICE_PORT")
}

func port(s string) (int, error) {
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 || v > 65535 {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	return v, nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "WBD_FEC_PROXY_FAIL", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) < 3 {
		usage()
		return errors.New("bad arguments")
	}
	mode := args[0]
	listenPort, err := port(args[1])
	if err != nil { return err }
	dtlsPort, err := port(args[2])
	if err != nil { return err }
	if mode != "client" && mode != "server" {
		usage()
		return fmt.Errorf("invalid mode %q", mode)
	}
	var servicePort int
	if mode == "server" {
		if len(args) != 4 { usage(); return errors.New("server requires SERVICE_PORT") }
		servicePort, err = port(args[3])
		if err != nil { return err }
	} else if len(args) != 3 {
		usage()
		return errors.New("client takes exactly two ports")
	}

	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: listenPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil { return err }
	defer conn.Close()
	if err := conn.SetReadBuffer(4 << 20); err != nil { return err }
	if err := conn.SetWriteBuffer(4 << 20); err != nil { return err }

	codec := fec.NewReedSolomon20x20()
	enc, err := fec.NewBlockEncoder(codec, maxPacketSize, flushAfter, 1)
	if err != nil { return err }
	dec, err := fec.NewBlockDecoder(codec, maxPacketSize, maxBlocks)
	if err != nil { return err }
	dtlsAddr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: dtlsPort}
	var serviceAddr *net.UDPAddr
	if mode == "server" {
		serviceAddr = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: servicePort}
	}
	var clientApp *net.UDPAddr

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	buf := make([]byte, 65535)
	fmt.Printf("READY role=%s listen=%d dtls=%d flush_ms=%d\n", mode, listenPort, dtlsPort, flushAfter.Milliseconds())
	for {
		select {
		case <-stop:
			if wire, err := enc.Flush(); err == nil { _ = sendWire(conn, dtlsAddr, wire) }
			return nil
		default:
		}

		_ = conn.SetReadDeadline(time.Now().Add(time.Millisecond))
		n, from, err := conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() { return err }
		} else if from.Port == dtlsPort {
			packets, done, err := dec.Add(buf[:n])
			if err != nil {
				if !errors.Is(err, fec.ErrDecoderFull) { return err }
			} else if done {
				var dst *net.UDPAddr
				if mode == "server" { dst = serviceAddr } else { dst = clientApp }
				if dst != nil {
					for _, p := range packets {
						if _, err := conn.WriteToUDP(p, dst); err != nil { return err }
					}
				}
			}
		} else {
			if mode == "client" {
				clientApp = from
			} else if from.Port != servicePort {
				continue
			}
			wire, err := enc.Add(buf[:n], now)
			if err != nil { return err }
			if err := sendWire(conn, dtlsAddr, wire); err != nil { return err }
		}

		wire, err := enc.FlushDue(now)
		if err != nil { return err }
		if err := sendWire(conn, dtlsAddr, wire); err != nil { return err }
	}
}

func sendWire(conn *net.UDPConn, dst *net.UDPAddr, wire [][]byte) error {
	for _, d := range wire {
		if _, err := conn.WriteToUDP(d, dst); err != nil { return err }
	}
	return nil
}
