package faketcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
)

const (
	carrierIPv4HeaderLen = 20
	carrierTCPHeaderLen  = 20
	carrierFrameHeaderLen = 16
	carrierFrameMagic     = uint32(0x57424446) // "WBDF"
	carrierFrameVersion   = byte(1)
	maxCarrierDatagramLen = 65535
	maxCarrierAssemblies  = MaxSteadyStateOutstandingDatagrams
)

var (
	ErrCarrierPathMTU        = errors.New("faketcp: carrier path MTU is too small")
	ErrCarrierDatagram       = errors.New("faketcp: invalid carrier datagram")
	ErrCarrierFrame          = errors.New("faketcp: invalid carrier frame")
	ErrCarrierReassemblyFull = errors.New("faketcp: carrier reassembly window full")
)

// CarrierPayloadBudget returns the largest FakeTCP data payload that fits in an
// IPv4 path MTU. Steady-state data packets carry no TCP options; SACK options
// are emitted only on payload-free ACKs, so the data-path outer overhead is the
// fixed 20-byte IPv4 header plus the fixed 20-byte TCP header.
func CarrierPayloadBudget(pathMTU int) (int, error) {
	budget := pathMTU - carrierIPv4HeaderLen - carrierTCPHeaderLen
	if budget <= 0 {
		return 0, ErrCarrierPathMTU
	}
	return budget, nil
}

// CarrierFragmenter frames every post-bootstrap UDP datagram before FakeTCP.
// Framing small datagrams too gives the receiver an unambiguous discriminator;
// TLS bootstrap bytes never pass through this object.
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
	chunkBudget := f.payloadBudget - carrierFrameHeaderLen
	count := (len(datagram) + chunkBudget - 1) / chunkBudget
	if count <= 0 || count > 0xffff {
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
		binary.BigEndian.PutUint32(frame[0:4], carrierFrameMagic)
		frame[4] = carrierFrameVersion
		frame[5] = 0
		binary.BigEndian.PutUint16(frame[6:8], uint16(i))
		binary.BigEndian.PutUint16(frame[8:10], uint16(count))
		binary.BigEndian.PutUint16(frame[10:12], uint16(len(datagram)))
		binary.BigEndian.PutUint32(frame[12:16], id)
		copy(frame[carrierFrameHeaderLen:], datagram[off:off+n])
		frames = append(frames, frame)
		off += n
	}
	return frames, nil
}

type carrierAssembly struct {
	count    uint16
	total    int
	parts    [][]byte
	present  []bool
	received int
	bytes    int
}

// CarrierReassembler restores the original UDP datagram before it is returned
// to wolfSSL. FakeTCP may deliver first-arrival payloads out of sequence, so
// reassembly is keyed by carrier datagram id rather than arrival order.
type CarrierReassembler struct {
	mu         sync.Mutex
	assemblies map[uint32]*carrierAssembly
}

func NewCarrierReassembler() *CarrierReassembler {
	return &CarrierReassembler{assemblies: make(map[uint32]*carrierAssembly)}
}

func (r *CarrierReassembler) Push(frame []byte) ([]byte, bool, error) {
	if r == nil || len(frame) < carrierFrameHeaderLen {
		return nil, false, ErrCarrierFrame
	}
	if binary.BigEndian.Uint32(frame[0:4]) != carrierFrameMagic || frame[4] != carrierFrameVersion || frame[5] != 0 {
		return nil, false, ErrCarrierFrame
	}
	index := binary.BigEndian.Uint16(frame[6:8])
	count := binary.BigEndian.Uint16(frame[8:10])
	total := int(binary.BigEndian.Uint16(frame[10:12]))
	id := binary.BigEndian.Uint32(frame[12:16])
	payload := frame[carrierFrameHeaderLen:]
	if id == 0 || count == 0 || index >= count || total <= 0 || total > maxCarrierDatagramLen || len(payload) == 0 || len(payload) > total {
		return nil, false, ErrCarrierFrame
	}
	if count == 1 {
		if index != 0 || len(payload) != total {
			return nil, false, ErrCarrierFrame
		}
		return append([]byte(nil), payload...), true, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.assemblies == nil {
		r.assemblies = make(map[uint32]*carrierAssembly)
	}
	a := r.assemblies[id]
	if a == nil {
		if len(r.assemblies) >= maxCarrierAssemblies {
			return nil, false, ErrCarrierReassemblyFull
		}
		a = &carrierAssembly{
			count:   count,
			total:   total,
			parts:   make([][]byte, int(count)),
			present: make([]bool, int(count)),
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
		return nil, false, ErrCarrierFrame
	}
	if a.received != int(a.count) {
		return nil, false, nil
	}
	if a.bytes != a.total {
		delete(r.assemblies, id)
		return nil, false, ErrCarrierFrame
	}
	out := make([]byte, 0, a.total)
	for _, part := range a.parts {
		if len(part) == 0 {
			delete(r.assemblies, id)
			return nil, false, ErrCarrierFrame
		}
		out = append(out, part...)
	}
	delete(r.assemblies, id)
	if len(out) != a.total {
		return nil, false, ErrCarrierFrame
	}
	return out, true, nil
}
