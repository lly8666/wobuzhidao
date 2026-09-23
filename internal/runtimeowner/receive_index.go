package runtimeowner

import (
	"container/heap"
	"time"
)

// receiveSpanHeap keeps only currently tracked out-of-order coverage. All spans
// are within the TCP serial-number half-space ahead of recvNext, so seqLT is a
// stable ordering even across uint32 wrap.
type receiveSpanHeap []*receiveSpan

func (h receiveSpanHeap) Len() int { return len(h) }
func (h receiveSpanHeap) Less(i, j int) bool { return seqLT(h[i].seq, h[j].seq) }
func (h receiveSpanHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].heapIndex = i
	h[j].heapIndex = j
}
func (h *receiveSpanHeap) Push(x any) {
	p := x.(*receiveSpan)
	p.heapIndex = len(*h)
	*h = append(*h, p)
}
func (h *receiveSpanHeap) Pop() any {
	old := *h
	n := len(old)
	p := old[n-1]
	old[n-1] = nil
	p.heapIndex = -1
	*h = old[:n-1]
	return p
}

func (t *laneTransport) addReceiveSpanLocked(seq, end uint32, first time.Time, fin bool) *receiveSpan {
	p := &receiveSpan{seq: seq, end: end, first: first, fin: fin, heapIndex: -1}
	t.received[seq] = p
	heap.Push(&t.recvHeap, p)
	return p
}

func (t *laneTransport) removeReceiveSpanLocked(p *receiveSpan) bool {
	if p == nil || t.received[p.seq] != p {
		return false
	}
	delete(t.received, p.seq)
	if p.heapIndex >= 0 && p.heapIndex < len(t.recvHeap) && t.recvHeap[p.heapIndex] == p {
		heap.Remove(&t.recvHeap, p.heapIndex)
	}
	return true
}

func (t *laneTransport) earliestReceiveSpanLocked() *receiveSpan {
	if len(t.recvHeap) == 0 {
		return nil
	}
	t.stats.GapIndexSteps++
	return t.recvHeap[0]
}
