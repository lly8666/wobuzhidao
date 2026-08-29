package faketcp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const DefaultBootstrapChunk = 1200

var (
	ErrBootstrapClosed  = errors.New("faketcp: bootstrap stream closed")
	ErrBootstrapTimeout = errors.New("faketcp: bootstrap stream deadline exceeded")
)

// BootstrapSend emits one TCP-shaped payload segment and returns the cumulative
// ACK value that proves that segment has arrived. Implementations must not
// enqueue another bootstrap segment until BootstrapWaitAck confirms this end.
type BootstrapSend func([]byte) (end uint32, err error)

type BootstrapWaitAck func(end uint32, deadline time.Time) error

// BootstrapStream is a deliberately temporary net.Conn adapter for the first
// few seconds of one FakeTCP association. It provides the reliable ordered byte
// semantics crypto/tls expects without assigning steady-state VPN payload to a
// kernel TCP byte stream. After TLS/admission completes the caller discards this
// adapter and reuses the exact same FakeTCP 4-tuple/sequence space in datagram
// mode.
type BootstrapStream struct {
	mu sync.Mutex

	next    uint32
	pending map[uint32][]byte
	readBuf bytes.Buffer
	notify  chan struct{}
	closed  bool

	readDeadline  time.Time
	writeDeadline time.Time

	chunk   int
	send    BootstrapSend
	waitAck BootstrapWaitAck
	local   net.Addr
	remote  net.Addr
}

func NewBootstrapStream(next uint32, send BootstrapSend, waitAck BootstrapWaitAck, local, remote net.Addr) (*BootstrapStream, error) {
	if send == nil || waitAck == nil {
		return nil, errors.New("faketcp: bootstrap send/wait callbacks are required")
	}
	return &BootstrapStream{
		next: next, pending: make(map[uint32][]byte), notify: make(chan struct{}, 1),
		chunk: DefaultBootstrapChunk, send: send, waitAck: waitAck, local: local, remote: remote,
	}, nil
}

// Feed accepts a first-arrival FakeTCP payload. Out-of-order payload is retained
// only for this short bootstrap phase; contiguous bytes become visible to TLS.
func (c *BootstrapStream) Feed(seq uint32, payload []byte) {
	if len(payload) == 0 {
		return
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	if seq < c.next {
		c.mu.Unlock()
		return
	}
	if seq == c.next {
		_, _ = c.readBuf.Write(payload)
		c.next += uint32(len(payload))
		for {
			p, ok := c.pending[c.next]
			if !ok {
				break
			}
			delete(c.pending, c.next)
			_, _ = c.readBuf.Write(p)
			c.next += uint32(len(p))
		}
	} else if _, exists := c.pending[seq]; !exists {
		c.pending[seq] = append([]byte(nil), payload...)
	}
	c.mu.Unlock()
	c.signal()
}

func (c *BootstrapStream) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if c.readBuf.Len() != 0 {
			n, err := c.readBuf.Read(p)
			c.mu.Unlock()
			return n, err
		}
		if c.closed {
			c.mu.Unlock()
			return 0, io.EOF
		}
		deadline := c.readDeadline
		c.mu.Unlock()
		if err := c.wait(deadline); err != nil {
			return 0, err
		}
	}
}

func (c *BootstrapStream) Write(p []byte) (int, error) {
	written := 0
	for len(p) != 0 {
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			if written != 0 {
				return written, ErrBootstrapClosed
			}
			return 0, ErrBootstrapClosed
		}
		chunk := c.chunk
		deadline := c.writeDeadline
		c.mu.Unlock()
		if chunk <= 0 {
			chunk = DefaultBootstrapChunk
		}
		if chunk > len(p) {
			chunk = len(p)
		}
		end, err := c.send(p[:chunk])
		if err != nil {
			return written, err
		}
		if err := c.waitAck(end, deadline); err != nil {
			return written, err
		}
		written += chunk
		p = p[chunk:]
	}
	return written, nil
}

func (c *BootstrapStream) Close() error {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	c.signal()
	if already {
		return nil
	}
	return nil
}

func (c *BootstrapStream) LocalAddr() net.Addr  { return c.local }
func (c *BootstrapStream) RemoteAddr() net.Addr { return c.remote }

func (c *BootstrapStream) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.mu.Unlock()
	c.signal()
	return nil
}

func (c *BootstrapStream) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.mu.Unlock()
	c.signal()
	return nil
}

func (c *BootstrapStream) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *BootstrapStream) signal() {
	select {
	case c.notify <- struct{}{}:
	default:
	}
}

func (c *BootstrapStream) wait(deadline time.Time) error {
	if deadline.IsZero() {
		<-c.notify
		return nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		return ErrBootstrapTimeout
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-c.notify:
		return nil
	case <-t.C:
		return ErrBootstrapTimeout
	}
}
