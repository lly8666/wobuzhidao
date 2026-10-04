package tunnel

import (
	"fmt"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
)

type FramedEndpoint struct {
	Raw Endpoint
}

func (e *FramedEndpoint) ReadPacket(dst []byte) (int, error) {
	wire := make([]byte, dataplane.HeaderLen+dataplane.MaxPacketLen)
	n, err := e.Raw.ReadPacket(wire)
	if err != nil {
		return 0, err
	}
	packet, err := dataplane.UnmarshalIP(wire[:n])
	if err != nil {
		return 0, fmt.Errorf("decode WBD IP datagram: %w", err)
	}
	if len(packet) > len(dst) {
		return 0, ErrMTU
	}
	copy(dst, packet)
	return len(packet), nil
}

func (e *FramedEndpoint) WritePacket(packet []byte) (int, error) {
	wire, err := dataplane.MarshalIP(packet)
	if err != nil {
		return 0, err
	}
	n, err := e.Raw.WritePacket(wire)
	if err != nil {
		return 0, err
	}
	if n != len(wire) {
		return 0, fmt.Errorf("%w: encoded %d/%d", ErrShortWrite, n, len(wire))
	}
	return len(packet), nil
}

func (e *FramedEndpoint) Close() error { return e.Raw.Close() }
