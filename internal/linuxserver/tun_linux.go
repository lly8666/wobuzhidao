//go:build linux

package linuxserver

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	iffTUN     = 0x0001
	iffNoPI    = 0x1000
	tunSetIFF  = 0x400454ca
)

var ErrTUNUnsupported = errors.New("linuxserver: shared TUN is unsupported on this platform")

type TUN struct {
	file *os.File
	name string
}

func OpenTUN(name string) (*TUN, error) {
	if name == "" {
		name = "wbdg0"
	}
	if len(name) >= tunNameSize {
		return nil, fmt.Errorf("linuxserver: TUN name too long: %q", name)
	}
	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var ifr [40]byte
	copy(ifr[:tunNameSize], name)
	flags := uint16(iffTUN | iffNoPI)
	ifr[16] = byte(flags)
	ifr[17] = byte(flags >> 8)
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(tunSetIFF),
		uintptr(unsafe.Pointer(&ifr[0])),
	)
	if errno != 0 {
		_ = f.Close()
		return nil, errno
	}
	actual := string(bytes.TrimRight(ifr[:tunNameSize], "\x00"))
	return &TUN{file: f, name: actual}, nil
}

func (t *TUN) Name() string {
	if t == nil {
		return ""
	}
	return t.name
}

func (t *TUN) ReadPacket(p []byte) (int, error) {
	if t == nil || t.file == nil {
		return 0, os.ErrClosed
	}
	return t.file.Read(p)
}

func (t *TUN) WritePacket(p []byte) (int, error) {
	if t == nil || t.file == nil {
		return 0, os.ErrClosed
	}
	return t.file.Write(p)
}

func (t *TUN) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	err := t.file.Close()
	t.file = nil
	return err
}
