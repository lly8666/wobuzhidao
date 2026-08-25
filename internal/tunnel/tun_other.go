//go:build !linux

package tunnel

import "errors"

type TUN struct{}

func OpenTUN(name string) (*TUN, error) {
	return nil, errors.New("native TUN is not implemented on this platform yet")
}

func (t *TUN) Name() string                    { return "" }
func (t *TUN) ReadPacket([]byte) (int, error)  { return 0, errors.New("unsupported") }
func (t *TUN) WritePacket([]byte) (int, error) { return 0, errors.New("unsupported") }
func (t *TUN) Close() error                    { return nil }
