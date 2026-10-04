package platformproxy

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func testTCPConfig() TCPReliabilityConfig {
	return TCPReliabilityConfig{
		ChunkSize:       4,
		MaxInFlight:     3,
		RTO:             100 * time.Millisecond,
		MaxRetransmits:  2,
		MaxReorderBytes: 12,
	}
}

func TestTCPTransmitChunkAckAndWindow(t *testing.T) {
	tx, err := NewTCPTransmit(7, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1, 0)
	frames, err := tx.Queue([]byte("abcdefghij"), false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 3 || frames[0].Offset != 0 || frames[1].Offset != 4 || frames[2].Offset != 8 {
		t.Fatalf("frames=%+v", frames)
	}
	if _, err := tx.Queue([]byte("x"), false, now); !errors.Is(err, ErrTCPWindowFull) {
		t.Fatalf("window err=%v", err)
	}

	// Cumulative ACKs may cut through one transmitted chunk. The confirmed
	// prefix is discarded so only the suffix remains eligible for retry.
	if err := tx.Ack(6); err != nil {
		t.Fatal(err)
	}
	if tx.InFlight() != 2 || tx.AckedOffset() != 6 {
		t.Fatalf("inflight=%d ack=%d", tx.InFlight(), tx.AckedOffset())
	}
	if tx.pending[0].frame.Offset != 6 || string(tx.pending[0].frame.Payload) != "gh" {
		t.Fatalf("partial=%+v", tx.pending[0].frame)
	}
	if _, err := tx.Queue([]byte("klmn"), false, now); err != nil {
		t.Fatal(err)
	}
}

func TestTCPTransmitRetransmitsOnlyUnacked(t *testing.T) {
	tx, err := NewTCPTransmit(8, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(2, 0)
	frames, err := tx.Queue([]byte("abcdefgh"), false, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Ack(4); err != nil {
		t.Fatal(err)
	}
	if got, err := tx.RetransmitDue(now.Add(99 * time.Millisecond)); err != nil || len(got) != 0 {
		t.Fatalf("early=%+v err=%v", got, err)
	}
	got, err := tx.RetransmitDue(now.Add(100 * time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Offset != frames[1].Offset || !bytes.Equal(got[0].Payload, frames[1].Payload) {
		t.Fatalf("got=%+v", got)
	}
	if _, err := tx.RetransmitDue(now.Add(200 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.RetransmitDue(now.Add(300 * time.Millisecond)); !errors.Is(err, ErrTCPRetryLimit) {
		t.Fatalf("retry err=%v", err)
	}
}

func TestTCPReceiveOutOfOrderDuplicateAndFIN(t *testing.T) {
	rx, err := NewTCPReceive(9, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}

	result, err := rx.Push(Frame{Kind: KindTCPData, FlowID: 9, Offset: 4, Payload: []byte("efgh")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Delivered) != 0 || result.Ack.Offset != 0 || rx.BufferedBytes() != 4 {
		t.Fatalf("result=%+v buffered=%d", result, rx.BufferedBytes())
	}

	result, err = rx.Push(Frame{Kind: KindTCPData, FlowID: 9, Offset: 0, Payload: []byte("abcd")})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Delivered) != "abcdefgh" || result.Ack.Offset != 8 {
		t.Fatalf("result=%+v", result)
	}

	result, err = rx.Push(Frame{Kind: KindTCPData, FlowID: 9, Offset: 0, Payload: []byte("abcd")})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Duplicate || len(result.Delivered) != 0 || result.Ack.Offset != 8 {
		t.Fatalf("duplicate=%+v", result)
	}

	result, err = rx.Push(Frame{Kind: KindTCPData, FlowID: 9, Offset: 8, Payload: []byte("ij"), FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(result.Delivered) != "ij" || result.Ack.Offset != 10 || !result.FIN || !rx.FINDelivered() {
		t.Fatalf("fin=%+v", result)
	}
}

func TestTCPReceiveFlowIsolationAndClose(t *testing.T) {
	a, err := NewTCPReceive(10, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewTCPReceive(11, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Push(Frame{Kind: KindTCPData, FlowID: 11, Offset: 0, Payload: []byte("x")}); !errors.Is(err, ErrMalformed) {
		t.Fatalf("flow err=%v", err)
	}
	result, err := b.Push(Frame{Kind: KindTCPData, FlowID: 11, Offset: 0, Payload: []byte("ok")})
	if err != nil || string(result.Delivered) != "ok" {
		t.Fatalf("b=%+v err=%v", result, err)
	}
	if err := a.Close(Frame{Kind: KindTCPClose, FlowID: 10}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Push(Frame{Kind: KindTCPData, FlowID: 10, Offset: 0, Payload: []byte("x")}); !errors.Is(err, ErrTCPClosed) {
		t.Fatalf("closed err=%v", err)
	}
	result, err = b.Push(Frame{Kind: KindTCPData, FlowID: 11, Offset: 2, Payload: []byte("ay")})
	if err != nil || string(result.Delivered) != "ay" {
		t.Fatalf("b2=%+v err=%v", result, err)
	}
}

func TestTCPTransmitFINAndAbort(t *testing.T) {
	tx, err := NewTCPTransmit(12, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(3, 0)
	frames, err := tx.Queue([]byte("hello"), true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(frames) != 2 || !frames[1].FIN {
		t.Fatalf("frames=%+v", frames)
	}
	if _, err := tx.Queue([]byte("later"), false, now); !errors.Is(err, ErrTCPClosed) {
		t.Fatalf("post-fin err=%v", err)
	}
	if err := tx.Ack(5); err != nil {
		t.Fatal(err)
	}
	if !tx.FINAcked() {
		t.Fatal("FIN not acknowledged")
	}
	closeFrame := tx.Abort()
	if closeFrame.Kind != KindTCPClose || closeFrame.FlowID != 12 {
		t.Fatalf("close=%+v", closeFrame)
	}
	if _, err := tx.RetransmitDue(now.Add(time.Second)); !errors.Is(err, ErrTCPClosed) {
		t.Fatalf("closed retransmit err=%v", err)
	}
}

func TestTCPReceiveZeroByteFIN(t *testing.T) {
	rx, err := NewTCPReceive(13, testTCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	result, err := rx.Push(Frame{Kind: KindTCPData, FlowID: 13, Offset: 0, FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FIN || result.Ack.Offset != 0 || !rx.FINDelivered() {
		t.Fatalf("zero FIN=%+v", result)
	}
}
