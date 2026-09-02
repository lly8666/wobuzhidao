package diag

import "testing"

func TestSessionIDStableAndShort(t *testing.T) {
	got := SessionID([]byte("0123456789abcdef0123456789abcdef"))
	if len(got) != 6 {
		t.Fatalf("SessionID length=%d want 6: %q", len(got), got)
	}
	if got != SessionID([]byte("0123456789abcdef0123456789abcdef")) {
		t.Fatal("SessionID is not stable")
	}
	if got == SessionID([]byte("different-session-material")) {
		t.Fatal("different material produced same short id in test vectors")
	}
}
