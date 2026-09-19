package linkdata

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

const (
	FragmentHeaderLen = 20
	MaxDatagramLen    = 65535
	MaxFragments      = 65535

	MaxReassemblyAssemblies = 16
	MaxRetiredPacketIDs     = 64
	MaxBufferedFragments    = MaxDatagramLen
	MaxBufferedBytes        = MaxReassemblyAssemblies * MaxDatagramLen

	AssemblyTTL = 5 * time.Second
	RetiredTTL  = 10 * time.Second
)

var (
	fragmentMagic = [8]byte{'W', 'B', 'D', 'L', 'F', 'R', 'G', '1'}

	ErrPacketTooLarge      = errors.New("linkdata: packet too large")
	ErrInvalidFragment     = errors.New("linkdata: invalid fragment")
	ErrConflictingFragment = errors.New("linkdata: conflicting fragment")
)

// Fragmenter emits LINK fragment frames for one logical datagram. It is owner
// scoped and intentionally has no internal synchronization.
type Fragmenter struct {
	mtu    int
	nextID uint32
}

func NewFragmenter(mtu int) (*Fragmenter, error) {
	if mtu <= FragmentHeaderLen || mtu > MaxDatagramLen {
		return nil, fmt.Errorf("linkdata fragment MTU=%d: %w", mtu, ErrPacketTooLarge)
	}
	return &Fragmenter{mtu: mtu, nextID: 1}, nil
}

func (f *Fragmenter) MTU() int {
	if f == nil {
		return 0
	}
	return f.mtu
}

func hasFragmentMagic(packet []byte) bool {
	return len(packet) >= len(fragmentMagic) && bytes.Equal(packet[:len(fragmentMagic)], fragmentMagic[:])
}

// Fragment preserves ordinary datagrams byte-for-byte when they fit the LINK
// MTU and do not begin with the reserved fragment magic. Oversized datagrams,
// and ordinary payloads that begin with that reserved magic, are wrapped before
// any later FEC stage.
func (f *Fragmenter) Fragment(packet []byte) ([][]byte, error) {
	if f == nil || f.mtu <= FragmentHeaderLen {
		return nil, fmt.Errorf("linkdata fragmenter unavailable: %w", ErrPacketTooLarge)
	}
	if len(packet) == 0 || len(packet) > MaxDatagramLen {
		return nil, fmt.Errorf("linkdata fragment input_bytes=%d max=%d: %w", len(packet), MaxDatagramLen, ErrPacketTooLarge)
	}
	if len(packet) <= f.mtu && !hasFragmentMagic(packet) {
		return [][]byte{packet}, nil
	}

	chunkBudget := f.mtu - FragmentHeaderLen
	count := (len(packet) + chunkBudget - 1) / chunkBudget
	if count <= 0 || count > MaxFragments {
		return nil, fmt.Errorf("linkdata fragment count=%d max=%d: %w", count, MaxFragments, ErrPacketTooLarge)
	}

	packetID := f.nextID
	f.nextID++
	if f.nextID == 0 {
		f.nextID = 1
	}

	frames := make([][]byte, 0, count)
	for index, off := 0, 0; off < len(packet); index++ {
		n := len(packet) - off
		if n > chunkBudget {
			n = chunkBudget
		}
		frame := make([]byte, FragmentHeaderLen+n)
		copy(frame[:8], fragmentMagic[:])
		frame[8] = 1
		frame[9] = 0
		binary.BigEndian.PutUint16(frame[10:12], uint16(index))
		binary.BigEndian.PutUint16(frame[12:14], uint16(count))
		binary.BigEndian.PutUint16(frame[14:16], uint16(len(packet)))
		binary.BigEndian.PutUint32(frame[16:20], packetID)
		copy(frame[FragmentHeaderLen:], packet[off:off+n])
		frames = append(frames, frame)
		off += n
	}
	return frames, nil
}

type fragmentAssembly struct {
	count     uint16
	total     int
	parts     map[uint16][]byte
	bytes     int
	createdAt time.Time
}

type ReassemblyState struct {
	Assemblies        int
	BufferedFragments int
	BufferedBytes     int
	RetiredPacketIDs  int
}

