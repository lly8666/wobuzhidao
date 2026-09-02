package gamelane

import (
	"encoding/binary"
	"errors"
	"math"
)

const (
	HeaderSize = 32
	MaxLanes   = 4
)

var magic = [4]byte{'W', 'G', 'L', '1'}

type SessionID [16]byte

var (
	ErrMalformed    = errors.New("gamelane: malformed race datagram")
	ErrWrongSession = errors.New("gamelane: wrong logical lane session")
	ErrPacketIDWrap = errors.New("gamelane: packet id exhausted")
	ErrReplayTooOld = errors.New("gamelane: packet id outside replay window")
)

type Header struct {
	SessionID  SessionID
	PacketID   uint64
	PayloadLen uint16
	LaneID     uint8
}

type LaneCopy struct {
	LaneID uint8
	Wire   []byte
}

type Encoder struct {
	sessionID SessionID
	nextID    uint64
}

func NewEncoder(sessionID SessionID, firstPacketID uint64) (*Encoder, error) {
	if sessionID == (SessionID{}) || firstPacketID == 0 {
		return nil, ErrMalformed
	}
	return &Encoder{sessionID: sessionID, nextID: firstPacketID}, nil
}

func (e *Encoder) SessionID() SessionID { return e.sessionID }

// Wrap keeps a one-lane compatibility helper for tests and non-racing callers.
func (e *Encoder) Wrap(payload []byte) (packetID uint64, wire []byte, err error) {
	id, copies, err := e.WrapCopies(payload, []uint8{1})
	if err != nil {
		return 0, nil, err
	}
	return id, copies[0].Wire, nil
}

// WrapCopies assigns one logical PacketID and emits lane-distinct plaintext
// envelopes for 1..4 independent WBD associations. All copies carry the same
// inner payload and PacketID, but LaneID differs before DTLS; with independent
// association keys/nonces and FakeTCP 5-tuples/sequence spaces, the outer wire
// traffic is genuinely distinct rather than the same ciphertext duplicated on
// one flow.
func (e *Encoder) WrapCopies(payload []byte, laneIDs []uint8) (packetID uint64, copies []LaneCopy, err error) {
	if len(payload) == 0 || len(payload) > math.MaxUint16 || len(laneIDs) == 0 || len(laneIDs) > MaxLanes {
		return 0, nil, ErrMalformed
	}
	seen := [MaxLanes + 1]bool{}
	for _, laneID := range laneIDs {
		if laneID == 0 || laneID > MaxLanes || seen[laneID] {
			return 0, nil, ErrMalformed
		}
		seen[laneID] = true
	}
	if e.nextID == math.MaxUint64 {
		return 0, nil, ErrPacketIDWrap
	}
	packetID = e.nextID
	e.nextID++
	copies = make([]LaneCopy, 0, len(laneIDs))
	for _, laneID := range laneIDs {
		wire := make([]byte, HeaderSize+len(payload))
		copy(wire[:4], magic[:])
		copy(wire[4:20], e.sessionID[:])
		binary.BigEndian.PutUint64(wire[20:28], packetID)
		binary.BigEndian.PutUint16(wire[28:30], uint16(len(payload)))
		wire[30] = laneID
		// byte 31 is reserved zero for future scheduling flags.
		copy(wire[HeaderSize:], payload)
		copies = append(copies, LaneCopy{LaneID: laneID, Wire: wire})
	}
	return packetID, copies, nil
}

type DecodeResult struct {
	PacketID  uint64
	LaneID    uint8
	Payload   []byte
	Deliver   bool
	Duplicate bool
	Stale     bool
}

type Decoder struct {
	sessionID SessionID
	window    uint64
	haveHigh  bool
	highest   uint64
	seen      map[uint64]struct{}
}

func NewDecoder(sessionID SessionID, replayWindow int) (*Decoder, error) {
	if sessionID == (SessionID{}) || replayWindow < 64 || replayWindow > 1<<20 {
		return nil, ErrMalformed
	}
	return &Decoder{sessionID: sessionID, window: uint64(replayWindow), seen: make(map[uint64]struct{}, replayWindow)}, nil
}

func (d *Decoder) SessionID() SessionID { return d.sessionID }
func (d *Decoder) HighestPacketID() (uint64, bool) { return d.highest, d.haveHigh }
func (d *Decoder) Recent() int { return len(d.seen) }

// Add implements first-arrival delivery across lanes. The first copy of a
// PacketID is returned immediately. Later copies from any other lane are
// suppressed. Unique packets may arrive out of order inside the bounded replay
// window and are still delivered independently; there is no cross-lane HOL.
func (d *Decoder) Add(wire []byte) (DecodeResult, error) {
	h, payload, err := Parse(wire)
	if err != nil {
		return DecodeResult{}, err
	}
	if h.SessionID != d.sessionID {
		return DecodeResult{}, ErrWrongSession
	}
	result := DecodeResult{PacketID: h.PacketID, LaneID: h.LaneID}
	if d.haveHigh && h.PacketID < d.highest && d.highest-h.PacketID >= d.window {
		result.Stale = true
		return result, ErrReplayTooOld
	}
	if _, ok := d.seen[h.PacketID]; ok {
		result.Duplicate = true
		return result, nil
	}

	if !d.haveHigh || h.PacketID > d.highest {
		d.highest, d.haveHigh = h.PacketID, true
		d.evictOld()
	}
	d.seen[h.PacketID] = struct{}{}
	result.Deliver = true
	result.Payload = append([]byte(nil), payload...)
	return result, nil
}

func (d *Decoder) evictOld() {
	if !d.haveHigh || d.highest < d.window {
		return
	}
	cutoff := d.highest - d.window + 1
	for id := range d.seen {
		if id < cutoff {
			delete(d.seen, id)
		}
	}
}

func Parse(wire []byte) (Header, []byte, error) {
	var h Header
	if len(wire) < HeaderSize || string(wire[:4]) != string(magic[:]) || wire[31] != 0 {
		return h, nil, ErrMalformed
	}
	copy(h.SessionID[:], wire[4:20])
	h.PacketID = binary.BigEndian.Uint64(wire[20:28])
	h.PayloadLen = binary.BigEndian.Uint16(wire[28:30])
	h.LaneID = wire[30]
	if h.SessionID == (SessionID{}) || h.PacketID == 0 || h.PayloadLen == 0 || h.LaneID == 0 || h.LaneID > MaxLanes || len(wire) != HeaderSize+int(h.PayloadLen) {
		return Header{}, nil, ErrMalformed
	}
	return h, wire[HeaderSize:], nil
}
