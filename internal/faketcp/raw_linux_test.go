//go:build linux

package faketcp

import (
	"os"
	"syscall"
	"testing"
)

func TestRawIOInterruptedOnlyRetriesEINTR(t *testing.T) {
	if !rawIOInterrupted(syscall.EINTR) {
		t.Fatal("plain EINTR must be retryable")
	}
	if !rawIOInterrupted(&os.SyscallError{Syscall: "recvfrom", Err: syscall.EINTR}) {
		t.Fatal("wrapped EINTR must be retryable")
	}
	for _, err := range []error{syscall.EAGAIN, syscall.EWOULDBLOCK, syscall.EBADF, syscall.ENETDOWN} {
		if rawIOInterrupted(err) {
			t.Fatalf("unexpected retryable raw I/O error: %v", err)
		}
	}
}
