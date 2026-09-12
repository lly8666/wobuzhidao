#!/usr/bin/env python3
import pathlib
import sys

root = pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else ".")

def replace_once(path, old, new, label):
    p = root / path
    s = p.read_text()
    n = s.count(old)
    if n != 1:
        raise SystemExit(f"{label}: marker count={n} in {path}")
    p.write_text(s.replace(old, new, 1))

# 1) The old 1536 receiver horizon was only ~2.4% above a 10-Mbit/FEC20:20
# 600-ms BDP and becomes equally tight again around 20 Mbit if merely doubled.
# Fresh admission is already decoupled from the 4096 shadow-repair ceiling, so
# use that full bounded horizon for receiver repair debt as well.
replace_once(
    "internal/faketcp/arq.go",
    '''\t// PartialReliabilityReorderSoftLimit is deliberately below the 4096-record\n\t// sender hard ceiling. Once this many first-arrival records are buffered\n\t// beyond an old hole, the receiver stops paying unbounded TCP-like repair\n\t// debt for that hole: it advances cumulative ACK to the oldest live SACK\n\t// range and lets normal ACK processing retire the abandoned sender state.\n\t// Three eighths of the hard window is 1536 records, approximately one 10-Mbit\n\t// FEC20:20 BDP at the project's 600ms target RTT, while leaving 2560 records\n\t// of headroom for ACK flight/loss before the hard debt ceiling.\n\tPartialReliabilityReorderSoftLimit = (MaxSteadyStateOutstandingDatagrams * 3) / 8\n''',
    '''\t// PartialReliabilityReorderSoftLimit bounds receiver-side TCP-like repair\n\t// debt, not application delivery. The previous 1536-record value was only\n\t// about one 10-Mbit/FEC20:20 BDP at 600ms RTT and therefore had essentially\n\t// no reordering/loss margin. Fresh admission is already independent of the\n\t// shadow-repair window, so use the full bounded 4096-record debt horizon.\n\t// Late first arrivals below a forgiven ACK remain eligible for steady-state\n\t// datagram delivery; crossing this horizon gives up repair, not real data.\n\tPartialReliabilityReorderSoftLimit = MaxSteadyStateOutstandingDatagrams\n\n\t// Keep just enough exact delivery history to suppress normal late repairs.\n\t// Once an entry ages out, steady-state carrier/DTLS replay protection remains\n\t// authoritative, so FakeTCP prefers a possible duplicate over a false drop.\n\trecentDeliveryHistoryLimit = MaxSteadyStateOutstandingDatagrams\n''',
    "reorder horizon",
)

replace_once(
    "internal/faketcp/arq.go",
    '''type ReceiverStats struct {\n\tDelivered      uint64\n\tDuplicates     uint64\n\tOutOfOrder     uint64\n\tForgivenGaps   uint64\n\tForgivenBytes  uint64\n\tPeakBufferedOO int\n}\n\ntype Receiver struct {\n\tnext           uint32\n\toutOfOrder     map[uint32]uint32\n\tsacksByStart   map[uint32]uint32\n\tsackStartByEnd map[uint32]uint32\n\trecentSACK     [4]uint32\n\trecentSACKN    int\n\tcoverageQueue  []uint32\n\tcoverageAt     int\n\tstats          ReceiverStats\n}\n''',
    '''type ReceiverStats struct {\n\tDelivered          uint64\n\tDuplicates         uint64\n\tOutOfOrder         uint64\n\tLateBelowACK       uint64\n\tBelowNextDrops     uint64\n\tBufferedDuplicates uint64\n\tForgivenGaps       uint64\n\tForgivenBytes      uint64\n\tPeakBufferedOO     int\n}\n\ntype Receiver struct {\n\tnext           uint32\n\toutOfOrder     map[uint32]uint32\n\tsacksByStart   map[uint32]uint32\n\tsackStartByEnd map[uint32]uint32\n\trecentSACK     [4]uint32\n\trecentSACKN    int\n\tcoverageQueue  []uint32\n\tcoverageAt     int\n\n\t// Bootstrap is strict TCP. Steady-state carrier traffic can instead deliver\n\t// a packet below a forgiven cumulative ACK when FakeTCP has no evidence that\n\t// exact record was already delivered. A bounded recent-delivery set suppresses\n\t// ordinary late repairs; older ambiguity is left to carrier/DTLS replay logic.\n\tsteadyStateDelivery bool\n\tsteadyFloor         uint32\n\tsteadyFloorActive   bool\n\tsteadyDelivered     uint64\n\tdeliveredBySeq      map[uint32]uint32\n\tdeliveryOrder       []uint32\n\tdeliveryHead        int\n\n\tstats ReceiverStats\n}\n''',
    "receiver structs",
)

