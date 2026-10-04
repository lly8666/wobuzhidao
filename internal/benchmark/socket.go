package benchmark

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

type SocketTransport uint8

const (
	SocketTCP SocketTransport = iota + 1
	SocketUDP
)

type SocketObservation struct {
	Arrival []time.Duration
}

// RunSocketFaultSmoke uses only 127.0.0.1 kernel sockets. The proxy delays
// records according to delay[i]. TCP forwards serially, so one delayed record
// blocks later bytes. UDP schedules each datagram independently, so later
// datagrams may bypass a stalled one.
func RunSocketFaultSmoke(ctx context.Context, transport SocketTransport, delays []time.Duration, payloadBytes int) (SocketObservation, error) {
	if len(delays) < 2 || payloadBytes <= 0 {
		return SocketObservation{}, errors.New("invalid socket smoke config")
	}
	switch transport {
	case SocketTCP:
		return runTCPSmoke(ctx, delays, payloadBytes)
	case SocketUDP:
		return runUDPSmoke(ctx, delays, payloadBytes)
	default:
		return SocketObservation{}, errors.New("invalid socket transport")
	}
}

func runTCPSmoke(ctx context.Context, delays []time.Duration, payloadBytes int) (SocketObservation, error) {
	sinkLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { return SocketObservation{}, err }
	defer sinkLn.Close()
	proxyLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil { return SocketObservation{}, err }
	defer proxyLn.Close()
	start := time.Now()
	arrivals := make([]time.Duration, len(delays))
	errCh := make(chan error, 3)
	go func() {
		c, err := sinkLn.Accept(); if err != nil { errCh <- err; return }
		defer c.Close()
		r := bufio.NewReader(c)
		for range delays {
			id, _, err := readRecord(r); if err != nil { errCh <- err; return }
			if int(id) >= len(arrivals) { errCh <- fmt.Errorf("bad tcp id %d", id); return }
			arrivals[id] = time.Since(start)
		}
		errCh <- nil
	}()
	go func() {
		in, err := proxyLn.Accept(); if err != nil { errCh <- err; return }
		defer in.Close()
		out, err := net.Dial("tcp4", sinkLn.Addr().String()); if err != nil { errCh <- err; return }
		defer out.Close()
		r := bufio.NewReader(in)
		for i := range delays {
			id, payload, err := readRecord(r); if err != nil { errCh <- err; return }
			if int(id) != i { errCh <- fmt.Errorf("tcp proxy order id=%d want=%d", id, i); return }
			if err := sleepContext(ctx, delays[i]); err != nil { errCh <- err; return }
			if err := writeRecord(out, id, payload); err != nil { errCh <- err; return }
		}
		errCh <- nil
	}()
	client, err := net.Dial("tcp4", proxyLn.Addr().String())
	if err != nil { return SocketObservation{}, err }
	for i := range delays {
		if err := writeRecord(client, uint32(i), make([]byte, payloadBytes)); err != nil { client.Close(); return SocketObservation{}, err }
	}
	_ = client.Close()
	for range 2 {
		select {
		case err := <-errCh:
			if err != nil { return SocketObservation{}, err }
		case <-ctx.Done():
			return SocketObservation{}, ctx.Err()
		}
	}
	return SocketObservation{Arrival: arrivals}, nil
}

func runUDPSmoke(ctx context.Context, delays []time.Duration, payloadBytes int) (SocketObservation, error) {
	sink, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil { return SocketObservation{}, err }
	defer sink.Close()
	proxy, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil { return SocketObservation{}, err }
	defer proxy.Close()
	client, err := net.DialUDP("udp4", nil, proxy.LocalAddr().(*net.UDPAddr))
	if err != nil { return SocketObservation{}, err }
	defer client.Close()
	start := time.Now()
	arrivals := make([]time.Duration, len(delays))
	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, payloadBytes+16)
		for range delays {
			n, _, err := sink.ReadFromUDP(buf); if err != nil { errCh <- err; return }
			if n < 4 { errCh <- io.ErrUnexpectedEOF; return }
			id := binary.BigEndian.Uint32(buf[:4])
			if int(id) >= len(arrivals) { errCh <- fmt.Errorf("bad udp id %d", id); return }
			arrivals[id] = time.Since(start)
		}
		errCh <- nil
	}()
	var wg sync.WaitGroup
	go func() {
		buf := make([]byte, payloadBytes+16)
		for range delays {
			n, _, err := proxy.ReadFromUDP(buf); if err != nil { errCh <- err; return }
			if n < 4 { errCh <- io.ErrUnexpectedEOF; return }
			pkt := append([]byte(nil), buf[:n]...)
			id := int(binary.BigEndian.Uint32(pkt[:4]))
			if id < 0 || id >= len(delays) { errCh <- fmt.Errorf("bad udp proxy id %d", id); return }
			wg.Add(1)
			go func(delay time.Duration, data []byte) {
				defer wg.Done()
				if sleepContext(ctx, delay) != nil { return }
				_, _ = proxy.WriteToUDP(data, sink.LocalAddr().(*net.UDPAddr))
			}(delays[id], pkt)
		}
		wg.Wait()
		errCh <- nil
	}()
	for i := range delays {
		pkt := make([]byte, 4+payloadBytes)
		binary.BigEndian.PutUint32(pkt[:4], uint32(i))
		if _, err := client.Write(pkt); err != nil { return SocketObservation{}, err }
	}
	for range 2 {
		select {
		case err := <-errCh:
			if err != nil { return SocketObservation{}, err }
		case <-ctx.Done():
			return SocketObservation{}, ctx.Err()
		}
	}
	return SocketObservation{Arrival: arrivals}, nil
}

func writeRecord(w io.Writer, id uint32, payload []byte) error {
	var h [8]byte
	binary.BigEndian.PutUint32(h[:4], id)
	binary.BigEndian.PutUint32(h[4:], uint32(len(payload)))
	if _, err := w.Write(h[:]); err != nil { return err }
	_, err := w.Write(payload)
	return err
}

func readRecord(r *bufio.Reader) (uint32, []byte, error) {
	var h [8]byte
	if _, err := io.ReadFull(r, h[:]); err != nil { return 0, nil, err }
	id := binary.BigEndian.Uint32(h[:4])
	ln := binary.BigEndian.Uint32(h[4:])
	if ln > 1<<20 { return 0, nil, errors.New("socket smoke record too large") }
	p := make([]byte, ln)
	if _, err := io.ReadFull(r, p); err != nil { return 0, nil, err }
	return id, p, nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 { return nil }
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
