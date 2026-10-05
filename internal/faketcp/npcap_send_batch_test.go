package faketcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func npcapBatchSegments(n int) []Segment {
	cfg := testNpcapConfig()
	segments := make([]Segment, n)
	for i := range segments {
		segments[i] = Segment{SrcIP: cfg.Flow.LocalIP, DstIP: cfg.Flow.PeerIP, SrcPort: cfg.Flow.LocalPort, DstPort: cfg.Flow.PeerPort,
			Seq: uint32(i * 300), Ack: 22, Flags: FlagACK | FlagPSH, Window: 65535, Payload: bytes.Repeat([]byte{byte(i + 1)}, 50+i*20)}
	}
	return segments
}

func npcapTestFrames(t *testing.T, queue []byte) ([][]byte, []int) {
	t.Helper()
	var frames [][]byte
	var ends []int
	for offset := 0; offset < len(queue); {
		if len(queue)-offset < npcapQueueHeaderBytes {
			t.Fatal("short queue header")
		}
		h := queue[offset : offset+npcapQueueHeaderBytes]
		if !bytes.Equal(h[:8], make([]byte, 8)) {
			t.Fatal("timestamp pacing must remain disabled")
		}
		size := int(binary.LittleEndian.Uint32(h[8:12]))
		if size != int(binary.LittleEndian.Uint32(h[12:16])) || size > len(queue)-offset-npcapQueueHeaderBytes {
			t.Fatal("bad frame length")
		}
		frames = append(frames, append([]byte(nil), queue[offset+npcapQueueHeaderBytes:offset+npcapQueueHeaderBytes+size]...))
		offset += npcapQueueHeaderBytes + size
		ends = append(ends, offset)
	}
	return frames, ends
}

func TestNpcapReadyBatchPreservesWireOrderAndBoundedChunks(t *testing.T) {
	segments := npcapBatchSegments(19)
	cfg := testNpcapConfig()
	id := uint16(65534)
	var got [][]byte
	calls := 0
	count, totalBytes := 0, 0
	n, err := writeNpcapReadySegments(segments, cfg, &id, make([]byte, npcapSendQueueBytes), func(b []byte) error { got = append(got, append([]byte(nil), b...)); calls++; return nil }, func(b []byte) (uint32, error) {
		frames, _ := npcapTestFrames(t, b)
		if len(frames) > 8 || len(b) > npcapSendQueueBytes {
			t.Fatal("unbounded send batch")
		}
		got = append(got, frames...)
		calls++
		return uint32(len(b)), nil
	}, func(n, b int) { count += n; totalBytes += b })
	if err != nil || n != 19 || count != 19 || calls != 3 || id != 17 {
		t.Fatalf("n=%d err=%v count=%d calls=%d id=%d", n, err, count, calls, id)
	}
	wantBytes := 0
	for i, s := range segments {
		packet, frame, err := encodeNpcapOutbound(s, cfg, uint16(65534+i))
		if err != nil || !bytes.Equal(got[i], frame) {
			t.Fatalf("wire changed index%d err%v", i, err)
		}
		if _, err := ParseIPv4TCP(got[i][14:]); err != nil {
			t.Fatalf("checksum/frame failed: %v", err)
		}
		wantBytes += len(packet)
	}
	if totalBytes != wantBytes {
		t.Fatal("packet bytes mixed with Ethernet or queue header bytes")
	}
}

func TestNpcapSparseAndUnavailableQueueSendImmediately(t *testing.T) {
	for _, n := range []int{1, 3} {
		cfg := testNpcapConfig()
		id := uint16(1)
		calls := 0
		sent, err := writeNpcapReadySegments(npcapBatchSegments(n), cfg, &id, nil, func([]byte) error { calls++; return nil }, nil, nil)
		if err != nil || sent != n || calls != n {
			t.Fatalf("fallback sent=%d calls=%d err=%v", sent, calls, err)
		}
	}
	id := uint16(1)
	single := 0
	n, err := writeNpcapReadySegments(npcapBatchSegments(1), testNpcapConfig(), &id, make([]byte, npcapSendQueueBytes), func([]byte) error { single++; return nil }, func([]byte) (uint32, error) { t.Fatal("sparse must not fill batch"); return 0, nil }, nil)
	if err != nil || n != 1 || single != 1 {
		t.Fatal("single packet was not emitted")
	}
}

func TestNpcapPartialQueueNeverReplaysPrefixOrSuffix(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		id := uint16(1)
		calls := 0
		written := 0
		n, err := writeNpcapReadySegments(npcapBatchSegments(5), testNpcapConfig(), &id, make([]byte, npcapSendQueueBytes), func([]byte) error { t.Fatal("must not replay partial queue through single send"); return nil }, func(b []byte) (uint32, error) {
			_, ends := npcapTestFrames(t, b)
			calls++
			ret := uint32(ends[1])
			if malformed {
				ret++
			}
			return ret, nil
		}, func(n, b int) { written += n })
		if n != 2 || calls != 1 || written != 2 || (malformed && !errors.Is(err, errNpcapBatchReceipt)) || (!malformed && !errors.Is(err, io.ErrShortWrite)) {
			t.Fatalf("malformed%v n%d calls%d written%d err%v", malformed, n, calls, written, err)
		}
	}
	for _, receipt := range []uint32{0, 1, 201, 1000} {
		n, err := npcapQueuePrefix(receipt, []int{100, 200})
		if err == nil || n != 0 {
			t.Fatalf("invalid receipt%d accepted n%d err%v", receipt, n, err)
		}
	}
}

func TestNpcapBatchRejectsForeignFlowBeforeAnyEmission(t *testing.T) {
	segments := npcapBatchSegments(9)
	segments[8].DstPort++
	id := uint16(1)
	n, err := writeNpcapReadySegments(segments, testNpcapConfig(), &id, make([]byte, npcapSendQueueBytes), func([]byte) error { t.Fatal("foreign batch emitted"); return nil }, func([]byte) (uint32, error) { t.Fatal("foreign batch emitted"); return 0, nil }, nil)
	if n != 0 || id != 1 || !errors.Is(err, ErrNpcapOutboundFlow) {
		t.Fatalf("n%d id%d err%v", n, id, err)
	}
}

func TestNpcapQueueByteCapacityFallsBackWithoutTruncation(t *testing.T) {
	segments := npcapBatchSegments(3)
	segments[0].Payload = bytes.Repeat([]byte{7}, 20000)
	id := uint16(1)
	var frames [][]byte
	n, err := writeNpcapReadySegments(segments, testNpcapConfig(), &id, make([]byte, npcapSendQueueBytes), func(b []byte) error { frames = append(frames, append([]byte(nil), b...)); return nil }, func(b []byte) (uint32, error) {
		more, _ := npcapTestFrames(t, b)
		frames = append(frames, more...)
		return uint32(len(b)), nil
	}, nil)
	if n != 3 || err != nil || len(frames) != 3 {
		t.Fatalf("n%d err%v frames%d", n, err, len(frames))
	}
	for i, s := range segments {
		_, want, _ := encodeNpcapOutbound(s, testNpcapConfig(), uint16(i+1))
		if !bytes.Equal(want, frames[i]) {
			t.Fatalf("truncated index%d", i)
		}
	}
}
