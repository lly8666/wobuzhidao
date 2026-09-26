package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sync"
)

// helloCaptureConn records client bytes only until the first server read. It is
// retained as a diagnostic correlation aid for the legacy mirror experiment;
// the newer same-entry Reality-like front does not depend on this hash for
// routing or authentication.
type helloCaptureConn struct {
	net.Conn
	mu      sync.Mutex
	capture bool
	buf     []byte
}

func newHelloCaptureConn(c net.Conn) *helloCaptureConn {
	return &helloCaptureConn{Conn: c, capture: true, buf: make([]byte, 0, 4096)}
}

func (c *helloCaptureConn) Write(p []byte) (int, error) {
	// net.Pipe (and a sufficiently back-pressured TCP socket) can let the peer
	// consume a whole ClientHello before the underlying Write returns. Record the
	// attempted TLS bytes before blocking so a concurrent diagnostic snapshot
	// cannot observe an empty capture. If Write is partial, trim the unsent tail.
	c.mu.Lock()
	start := len(c.buf)
	capturing := c.capture
	if capturing {
		c.buf = append(c.buf, p...)
	}
	c.mu.Unlock()

	n, err := c.Conn.Write(p)
	if capturing && n < len(p) {
		c.mu.Lock()
		want := start + n
		if want >= 0 && want <= len(c.buf) {
			c.buf = c.buf[:want]
		}
		c.mu.Unlock()
	}
	return n, err
}

func (c *helloCaptureConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	c.capture = false
	c.mu.Unlock()
	return c.Conn.Read(p)
}

func (c *helloCaptureConn) SHA256Hex() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.buf) == 0 {
		return ""
	}
	sum := sha256.Sum256(c.buf)
	return hex.EncodeToString(sum[:])
}
