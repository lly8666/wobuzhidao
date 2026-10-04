package platformproxy

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	ErrTCPClosed     = errors.New("platformproxy: TCP flow closed")
	ErrTCPWindowFull = errors.New("platformproxy: TCP send window full")
	ErrTCPRetryLimit = errors.New("platformproxy: TCP retransmission limit reached")
)

// TCPReliabilityConfig bounds one direction of one platform TCP flow. It is an
// adapter policy above WBDP. It must not be confused with, or coupled to,
// FakeTCP recovery/FEC/DTLS policy below the opaque WBD datagram boundary.
type TCPReliabilityConfig struct {
	ChunkSize       int
	MaxInFlight     int
	RTO             time.Duration
	MaxRetransmits  int
	MaxReorderBytes int
}

func DefaultTCPReliabilityConfig() TCPReliabilityConfig {
	return TCPReliabilityConfig{
		ChunkSize:       MaxPayload,
		MaxInFlight:     16,
		RTO:             500 * time.Millisecond,
		MaxRetransmits:  8,
		MaxReorderBytes: 16 * MaxPayload,
	}
}

func (c TCPReliabilityConfig) validate() error {
	if c.ChunkSize <= 0 || c.ChunkSize > MaxPayload || c.MaxInFlight <= 0 || c.RTO <= 0 || c.MaxRetransmits < 0 || c.MaxReorderBytes < c.ChunkSize {
		return fmt.Errorf("%w: invalid TCP reliability config", ErrMalformed)
	}
	return nil
}

type tcpTxSegment struct {
	frame         Frame
	sentAt        time.Time
	transmissions int
}

// TCPTransmit owns one reliable byte-stream direction. Queue never waits for
// space: the socket adapter must stop reading that application connection when
// ErrTCPWindowFull is returned. This keeps memory bounded per flow and, more
// importantly, prevents one blocked TCP application flow from gating another.
//
// TCPTransmit is intentionally transport-agnostic and is not concurrency-safe;
// one platform-flow goroutine should own it.
type TCPTransmit struct {
	flowID uint64
	cfg    TCPReliabilityConfig

	nextOffset  uint64
	ackedOffset uint64
	pending     []*tcpTxSegment

	finOffset *uint64
	finAcked  bool
	closed    bool
}

