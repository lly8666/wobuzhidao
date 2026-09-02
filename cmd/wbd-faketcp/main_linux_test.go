//go:build linux

package main

import (
	"errors"
	"syscall"
	"testing"
)

func TestReadLinuxRawPacketRetriesEINTR(t *testing.T) {
	calls := 0
	recv := func(_ int, _ []byte, _ int) (int, syscall.Sockaddr, error) {
		calls++
		if calls == 1 {
			return 0, nil, syscall.EINTR
		}
		return 37, nil, nil
	}

	n, err := readLinuxRawPacket(recv, 123, make([]byte, 64))
	if err != nil {
		t.Fatalf("readLinuxRawPacket: %v", err)
	}
	if n != 37 {
		t.Fatalf("bytes=%d want 37", n)
	}
	if calls != 2 {
		t.Fatalf("recv calls=%d want 2", calls)
	}
}

func TestReadLinuxRawPacketMapsWouldBlockToTimeout(t *testing.T) {
	for _, wantErr := range []error{syscall.EAGAIN, syscall.EWOULDBLOCK} {
		t.Run(wantErr.Error(), func(t *testing.T) {
			recv := func(_ int, _ []byte, _ int) (int, syscall.Sockaddr, error) {
				return 0, nil, wantErr
			}
			n, err := readLinuxRawPacket(recv, 123, make([]byte, 64))
			if n != 0 {
				t.Fatalf("bytes=%d want 0", n)
			}
			if !errors.Is(err, errRawTimeout) {
				t.Fatalf("error=%v want errRawTimeout", err)
			}
		})
	}
}
