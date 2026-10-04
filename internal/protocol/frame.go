package protocol

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	Version1 byte = 1

	// MaxValue keeps protocol integers within the QUIC-style 62-bit space and
	// avoids cross-language signed/overflow ambiguity later.
	MaxValue uint64 = (1 << 62) - 1

	// MaxDatagramID leaves one representable endpoint for the half-open ACK
	// range [datagram_id, datagram_id+1).
	MaxDatagramID uint64 = MaxValue - 1

	// MaxPayload is a safety bound, not a target chunk size. Scheduler/benchmark
	// work will choose much smaller normal chunks later.
	MaxPayload   = 1 << 20 // 1 MiB
	MaxAckRanges = 64
)

type FrameType byte

const (
	FrameData FrameType = 1 + iota
	FrameDatagram
	FrameAck
	FrameGapHint
)

const (
	FlagFIN byte = 1 << 0
)

type AckKind byte

const (
	AckStream AckKind = 1 + iota
	AckDatagram
)

// Range is half-open: [Start, End).
type Range struct {
	Start uint64
	End   uint64
}

type DataFrame struct {
	FlowID         FlowID
	Offset         StreamOffset
	TransmissionID TransmissionID
	FIN            bool
	Payload        []byte
}

type DatagramFrame struct {
	FlowID         FlowID
	DatagramID     DatagramID
	TransmissionID TransmissionID
	Payload        []byte
}

type AckFrame struct {
	FlowID FlowID
	Kind   AckKind
	// FIN means the receiver has observed a consistent STREAM FIN. It is not
	// valid for DATAGRAM ACKs. FIN may be true with zero ranges for an empty
	// stream.
	FIN    bool
	Ranges []Range
}

type GapHintFrame struct {
	FlowID FlowID
	Kind   AckKind
	Start  uint64
	End    uint64
}

var (
	ErrMalformed     = errors.New("malformed WBD frame")
	ErrUnsupported   = errors.New("unsupported WBD frame")
	ErrLimit         = errors.New("WBD frame limit exceeded")
	ErrTrailingBytes = errors.New("trailing bytes after WBD frame")
)

// MarshalFrame deterministically encodes exactly one WBD v1 frame.
// Wire envelope:
//
//	version: 1 byte
//	type:    1 byte
//	flags:   1 byte
//	bodyLen: uvarint
//	body:    type-specific fields
func MarshalFrame(frame any) ([]byte, error) {
	var typ FrameType
	var flags byte
	body := make([]byte, 0, 128)

	switch f := frame.(type) {
	case DataFrame:
		typ = FrameData
		if f.FIN {
			flags |= FlagFIN
		}
		var err error
		body, err = appendData(body, f)
		if err != nil {
			return nil, err
		}
	case *DataFrame:
		if f == nil {
			return nil, fmt.Errorf("%w: nil data frame", ErrMalformed)
		}
		return MarshalFrame(*f)
	case DatagramFrame:
		typ = FrameDatagram
		var err error
		body, err = appendDatagram(body, f)
		if err != nil {
			return nil, err
		}
	case *DatagramFrame:
		if f == nil {
			return nil, fmt.Errorf("%w: nil datagram frame", ErrMalformed)
		}
		return MarshalFrame(*f)
	case AckFrame:
		typ = FrameAck
		if f.FIN {
			flags |= FlagFIN
		}
		var err error
		body, err = appendAck(body, f)
		if err != nil {
			return nil, err
		}
	case *AckFrame:
		if f == nil {
			return nil, fmt.Errorf("%w: nil ack frame", ErrMalformed)
		}
		return MarshalFrame(*f)
	case GapHintFrame:
		typ = FrameGapHint
		var err error
		body, err = appendGap(body, f)
		if err != nil {
			return nil, err
		}
	case *GapHintFrame:
		if f == nil {
			return nil, fmt.Errorf("%w: nil gap frame", ErrMalformed)
		}
		return MarshalFrame(*f)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupported, frame)
	}

	out := make([]byte, 0, 3+10+len(body))
	out = append(out, Version1, byte(typ), flags)
	out = appendUvarint(out, uint64(len(body)))
	out = append(out, body...)
	return out, nil
}

