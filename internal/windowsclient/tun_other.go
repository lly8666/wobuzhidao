//go:build !windows

package windowsclient

import "errors"

var ErrWintunUnsupported = errors.New("windowsclient: Wintun is unsupported on this platform")

type TUN struct{}

func OpenTUN(string) (*TUN, error) { return nil, ErrWintunUnsupported }
func (t *TUN) Name() string { return "" }
func (t *TUN) ReadPacket([]byte) (int, error) { return 0, ErrWintunUnsupported }
func (t *TUN) WritePacket([]byte) (int, error) { return 0, ErrWintunUnsupported }
func (t *TUN) Close() error { return nil }