replace_once(
    "internal/faketcp/arq.go",
    '''func (r *Receiver) Next() uint32          { return r.next }\nfunc (r *Receiver) Stats() ReceiverStats { return r.stats }\n\nfunc (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, sackNeeded bool) {\n\tif payloadLen <= 0 {\n\t\treturn false, false\n\t}\n\tend := seq + uint32(payloadLen)\n\tif seqLT(seq, r.next) {\n\t\tr.stats.Duplicates++\n\t\treturn false, len(r.sacksByStart) != 0\n\t}\n\tif _, exists := r.outOfOrder[seq]; exists {\n\t\tr.stats.Duplicates++\n\t\treturn false, len(r.sacksByStart) != 0\n\t}\n\tr.stats.Delivered++\n\tif seq != r.next {\n\t\tr.stats.OutOfOrder++\n\t\tr.outOfOrder[seq] = end\n\t\tif n := len(r.outOfOrder); n > r.stats.PeakBufferedOO {\n\t\t\tr.stats.PeakBufferedOO = n\n\t\t}\n\t\tr.insertSACKRange(seq, end)\n\t\tr.maybeForgiveReorderPressure()\n\t\treturn true, len(r.sacksByStart) != 0\n\t}\n\n\tr.next = end\n\tr.consumeContiguousSACKs()\n\treturn true, len(r.sacksByStart) != 0\n}\n''',
    '''func (r *Receiver) Next() uint32          { return r.next }\nfunc (r *Receiver) Stats() ReceiverStats { return r.stats }\n\n// EnableSteadyStateDelivery is called only after the TLS/bootstrap stream has\n// finished. From this point onward payload is datagram carrier/DTLS traffic:\n// advancing cumulative ACK may abandon optional FakeTCP repair, but must not\n// make a possibly fresh late record undeliverable.\nfunc (r *Receiver) EnableSteadyStateDelivery() {\n\tif r.steadyStateDelivery {\n\t\treturn\n\t}\n\tr.steadyStateDelivery = true\n\tr.steadyFloor = r.next\n\tr.steadyFloorActive = true\n\tr.deliveredBySeq = make(map[uint32]uint32)\n}\n\nfunc (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, sackNeeded bool) {\n\tif payloadLen <= 0 {\n\t\treturn false, false\n\t}\n\tend := seq + uint32(payloadLen)\n\tif seqLT(seq, r.next) {\n\t\t// Keep pre-steady/bootstrap sequence space strict. During the first bounded\n\t\t// steady-state history window this also prevents a straggling TLS repair\n\t\t// from being mistaken for a DTLS datagram.\n\t\tif !r.steadyStateDelivery || (r.steadyFloorActive && seqLT(seq, r.steadyFloor)) {\n\t\t\tr.stats.Duplicates++\n\t\t\tr.stats.BelowNextDrops++\n\t\t\treturn false, len(r.sacksByStart) != 0\n\t\t}\n\t\tif _, alreadyDelivered := r.deliveredBySeq[seq]; alreadyDelivered {\n\t\t\tr.stats.Duplicates++\n\t\t\tr.stats.BelowNextDrops++\n\t\t\treturn false, len(r.sacksByStart) != 0\n\t\t}\n\n\t\t// Cumulative ACK has already crossed this sequence, so it cannot recreate\n\t\t// sender repair debt or SACK state. Deliver once locally and let the bounded\n\t\t// carrier/DTLS replay layer resolve ambiguity older than our exact history.\n\t\tr.stats.Delivered++\n\t\tr.stats.LateBelowACK++\n\t\tr.rememberDelivered(seq, end)\n\t\treturn true, len(r.sacksByStart) != 0\n\t}\n\tif _, exists := r.outOfOrder[seq]; exists {\n\t\tr.stats.Duplicates++\n\t\tr.stats.BufferedDuplicates++\n\t\treturn false, len(r.sacksByStart) != 0\n\t}\n\tr.stats.Delivered++\n\tr.rememberDelivered(seq, end)\n\tif seq != r.next {\n\t\tr.stats.OutOfOrder++\n\t\tr.outOfOrder[seq] = end\n\t\tif n := len(r.outOfOrder); n > r.stats.PeakBufferedOO {\n\t\t\tr.stats.PeakBufferedOO = n\n\t\t}\n\t\tr.insertSACKRange(seq, end)\n\t\tr.maybeForgiveReorderPressure()\n\t\treturn true, len(r.sacksByStart) != 0\n\t}\n\n\tr.next = end\n\tr.consumeContiguousSACKs()\n\treturn true, len(r.sacksByStart) != 0\n}\n\nfunc (r *Receiver) rememberDelivered(seq, end uint32) {\n\tif !r.steadyStateDelivery {\n\t\treturn\n\t}\n\tif r.deliveredBySeq == nil {\n\t\tr.deliveredBySeq = make(map[uint32]uint32)\n\t}\n\tif _, exists := r.deliveredBySeq[seq]; exists {\n\t\treturn\n\t}\n\tr.deliveredBySeq[seq] = end\n\tr.deliveryOrder = append(r.deliveryOrder, seq)\n\tr.steadyDelivered++\n\tif r.steadyDelivered >= recentDeliveryHistoryLimit {\n\t\tr.steadyFloorActive = false\n\t}\n\tfor len(r.deliveredBySeq) > recentDeliveryHistoryLimit {\n\t\tfor r.deliveryHead < len(r.deliveryOrder) {\n\t\t\told := r.deliveryOrder[r.deliveryHead]\n\t\t\tr.deliveryHead++\n\t\t\tif _, live := r.deliveredBySeq[old]; live {\n\t\t\t\tdelete(r.deliveredBySeq, old)\n\t\t\t\tbreak\n\t\t\t}\n\t\t}\n\t}\n\tif r.deliveryHead >= recentDeliveryHistoryLimit && r.deliveryHead*2 >= len(r.deliveryOrder) {\n\t\tcopy(r.deliveryOrder, r.deliveryOrder[r.deliveryHead:])\n\t\tr.deliveryOrder = r.deliveryOrder[:len(r.deliveryOrder)-r.deliveryHead]\n\t\tr.deliveryHead = 0\n\t}\n}\n''',
    "receiver accept",
)

