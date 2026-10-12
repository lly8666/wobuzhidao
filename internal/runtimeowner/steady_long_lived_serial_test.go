package runtimeowner

import (
    "testing"

    "github.com/lly8666/wobuzhidao/internal/faketcp"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// This is an arithmetic long-lived stream regression, not synthetic line
// loss, and not a product-controller or netem shortcut. Every check uses the
// production routing predicate that used to compare against handshake-origin
// recvStart even after more than half the uint32 serial space had elapsed.
func TestLongLivedReceiveSerialReplayFloorAcrossHalfAndFullRollover(t *testing.T) {
    const oneMiB = uint64(1 << 20)
    const twoGiB = uint64(1) << 31
    const fourGiB = uint64(1) << 32
    for _, origin := range []uint32{0x10001000, 0xffffe000} {
        ref := logicaltunnel.LaneRef{ID: 1, Generation: 19}
        lane := &laneTransport{recvStart: origin, recvNext: origin}
        runtime := &Runtime{lanes: map[logicaltunnel.LaneRef]*laneTransport{ref: lane}}
        for advanced := uint64(0); advanced <= fourGiB+8*oneMiB; advanced += oneMiB {
            current := origin + uint32(advanced)
            lane.mu.Lock()
            lane.recvNext = current
            lane.mu.Unlock()
            fresh := faketcp.Segment{Seq: current, Payload: []byte{0x01}}
            if !runtime.steadyOwnsSegment(ref, fresh) {
                t.Fatalf("origin=%08x fresh dropped after %d bytes, including 2^31 or full-ring rollover",
                    origin, advanced)
            }
            lane.mu.Lock()
            floor, next := lane.recvStart, lane.recvNext
            lane.mu.Unlock()
            // The floor remains within the serial half-space. Keeping the
            // old fixed origin after 2 GiB caused the observed C03 outage.
            if distance := uint32(next-floor); distance > steadyReceiveSerialHistoryBytes {
                t.Fatalf("origin=%08x advanced=%d unbounded receive floor distance %d", origin, advanced, distance)
            }
            replay := faketcp.Segment{Seq: floor - 1, Payload: []byte{0x02}}
            if runtime.steadyOwnsSegment(ref, replay) {
                t.Fatalf("origin=%08x advanced=%d accepted data before bounded receive floor", origin, advanced)
            }
            if advanced >= 16*oneMiB {
                delayed := faketcp.Segment{Seq: next - uint32(4*oneMiB), Payload: []byte{0x03}}
                if !runtime.steadyOwnsSegment(ref, delayed) {
                    t.Fatalf("origin=%08x advanced=%d wrongly blocked recent late authenticated data", origin, advanced)
                }
            }
            if advanced == twoGiB {
                if floor == origin {
                    t.Fatal("receive serial floor did not rotate before 2^31 half-space boundary")
                }
            }
        }
    }
}

// Before the first GiB has actually been received the handshake floor is
// unchanged: pre-admission bytes must still be refused. The bounded cursor
// does not alter the immutable same-seq ciphertext / FEC replay checks.
func TestReceiveSerialFloorPreservesInitialAdmissionBarrier(t *testing.T) {
    start := uint32(0x7fff0000)
    lane := &laneTransport{recvStart: start, recvNext: start + 1024}
    lane.mu.Lock()
    got := lane.receiveSequenceFloorLocked()
    lane.mu.Unlock()
    if got != start {
        t.Fatalf("advanced handshake floor for short connection: %x != %x", got, start)
    }
    lane.mu.Lock()
    lane.recvNext = start + steadyReceiveSerialHistoryBytes + 1
    got = lane.receiveSequenceFloorLocked()
    lane.mu.Unlock()
    if got != start+1 {
        t.Fatalf("unexpected replay floor rotation: got%x want%x", got, start+1)
    }
}
