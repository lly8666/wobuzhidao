package platformflow

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

type fencedStreamConn struct {
	startupHelloConn
	entered chan struct{}
	release chan struct{}
	halfClosed chan struct{}
	once sync.Once
	closed atomic.Int32
	mu sync.Mutex
	events []string
}

func (c *fencedStreamConn) Write(p []byte) (int, error) {
	c.once.Do(func() { close(c.entered); <-c.release })
	c.mu.Lock()
	c.events = append(c.events, "write:"+string(p))
	c.mu.Unlock()
	return len(p), nil
}

func (c *fencedStreamConn) CloseWrite() error {
	c.mu.Lock()
	c.events = append(c.events, "fin")
	c.mu.Unlock()
	select { case c.halfClosed <- struct{}{}: default: }
	return nil
}

func (c *fencedStreamConn) Close() error { c.closed.Add(1); return nil }

func TestTCPFlowDeliveryCommitsBeforeFINOrClose(t *testing.T) {
	for _, serverSide := range []bool{false, true} {
		for _, ackWhileWriting := range []bool{false, true} {
			t.Run(fmt.Sprintf("server%t-ack%t", serverSide, ackWhileWriting), func(t *testing.T) {
				lease := testLease(t)
				owner, _ := datapath.NewLeasedTunnelOwner(lease, 1, 8)
				t.Cleanup(owner.Close)
				if _, err := owner.AttachInitial(1, testLane(t, datapath.RoleClient, lease, 31)); err != nil { t.Fatal(err) }
				channel, _ := NewTunnelChannel(owner, func(Outbound) error { return nil })
				tunnel, _ := channel.OpenFlow()
				t.Cleanup(func() { tunnel.Close() })
				conn := &fencedStreamConn{entered:make(chan struct{}), release:make(chan struct{}), halfClosed:make(chan struct{}, 2)}
				var release sync.Once
				unblock := func() { release.Do(func() { close(conn.release) }) }
				t.Cleanup(unblock)
				tx, _ := NewTCPTransmit(7, DefaultTCPReliabilityConfig())
				rx, _ := NewTCPReceive(7, DefaultTCPReliabilityConfig())
				now := time.Unix(100, 0)
				if ackWhileWriting { tx.Queue([]byte("local"), true, now) }
				var data func(Frame) error
				var ack func(Frame) error
				if serverSide {
					f := &tcpServerFlow{id:7, conn:conn, tunnel:tunnel, tx:tx, rx:rx}
					f.cond = sync.NewCond(&f.mu)
					s := &TCPServer{flows:map[uint64]*tcpServerFlow{7:f}, retired:newTCPRetiredSet(time.Second, 8)}
					data = func(frame Frame) error { return s.handleData(f, frame, now) }
					ack = func(frame Frame) error { return s.handleAck(f, frame, now) }
				} else {
					f := &tcpClientFlow{id:7, conn:conn, tunnel:tunnel, tx:tx, rx:rx, opened:true}
					f.cond = sync.NewCond(&f.mu)
					c := &TCPClient{flows:map[uint64]*tcpClientFlow{7:f}, retired:newTCPRetiredSet(time.Second, 8)}
					data = func(frame Frame) error { return c.handleData(f, frame, now) }
					ack = func(frame Frame) error { return c.handleAck(f, frame, now) }
				}
				first := make(chan error, 1)
				go func() { first <- data(Frame{Kind:KindTCPData, FlowID:7, Payload:[]byte("response"), FIN:ackWhileWriting}) }()
				select { case <-conn.entered: case <-time.After(3*time.Second): t.Fatal("first write not entered") }
				second := make(chan error, 1)
				if ackWhileWriting {
					if err := ack(Frame{Kind:KindTCPAck, FlowID:7, Offset:tx.NextOffset()}); err != nil { t.Fatal(err) }
					if conn.closed.Load() != 0 { t.Fatal("ACK closed a stream before pending local delivery committed") }
				} else {
					go func() { second <- data(Frame{Kind:KindTCPData, FlowID:7, Offset:8, FIN:true}) }()
					select {
					case <-conn.halfClosed: t.Fatal("FIN overtook an earlier local write")
					case err := <-second: t.Fatalf("FIN completed while earlier write blocked: %v", err)
					case <-time.After(100*time.Millisecond):
					}
				}
				unblock()
				select { case err := <-first: if err != nil { t.Fatal(err) }; case <-time.After(3*time.Second): t.Fatal("first delivery stuck") }
				if !ackWhileWriting { select { case err := <-second: if err != nil { t.Fatal(err) }; case <-time.After(3*time.Second): t.Fatal("FIN stuck") } }
				conn.mu.Lock()
				events := append([]string(nil), conn.events...)
				conn.mu.Unlock()
				if fmt.Sprint(events) != "[write:response fin]" { t.Fatalf("local stream order=%v", events) }
			})
		}
	}
}