func NewTCPTransmit(flowID uint64, cfg TCPReliabilityConfig) (*TCPTransmit, error) {
	if flowID == 0 {
		return nil, fmt.Errorf("%w: zero TCP flow id", ErrMalformed)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &TCPTransmit{flowID: flowID, cfg: cfg}, nil
}

// Queue splits application bytes into WBDP DATA frames and places every new
// frame in the bounded retransmission window. FIN, when requested, is attached
// to the final chunk (or emitted as a zero-byte FIN frame for an empty write
// direction). No bytes may be queued after FIN.
func (t *TCPTransmit) Queue(data []byte, fin bool, now time.Time) ([]Frame, error) {
	if t.closed || t.finOffset != nil {
		return nil, ErrTCPClosed
	}
	if len(data) == 0 && !fin {
		return nil, nil
	}

	chunks := (len(data) + t.cfg.ChunkSize - 1) / t.cfg.ChunkSize
	if len(data) == 0 && fin {
		chunks = 1
	}
	if len(t.pending)+chunks > t.cfg.MaxInFlight {
		return nil, ErrTCPWindowFull
	}

	frames := make([]Frame, 0, chunks)
	pos := 0
	for i := 0; i < chunks; i++ {
		n := t.cfg.ChunkSize
		if remain := len(data) - pos; remain < n {
			n = remain
		}
		payload := append([]byte(nil), data[pos:pos+n]...)
		f := Frame{
			Kind:    KindTCPData,
			FlowID:  t.flowID,
			Offset:  t.nextOffset,
			Payload: payload,
		}
		if fin && i == chunks-1 {
			f.FIN = true
		}
		if uint64(n) > math.MaxUint64-t.nextOffset {
			return nil, fmt.Errorf("%w: TCP offset overflow", ErrLimit)
		}
		t.nextOffset += uint64(n)
		if f.FIN {
			v := t.nextOffset
			t.finOffset = &v
		}
		t.pending = append(t.pending, &tcpTxSegment{
			frame:         f,
			sentAt:        now,
			transmissions: 1,
		})
		frames = append(frames, cloneTCPFrame(f))
		pos += n
	}
	return frames, nil
}

// Ack applies a cumulative next-byte ACK. Stale ACKs are harmless. A partial
// ACK trims the acknowledged prefix from the one surviving segment so future
// retransmission contains only bytes that remain unacknowledged.
func (t *TCPTransmit) Ack(next uint64) error {
	if t.closed {
		return ErrTCPClosed
	}
	if next < t.ackedOffset {
		return nil
	}
	if next > t.nextOffset {
		return fmt.Errorf("%w: TCP ACK=%d beyond sent=%d", ErrMalformed, next, t.nextOffset)
	}
	t.ackedOffset = next

	kept := t.pending[:0]
	for _, seg := range t.pending {
		start := seg.frame.Offset
		end := start + uint64(len(seg.frame.Payload))
		if end <= next {
			continue
		}
		if start < next {
			trim := int(next - start)
			seg.frame.Offset = next
			seg.frame.Payload = append([]byte(nil), seg.frame.Payload[trim:]...)
		}
		kept = append(kept, seg)
	}
	t.pending = kept
	if t.finOffset != nil && next >= *t.finOffset {
		t.finAcked = true
	}
	return nil
}

// RetransmitDue returns only currently-unacknowledged segments whose RTO has
// elapsed. MaxRetransmits counts retries after the original transmission.
func (t *TCPTransmit) RetransmitDue(now time.Time) ([]Frame, error) {
	if t.closed {
		return nil, ErrTCPClosed
	}
	out := make([]Frame, 0)
	for _, seg := range t.pending {
		if now.Before(seg.sentAt) || now.Sub(seg.sentAt) < t.cfg.RTO {
			continue
		}
		if seg.transmissions > t.cfg.MaxRetransmits {
			return nil, ErrTCPRetryLimit
		}
		seg.sentAt = now
		seg.transmissions++
		out = append(out, cloneTCPFrame(seg.frame))
	}
	return out, nil
}

// Abort terminally closes this local direction and drops retransmission state.
// TCPClose is deliberately a platform-flow control frame, not a WBD transport
// close and not a FakeTCP state transition.
func (t *TCPTransmit) Abort() Frame {
	t.closed = true
	t.pending = nil
	return Frame{Kind: KindTCPClose, FlowID: t.flowID}
}

func (t *TCPTransmit) InFlight() int      { return len(t.pending) }
func (t *TCPTransmit) NextOffset() uint64 { return t.nextOffset }
func (t *TCPTransmit) AckedOffset() uint64 {
	return t.ackedOffset
}
func (t *TCPTransmit) FINAcked() bool { return t.finAcked }

type tcpRxSegment struct {
	offset  uint64
	payload []byte
	fin     bool
}

// TCPReceiveResult contains newly-contiguous bytes and the cumulative ACK that
// should be sent after processing the input DATA frame. Delivered never
// contains bytes that were already returned by an earlier Push call.
type TCPReceiveResult struct {
	Delivered []byte
	Ack       Frame
	FIN       bool
	Duplicate bool
}

// TCPReceive owns one incoming byte-stream direction and buffers only a bounded
// amount of out-of-order data. Exact retransmission duplicates are idempotent;
// conflicting overlaps are rejected because WBDP peers created by this adapter
// produce fixed, non-overlapping byte ranges.
type TCPReceive struct {
	flowID uint64
	cfg    TCPReliabilityConfig

	nextOffset uint64
	pending    map[uint64]tcpRxSegment
	buffered   int

	finOffset    *uint64
	finDelivered bool
	closed       bool
}

func NewTCPReceive(flowID uint64, cfg TCPReliabilityConfig) (*TCPReceive, error) {
	if flowID == 0 {
		return nil, fmt.Errorf("%w: zero TCP flow id", ErrMalformed)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &TCPReceive{
		flowID:  flowID,
		cfg:     cfg,
		pending: make(map[uint64]tcpRxSegment),
	}, nil
}

func (r *TCPReceive) Push(f Frame) (TCPReceiveResult, error) {
	result := TCPReceiveResult{Ack: Frame{Kind: KindTCPAck, FlowID: r.flowID, Offset: r.nextOffset}}
	if r.closed {
		return result, ErrTCPClosed
	}
	if f.Kind != KindTCPData || f.FlowID != r.flowID {
		return result, fmt.Errorf("%w: TCP DATA flow/kind mismatch", ErrMalformed)
	}
	if len(f.Payload) > r.cfg.ChunkSize {
		return result, fmt.Errorf("%w: TCP DATA chunk=%d", ErrLimit, len(f.Payload))
	}
	if len(f.Payload) == 0 && !f.FIN {
		return result, fmt.Errorf("%w: empty TCP DATA", ErrMalformed)
	}
	if uint64(len(f.Payload)) > math.MaxUint64-f.Offset {
		return result, fmt.Errorf("%w: TCP DATA offset overflow", ErrLimit)
	}
	end := f.Offset + uint64(len(f.Payload))
	if r.finOffset != nil && end > *r.finOffset {
		return result, fmt.Errorf("%w: TCP DATA beyond FIN", ErrMalformed)
	}
	if f.FIN {
		if r.finOffset != nil && *r.finOffset != end {
			return result, fmt.Errorf("%w: conflicting TCP FIN offsets", ErrMalformed)
		}
		v := end
		r.finOffset = &v
	}

	// A zero-byte FIN at exactly the next expected byte is a new half-close
	// event the first time it is seen. Only subsequent copies are duplicates.
	if len(f.Payload) == 0 && f.FIN && f.Offset == r.nextOffset && !r.finDelivered {
		r.finDelivered = true
		result.FIN = true
		result.Ack.Offset = r.nextOffset
		return result, nil
	}
	if end <= r.nextOffset {
		result.Duplicate = true
		result.FIN = r.finOffset != nil && r.nextOffset >= *r.finOffset
		result.Ack.Offset = r.nextOffset
		return result, nil
	}

	start := f.Offset
	payload := append([]byte(nil), f.Payload...)
	if start < r.nextOffset {
		trim := int(r.nextOffset - start)
		start = r.nextOffset
		payload = payload[trim:]
	}
	if err := r.insert(start, payload, f.FIN); err != nil {
		return result, err
	}

	var delivered []byte
	for {
		seg, ok := r.pending[r.nextOffset]
		if !ok {
			break
		}
		delete(r.pending, r.nextOffset)
		r.buffered -= len(seg.payload)
		delivered = append(delivered, seg.payload...)
		r.nextOffset += uint64(len(seg.payload))
		if seg.fin {
			v := r.nextOffset
			if r.finOffset != nil && *r.finOffset != v {
				return result, fmt.Errorf("%w: TCP FIN delivery mismatch", ErrMalformed)
			}
			r.finOffset = &v
		}
		// A zero-byte FIN segment cannot advance nextOffset, so stop after
		// consuming it rather than looking up the same map key again.
		if len(seg.payload) == 0 {
			break
		}
	}
	if r.finOffset != nil && r.nextOffset == *r.finOffset {
		r.finDelivered = true
	}
	result.Delivered = delivered
	result.Ack.Offset = r.nextOffset
	result.FIN = r.finDelivered
	return result, nil
}

func (r *TCPReceive) insert(start uint64, payload []byte, fin bool) error {
	end := start + uint64(len(payload))
	for off, existing := range r.pending {
		existingEnd := off + uint64(len(existing.payload))
		if start == off && end == existingEnd {
			if bytes.Equal(payload, existing.payload) && fin == existing.fin {
				return nil
			}
			return fmt.Errorf("%w: conflicting duplicate TCP segment", ErrMalformed)
		}
		if start < existingEnd && off < end {
			return fmt.Errorf("%w: overlapping TCP segments", ErrMalformed)
		}
	}
	if r.buffered+len(payload) > r.cfg.MaxReorderBytes {
		return fmt.Errorf("%w: TCP reorder buffer", ErrLimit)
	}
	r.pending[start] = tcpRxSegment{
		offset:  start,
		payload: append([]byte(nil), payload...),
		fin:     fin,
	}
	r.buffered += len(payload)
	return nil
}

// Close applies a terminal WBDP flow close. It intentionally does not alter
// the WBD association/session that carries this platform flow.
func (r *TCPReceive) Close(f Frame) error {
	if f.Kind != KindTCPClose || f.FlowID != r.flowID {
		return fmt.Errorf("%w: TCP CLOSE flow/kind mismatch", ErrMalformed)
	}
	r.closed = true
	r.pending = make(map[uint64]tcpRxSegment)
	r.buffered = 0
	return nil
}

func (r *TCPReceive) NextOffset() uint64 { return r.nextOffset }
func (r *TCPReceive) BufferedBytes() int { return r.buffered }
func (r *TCPReceive) FINDelivered() bool { return r.finDelivered }

func cloneTCPFrame(f Frame) Frame {
	f.Payload = append([]byte(nil), f.Payload...)
	return f
}
