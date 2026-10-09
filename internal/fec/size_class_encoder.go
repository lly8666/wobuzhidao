package fec

import (
	"fmt"
	"time"
)

// SizeClassEncoder prevents a large source from inflating every small source's
// parity in 20:20. There are at most three lane-local groups, with ceilings 256,
// 512 and the source MTU. Sources still leave on Add; each group's first source
// starts its own absolute flush deadline. No group waits for another group.
// The codec and borrowed output contract are unchanged from FastBlockEncoder.
// Like that encoder, this type must only be used by its lane owner.
// Lower redundancy profiles keep their original single group: splitting those
// groups would change how often min(N,R) partial parity exceeds their full ratio.
type SizeClassEncoder struct {
	groups      [3]*FastBlockEncoder
	count       int
	nextBlockID uint32
	out         [3 * ParityShards][]byte
}

func NewSizeClassEncoder(codec Codec, maxPacketSize int, flushAfter time.Duration, firstBlockID uint32, parityShards int) (*SizeClassEncoder, error) {
	// Validate through the existing constructor, including the shared codec and
	// profile. Smaller groups allocate only their own ceiling, not three MTUs.
	last, err := NewFastBlockEncoderWithParity(codec, maxPacketSize, flushAfter, firstBlockID, parityShards)
	if err != nil {
		return nil, err
	}
	e := &SizeClassEncoder{nextBlockID: firstBlockID}
	for _, ceiling := range [...]int{256, 512} {
		if parityShards != ParityShards || ceiling >= maxPacketSize {
			break
		}
		group, err := NewFastBlockEncoderWithParity(codec, ceiling, flushAfter, firstBlockID, parityShards)
		if err != nil {
			return nil, err
		}
		e.groups[e.count] = group
		e.count++
	}
	e.groups[e.count] = last
	e.count++
	return e, nil
}

// NewSizeClassEncoderWithLargestWindow is an explicit, bounded latency tradeoff
// for the >512B group in 20:20 mode. Small packet groups retain flushAfter;
// original systematic source sends are untouched. Call only at construction,
// before any Add. Lower parity profiles retain their historical single group.
func NewSizeClassEncoderWithLargestWindow(codec Codec, maxPacketSize int, flushAfter, largestFlushAfter time.Duration, firstBlockID uint32, parityShards int) (*SizeClassEncoder, error) {
	if largestFlushAfter < flushAfter {
		return nil, fmt.Errorf("fec: largest-group window cannot precede normal deadline")
	}
	e, err := NewSizeClassEncoder(codec, maxPacketSize, flushAfter, firstBlockID, parityShards)
	if err != nil { return nil, err }
	if e.count == 3 && parityShards == ParityShards {
		e.groups[2].flushAfter = largestFlushAfter
	}
	return e, nil
}

func (e *SizeClassEncoder) Add(packet []byte, now time.Time) ([][]byte, error) {
	return e.addWithClassHint(packet, now, false)
}

// AddFragmentOfLargeDatagram keeps a small final LINK fragment in the same
// largest >512B FEC size class as its sibling fragments. Without this hint a
// 1372B datagram's ~200B tail gets an independent partial-parity group,
// defeating recovery of the complete large datagram under loss. The existing
// small groups retain the SAME configured deadline (32ms by default).
func (e *SizeClassEncoder) AddFragmentOfLargeDatagram(packet []byte, now time.Time) ([][]byte, error) {
	return e.addWithClassHint(packet, now, true)
}

func (e *SizeClassEncoder) addWithClassHint(packet []byte, now time.Time, largest bool) ([][]byte, error) {
	if len(packet) == 0 || len(packet) > e.groups[e.count-1].maxPacketSize {
		return nil, ErrPacketTooLarge
	}
	for i, group := range e.groups[:e.count] {
		if largest && i != e.count-1 {
			continue
		}
		if len(packet) > group.maxPacketSize {
			continue
		}
		if group.Pending() == 0 {
			// Allocate globally at block START, not flush. Interleaved groups
			// must never reuse a BlockID or multiply generation ages by a stride.
			group.nextBlockID = e.nextBlockID
			e.nextBlockID++
		}
		return group.Add(packet, now)
	}
	panic("fec: unreachable size class")
}

func (e *SizeClassEncoder) FlushDue(now time.Time) ([][]byte, error) {
	return e.flush(now, false)
}

func (e *SizeClassEncoder) Flush() ([][]byte, error) {
	return e.flush(time.Time{}, true)
}

func (e *SizeClassEncoder) flush(now time.Time, all bool) ([][]byte, error) {
	n := 0
	for _, group := range e.groups[:e.count] {
		var wire [][]byte
		var err error
		if all {
			wire, err = group.Flush()
		} else {
			wire, err = group.FlushDue(now)
		}
		if err != nil {
			return nil, err
		}
		n += copy(e.out[n:], wire)
	}
	return e.out[:n], nil
}

// NextFlushDeadline is the earliest pending group's absolute expiry. There
// are at most three fixed size classes; an empty group never schedules work.
// No timers, goroutines, polling or extra per-packet allocations are added.
func (e *SizeClassEncoder) NextFlushDeadline() time.Time {
	if e == nil {
		return time.Time{}
	}
	var first time.Time
	for _, group := range e.groups[:e.count] {
		due := group.NextFlushDeadline()
		if !due.IsZero() && (first.IsZero() || due.Before(first)) {
			first = due
		}
	}
	return first
}

func (e *SizeClassEncoder) Pending() int {
	return e.Stats().PendingSources
}

func (e *SizeClassEncoder) Stats() FastBlockEncoderStats {
	if e == nil {
		return FastBlockEncoderStats{}
	}
	out := FastBlockEncoderStats{SizeClasses: e.count}
	for _, group := range e.groups[:e.count] {
		s := group.Stats()
		out.SourceShards += s.SourceShards
		out.SourceBytes += s.SourceBytes
		out.ParityShards += s.ParityShards
		out.ParityBytes += s.ParityBytes
		out.FullBlocks += s.FullBlocks
		out.PartialBlocks += s.PartialBlocks
		out.PendingSources += s.PendingSources
		if s.PendingSources != 0 {
			out.PendingBlocks++
		}
	}
	return out
}
