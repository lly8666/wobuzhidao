package platformflow

import (
    "bytes"
    "testing"
    "time"
)

// This is an E0 diagnosis, not an approved change to the 500ms RTO.
// It demonstrates why a no-loss 300ms-per-way path can retransmit a
// reliably-delivered frame before its first ACK can return.
func TestTCPDefaultRTOCanRetransmitBeforeLongRTTACK(t *testing.T) {
    cfg := DefaultTCPReliabilityConfig()
    if cfg.RTO != 500*time.Millisecond {
        t.Fatalf("diagnostic baseline changed: RTO=%v", cfg.RTO)
    }
    start := time.Unix(1000, 0)
    tx, err := NewTCPTransmit(17, cfg)
    if err != nil { t.Fatal(err) }
    rx, err := NewTCPReceive(17, cfg)
    if err != nil { t.Fatal(err) }

    sent, err := tx.Queue([]byte("app-data"), false, start)
    if err != nil || len(sent) != 1 { t.Fatalf("queue=%v err=%v", sent, err) }
    received, err := rx.Push(sent[0]) // first arrival at receiver after ~300ms
    if err != nil || !bytes.Equal(received.Delivered, []byte("app-data")) {
        t.Fatalf("first delivery=%+v err=%v", received, err)
    }
    early, err := tx.RetransmitDue(start.Add(499*time.Millisecond))
    if err != nil || len(early) != 0 { t.Fatalf("early retry=%d err=%v", len(early), err) }
    retry, err := tx.RetransmitDue(start.Add(500*time.Millisecond))
    if err != nil || len(retry) != 1 { t.Fatalf("premature retry=%d err=%v", len(retry), err) }
    if retry[0].Offset != sent[0].Offset || !bytes.Equal(retry[0].Payload, sent[0].Payload) {
        t.Fatalf("retry changed logical TCP bytes: %v vs %v", retry[0], sent[0])
    }
    duplicate, err := rx.Push(retry[0])
    if err != nil || !duplicate.Duplicate || len(duplicate.Delivered) != 0 {
        t.Fatalf("retransmission must not deliver duplicate bytes: %+v err=%v", duplicate, err)
    }
    if err := tx.Ack(received.Ack.Offset); err != nil { t.Fatal(err) } // ACK at ~600ms
    if tx.InFlight() != 0 { t.Fatal("ACK failed to retire in-flight frame") }
    due, err := tx.RetransmitDue(start.Add(time.Second))
    if err != nil || len(due) != 0 { t.Fatalf("ACKed frame still due: n=%d err=%v", len(due), err) }
}

// A hypothetical larger RTO avoids one no-loss redundant retransmission,
// but also delays the first repair for a truly lost frame. Both halves must
// be preserved in later weak-network acceptance gates (5205/5305).
func TestTCPLongRTOTimingTradeoffNoLossAndRealLoss(t *testing.T) {
    cfg := DefaultTCPReliabilityConfig()
    cfg.RTO = 700*time.Millisecond
    start := time.Unix(1100, 0)

    tx, err := NewTCPTransmit(21, cfg)
    if err != nil { t.Fatal(err) }
    sent, err := tx.Queue([]byte("clean"), false, start)
    if err != nil || len(sent) != 1 { t.Fatalf("queue: %v, %v", sent, err) }
    due, err := tx.RetransmitDue(start.Add(600*time.Millisecond))
    if err != nil || len(due) != 0 { t.Fatalf("unexpected 600ms retry: %d %v", len(due), err) }
    if err := tx.Ack(uint64(len(sent[0].Payload))); err != nil { t.Fatal(err) }
    due, err = tx.RetransmitDue(start.Add(700*time.Millisecond))
    if err != nil || len(due) != 0 { t.Fatalf("ACKed frame retried: %d %v", len(due), err) }

    lostTX, err := NewTCPTransmit(22, cfg)
    if err != nil { t.Fatal(err) }
    original, err := lostTX.Queue([]byte("truly-lost"), false, start)
    if err != nil || len(original) != 1 { t.Fatalf("queue lost frame: %d %v", len(original), err) }
    due, err = lostTX.RetransmitDue(start.Add(600*time.Millisecond))
    if err != nil || len(due) != 0 { t.Fatalf("repair fired earlier than configured RTO") }
    due, err = lostTX.RetransmitDue(start.Add(700*time.Millisecond))
    if err != nil || len(due) != 1 || !bytes.Equal(due[0].Payload, original[0].Payload) {
        t.Fatalf("actual loss not repaired at RTO: %v %v", due, err)
    }
    rx, err := NewTCPReceive(22, cfg)
    if err != nil { t.Fatal(err) }
    delivered, err := rx.Push(due[0])
    if err != nil || !bytes.Equal(delivered.Delivered, []byte("truly-lost")) {
        t.Fatalf("lost frame repair invalid: %+v %v", delivered, err)
    }
    if err := lostTX.Ack(delivered.Ack.Offset); err != nil || lostTX.InFlight() != 0 {
        t.Fatalf("repair ACK incomplete: inflight=%d err=%v", lostTX.InFlight(), err)
    }
}
