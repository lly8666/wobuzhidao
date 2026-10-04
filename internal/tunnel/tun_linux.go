//go:build linux

package tunnel

import (
	"bytes"
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

const (
	ifNameSize = 16
	iffTUN     = 0x0001
	iffNoPI    = 0x1000
	tunSetIFF  = 0x400454ca
)

type TUN struct {
	file *os.File
	name string
}

func OpenTUN(name string) (*TUN, error) {
	if name == "" {
		name = "wbd%d"
	}
	if len(name) >= ifNameSize {
		return nil, fmt.Errorf("TUN name too long: %q", name)
	}

	f, err := os.OpenFile("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}

	var ifr [40]byte
	copy(ifr[:ifNameSize], name)
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

	actual := string(bytes.TrimRight(ifr[:ifNameSize], "\x00"))
	return &TUN{file: f, name: actual}, nil
}

func (t *TUN) Name() string                      { return t.name }
func (t *TUN) ReadPacket(p []byte) (int, error)  { return t.file.Read(p) }
func (t *TUN) WritePacket(p []byte) (int, error) { return t.file.Write(p) }
func (t *TUN) Close() error                      { return t.file.Close() }
