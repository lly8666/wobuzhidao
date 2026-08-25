package main

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sync"
)

// helloCaptureConn records client bytes only until the first server read. For a
// normal TLS handshake this is the complete initial ClientHello flight. It is a
// diagnostic correlation aid, not a secret or an authentication credential.
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
	n, err := c.Conn.Write(p)
	if n > 0 {
		c.mu.Lock()
		if c.capture {
			c.buf = append(c.buf, p[:n]...)
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
