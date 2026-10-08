//go:build linux

package faketcp

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

func TestRawReceiveBufferScopedFallbackAndPermissionDegradation(t *testing.T) {
	for _, tc := range []struct {
		name string
		request, initial, final int
		forceErr error
		wantForce, wantLimited bool
	}{
		{"inherit", 0, 212992, 212992, nil, false, false},
		{"ordinary-sufficient", 524288, 1048576, 1048576, nil, false, false},
		{"privileged-scoped", 524288, 425984, 1048576, nil, true, false},
		{"permission-denied", 524288, 425984, 425984, unix.EPERM, true, true},
		{"unsupported", 524288, 425984, 425984, unix.ENOPROTOOPT, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var options []int
			reads := 0
			set := func(fd, level, option, value int) error {
				if fd != 17 || level != unix.SOL_SOCKET || value != tc.request { t.Fatal("wrong socket request") }
				options = append(options, option)
				if option == unix.SO_RCVBUFFORCE { return tc.forceErr }
				if option != unix.SO_RCVBUF { t.Fatal("unexpected socket option") }
				return nil
			}
			get := func(fd, level, option int) (int, error) {
				if fd != 17 || level != unix.SOL_SOCKET || option != unix.SO_RCVBUF { t.Fatal("wrong readback") }
				reads++
				if reads == 1 { return tc.initial, nil }
				return tc.final, nil
			}
			s, err := configureRawReceiveBufferWithOps(17, tc.request, set, get)
			if err != nil || s.EffectiveBytes != tc.final || s.ForceAttempted != tc.wantForce || s.Limited != tc.wantLimited || s.Forced != (tc.wantForce && tc.forceErr == nil) { t.Fatalf("status=%+v err=%v", s, err) }
			wantSets, wantReads := 0, 1
			if tc.request > 0 { wantSets++ }
			if tc.wantForce { wantSets++ }
			if s.Forced { wantReads++ }
			if len(options) != wantSets || reads != wantReads { t.Fatalf("unexpected syscall sequence %v reads%d", options, reads) }
			if (s.ForceError != "") != (tc.forceErr != nil) { t.Fatalf("missing degradation reason %+v", s) }
		})
	}
}

func TestRawReceiveBufferForcedReadbackFailureIsFatal(t *testing.T) {
	for _, result := range []struct { value int; err error }{{0, unix.EBADF}, {0, nil}} {
		reads := 0
		_, err := configureRawReceiveBufferWithOps(17, 524288,
			func(int, int, int, int) error { return nil },
			func(int, int, int) (int, error) { reads++; if reads==1 { return 425984,nil }; return result.value,result.err })
		if err == nil { t.Fatal("forced readback failure hidden") }
		if result.err != nil && !errors.Is(err,result.err) { t.Fatalf("readback error lost: %v",err) }
	}
}

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
