package linkdata

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

const (
	linkFragmentHeaderLen    = 20
	maxLinkDatagramLen       = 65535
	maxLinkFragmentAssemblies = 16
	linkFragmentAssemblyTTL  = 5 * time.Second
	linkFragmentRetiredTTL   = 10 * time.Second
	maxLinkFragmentRetired   = 64
)

var linkFragmentMagic = [8]byte{'W', 'B', 'D', 'L', 'F', 'R', 'G', '1'}

type datagramFragmenter struct {
	mtu    int
	nextID uint32
}

func newDatagramFragmenter(mtu int) (*datagramFragmenter, error) {
	if mtu <= linkFragmentHeaderLen || mtu > 0xffff {
		return nil, fmt.Errorf("linkdata fragment MTU=%d: %w", mtu, fec.ErrPacketTooLarge)
	}
	return &datagramFragmenter{mtu: mtu, nextID: 1}, nil
}

func hasLinkFragmentMagic(packet []byte) bool {
	return len(packet) >= len(linkFragmentMagic) && bytes.Equal(packet[:len(linkFragmentMagic)], linkFragmentMagic[:])
}

// Fragment keeps the ordinary first-arrival path byte-for-byte compatible for
// legal datagrams. Only a datagram that exceeds the negotiated LINK MTU (or
// begins with the reserved fragment magic) is wrapped. Each returned frame is
// itself a legal LINK plaintext datagram and may then be protected by FEC.
func (f *datagramFragmenter) Fragment(packet []byte) ([][]byte, error) {
	if f == nil || f.mtu <= linkFragmentHeaderLen {
		return nil, fmt.Errorf("linkdata fragmenter unavailable: %w", fec.ErrPacketTooLarge)
	}
	if len(packet) == 0 || len(packet) > maxLinkDatagramLen {
		return nil, fmt.Errorf("linkdata fragment input_bytes=%d max=%d: %w", len(packet), maxLinkDatagramLen, fec.ErrPacketTooLarge)
	}
	if len(packet) <= f.mtu && !hasLinkFragmentMagic(packet) {
		return [][]byte{packet}, nil
	}

	chunkBudget := f.mtu - linkFragmentHeaderLen
	count := (len(packet) + chunkBudget - 1) / chunkBudget
	if count <= 0 || count > 0xffff {
		return nil, fmt.Errorf("linkdata fragment count=%d: %w", count, fec.ErrPacketTooLarge)
	}
	id := f.nextID
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
		frame := make([]byte, linkFragmentHeaderLen+n)
		copy(frame[:8], linkFragmentMagic[:])
		frame[8] = 1
		frame[9] = 0
		binary.BigEndian.PutUint16(frame[10:12], uint16(index))
		binary.BigEndian.PutUint16(frame[12:14], uint16(count))
		binary.BigEndian.PutUint16(frame[14:16], uint16(len(packet)))
		binary.BigEndian.PutUint32(frame[16:20], id)
		copy(frame[linkFragmentHeaderLen:], packet[off:off+n])
		frames = append(frames, frame)
		off += n
	}
	return frames, nil
}

type linkFragmentAssembly struct {
	count     uint16
	total     int
	parts     [][]byte
	present   []bool
	received  int
	bytes     int
	createdAt time.Time
}

type datagramReassembler struct {
	assemblies map[uint32]*linkFragmentAssembly
	retired    map[uint32]time.Time
}

func newDatagramReassembler() *datagramReassembler {
	return &datagramReassembler{
		assemblies: make(map[uint32]*linkFragmentAssembly),
		retired:    make(map[uint32]time.Time),
	}
}

