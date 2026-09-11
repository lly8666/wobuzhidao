package faketcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"time"
)

const (
	carrierIPv4HeaderLen  = 20
	carrierTCPHeaderLen   = 20
	carrierFrameHeaderLen = 20
	carrierFrameMagic     = uint64(0x5742444652414731) // "WBDFRAG1"
	carrierFrameVersion   = byte(1)
	maxCarrierDatagramLen = 65535
	maxCarrierAssemblies  = MaxSteadyStateOutstandingDatagrams

	// Carrier fragments are optional recovery state below the real first-arrival
	// path. Five seconds is deliberately much larger than the project's 600ms
	// target RTT, but short enough that a datagram abandoned by the partial-
	// reliability ACK horizon cannot pin fragment memory for the association's
	// lifetime. Duplicate fragments do not refresh this deadline.
	carrierAssemblyTTL = 5 * time.Second

	// A tiny ID-only tombstone stops late fragments from resurrecting a datagram
	// that already completed or whose incomplete assembly was deliberately
	// abandoned. No payload is retained here and the set is independently bounded.
	carrierRetiredTTL    = 10 * time.Second
	maxCarrierRetiredIDs = MaxSteadyStateOutstandingDatagrams
)

var (
	ErrCarrierPathMTU        = errors.New("faketcp: carrier path MTU is too small")
	ErrCarrierDatagram       = errors.New("faketcp: invalid carrier datagram")
	ErrCarrierFrame          = errors.New("faketcp: invalid carrier frame")
	ErrCarrierReassemblyFull = errors.New("faketcp: carrier reassembly window full")
)

func CarrierPayloadBudget(pathMTU int) (int, error) {
	budget := pathMTU - carrierIPv4HeaderLen - carrierTCPHeaderLen
	if budget <= 0 {
		return 0, ErrCarrierPathMTU
	}
	maxPayload := 65535 - carrierIPv4HeaderLen - carrierTCPHeaderLen
	if budget > maxPayload {
		budget = maxPayload
	}
	return budget, nil
}

type CarrierFragmenter struct {
	mu            sync.Mutex
	payloadBudget int
	nextID        uint32
}

func NewCarrierFragmenter(pathMTU int) (*CarrierFragmenter, error) {
	budget, err := CarrierPayloadBudget(pathMTU)
	if err != nil {
		return nil, err
	}
	if budget <= carrierFrameHeaderLen {
		return nil, ErrCarrierPathMTU
	}
	return &CarrierFragmenter{payloadBudget: budget, nextID: 1}, nil
}

func (f *CarrierFragmenter) Fragment(datagram []byte) ([][]byte, error) {
	if f == nil || f.payloadBudget <= carrierFrameHeaderLen {
		return nil, ErrCarrierPathMTU
	}
	if len(datagram) == 0 || len(datagram) > maxCarrierDatagramLen {
		return nil, ErrCarrierDatagram
	}
	if len(datagram) <= f.payloadBudget {
		return [][]byte{append([]byte(nil), datagram...)}, nil
	}

	chunkBudget := f.payloadBudget - carrierFrameHeaderLen
	count := (len(datagram) + chunkBudget - 1) / chunkBudget
	if count < 2 || count > 0xffff {
		return nil, ErrCarrierDatagram
	}

	f.mu.Lock()
	id := f.nextID
	f.nextID++
	if f.nextID == 0 {
		f.nextID = 1
	}
	f.mu.Unlock()

	frames := make([][]byte, 0, count)
	for i, off := 0, 0; off < len(datagram); i++ {
		n := len(datagram) - off
		if n > chunkBudget {
			n = chunkBudget
		}
		frame := make([]byte, carrierFrameHeaderLen+n)
		binary.BigEndian.PutUint64(frame[0:8], carrierFrameMagic)
		frame[8] = carrierFrameVersion
		frame[9] = 0
		binary.BigEndian.PutUint16(frame[10:12], uint16(i))
		binary.BigEndian.PutUint16(frame[12:14], uint16(count))
		binary.BigEndian.PutUint16(frame[14:16], uint16(len(datagram)))
		binary.BigEndian.PutUint32(frame[16:20], id)
		copy(frame[carrierFrameHeaderLen:], datagram[off:off+n])
		frames = append(frames, frame)
		off += n
	}
	return frames, nil
}

type carrierAssembly struct {
	count     uint16
	total     int
	parts     [][]byte
	present   []bool
	received  int
	bytes     int
	createdAt time.Time
}

type CarrierReassembler struct {
	mu         sync.Mutex
	assemblies map[uint32]*carrierAssembly
	retired    map[uint32]time.Time
}