// UnmarshalFrame decodes exactly one WBD v1 frame and rejects trailing bytes.
func UnmarshalFrame(data []byte) (any, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("%w: short envelope", ErrMalformed)
	}
	if data[0] != Version1 {
		return nil, fmt.Errorf("%w: version %d", ErrUnsupported, data[0])
	}
	typ := FrameType(data[1])
	flags := data[2]
	bodyLen, n := binary.Uvarint(data[3:])
	if n <= 0 {
		return nil, fmt.Errorf("%w: invalid body length", ErrMalformed)
	}
	headerLen := 3 + n
	if bodyLen > uint64(len(data)-headerLen) {
		return nil, fmt.Errorf("%w: truncated body", ErrMalformed)
	}
	end := headerLen + int(bodyLen)
	if end != len(data) {
		return nil, ErrTrailingBytes
	}
	body := data[headerLen:end]

	switch typ {
	case FrameData:
		if flags & ^FlagFIN != 0 {
			return nil, fmt.Errorf("%w: DATA flags 0x%x", ErrUnsupported, flags)
		}
		return parseData(body, flags&FlagFIN != 0)
	case FrameDatagram:
		if flags != 0 {
			return nil, fmt.Errorf("%w: DATAGRAM flags 0x%x", ErrUnsupported, flags)
		}
		return parseDatagram(body)
	case FrameAck:
		if flags&^FlagFIN != 0 {
			return nil, fmt.Errorf("%w: ACK flags 0x%x", ErrUnsupported, flags)
		}
		return parseAck(body, flags&FlagFIN != 0)
	case FrameGapHint:
		if flags != 0 {
			return nil, fmt.Errorf("%w: GAP_HINT flags 0x%x", ErrUnsupported, flags)
		}
		return parseGap(body)
	default:
		return nil, fmt.Errorf("%w: type %d", ErrUnsupported, typ)
	}
}

func appendData(dst []byte, f DataFrame) ([]byte, error) {
	if err := checkValue(uint64(f.FlowID), "flow_id"); err != nil {
		return nil, err
	}
	if err := checkValue(uint64(f.Offset), "offset"); err != nil {
		return nil, err
	}
	if err := checkValue(uint64(f.TransmissionID), "transmission_id"); err != nil {
		return nil, err
	}
	if len(f.Payload) > MaxPayload {
		return nil, fmt.Errorf("%w: payload %d", ErrLimit, len(f.Payload))
	}
	dst = appendUvarint(dst, uint64(f.FlowID))
	dst = appendUvarint(dst, uint64(f.Offset))
	dst = appendUvarint(dst, uint64(f.TransmissionID))
	dst = appendUvarint(dst, uint64(len(f.Payload)))
	dst = append(dst, f.Payload...)
	return dst, nil
}

func appendDatagram(dst []byte, f DatagramFrame) ([]byte, error) {
	if err := checkValue(uint64(f.FlowID), "flow_id"); err != nil {
		return nil, err
	}
	if uint64(f.DatagramID) > MaxDatagramID {
		return nil, fmt.Errorf("%w: datagram_id=%d", ErrLimit, f.DatagramID)
	}
	if err := checkValue(uint64(f.TransmissionID), "transmission_id"); err != nil {
		return nil, err
	}
	if len(f.Payload) > MaxPayload {
		return nil, fmt.Errorf("%w: payload %d", ErrLimit, len(f.Payload))
	}
	dst = appendUvarint(dst, uint64(f.FlowID))
	dst = appendUvarint(dst, uint64(f.DatagramID))
	dst = appendUvarint(dst, uint64(f.TransmissionID))
	dst = appendUvarint(dst, uint64(len(f.Payload)))
	dst = append(dst, f.Payload...)
	return dst, nil
}

