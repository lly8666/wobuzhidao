package singleflow

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
)

const (
	// BootstrapMaxPayload keeps TLS records split into ordinary TCP-sized
	// segments while the public FakeTCP association is in its short ordered
	// bootstrap phase. Steady-state DTLS datagrams are not constrained by this
	// stream helper and retain the existing packet-preserving data path.
	BootstrapMaxPayload = 1360

	switchVersion  = byte(1)
	switchRequest  = byte(1)
	switchAck      = byte(2)
	switchDigestN  = 16
	SwitchFrameLen = 4 + 1 + 1 + switchDigestN
)

var switchMagic = [4]byte{'W', 'B', 'S', 'F'}

var ErrBadSwitchFrame = errors.New("singleflow: invalid mode-switch frame")

// OrderedAssembler exists only for the Reality-like bootstrap phase. It turns
// first-arrival FakeTCP segments back into an ordered byte stream for TLS. The
// assembler is discarded at the mode-switch barrier; steady-state DTLS/FEC
// continues to use the existing first-arrival, no-HOL FakeTCP receiver.
type OrderedAssembler struct {
	next    uint32
	pending map[uint32][]byte
}

func NewOrderedAssembler(next uint32) *OrderedAssembler {
	return &OrderedAssembler{next: next, pending: make(map[uint32][]byte)}
}

func (a *OrderedAssembler) Next() uint32 { return a.next }

// Push returns only bytes that have become contiguous. Out-of-order payload is
// buffered during bootstrap, and retransmitted/old payload is ignored. WBD's
// sender emits non-overlapping segment boundaries, so exact-sequence buffering
// is sufficient and keeps the bootstrap reassembler deliberately small.
func (a *OrderedAssembler) Push(seq uint32, payload []byte) []byte {
	if len(payload) == 0 || seqLT(seq, a.next) {
		return nil
	}
	if seq != a.next {
		if _, exists := a.pending[seq]; !exists {
			a.pending[seq] = append([]byte(nil), payload...)
		}
		return nil
	}

	out := append([]byte(nil), payload...)
	a.next += uint32(len(payload))
	for {
		p, ok := a.pending[a.next]
		if !ok {
			break
		}
		delete(a.pending, a.next)
		out = append(out, p...)
		a.next += uint32(len(p))
	}
	return out
}

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }

// SwitchRequest is encrypted as TLS 1.3 application data inside the bounded
// Reality-like bootstrap stream. The frame never appears as plaintext on the
// public carrier. It binds the transition to the admitted one-time ticket
// without exposing that ticket. No FIN/RST/new SYN is sent at the boundary.
func SwitchRequest(ticket []byte) []byte { return makeSwitchFrame(switchRequest, ticket) }

// SwitchAck is likewise carried inside TLS 1.3 application data. The server
// starts the DTLS worker before writing this ACK and switches its raw receiver
// to datagram mode immediately after the TLS record has been queued. A client
// that can decrypt the ACK therefore knows the peer is already ready for the
// first DTLS ClientHello on the same TCP-shaped 4-tuple.
func SwitchAck(ticket []byte) []byte { return makeSwitchFrame(switchAck, ticket) }

func IsSwitchRequest(frame, ticket []byte) bool { return verifySwitchFrame(frame, switchRequest, ticket) }
func IsSwitchAck(frame, ticket []byte) bool     { return verifySwitchFrame(frame, switchAck, ticket) }

func makeSwitchFrame(kind byte, ticket []byte) []byte {
	h := sha256.Sum256(ticket)
	out := make([]byte, SwitchFrameLen)
	copy(out[:4], switchMagic[:])
	out[4] = switchVersion
	out[5] = kind
	copy(out[6:], h[:switchDigestN])
	return out
}

func verifySwitchFrame(frame []byte, kind byte, ticket []byte) bool {
	if len(frame) != SwitchFrameLen || frame[4] != switchVersion || frame[5] != kind ||
		subtle.ConstantTimeCompare(frame[:4], switchMagic[:]) != 1 {
		return false
	}
	h := sha256.Sum256(ticket)
	return subtle.ConstantTimeCompare(frame[6:], h[:switchDigestN]) == 1
}