// Reassembler owns incomplete LINK datagrams for one lane/path. Fragment
// payloads are copied because the caller's decode storage may be reused after
// Push returns. It is owner scoped and intentionally has no internal locking.
type Reassembler struct {
	mtu               int
	assemblies        map[uint32]*fragmentAssembly
	retired           map[uint32]time.Time
	bufferedFragments int
	bufferedBytes     int
}

func NewReassembler(mtu int) (*Reassembler, error) {
	if mtu <= FragmentHeaderLen || mtu > MaxDatagramLen {
		return nil, fmt.Errorf("linkdata reassembly MTU=%d: %w", mtu, ErrPacketTooLarge)
	}
	return &Reassembler{
		mtu:        mtu,
		assemblies: make(map[uint32]*fragmentAssembly),
		retired:    make(map[uint32]time.Time),
	}, nil
}

func (r *Reassembler) State() ReassemblyState {
	if r == nil {
		return ReassemblyState{}
	}
	return ReassemblyState{
		Assemblies:        len(r.assemblies),
		BufferedFragments: r.bufferedFragments,
		BufferedBytes:     r.bufferedBytes,
		RetiredPacketIDs:  len(r.retired),
	}
}

// Push returns complete=true for an ordinary datagram immediately, or once all
// fragments for exactly one PacketID have arrived. Missing fragments for one
// PacketID never gate another PacketID.
func (r *Reassembler) Push(frame []byte, now time.Time) ([]byte, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("linkdata reassembler unavailable: %w", ErrInvalidFragment)
	}
	if len(frame) == 0 {
		return nil, false, fmt.Errorf("linkdata empty frame: %w", ErrInvalidFragment)
	}
	if len(frame) > r.mtu {
		return nil, false, fmt.Errorf("linkdata frame_bytes=%d mtu=%d: %w", len(frame), r.mtu, ErrPacketTooLarge)
	}

	r.Expire(now)
	if !hasFragmentMagic(frame) {
		return frame, true, nil
	}
	if len(frame) < FragmentHeaderLen || frame[8] != 1 || frame[9] != 0 {
		return nil, false, fmt.Errorf("linkdata invalid fragment header: %w", ErrInvalidFragment)
	}

	index := binary.BigEndian.Uint16(frame[10:12])
	count := binary.BigEndian.Uint16(frame[12:14])
	total := int(binary.BigEndian.Uint16(frame[14:16]))
	packetID := binary.BigEndian.Uint32(frame[16:20])
	payload := frame[FragmentHeaderLen:]
	if packetID == 0 || count == 0 || int(count) > MaxFragments || index >= count || total <= 0 || total > MaxDatagramLen || len(payload) == 0 || len(payload) > total {
		return nil, false, fmt.Errorf("linkdata invalid fragment metadata: %w", ErrInvalidFragment)
	}

	if _, done := r.retired[packetID]; done {
		return nil, false, nil
	}
	if count == 1 {
		if index != 0 || len(payload) != total {
			return nil, false, fmt.Errorf("linkdata invalid escaped datagram: %w", ErrInvalidFragment)
		}
		r.retire(packetID, now)
		return append([]byte(nil), payload...), true, nil
	}

	a := r.assemblies[packetID]
	if a == nil {
		if len(r.assemblies) >= MaxReassemblyAssemblies {
			r.evictOldest(now)
		}
		if _, retired := r.retired[packetID]; retired {
			return nil, false, nil
		}
		a = &fragmentAssembly{
			count:     count,
			total:     total,
			parts:     make(map[uint16][]byte),
			createdAt: now,
		}
		r.assemblies[packetID] = a
	} else if a.count != count || a.total != total {
		return nil, false, fmt.Errorf("linkdata conflicting metadata packet_id=%d: %w", packetID, ErrConflictingFragment)
	}

	if previous, exists := a.parts[index]; exists {
		if !bytes.Equal(previous, payload) {
			return nil, false, fmt.Errorf("linkdata conflicting duplicate packet_id=%d index=%d: %w", packetID, index, ErrConflictingFragment)
		}
		return nil, false, nil
	}
	if a.bytes+len(payload) > a.total {
		r.dropAssembly(packetID)
		r.retire(packetID, now)
		return nil, false, fmt.Errorf("linkdata fragment bytes exceed total packet_id=%d: %w", packetID, ErrInvalidFragment)
	}

	for r.bufferedFragments+1 > MaxBufferedFragments || r.bufferedBytes+len(payload) > MaxBufferedBytes {
		evicted := r.evictOldest(now)
		if evicted == 0 {
			return nil, false, fmt.Errorf("linkdata reassembly capacity unavailable: %w", ErrInvalidFragment)
		}
		if evicted == packetID {
			return nil, false, nil
		}
	}

	a.parts[index] = append([]byte(nil), payload...)
	a.bytes += len(payload)
	r.bufferedFragments++
	r.bufferedBytes += len(payload)
	if len(a.parts) != int(a.count) {
		return nil, false, nil
	}
	if a.bytes != a.total {
		r.dropAssembly(packetID)
		r.retire(packetID, now)
		return nil, false, fmt.Errorf("linkdata completed fragment bytes=%d total=%d packet_id=%d: %w", a.bytes, a.total, packetID, ErrInvalidFragment)
	}

	out := make([]byte, 0, a.total)
	for i := uint16(0); i < a.count; i++ {
		part, ok := a.parts[i]
		if !ok || len(part) == 0 {
			r.dropAssembly(packetID)
			r.retire(packetID, now)
			return nil, false, fmt.Errorf("linkdata missing completed fragment packet_id=%d index=%d: %w", packetID, i, ErrInvalidFragment)
		}
		out = append(out, part...)
	}
	r.dropAssembly(packetID)
	r.retire(packetID, now)
	return out, true, nil
}

