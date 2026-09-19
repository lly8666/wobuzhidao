package gamelane

import (
	"testing"
	"time"
)

func TestInnerPacerSerializesLogicalBytes(t *testing.T) {
	p, err := NewInnerPacer(8) // 1 MB/s
	if err != nil { t.Fatal(err) }
	now := time.Unix(100, 0)
	if got := p.Reserve(1000, now); got != 0 {
		t.Fatalf("first wait=%s", got)
	}
	if got := p.Reserve(1000, now); got != time.Millisecond {
		t.Fatalf("second wait=%s", got)
	}
	if got := p.Reserve(1000, now.Add(500*time.Microsecond)); got != 1500*time.Microsecond {
		t.Fatalf("third wait=%s", got)
	}
}

func TestInnerPacerZeroIsUnlimitedAndCanHotUpdate(t *testing.T) {
	p, err := NewInnerPacer(0)
	if err != nil { t.Fatal(err) }
	now := time.Unix(200, 0)
	if got := p.Reserve(64000, now); got != 0 { t.Fatalf("unlimited wait=%s", got) }
	if err := p.SetMbps(16, now); err != nil { t.Fatal(err) }
	if got := p.Reserve(2000, now); got != 0 { t.Fatalf("first paced wait=%s", got) }
	if got := p.Reserve(2000, now); got != time.Millisecond { t.Fatalf("paced wait=%s", got) }
	if err := p.SetMbps(0, now.Add(time.Second)); err != nil { t.Fatal(err) }
	if got := p.Reserve(64000, now.Add(time.Second)); got != 0 { t.Fatalf("disabled wait=%s", got) }
}

func TestInnerPacerRejectsNegativeRate(t *testing.T) {
	if _, err := NewInnerPacer(-1); err == nil { t.Fatal("expected error") }
}