replace_once(
    "internal/faketcp/arq.go",
    '''// No extra control packet is required. The next ordinary ACK carries the new\n// cumulative value, and normal sender ACK processing releases both unresolved\n// payload and SACK-retired tombstones below it. A very late repair then lands\n// below r.next and is treated as a duplicate, so abandonment cannot create a\n// second repair cascade.\n''',
    '''// No extra control packet is required. The next ordinary ACK carries the new\n// cumulative value, and normal sender ACK processing releases both unresolved\n// payload and SACK-retired tombstones below it. In strict/bootstrap mode a late\n// packet below r.next is discarded as before. In steady-state datagram mode an\n// exact recent duplicate is suppressed, while an unknown late record may still\n// be delivered upward without recreating ACK/SACK repair debt.\n''',
    "forgiveness comment",
)

# Server association delegates the steady-state transition under its own lock.
replace_once(
    "internal/faketcp/server_mux.go",
    '''func (a *ServerAssociation) State() ServerAssociationState {\n\ta.mu.Lock()\n\tdefer a.mu.Unlock()\n\treturn a.state\n}\n\nfunc (a *ServerAssociation) SYNACK() (seq, ack uint32, err error) {\n''',
    '''func (a *ServerAssociation) State() ServerAssociationState {\n\ta.mu.Lock()\n\tdefer a.mu.Unlock()\n\treturn a.state\n}\n\nfunc (a *ServerAssociation) EnableSteadyStateDelivery() {\n\ta.mu.Lock()\n\ta.receiver.EnableSteadyStateDelivery()\n\ta.mu.Unlock()\n}\n\nfunc (a *ServerAssociation) SYNACK() (seq, ack uint32, err error) {\n''',
    "server association steady mode",
)

