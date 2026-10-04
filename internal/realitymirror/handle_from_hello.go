package realitymirror

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
)

// HandleFromHello continues a mirror session after the caller has already
// consumed and validated the complete TLS ClientHello with ReadClientHello.
func HandleFromHello(ctx context.Context, client net.Conn, cfg Config, info HelloInfo, rawHello []byte) (Result, error) {
	return HandleFromHelloObserved(ctx, client, cfg, info, rawHello, nil)
}

// HandleFromHelloObserved is identical to HandleFromHello, but invokes
// onFirstDownstream exactly once after the genuine target has produced bytes
// and immediately before those first bytes are exposed to the client. The demo
// mirror uses this edge to publish its local ClientHello witness before a client
// can finish the genuine TLS preflight and race into DEMO_BIND.
//
// A callback error aborts the mirror before any target byte is forwarded. This
// keeps witness publication and the visible preflight ordered without inserting
// WBD application data into the genuine target TLS connection.
func HandleFromHelloObserved(ctx context.Context, client net.Conn, cfg Config, info HelloInfo, rawHello []byte, onFirstDownstream func() error) (Result, error) {
	var out Result
	if err := cfg.validate(); err != nil {
		return out, err
	}
	if len(rawHello) == 0 {
		return out, ErrMalformedHello
	}
	if !sameName(info.ServerName, cfg.ServerName) {
		return out, fmt.Errorf("%w: got %q want %q", ErrSNIMismatch, info.ServerName, cfg.ServerName)
	}

	dialer := net.Dialer{Timeout: cfg.DialTimeout}
	target, err := dialer.DialContext(ctx, "tcp", cfg.Target)
	if err != nil {
		return out, err
	}
	defer target.Close()
	out.Hello, out.Target = info, cfg.Target

	if cfg.SessionTimeout > 0 {
		deadline := time.Now().Add(cfg.SessionTimeout)
		_ = client.SetDeadline(deadline)
		_ = target.SetDeadline(deadline)
		defer client.SetDeadline(time.Time{})
	}
	if _, err := target.Write(rawHello); err != nil {
		return out, err
	}
	out.UpBytes = int64(len(rawHello))

	type copyResult struct {
		direction byte
		n         int64
		err       error
	}
	ch := make(chan copyResult, 2)
	downstream := &firstReadObserver{conn: target, callback: onFirstDownstream}
	go func() {
		n, err := copyLimited(client, downstream, cfg.MaxBytes)
		closeWrite(client)
		ch <- copyResult{direction: 'd', n: n, err: err}
	}()
	go func() {
		n, err := copyLimited(target, client, cfg.MaxBytes)
		closeWrite(target)
		ch <- copyResult{direction: 'u', n: n, err: err}
	}()

	first := <-ch
	if first.err != nil {
		_ = client.Close()
		_ = target.Close()
	}
	second := <-ch
	for _, r := range []copyResult{first, second} {
		if r.direction == 'u' {
			out.UpBytes += r.n
		} else {
			out.DownBytes += r.n
		}
	}
	if !benignCopyError(first.err) {
		return out, first.err
	}
	if !benignCopyError(second.err) {
		return out, second.err
	}
	return out, nil
}

type firstReadObserver struct {
	conn     net.Conn
	callback func() error
	once     sync.Once
	err      error
}

func (r *firstReadObserver) Read(p []byte) (int, error) {
	n, err := r.conn.Read(p)
	if n > 0 && r.callback != nil {
		r.once.Do(func() { r.err = r.callback() })
		if r.err != nil {
			return 0, r.err
		}
	}
	return n, err
}
