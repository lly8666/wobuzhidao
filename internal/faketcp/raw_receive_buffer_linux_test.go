//go:build linux

package faketcp

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestRawReceiveBufferRequestValidationAndLimitClassification(t *testing.T) {
	for _, v := range []int{0, 1, DefaultRawReceiveBufferRequestBytes, MaxRawReceiveBufferRequestBytes} {
		if err := ValidateRawReceiveBufferRequest(v); err != nil {
			t.Fatalf("request %d rejected: %v", v, err)
		}
	}
	for _, v := range []int{-1, MaxRawReceiveBufferRequestBytes + 1} {
		if err := ValidateRawReceiveBufferRequest(v); err == nil {
			t.Fatalf("invalid request %d accepted", v)
		}
	}
	inherited := rawReceiveBufferStatus(0, 212992)
	if !inherited.Inherited || inherited.Limited || inherited.ExpectedEffectiveBytes != 0 {
		t.Fatalf("inherited status=%+v", inherited)
	}
	limited := rawReceiveBufferStatus(512<<10, 425984)
	if limited.Inherited || !limited.Limited || limited.ExpectedEffectiveBytes != 1<<20 {
		t.Fatalf("limited status=%+v", limited)
	}
	full := rawReceiveBufferStatus(512<<10, 1<<20)
	if full.Inherited || full.Limited || full.ExpectedEffectiveBytes != 1<<20 {
		t.Fatalf("full status=%+v", full)
	}
}

func TestRawReceiveBufferKernelReadbackOnSocket(t *testing.T) {
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_DGRAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if fds[0] >= 0 { _ = unix.Close(fds[0]) }
		if fds[1] >= 0 { _ = unix.Close(fds[1]) }
	}()

	inherited, err := configureRawReceiveBuffer(fds[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := unix.GetsockoptInt(fds[0], unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil || inherited.EffectiveBytes != actual || !inherited.Inherited {
		t.Fatalf("inherited=%+v actual=%d err=%v", inherited, actual, err)
	}

	const requested = 64 << 10
	configured, err := configureRawReceiveBuffer(fds[0], requested)
	if err != nil {
		t.Fatal(err)
	}
	actual, err = unix.GetsockoptInt(fds[0], unix.SOL_SOCKET, unix.SO_RCVBUF)
	if err != nil {
		t.Fatal(err)
	}
	if configured.RequestedBytes != requested || configured.EffectiveBytes != actual ||
		configured.Limited != (actual < requested*2) {
		t.Fatalf("configured=%+v actual=%d", configured, actual)
	}

	if _, err := configureRawReceiveBuffer(fds[0], -1); err == nil {
		t.Fatal("negative request accepted")
	}
	if err := unix.Close(fds[0]); err != nil { t.Fatal(err) }
	fds[0] = -1
	if _, err := configureRawReceiveBuffer(fds[0], requested); err == nil {
		t.Fatal("closed fd set failure was not surfaced")
	}
}
