package faketcp

import (
	"bytes"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"
)

func newTestBootstrap(t *testing.T, next uint32, send BootstrapSend, wait BootstrapWaitAck) *BootstrapStream {
	t.Helper()
	s, err := NewBootstrapStream(next, send, wait, &net.TCPAddr{}, &net.TCPAddr{})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestBootstrapStreamReordersOnlyBootstrapBytes(t *testing.T) {
	var mu sync.Mutex
	seq := uint32(1000)
	s := newTestBootstrap(t, 5000,
		func(p []byte) (uint32, error) {
			mu.Lock()
			defer mu.Unlock()
			seq += uint32(len(p))
			return seq, nil
		},
		func(uint32, time.Time) error { return nil },
	)
	s.Feed(5005, []byte("world"))
	s.Feed(5000, []byte("hello"))
	buf := make([]byte, 10)
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatal(err)
	}
	if got := string(buf); got != "helloworld" {
		t.Fatalf("got %q", got)
	}
}

func TestBootstrapStreamWriteWaitsForEachAck(t *testing.T) {
	var sent [][]byte
	var waits []uint32
	seq := uint32(2000)
	s := newTestBootstrap(t, 1,
		func(p []byte) (uint32, error) {
			sent = append(sent, append([]byte(nil), p...))
			seq += uint32(len(p))
			return seq, nil
		},
		func(end uint32, _ time.Time) error {
			waits = append(waits, end)
			return nil
		},
	)
	payload := bytes.Repeat([]byte{'x'}, DefaultBootstrapChunk*2+7)
	n, err := s.Write(payload)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(payload) {
		t.Fatalf("write=%d want=%d", n, len(payload))
	}
	if len(sent) != 3 || len(waits) != 3 {
		t.Fatalf("sent=%d waits=%d", len(sent), len(waits))
	}
	if len(sent[0]) != DefaultBootstrapChunk || len(sent[1]) != DefaultBootstrapChunk || len(sent[2]) != 7 {
		t.Fatalf("chunk sizes=%d,%d,%d", len(sent[0]), len(sent[1]), len(sent[2]))
	}
	if !(waits[0] < waits[1] && waits[1] < waits[2]) {
		t.Fatalf("waits=%v", waits)
	}
}

func TestBootstrapStreamDeadline(t *testing.T) {
	s := newTestBootstrap(t, 1,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
	)
	_ = s.SetReadDeadline(time.Now().Add(5 * time.Millisecond))
	buf := make([]byte, 1)
	_, err := s.Read(buf)
	if !errors.Is(err, ErrBootstrapTimeout) {
		t.Fatalf("err=%v", err)
	}
}

func TestBootstrapStreamRejectsUnboundedOutOfOrderInput(t *testing.T) {
	s := newTestBootstrap(t, 1000,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
	)
	for i := 0; i < MaxBootstrapPendingChunks+1; i++ {
		s.Feed(uint32(2000+i*2), []byte{1})
	}
	buf := make([]byte, 1)
	_, err := s.Read(buf)
	if !errors.Is(err, ErrBootstrapOverflow) {
		t.Fatalf("err=%v want overflow", err)
	}
}

func TestBootstrapStreamRejectsBufferedByteOverflow(t *testing.T) {
	s := newTestBootstrap(t, 1000,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
	)
	s.Feed(1000, bytes.Repeat([]byte{1}, MaxBootstrapBufferedBytes+1))
	buf := make([]byte, 1)
	_, err := s.Read(buf)
	if !errors.Is(err, ErrBootstrapOverflow) {
		t.Fatalf("err=%v want overflow", err)
	}
}

func TestBootstrapStreamSequenceWrap(t *testing.T) {
	next := uint32(math.MaxUint32 - 3)
	s := newTestBootstrap(t, next,
		func(p []byte) (uint32, error) { return uint32(len(p)), nil },
		func(uint32, time.Time) error { return nil },
	)
	s.Feed(1, []byte("Z"))
	s.Feed(next, []byte("abcde"))
	buf := make([]byte, 6)
	if _, err := io.ReadFull(s, buf); err != nil {
		t.Fatal(err)
	}
	if got := string(buf); got != "abcdeZ" {
		t.Fatalf("got %q", got)
	}
}

func TestBootstrapMarkerIsSynchronousOnly(t *testing.T) {
	payload := []byte("tls-bootstrap")
	var markedInside bool
	s := newTestBootstrap(t, 1,
		func(p []byte) (uint32, error) {
			markedInside = isBootstrapPayload(p)
			return uint32(1 + len(p)), nil
		},
		func(uint32, time.Time) error { return nil },
	)
	if _, err := s.Write(payload); err != nil {
		t.Fatal(err)
	}
	if !markedInside {
		t.Fatal("bootstrap payload was not marked during synchronous send callback")
	}
	if isBootstrapPayload(payload) {
		t.Fatal("bootstrap marker leaked after send callback returned")
	}
}

func TestBootstrapCloseUnblocksReadWithEOF(t *testing.T) {
	s := newTestBootstrap(t, 1,
		func(p []byte) (uint32, error) { return uint32(1 + len(p)), nil },
		func(uint32, time.Time) error { return nil },
	)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	n, err := s.Read(buf)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("read n=%d err=%v", n, err)
	}
}