// Push returns complete=true for an ordinary datagram immediately, or once all
// fragments of one oversized logical datagram have arrived. Fragment payloads
// are copied because FEC decoder storage may be reused after Decode returns.
func (r *datagramReassembler) Push(frame []byte, now time.Time) ([]byte, bool, error) {
	if r == nil {
		return nil, false, fmt.Errorf("linkdata fragment reassembler unavailable: %w", fec.ErrInvalidShardSet)
	}
	if !hasLinkFragmentMagic(frame) {
		return frame, true, nil
	}
	if len(frame) < linkFragmentHeaderLen || frame[8] != 1 || frame[9] != 0 {
		return nil, false, fmt.Errorf("linkdata invalid fragment header: %w", fec.ErrInvalidShardSet)
	}
	index := binary.BigEndian.Uint16(frame[10:12])
	count := binary.BigEndian.Uint16(frame[12:14])
	total := int(binary.BigEndian.Uint16(frame[14:16]))
	id := binary.BigEndian.Uint32(frame[16:20])
	payload := frame[linkFragmentHeaderLen:]
	if id == 0 || count == 0 || index >= count || total <= 0 || total > maxLinkDatagramLen || len(payload) == 0 || len(payload) > total {
		return nil, false, fmt.Errorf("linkdata invalid fragment metadata: %w", fec.ErrInvalidShardSet)
	}

	r.expire(now)
	if _, done := r.retired[id]; done {
		return nil, false, nil
	}
	if count == 1 {
		if index != 0 || len(payload) != total {
			return nil, false, fmt.Errorf("linkdata invalid escaped datagram: %w", fec.ErrInvalidShardSet)
		}
		r.retire(id, now)
		return append([]byte(nil), payload...), true, nil
	}

	a := r.assemblies[id]
	if a == nil {
		if len(r.assemblies) >= maxLinkFragmentAssemblies {
			r.evictOldest(now)
		}
		a = &linkFragmentAssembly{
			count: count,
			total: total,
			parts: make([][]byte, int(count)),
			present: make([]bool, int(count)),
			createdAt: now,
		}
		r.assemblies[id] = a
	} else if a.count != count || a.total != total {
		return nil, false, fmt.Errorf("linkdata conflicting fragment metadata id=%d: %w", id, fec.ErrInvalidShardSet)
	}

	i := int(index)
	if a.present[i] {
		if !bytes.Equal(a.parts[i], payload) {
			return nil, false, fmt.Errorf("linkdata conflicting duplicate fragment id=%d index=%d: %w", id, index, fec.ErrInvalidShardSet)
		}
		return nil, false, nil
	}
	a.parts[i] = append([]byte(nil), payload...)
	a.present[i] = true
	a.received++
	a.bytes += len(payload)
	if a.bytes > a.total {
		delete(r.assemblies, id)
		r.retire(id, now)
		return nil, false, fmt.Errorf("linkdata fragment bytes exceed declared total id=%d: %w", id, fec.ErrInvalidShardSet)
	}
	if a.received != int(a.count) {
		return nil, false, nil
	}
	if a.bytes != a.total {
		delete(r.assemblies, id)
		r.retire(id, now)
		return nil, false, fmt.Errorf("linkdata incomplete fragment byte total id=%d: %w", id, fec.ErrInvalidShardSet)
	}
	out := make([]byte, 0, a.total)
	for _, part := range a.parts {
		if len(part) == 0 {
			delete(r.assemblies, id)
			r.retire(id, now)
			return nil, false, fmt.Errorf("linkdata missing completed fragment id=%d: %w", id, fec.ErrInvalidShardSet)
		}
		out = append(out, part...)
	}
	delete(r.assemblies, id)
	r.retire(id, now)
	return out, true, nil
}

func (r *datagramReassembler) expire(now time.Time) {
	if now.IsZero() {
		return
	}
	for id, a := range r.assemblies {
		if a == nil || (!a.createdAt.IsZero() && now.Sub(a.createdAt) >= linkFragmentAssemblyTTL) {
			delete(r.assemblies, id)
			r.retire(id, now)
		}
	}
	for id, retiredAt := range r.retired {
		if !retiredAt.IsZero() && now.Sub(retiredAt) >= linkFragmentRetiredTTL {
			delete(r.retired, id)
		}
	}
}

func (r *datagramReassembler) evictOldest(now time.Time) {
	var oldestID uint32
	var oldest time.Time
	for id, a := range r.assemblies {
		if a == nil {
			oldestID = id
			break
		}
		if oldestID == 0 || a.createdAt.Before(oldest) {
			oldestID = id
			oldest = a.createdAt
		}
	}
	if oldestID != 0 {
		delete(r.assemblies, oldestID)
		r.retire(oldestID, now)
	}
}

func (r *datagramReassembler) retire(id uint32, now time.Time) {
	if id == 0 {
		return
	}
	if len(r.retired) >= maxLinkFragmentRetired {
		var oldestID uint32
		var oldest time.Time
		for candidate, retiredAt := range r.retired {
			if oldestID == 0 || retiredAt.Before(oldest) {
				oldestID = candidate
				oldest = retiredAt
			}
		}
		delete(r.retired, oldestID)
	}
	r.retired[id] = now
}
