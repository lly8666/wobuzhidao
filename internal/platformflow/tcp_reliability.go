package platformflow

import (
	"bytes"
	"fmt"
	"math"
	"time"
)

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
	if c.ChunkSize <= 0 || c.ChunkSize > MaxPayload ||
		c.MaxInFlight <= 0 || c.RTO <= 0 || c.MaxRetransmits < 0 ||
		c.MaxReorderBytes < c.ChunkSize {
		return fmt.Errorf("%w: invalid TCP reliability config", ErrMalformed)
	}
	return nil
}

type tcpTxSegment struct {
	frame         Frame
	sentAt        time.Time
	transmissions int
}

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

func (t *TCPTransmit) Queue(data []byte, fin bool, now time.Time) ([]Frame, error) {
	if t.closed || t.finOffset != nil {
		return nil, ErrClosed
	}
	if len(data) == 0 && !fin {
		return nil, nil
	}
	chunks := (len(data) + t.cfg.ChunkSize - 1) / t.cfg.ChunkSize
	if len(data) == 0 && fin {
		chunks = 1
	}
	if len(t.pending)+chunks > t.cfg.MaxInFlight {
		return nil, ErrWindowFull
	}

	frames := make([]Frame, 0, chunks)
	pos := 0
	for i := 0; i < chunks; i++ {
		n := t.cfg.ChunkSize
		if remain := len(data) - pos; remain < n {
			n = remain
		}
		if uint64(n) > math.MaxUint64-t.nextOffset {
			return nil, ErrLimit
		}
		f := Frame{
			Kind: KindTCPData, FlowID: t.flowID, Offset: t.nextOffset,
			Payload: append([]byte(nil), data[pos:pos+n]...),
		}
		if fin && i == chunks-1 {
			f.FIN = true
		}
		t.nextOffset += uint64(n)
		if f.FIN {
			v := t.nextOffset
			t.finOffset = &v
		}
		t.pending = append(t.pending, &tcpTxSegment{frame: f, sentAt: now, transmissions: 1})
		frames = append(frames, cloneFrame(f))
		pos += n
	}
	return frames, nil
}

func (t *TCPTransmit) Ack(next uint64) error {
	if t.closed {
		return ErrClosed
	}
	if next < t.ackedOffset {
		return nil
	}
	if next > t.nextOffset {
		return fmt.Errorf("%w: ACK=%d sent=%d", ErrMalformed, next, t.nextOffset)
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

func (t *TCPTransmit) RetransmitDue(now time.Time) ([]Frame, error) {
	if t.closed {
		return nil, ErrClosed
	}
	var out []Frame
	for _, seg := range t.pending {
		if now.Before(seg.sentAt) || now.Sub(seg.sentAt) < t.cfg.RTO {
			continue
		}
		if seg.transmissions > t.cfg.MaxRetransmits {
			return nil, ErrRetryLimit
		}
		seg.sentAt = now
		seg.transmissions++
		out = append(out, cloneFrame(seg.frame))
	}
	return out, nil
}

func (t *TCPTransmit) Abort() {
	t.closed = true
	t.pending = nil
}

func (t *TCPTransmit) InFlight() int      { return len(t.pending) }
func (t *TCPTransmit) NextOffset() uint64 { return t.nextOffset }
func (t *TCPTransmit) AckedOffset() uint64 { return t.ackedOffset }
func (t *TCPTransmit) FINAcked() bool     { return t.finAcked }

type tcpRxSegment struct {
	payload []byte
	fin     bool
}

type TCPReceiveResult struct {
	Delivered []byte
	Ack       Frame
	FIN       bool
	Duplicate bool
}

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
		return nil, ErrMalformed
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &TCPReceive{flowID: flowID, cfg: cfg, pending: make(map[uint64]tcpRxSegment)}, nil
}

func (r *TCPReceive) Push(f Frame) (TCPReceiveResult, error) {
	out := TCPReceiveResult{Ack: Frame{Kind: KindTCPAck, FlowID: r.flowID, Offset: r.nextOffset}}
	if r.closed {
		return out, ErrClosed
	}
	if f.Kind != KindTCPData || f.FlowID != r.flowID || len(f.Payload) > r.cfg.ChunkSize {
		return out, ErrMalformed
	}
	if len(f.Payload) == 0 && !f.FIN {
		return out, ErrMalformed
	}
	if uint64(len(f.Payload)) > math.MaxUint64-f.Offset {
		return out, ErrLimit
	}
	end := f.Offset + uint64(len(f.Payload))
	if r.finOffset != nil && end > *r.finOffset {
		return out, ErrMalformed
	}
	if f.FIN {
		if r.finOffset != nil && *r.finOffset != end {
			return out, ErrMalformed
		}
		v := end
		r.finOffset = &v
	}

	if len(f.Payload) == 0 && f.FIN && f.Offset == r.nextOffset && !r.finDelivered {
		r.finDelivered = true
		out.FIN = true
		out.Ack.Offset = r.nextOffset
		return out, nil
	}
	if end <= r.nextOffset {
		out.Duplicate = true
		out.FIN = r.finOffset != nil && r.nextOffset >= *r.finOffset
		out.Ack.Offset = r.nextOffset
		return out, nil
	}

	start := f.Offset
	payload := append([]byte(nil), f.Payload...)
	if start < r.nextOffset {
		trim := int(r.nextOffset - start)
		start = r.nextOffset
		payload = payload[trim:]
	}
	if err := r.insert(start, payload, f.FIN); err != nil {
		return out, err
	}

	for {
		seg, ok := r.pending[r.nextOffset]
		if !ok {
			break
		}
		delete(r.pending, r.nextOffset)
		r.buffered -= len(seg.payload)
		out.Delivered = append(out.Delivered, seg.payload...)
		r.nextOffset += uint64(len(seg.payload))
		if seg.fin {
			v := r.nextOffset
			if r.finOffset != nil && *r.finOffset != v {
				return out, ErrMalformed
			}
			r.finOffset = &v
		}
		if len(seg.payload) == 0 {
			break
		}
	}
	if r.finOffset != nil && r.nextOffset == *r.finOffset {
		r.finDelivered = true
	}
	out.Ack.Offset = r.nextOffset
	out.FIN = r.finDelivered
	return out, nil
}

func (r *TCPReceive) insert(start uint64, payload []byte, fin bool) error {
	end := start + uint64(len(payload))
	for off, existing := range r.pending {
		existingEnd := off + uint64(len(existing.payload))
		if start == off && end == existingEnd {
			if bytes.Equal(payload, existing.payload) && fin == existing.fin {
				return nil
			}
			return ErrMalformed
		}
		if start < existingEnd && off < end {
			return ErrMalformed
		}
	}
	if r.buffered+len(payload) > r.cfg.MaxReorderBytes {
		return ErrLimit
	}
	r.pending[start] = tcpRxSegment{payload: append([]byte(nil), payload...), fin: fin}
	r.buffered += len(payload)
	return nil
}

func (r *TCPReceive) Close() {
	r.closed = true
	r.pending = make(map[uint64]tcpRxSegment)
	r.buffered = 0
}

func (r *TCPReceive) NextOffset() uint64 { return r.nextOffset }
func (r *TCPReceive) BufferedBytes() int { return r.buffered }
func (r *TCPReceive) FINDelivered() bool { return r.finDelivered }

func cloneFrame(f Frame) Frame {
	f.Payload = append([]byte(nil), f.Payload...)
	return f
}