func NewCarrierReassembler() *CarrierReassembler {
	return &CarrierReassembler{
		assemblies: make(map[uint32]*carrierAssembly),
		retired:    make(map[uint32]time.Time),
	}
}

func (r *CarrierReassembler) Push(frame []byte) ([]byte, bool, error) {
	return r.pushAt(frame, time.Now())
}

func (r *CarrierReassembler) pushAt(frame []byte, now time.Time) ([]byte, bool, error) {
	if r == nil {
		return nil, false, ErrCarrierFrame
	}
	if len(frame) < carrierFrameHeaderLen ||
		binary.BigEndian.Uint64(frame[0:8]) != carrierFrameMagic ||
		frame[8] != carrierFrameVersion || frame[9] != 0 {
		if len(frame) == 0 || len(frame) > maxCarrierDatagramLen {
			return nil, false, ErrCarrierFrame
		}
		return append([]byte(nil), frame...), true, nil
	}

	index := binary.BigEndian.Uint16(frame[10:12])
	count := binary.BigEndian.Uint16(frame[12:14])
	total := int(binary.BigEndian.Uint16(frame[14:16]))
	id := binary.BigEndian.Uint32(frame[16:20])
	payload := frame[carrierFrameHeaderLen:]
	if id == 0 || count < 2 || index >= count || total <= 0 || total > maxCarrierDatagramLen || len(payload) == 0 || len(payload) > total {
		return nil, false, ErrCarrierFrame
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assemblies == nil {
		r.assemblies = make(map[uint32]*carrierAssembly)
	}
	if r.retired == nil {
		r.retired = make(map[uint32]time.Time)
	}
	r.expireLocked(now)
	a := r.assemblies[id]
	if a == nil {
		if _, retired := r.retired[id]; retired {
			return nil, false, nil
		}
		if len(r.assemblies) >= maxCarrierAssemblies {
			return nil, false, ErrCarrierReassemblyFull
		}
		a = &carrierAssembly{
			count: count, total: total,
			parts: make([][]byte, int(count)), present: make([]bool, int(count)),
			createdAt: now,
		}
		r.assemblies[id] = a
	} else if a.count != count || a.total != total {
		return nil, false, fmt.Errorf("%w: conflicting metadata for datagram %d", ErrCarrierFrame, id)
	}

	i := int(index)
	if a.present[i] {
		if !bytes.Equal(a.parts[i], payload) {
			return nil, false, fmt.Errorf("%w: conflicting duplicate fragment for datagram %d index %d", ErrCarrierFrame, id, index)
		}
		return nil, false, nil
	}
	a.parts[i] = append([]byte(nil), payload...)
	a.present[i] = true
	a.received++
	a.bytes += len(payload)
	if a.bytes > a.total {
		delete(r.assemblies, id)
		r.retireLocked(id, now)
		return nil, false, ErrCarrierFrame
	}
	if a.received != int(a.count) {
		return nil, false, nil
	}
	if a.bytes != a.total {
		delete(r.assemblies, id)
		r.retireLocked(id, now)
		return nil, false, ErrCarrierFrame
	}
	out := make([]byte, 0, a.total)
	for _, part := range a.parts {
		if len(part) == 0 {
			delete(r.assemblies, id)
			r.retireLocked(id, now)
			return nil, false, ErrCarrierFrame
		}
		out = append(out, part...)
	}
	delete(r.assemblies, id)
	r.retireLocked(id, now)
	if len(out) != a.total {
		return nil, false, ErrCarrierFrame
	}
	return out, true, nil
}

func (r *CarrierReassembler) expireLocked(now time.Time) {
	if now.IsZero() {
		return
	}
	for id, a := range r.assemblies {
		if a == nil || (!a.createdAt.IsZero() && now.Sub(a.createdAt) >= carrierAssemblyTTL) {
			delete(r.assemblies, id)
			r.retireLocked(id, now)
		}
	}
	for id, retiredAt := range r.retired {
		if !retiredAt.IsZero() && now.Sub(retiredAt) >= carrierRetiredTTL {
			delete(r.retired, id)
		}
	}
}

func (r *CarrierReassembler) retireLocked(id uint32, now time.Time) {
	if id == 0 {
		return
	}
	if len(r.retired) >= maxCarrierRetiredIDs {
		var oldestID uint32
		var oldest time.Time
		for candidate, retiredAt := range r.retired {
			if oldest.IsZero() || retiredAt.Before(oldest) {
				oldestID, oldest = candidate, retiredAt
			}
		}
		delete(r.retired, oldestID)
	}
	r.retired[id] = now
}
