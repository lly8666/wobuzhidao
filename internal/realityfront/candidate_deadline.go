package realityfront

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

var ErrCandidateDeadline = errors.New("realityfront: candidate establishment deadline exceeded")

type candidateDeadline struct {
	conn     net.Conn
	ctx      context.Context
	deadline time.Time
	done     chan struct{}
	once     sync.Once
}

func beginCandidateDeadline(ctx context.Context, conn net.Conn, timeout time.Duration) (*candidateDeadline, error) {
	if conn == nil {
		return nil, errors.New("realityfront: nil candidate connection")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	g := &candidateDeadline{
		conn:     conn,
		ctx:      ctx,
		deadline: deadline,
		done:     make(chan struct{}),
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.SetDeadline(time.Now())
		case <-g.done:
		}
	}()
	return g, nil
}

func (g *candidateDeadline) Remaining() time.Duration {
	d := time.Until(g.deadline)
	if d <= 0 {
		return time.Nanosecond
	}
	return d
}

func (g *candidateDeadline) Rearm() error {
	if err := g.ctx.Err(); err != nil {
		_ = g.conn.SetDeadline(time.Now())
		return err
	}
	if !time.Now().Before(g.deadline) {
		_ = g.conn.SetDeadline(time.Now())
		return ErrCandidateDeadline
	}
	return g.conn.SetDeadline(g.deadline)
}

func (g *candidateDeadline) Finish(clear bool) {
	g.once.Do(func() { close(g.done) })
	if clear {
		_ = g.conn.SetDeadline(time.Time{})
	}
}

func candidateError(ctx context.Context, err error) error {
	if ctx != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	return err
}
