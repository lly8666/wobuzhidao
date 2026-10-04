package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// MaxFrameBody bounds allocation before a complete frame is available. The
// current largest useful body is a MaxPayload DATA/DATAGRAM plus small varint
// metadata; 4 KiB of metadata headroom also covers ACK ranges comfortably.
const MaxFrameBody = MaxPayload + 4096

// WriteFrame writes exactly one encoded WBD frame, tolerating short writes.
func WriteFrame(w io.Writer, frame any) error {
	if w == nil {
		return fmt.Errorf("%w: nil writer", ErrMalformed)
	}
	b, err := MarshalFrame(frame)
	if err != nil {
		return err
	}
	for len(b) > 0 {
		n, err := w.Write(b)
		if n > 0 {
			b = b[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

// ReadFrame incrementally reads exactly one WBD frame from a byte stream.
// TCP segmentation/coalescing is intentionally invisible to callers.
func ReadFrame(r io.Reader) (any, error) {
	if r == nil {
		return nil, fmt.Errorf("%w: nil reader", ErrMalformed)
	}
	var fixed [3]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		return nil, err
	}

	bodyLen, varintBytes, err := readUvarintBytes(r)
	if err != nil {
		return nil, err
	}
	if bodyLen > MaxFrameBody {
		return nil, fmt.Errorf("%w: streamed body length %d", ErrLimit, bodyLen)
	}

	wire := make([]byte, 0, 3+len(varintBytes)+int(bodyLen))
	wire = append(wire, fixed[:]...)
	wire = append(wire, varintBytes...)
	bodyStart := len(wire)
	wire = append(wire, make([]byte, int(bodyLen))...)
	if bodyLen > 0 {
		if _, err := io.ReadFull(r, wire[bodyStart:]); err != nil {
			return nil, err
		}
	}
	return UnmarshalFrame(wire)
}

func readUvarintBytes(r io.Reader) (uint64, []byte, error) {
	buf := make([]byte, 0, binary.MaxVarintLen64)
	var one [1]byte
	for i := 0; i < binary.MaxVarintLen64; i++ {
		if _, err := io.ReadFull(r, one[:]); err != nil {
			return 0, nil, err
		}
		buf = append(buf, one[0])
		if one[0] < 0x80 {
			v, n := binary.Uvarint(buf)
			if n <= 0 {
				return 0, nil, fmt.Errorf("%w: streamed body length", ErrMalformed)
			}
			return v, buf, nil
		}
	}
	return 0, nil, fmt.Errorf("%w: streamed body length varint too long", ErrMalformed)
}