# Generic/client endpoint: after handshake/bootstrap, all following carrier payload
# is datagram/DTLS traffic, so activate late-first-arrival delivery exactly once.
replace_once(
    "cmd/wbd-faketcp/main.go",
    '''\te.senderMu.Lock()\n\tstartupRTO := e.sender.RTO()\n''',
    '''\te.receiverMu.Lock()\n\te.receiver.EnableSteadyStateDelivery()\n\te.receiverMu.Unlock()\n\n\te.senderMu.Lock()\n\tstartupRTO := e.sender.RTO()\n''',
    "endpoint steady mode",
)

# Mux server: activate receiver datagram semantics before the final ACK is allowed
# to fall through carrying first DTLS bytes.
replace_once(
    "cmd/wbd-faketcp-mux/main_linux.go",
    '''func (s *muxServer) activateDTLS(sess *muxSession) error {\n\tlinkAddr, err := net.ResolveUDPAddr("udp4", s.cfg.linkTarget)\n\tif err != nil {\n\t\treturn err\n\t}\n\tworker, err := dtlsworker.StartServer''',
    '''func (s *muxServer) activateDTLS(sess *muxSession) error {\n\tlinkAddr, err := net.ResolveUDPAddr("udp4", s.cfg.linkTarget)\n\tif err != nil {\n\t\treturn err\n\t}\n\tsess.assoc.EnableSteadyStateDelivery()\n\tworker, err := dtlsworker.StartServer''',
    "mux steady mode",
)

# Existing invariant test: the receiver horizon is now the full bounded repair
# debt ceiling rather than a 3/8 sub-window.
replace_once(
    "internal/faketcp/partial_reliability_ack_test.go",
    '''func TestPartialReliabilitySoftLimitLeavesHardWindowHeadroom(t *testing.T) {\n\tif PartialReliabilityReorderSoftLimit <= 0 || PartialReliabilityReorderSoftLimit >= MaxSteadyStateOutstandingDatagrams {\n\t\tt.Fatalf("soft=%d hard=%d", PartialReliabilityReorderSoftLimit, MaxSteadyStateOutstandingDatagrams)\n\t}\n\tif got, want := PartialReliabilityReorderSoftLimit, (MaxSteadyStateOutstandingDatagrams*3)/8; got != want {\n\t\tt.Fatalf("soft=%d want=%d", got, want)\n\t}\n}\n''',
    '''func TestPartialReliabilityHorizonUsesFullBoundedRepairWindow(t *testing.T) {\n\tif got, want := PartialReliabilityReorderSoftLimit, MaxSteadyStateOutstandingDatagrams; got != want {\n\t\tt.Fatalf("receiver horizon=%d want=%d", got, want)\n\t}\n\t// 10 Mbit/s of 1000-byte source packets with FEC20:20 is about 2500\n\t// carrier records/s; at 600ms RTT that is ~1500 records in flight. The old\n\t// 1536 threshold left ~2.4%% margin. 4096 leaves substantial room while\n\t// remaining independently bounded.\n\tif PartialReliabilityReorderSoftLimit <= 2*1500 {\n\t\tt.Fatalf("receiver horizon=%d leaves insufficient 10M/600ms BDP margin", PartialReliabilityReorderSoftLimit)\n\t}\n}\n''',
    "partial reliability limit test",
)

