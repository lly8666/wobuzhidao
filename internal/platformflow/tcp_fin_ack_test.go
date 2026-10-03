package platformflow

import (
	"testing"
	"time"
)

func TestTCPStandaloneFINIsNotRetiredByEarlierDataACK(t *testing.T) {
	now := time.Unix(100, 0)
	cfg := DefaultTCPReliabilityConfig()
	tx, _ := NewTCPTransmit(7, cfg)
	rx, _ := NewTCPReceive(7, cfg)
	data, _ := tx.Queue([]byte("payload"), false, now)
	received, err := rx.Push(data[0])
	if err != nil { t.Fatal(err) }
	fin, err := tx.Queue(nil, true, now)
	if err != nil { t.Fatal(err) }
	// This ACK was generated before FIN was queued; FIN itself is lost.
	if err := tx.Ack(received.Ack.Offset); err != nil { t.Fatal(err) }
	if tx.FINAcked() || tx.InFlight() != 1 { t.Fatal("payload ACK incorrectly retired the unseen FIN") }
	repair, err := tx.RetransmitDue(now.Add(cfg.RTO))
	if err != nil || len(repair) != 1 || !repair[0].FIN || repair[0].Offset != fin[0].Offset || len(repair[0].Payload) != 0 {
		t.Fatalf("FIN repair=%+v err=%v", repair, err)
	}
	result, err := rx.Push(repair[0])
	if err != nil || !result.FIN || result.Ack.Offset != received.Ack.Offset+1 { t.Fatalf("FIN receive=%+v err=%v", result, err) }
	if err := tx.Ack(result.Ack.Offset); err != nil || !tx.FINAcked() || tx.InFlight() != 0 { t.Fatalf("FIN ACK state err=%v", err) }
	duplicate, err := rx.Push(fin[0])
	if err != nil || !duplicate.Duplicate || duplicate.Ack.Offset != result.Ack.Offset { t.Fatalf("FIN duplicate=%+v err=%v", duplicate, err) }
}

func TestTCPFINOnAlreadyDeliveredBytesStillCommitsHalfClose(t *testing.T) {
	rx, _ := NewTCPReceive(7, DefaultTCPReliabilityConfig())
	frame := Frame{Kind:KindTCPData, FlowID:7, Payload:[]byte("data")}
	first, _ := rx.Push(frame)
	frame.FIN = true
	fin, err := rx.Push(frame)
	if err != nil || fin.Duplicate || !fin.FIN || len(fin.Delivered) != 0 || fin.Ack.Offset != first.Ack.Offset+1 || !rx.FINDelivered() {
		t.Fatalf("late first FIN=%+v err=%v", fin, err)
	}
}