func appendAck(dst []byte, f AckFrame) ([]byte, error) {
	if err := checkValue(uint64(f.FlowID), "flow_id"); err != nil {
		return nil, err
	}
	if err := checkKind(f.Kind); err != nil {
		return nil, err
	}
	if f.FIN && f.Kind != AckStream {
		return nil, fmt.Errorf("%w: FIN on non-stream ACK", ErrMalformed)
	}
	if len(f.Ranges) == 0 && !f.FIN {
		return nil, fmt.Errorf("%w: empty ACK without FIN", ErrLimit)
	}
	if len(f.Ranges) > MaxAckRanges {
		return nil, fmt.Errorf("%w: ack ranges %d", ErrLimit, len(f.Ranges))
	}
	dst = appendUvarint(dst, uint64(f.FlowID))
	dst = append(dst, byte(f.Kind))
	dst = appendUvarint(dst, uint64(len(f.Ranges)))
	var prevEnd uint64
	for i, r := range f.Ranges {
		if err := checkRange(r); err != nil {
			return nil, err
		}
		if i > 0 && r.Start < prevEnd {
			return nil, fmt.Errorf("%w: ack ranges overlap/out of order", ErrMalformed)
		}
		dst = appendUvarint(dst, r.Start)
		dst = appendUvarint(dst, r.End)
		prevEnd = r.End
	}
	return dst, nil
}

func appendGap(dst []byte, f GapHintFrame) ([]byte, error) {
	if err := checkValue(uint64(f.FlowID), "flow_id"); err != nil {
		return nil, err
	}
	if err := checkKind(f.Kind); err != nil {
		return nil, err
	}
	if err := checkRange(Range{Start: f.Start, End: f.End}); err != nil {
		return nil, err
	}
	dst = appendUvarint(dst, uint64(f.FlowID))
	dst = append(dst, byte(f.Kind))
	dst = appendUvarint(dst, f.Start)
	dst = appendUvarint(dst, f.End)
	return dst, nil
}

func parseData(body []byte, fin bool) (DataFrame, error) {
	flow, rest, err := takeUvarint(body)
	if err != nil {
		return DataFrame{}, err
	}
	off, rest, err := takeUvarint(rest)
	if err != nil {
		return DataFrame{}, err
	}
	tx, rest, err := takeUvarint(rest)
	if err != nil {
		return DataFrame{}, err
	}
	payload, err := takePayload(rest)
	if err != nil {
		return DataFrame{}, err
	}
	return DataFrame{FlowID: FlowID(flow), Offset: StreamOffset(off), TransmissionID: TransmissionID(tx), FIN: fin, Payload: payload}, nil
}

func parseDatagram(body []byte) (DatagramFrame, error) {
	flow, rest, err := takeUvarint(body)
	if err != nil {
		return DatagramFrame{}, err
	}
	id, rest, err := takeUvarint(rest)
	if err != nil {
		return DatagramFrame{}, err
	}
	if id > MaxDatagramID {
		return DatagramFrame{}, fmt.Errorf("%w: datagram_id=%d", ErrLimit, id)
	}
	tx, rest, err := takeUvarint(rest)
	if err != nil {
		return DatagramFrame{}, err
	}
	payload, err := takePayload(rest)
	if err != nil {
		return DatagramFrame{}, err
	}
	return DatagramFrame{FlowID: FlowID(flow), DatagramID: DatagramID(id), TransmissionID: TransmissionID(tx), Payload: payload}, nil
}

