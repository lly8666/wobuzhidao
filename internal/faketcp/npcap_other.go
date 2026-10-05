//go:build !windows

package faketcp

type NpcapEndpoint struct{}

func (*NpcapEndpoint) SetIODiagnostics(bool)           {}
func (*NpcapEndpoint) IODiagnostic() NpcapIODiagnostic { return NpcapIODiagnostic{} }

func OpenNpcapEndpoint(NpcapConfig) (*NpcapEndpoint, error) {
	return nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) ReadSegment(uint64) (Segment, []byte, error) {
	return Segment{}, nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) WriteSegment(uint64, Segment) ([]byte, error) {
	return nil, ErrNpcapUnsupported
}

func (*NpcapEndpoint) WriteSegments(uint64, []Segment) (int, error) { return 0, ErrNpcapUnsupported }

func (*NpcapEndpoint) Close() error {
	return nil
}
