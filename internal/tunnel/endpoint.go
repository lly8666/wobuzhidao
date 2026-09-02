package tunnel

import (
	"errors"
	"io"
)

var (
	ErrPeerUnknown = errors.New("tunnel transport peer is not known yet")
	ErrShortWrite  = errors.New("short packet write")
	ErrMTU         = errors.New("packet exceeds configured MTU")
)

type Endpoint interface {
	ReadPacket([]byte) (int, error)
	WritePacket([]byte) (int, error)
	Close() error
}

type IOEndpoint struct {
	RWC io.ReadWriteCloser
}

func (e IOEndpoint) ReadPacket(p []byte) (int, error)  { return e.RWC.Read(p) }
func (e IOEndpoint) WritePacket(p []byte) (int, error) { return e.RWC.Write(p) }
func (e IOEndpoint) Close() error                      { return e.RWC.Close() }
