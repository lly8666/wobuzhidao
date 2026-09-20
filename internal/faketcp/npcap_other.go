//go:build !windows

package faketcp

type NpcapEndpoint struct{}

func OpenNpcapEndpoint(NpcapConfig) (*NpcapEndpoint, error) {
	return nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) ReadSegment(uint64) (Segment, []byte, error) {
	return Segment{}, nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) WriteSegment(uint64, Segment) ([]byte, error) {
	return nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) Close() error {
	return nil
}