new_test = root / "internal/faketcp/late_first_arrival_test.go"
if new_test.exists():
    raise SystemExit("late_first_arrival_test.go already exists")
new_test.write_text(r'''package faketcp

import "testing"

func TestSteadyStateLateFirstArrivalSurvivesForgivenACK(t *testing.T) {
	const (
		start  = uint32(10000)
		record = 100
	)
	r := NewReceiver(start)
	r.EnableSteadyStateDelivery()

	// Lose the oldest record while enough later records arrive to cross the
	// bounded repair-debt horizon and advance cumulative ACK over the hole.
	for i := 1; i <= PartialReliabilityReorderSoftLimit; i++ {
		seq := start + uint32(i*record)
		deliver, _ := r.Accept(seq, record)
		if !deliver {
			t.Fatalf("record %d was not first-arrival delivered", i)
		}
	}
	if !seqLT(start, r.Next()) {
		t.Fatalf("receiver did not forgive original hole: next=%d start=%d", r.Next(), start)
	}

	// The missing record now arrives for the first time. ACK repair debt is gone,
	// but real steady-state data must still reach carrier/DTLS.
	deliver, sack := r.Accept(start, record)
	if !deliver {
		t.Fatal("late first arrival below forgiven ACK was locally dropped")
	}
	if sack {
		t.Fatal("late below-ACK delivery recreated SACK debt")
	}
	st := r.Stats()
	if st.LateBelowACK != 1 {
		t.Fatalf("late below-ACK deliveries=%d want=1", st.LateBelowACK)
	}

	// Exact recent retry is still suppressed locally.
	deliver, _ = r.Accept(start, record)
	if deliver {
		t.Fatal("recent duplicate below ACK was delivered twice")
	}
	if r.Stats().BelowNextDrops == 0 {
		t.Fatal("recent below-ACK duplicate was not accounted")
	}
}

func TestSteadyStateNeverReclassifiesBootstrapSequenceAsDatagram(t *testing.T) {
	const (
		start  = uint32(20000)
		record = 100
	)
	r := NewReceiver(start)
	if deliver, _ := r.Accept(start, record); !deliver {
		t.Fatal("bootstrap record was not delivered")
	}
	floor := r.Next()
	r.EnableSteadyStateDelivery()

	// A late bootstrap retry sits below the mode-transition floor and must remain
	// strict even though steady-state late-first-arrival delivery is now enabled.
	if deliver, _ := r.Accept(start, record); deliver {
		t.Fatal("pre-steady sequence was reclassified as datagram payload")
	}
	if r.steadyFloor != floor || !r.steadyFloorActive {
		t.Fatalf("steady floor=%d active=%t want=%d,true", r.steadyFloor, r.steadyFloorActive, floor)
	}
}

func TestSteadyStateDeliveryHistoryIsBounded(t *testing.T) {
	const record = 100
	r := NewReceiver(30000)
	r.EnableSteadyStateDelivery()

	for i := 0; i < recentDeliveryHistoryLimit+512; i++ {
		seq := r.Next()
		if deliver, _ := r.Accept(seq, record); !deliver {
			t.Fatalf("in-order record %d was not delivered", i)
		}
	}
	if got := len(r.deliveredBySeq); got > recentDeliveryHistoryLimit {
		t.Fatalf("delivery history=%d limit=%d", got, recentDeliveryHistoryLimit)
	}
	if r.steadyFloorActive {
		t.Fatal("bootstrap floor remained permanently active across long steady state")
	}
	if r.deliveryHead >= recentDeliveryHistoryLimit && r.deliveryHead*2 >= len(r.deliveryOrder) {
		t.Fatalf("delivery queue was not compacted: head=%d len=%d", r.deliveryHead, len(r.deliveryOrder))
	}
}
''')