// Expire is intentionally callable without receiving a new frame so the lane
// owner can retire incomplete datagrams even after traffic stops.
func (r *Reassembler) Expire(now time.Time) {
	if r == nil || now.IsZero() {
		return
	}
	for packetID, a := range r.assemblies {
		if a == nil || (!a.createdAt.IsZero() && now.Sub(a.createdAt) >= AssemblyTTL) {
			r.dropAssembly(packetID)
			r.retire(packetID, now)
		}
	}
	for packetID, retiredAt := range r.retired {
		if !retiredAt.IsZero() && now.Sub(retiredAt) >= RetiredTTL {
			delete(r.retired, packetID)
		}
	}
}

func (r *Reassembler) evictOldest(now time.Time) uint32 {
	var oldestID uint32
	var oldestTime time.Time
	for packetID, a := range r.assemblies {
		createdAt := time.Time{}
		if a != nil {
			createdAt = a.createdAt
		}
		if oldestID == 0 || createdAt.Before(oldestTime) || (createdAt.Equal(oldestTime) && packetID < oldestID) {
			oldestID = packetID
			oldestTime = createdAt
		}
	}
	if oldestID != 0 {
		r.dropAssembly(oldestID)
		r.retire(oldestID, now)
	}
	return oldestID
}

func (r *Reassembler) dropAssembly(packetID uint32) {
	a := r.assemblies[packetID]
	if a == nil {
		delete(r.assemblies, packetID)
		return
	}
	r.bufferedFragments -= len(a.parts)
	r.bufferedBytes -= a.bytes
	if r.bufferedFragments < 0 {
		r.bufferedFragments = 0
	}
	if r.bufferedBytes < 0 {
		r.bufferedBytes = 0
	}
	delete(r.assemblies, packetID)
}

func (r *Reassembler) retire(packetID uint32, now time.Time) {
	if packetID == 0 {
		return
	}
	if _, exists := r.retired[packetID]; !exists && len(r.retired) >= MaxRetiredPacketIDs {
		var oldestID uint32
		var oldestTime time.Time
		for candidate, retiredAt := range r.retired {
			if oldestID == 0 || retiredAt.Before(oldestTime) || (retiredAt.Equal(oldestTime) && candidate < oldestID) {
				oldestID = candidate
				oldestTime = retiredAt
			}
		}
		delete(r.retired, oldestID)
	}
	r.retired[packetID] = now
}