func parseAck(body []byte, fin bool) (AckFrame, error) {
	flow, rest, err := takeUvarint(body)
	if err != nil {
		return AckFrame{}, err
	}
	if len(rest) < 1 {
		return AckFrame{}, fmt.Errorf("%w: missing ack kind", ErrMalformed)
	}
	kind := AckKind(rest[0])
	rest = rest[1:]
	if err := checkKind(kind); err != nil {
		return AckFrame{}, err
	}
	count, rest, err := takeUvarint(rest)
	if err != nil {
		return AckFrame{}, err
	}
	if fin && kind != AckStream {
		return AckFrame{}, fmt.Errorf("%w: FIN on non-stream ACK", ErrMalformed)
	}
	if count == 0 && !fin {
		return AckFrame{}, fmt.Errorf("%w: empty ACK without FIN", ErrLimit)
	}
	if count > MaxAckRanges {
		return AckFrame{}, fmt.Errorf("%w: ack ranges %d", ErrLimit, count)
	}
	var ranges []Range
	if count > 0 {
		ranges = make([]Range, 0, count)
	}
	var prevEnd uint64
	for i := uint64(0); i < count; i++ {
		start, next, err := takeUvarint(rest)
		if err != nil {
			return AckFrame{}, err
		}
		rest = next
		end, next, err := takeUvarint(rest)
		if err != nil {
			return AckFrame{}, err
		}
		rest = next
		r := Range{Start: start, End: end}
		if err := checkRange(r); err != nil {
			return AckFrame{}, err
		}
		if i > 0 && r.Start < prevEnd {
			return AckFrame{}, fmt.Errorf("%w: ack ranges overlap/out of order", ErrMalformed)
		}
		ranges = append(ranges, r)
		prevEnd = r.End
	}
	if len(rest) != 0 {
		return AckFrame{}, ErrTrailingBytes
	}
	return AckFrame{FlowID: FlowID(flow), Kind: kind, FIN: fin, Ranges: ranges}, nil
}

func parseGap(body []byte) (GapHintFrame, error) {
	flow, rest, err := takeUvarint(body)
	if err != nil {
		return GapHintFrame{}, err
	}
	if len(rest) < 1 {
		return GapHintFrame{}, fmt.Errorf("%w: missing gap kind", ErrMalformed)
	}
	kind := AckKind(rest[0])
	rest = rest[1:]
	if err := checkKind(kind); err != nil {
		return GapHintFrame{}, err
	}
	start, rest, err := takeUvarint(rest)
	if err != nil {
		return GapHintFrame{}, err
	}
	end, rest, err := takeUvarint(rest)
	if err != nil {
		return GapHintFrame{}, err
	}
	if len(rest) != 0 {
		return GapHintFrame{}, ErrTrailingBytes
	}
	if err := checkRange(Range{Start: start, End: end}); err != nil {
		return GapHintFrame{}, err
	}
	return GapHintFrame{FlowID: FlowID(flow), Kind: kind, Start: start, End: end}, nil
}

func appendUvarint(dst []byte, v uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], v)
	return append(dst, buf[:n]...)
}

func takeUvarint(data []byte) (uint64, []byte, error) {
	v, n := binary.Uvarint(data)
	if n <= 0 {
		return 0, nil, fmt.Errorf("%w: invalid uvarint", ErrMalformed)
	}
	if v > MaxValue {
		return 0, nil, fmt.Errorf("%w: integer %d", ErrLimit, v)
	}
	return v, data[n:], nil
}

func takePayload(rest []byte) ([]byte, error) {
	ln, rest, err := takeUvarint(rest)
	if err != nil {
		return nil, err
	}
	if ln > MaxPayload {
		return nil, fmt.Errorf("%w: payload %d", ErrLimit, ln)
	}
	if uint64(len(rest)) != ln {
		return nil, fmt.Errorf("%w: payload length want=%d got=%d", ErrMalformed, ln, len(rest))
	}
	out := make([]byte, len(rest))
	copy(out, rest)
	return out, nil
}

func checkValue(v uint64, name string) error {
	if v > MaxValue {
		return fmt.Errorf("%w: %s=%d", ErrLimit, name, v)
	}
	return nil
}

func checkKind(k AckKind) error {
	if k != AckStream && k != AckDatagram {
		return fmt.Errorf("%w: ack kind %d", ErrUnsupported, k)
	}
	return nil
}

func checkRange(r Range) error {
	if r.Start > MaxValue || r.End > MaxValue {
		return fmt.Errorf("%w: range [%d,%d)", ErrLimit, r.Start, r.End)
	}
	if r.End <= r.Start {
		return fmt.Errorf("%w: invalid range [%d,%d)", ErrMalformed, r.Start, r.End)
	}
	return nil
}
