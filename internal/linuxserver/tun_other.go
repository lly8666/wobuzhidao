//go:build !linux

package linuxserver

import "errors"

var ErrTUNUnsupported = errors.New("linuxserver: shared TUN is unsupported on this platform")

type TUN struct{}

func OpenTUN(string) (*TUN, error) { return nil, ErrTUNUnsupported }
func (t *TUN) Name() string { return "" }
func (t *TUN) ReadPacket([]byte) (int, error) { return 0, ErrTUNUnsupported }
func (t *TUN) WritePacket([]byte) (int, error) { return 0, ErrTUNUnsupported }
func (t *TUN) Close() error { return nil }
