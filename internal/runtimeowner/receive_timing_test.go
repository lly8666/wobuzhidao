package runtimeowner

import (
	"bytes"
	"errors"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestReceiveTimingDeliversBeforeBlockedACKAndPreservesErrors(t *testing.T) {
	for _, tc := range []struct {
		name                 string
		base, feedback, fail bool
	}{
		{"off", false, false, false},
		{"ordinary base diagnostics", true, false, false},
		{"feedback flag alone", false, true, false},
		{"qualified", true, true, false},
		{"qualified emit failure", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lease := runtimeLease(t)
			addr, _ := lease.Config.LeaseIPv4()
			clientOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
			if err != nil {
				t.Fatal(err)
			}
			serverOwner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
			if err != nil {
				t.Fatal(err)
			}
			delivered := make(chan []byte, 1)
			server, err := New(serverOwner, func(packets [][]byte, _ time.Time) error {
				for _, p := range packets {
					delivered <- append([]byte(nil), p...)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			client, err := New(clientOwner, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			var fresh faketcp.Segment
			ackEntered := make(chan faketcp.Segment, 1)
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			writeErr := errors.New("qualified ACK write failure")
			clientCfg, serverCfg := transportPair(func(seg faketcp.Segment) error {
				fresh = seg
				return nil
			}, func(seg faketcp.Segment) error {
				ackEntered <- seg
				<-release
				if tc.fail {
					return writeErr
				}
				return nil
			}, 1, 1000)
			serverCfg.ObserveFeedbackTiming = tc.feedback
			if _, err := client.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 7), clientCfg); err != nil {
				t.Fatal(err)
			}
			snap, err := server.AttachInitial(1, runtimeLane(t, datapath.RoleServer, lease, 0, 7), serverCfg)
			if err != nil {
				t.Fatal(err)
			}
			if tc.base {
				if err := server.SetTimingDiagnostics(snap.Ref, true); err != nil {
					t.Fatal(err)
				}
			}
			packet := runtimeIPv4(addr, netip.MustParseAddr("1.1.1.1"), []byte("independent first arrival"))
			now := time.Unix(1000, 0)
			if err := client.SendPacket(packet, now); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- server.HandleSegment(snap.Ref, fresh, now) }()
			select {
			case ack := <-ackEntered:
				if len(ack.Payload) != 0 || ack.Ack != fresh.Seq+uint32(len(fresh.Payload)) {
					t.Fatalf("ACK changed: %+v", ack)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("ACK emit did not start")
			}
			select {
			case got := <-delivered:
				if !bytes.Equal(got, packet) {
					t.Fatal("delivered packet changed")
				}
			default:
				t.Fatal("delivery waited for ACK emit")
			}
			// Concurrent snapshots must remain available while the native emit is
			// blocked; the ACK clock is recorded only after that call completes.
			for i := 0; i < 16; i++ {
				st, ok := server.TransportStats(snap.Ref)
				if !ok || st.ACKFeedbackSamples != 0 {
					t.Fatalf("in-flight snapshot=%+v", st)
				}
			}
			unblock()
			select {
			case err := <-done:
				if (tc.fail && !errors.Is(err, writeErr)) || (!tc.fail && err != nil) {
					t.Fatalf("write error changed: %v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("handler did not return after emit release")
			}
			st, _ := server.TransportStats(snap.Ref)
			want := uint64(0)
			if tc.base && tc.feedback {
				want = 1
			}
			if st.ACKFeedbackSamples != want || st.FeedbackTimingEnabled != (want == 1) || st.SelectedRepairSamples != 0 {
				t.Fatalf("separate feedback counters=%+v", st)
			}
			if tc.base && (st.OwnerNS == 0 || st.DeliverNS == 0) {
				t.Fatal("decode/delivery timing missing")
			}
			if !tc.base && (st.TimingSamples != 0 || st.OwnerNS != 0 || st.DeliverNS != 0) {
				t.Fatal("off path gathered per-record timing")
			}
			if want == 1 && (st.ACKFeedbackNS == 0 || st.ACKFeedbackMaxNS == 0) {
				t.Fatal("blocked ACK elapsed missing")
			}
			if want == 0 && (st.ACKFeedbackNS != 0 || st.ACKFeedbackMaxNS != 0) {
				t.Fatal("ordinary path gathered feedback timing")
			}
		})
	}
}

func TestReceiveTimingPureACKSelectedRepairPreservesWireAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fail], func(t *testing.T) {
			lease := runtimeLease(t)
			owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
			if err != nil {
				t.Fatal(err)
			}
			rt, err := New(owner, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer rt.Close()
			var wire []faketcp.Segment
			writeErr := errors.New("qualified repair write failure")
			cfg, _ := transportPair(func(seg faketcp.Segment) error {
				wire = append(wire, seg)
				if len(wire) == 6 && fail {
					return writeErr
				}
				return nil
			}, func(faketcp.Segment) error { return nil }, 1, 33000)
			cfg.SACKPermitted = true
			cfg.ObserveFeedbackTiming = true
			snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 81), cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := rt.SetTimingDiagnostics(snap.Ref, true); err != nil {
				t.Fatal(err)
			}
			tr := rt.lanes[snap.Ref]
			t0 := time.Unix(9000, 0)
			records := make([]datapath.WireRecord, 5)
			for i := range records {
				records[i].Wire = bytes.Repeat([]byte{byte(i + 1)}, 32)
			}
			if err := tr.send(records[:1], t0); err != nil {
				t.Fatal(err)
			}
			if err := tr.send(records[1:], t0.Add(16*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			ack := faketcp.Segment{SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP, SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort, Seq: cfg.ReceiveNext, Ack: cfg.SendNext, Flags: faketcp.FlagACK, Window: 65535, SACKN: 1}
			ack.SACK[0] = faketcp.SACKBlock{Start: wire[1].Seq, End: wire[4].Seq + uint32(len(wire[4].Payload))}
			err = rt.HandleSegment(snap.Ref, ack, t0.Add(40*time.Millisecond))
			if (fail && !errors.Is(err, writeErr)) || (!fail && err != nil) {
				t.Fatalf("repair error changed: %v", err)
			}
			if len(wire) != 6 || wire[5].Seq != wire[0].Seq || !bytes.Equal(wire[5].Payload, wire[0].Payload) {
				t.Fatal("selected repair wire changed")
			}
			st, _ := rt.TransportStats(snap.Ref)
			if st.SelectedRepairSamples != 1 || st.SelectedRepairNS == 0 || st.ACKFeedbackSamples != 0 || st.OwnerNS != 0 || st.DeliverNS != 0 {
				t.Fatalf("pure ACK repair stage=%+v", st)
			}
			// A cumulative ACK retires the remaining record. Nil selection must
			// not appear as a repair timing sample, even after a failed emit.
			ack.Ack = wire[4].Seq + uint32(len(wire[4].Payload))
			ack.SACKN = 0
			if err := rt.HandleSegment(snap.Ref, ack, t0.Add(41*time.Millisecond)); err != nil {
				t.Fatal(err)
			}
			st, _ = rt.TransportStats(snap.Ref)
			if st.SelectedRepairSamples != 1 {
				t.Fatal("nil repair counted")
			}
		})
	}
}

func TestFeedbackDurationBoundedThresholdCounters(t *testing.T) {
	var d feedbackDuration
	for _, elapsed := range []time.Duration{-time.Second, time.Millisecond, 2 * time.Millisecond, 10 * time.Millisecond, 11 * time.Millisecond} {
		d.observe(elapsed)
	}
	if d.samples.Load() != 5 || d.over1MS.Load() != 3 || d.over10MS.Load() != 1 || d.duration.total.Load() != uint64(24*time.Millisecond) || d.duration.max.Load() != uint64(11*time.Millisecond) {
		t.Fatal("duration boundaries incorrect")
	}
}
