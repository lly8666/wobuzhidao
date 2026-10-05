//go:build windows

package faketcp

import (
	"testing"
	"unsafe"
)

func TestNpcapSendQueueWindowsABI(t *testing.T) {
	var q npcapSendQueue
	var h pcapHeader
	if unsafe.Offsetof(q.Len) != 4 || unsafe.Offsetof(q.Buffer) != 8 || unsafe.Sizeof(h) != 16 {
		t.Fatal("Npcap native queue/header ABI mismatch")
	}
	if unsafe.Sizeof(q) != 8+unsafe.Sizeof(q.Buffer) {
		t.Fatal("native queue size mismatch")
	}
}
