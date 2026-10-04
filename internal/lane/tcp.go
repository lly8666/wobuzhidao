package lane

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

var ErrHalfCloseUnsupported = errors.New("WBD lane connection does not support CloseWrite")

// TCP is one real kernel TCP carrier. It deliberately contains no scheduling,
// reinjection, FEC or session policy.
type TCP struct {
	conn    net.Conn
	readMu  sync.Mutex
	writeMu sync.Mutex
}

func WrapTCP(conn net.Conn) *TCP {
	if conn == nil {
		return nil
	}
	return &TCP{conn: conn}
}

func DialTCP(ctx context.Context, address string) (*TCP, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	return &TCP{conn: conn}, nil
}

func (l *TCP) Send(frame any) error {
	l.writeMu.Lock()
	defer l.writeMu.Unlock()
	return protocol.WriteFrame(l.conn, frame)
}

func (l *TCP) Receive() (any, error) {
	l.readMu.Lock()
	defer l.readMu.Unlock()
	return protocol.ReadFrame(l.conn)
}

func (l *TCP) CloseWrite() error {
	if cw, ok := l.conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return ErrHalfCloseUnsupported
}

func (l *TCP) Close() error                       { return l.conn.Close() }
func (l *TCP) LocalAddr() net.Addr                { return l.conn.LocalAddr() }
func (l *TCP) RemoteAddr() net.Addr               { return l.conn.RemoteAddr() }
func (l *TCP) SetDeadline(t time.Time) error      { return l.conn.SetDeadline(t) }
func (l *TCP) SetReadDeadline(t time.Time) error  { return l.conn.SetReadDeadline(t) }
func (l *TCP) SetWriteDeadline(t time.Time) error { return l.conn.SetWriteDeadline(t) }
