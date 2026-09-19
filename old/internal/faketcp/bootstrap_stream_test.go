package faketcp

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestBootstrapStreamReordersOnlyBootstrapBytes(t *testing.T) {
	var mu sync.Mutex
	seq := uint32(1000)
	s, err := NewBootstrapStream(5000,
		func(p []byte) (uint32, error) {
			mu.Lock(); seq += uint32(len(p)); end := seq; mu.Unlock(); return end, nil
		},
		func(uint32, time.Time) error { return nil },
		&net.TCPAddr{}, &net.TCPAddr{},
	)
	if err != nil { t.Fatal(err) }
	s.Feed(5005, []byte("world"))
	s.Feed(5000, []byte("hello"))
	buf := make([]byte, 10)
	if _, err := io.ReadFull(s, buf); err != nil { t.Fatal(err) }
	if got := string(buf); got != "helloworld" { t.Fatalf("got %q", got) }
}

func TestBootstrapStreamWriteWaitsForEachAck(t *testing.T) {
	var sent [][]byte
	var waits []uint32
	seq := uint32(2000)
	s, err := NewBootstrapStream(1,
		func(p []byte) (uint32, error) { sent = append(sent, append([]byte(nil), p...)); seq += uint32(len(p)); return seq, nil },
		func(end uint32, _ time.Time) error { waits = append(waits, end); return nil },
		&net.TCPAddr{}, &net.TCPAddr{},
	)
	if err != nil { t.Fatal(err) }
	payload := bytes.Repeat([]byte{'x'}, DefaultBootstrapChunk*2+7)
	n, err := s.Write(payload)
	if err != nil { t.Fatal(err) }
	if n != len(payload) { t.Fatalf("write=%d want=%d", n, len(payload)) }
	if len(sent) != 3 || len(waits) != 3 { t.Fatalf("sent=%d waits=%d", len(sent), len(waits)) }
	if len(sent[0]) != DefaultBootstrapChunk || len(sent[1]) != DefaultBootstrapChunk || len(sent[2]) != 7 { t.Fatalf("chunk sizes=%d,%d,%d", len(sent[0]), len(sent[1]), len(sent[2])) }
	if !(waits[0] < waits[1] && waits[1] < waits[2]) { t.Fatalf("waits=%v", waits) }
}

func TestBootstrapStreamDeadline(t *testing.T) {
	s, err := NewBootstrapStream(1,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
		&net.TCPAddr{}, &net.TCPAddr{},
	)
	if err != nil { t.Fatal(err) }
	_ = s.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
	buf := make([]byte, 1)
	_, err = s.Read(buf)
	if !errors.Is(err, ErrBootstrapTimeout) { t.Fatalf("err=%v", err) }
}

func TestBootstrapStreamRejectsUnboundedOutOfOrderInput(t *testing.T) {
	s, err := NewBootstrapStream(1000,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
		&net.TCPAddr{}, &net.TCPAddr{},
	)
	if err != nil { t.Fatal(err) }
	for i := 0; i < MaxBootstrapPendingChunks+1; i++ {
		s.Feed(uint32(2000+i*2), []byte{1})
	}
	buf := make([]byte, 1)
	_, err = s.Read(buf)
	if !errors.Is(err, ErrBootstrapOverflow) { t.Fatalf("err=%v want overflow", err) }
}

// The ordered bootstrap adapter is intentionally discarded at the mode barrier.
// Steady-state uses Receiver directly, whose contract is earliest-complete
// datagram delivery: a later independent payload is deliverable even when an
// earlier sequence range is missing.
func TestPostBootstrapReceiverStillDeliversLaterDatagramAcrossHole(t *testing.T) {
	r := NewReceiver(1000)
	if deliver, _ := r.Accept(1200, 100); !deliver { t.Fatal("later datagram was HOL-blocked by earlier hole") }
	if got := r.Next(); got != 1000 { t.Fatalf("cumulative ACK advanced across hole: %d", got) }
	if deliver, _ := r.Accept(1000, 200); !deliver { t.Fatal("repair/original hole payload not delivered") }
	if got := r.Next(); got != 1300 { t.Fatalf("cumulative ACK after filling hole=%d want=1300", got) }
}
