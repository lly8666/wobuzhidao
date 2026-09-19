package persona

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestWitnessRoundTripAndSingleUse(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1700000000, 123)
	id := WitnessFromClientHello([]byte("tls-clienthello"))
	if err := RecordWitness(dir, id, "Example.COM.", now); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Fatalf("dir mode=%o", st.Mode().Perm())
	}
	if err := ConsumeWitness(dir, id, "example.com", now.Add(2*time.Second), 5*time.Second); err != nil {
		t.Fatal(err)
	}
	if err := ConsumeWitness(dir, id, "example.com", now.Add(3*time.Second), 5*time.Second); !errors.Is(err, ErrWitnessMissing) {
		t.Fatalf("second consume=%v want missing", err)
	}
}

func TestWitnessExpiryConsumesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1700000000, 0)
	id := WitnessFromClientHello([]byte("old"))
	if err := RecordWitness(dir, id, "example.com", now); err != nil {
		t.Fatal(err)
	}
	if err := ConsumeWitness(dir, id, "example.com", now.Add(11*time.Second), 10*time.Second); !errors.Is(err, ErrWitnessExpired) {
		t.Fatalf("consume=%v want expired", err)
	}
	if err := ConsumeWitness(dir, id, "example.com", now.Add(12*time.Second), 10*time.Second); !errors.Is(err, ErrWitnessMissing) {
		t.Fatalf("expired witness must be one-shot removed, got %v", err)
	}
}

func TestWitnessTargetMismatchConsumesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Unix(1700000000, 0)
	id := WitnessFromClientHello([]byte("wrong-target"))
	if err := RecordWitness(dir, id, "a.example", now); err != nil {
		t.Fatal(err)
	}
	if err := ConsumeWitness(dir, id, "b.example", now.Add(time.Second), 10*time.Second); !errors.Is(err, ErrWitnessName) {
		t.Fatalf("consume=%v want target mismatch", err)
	}
	if err := ConsumeWitness(dir, id, "a.example", now.Add(2*time.Second), 10*time.Second); !errors.Is(err, ErrWitnessMissing) {
		t.Fatalf("mismatched witness must not be reusable, got %v", err)
	}
}

func TestWitnessHexParsing(t *testing.T) {
	id := WitnessFromClientHello([]byte("hello"))
	got, err := ParseWitnessHex(id.Hex())
	if err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("got=%x want=%x", got, id)
	}
	if _, err := ParseWitnessHex("abcd"); !errors.Is(err, ErrBadWitness) {
		t.Fatalf("short hex err=%v", err)
	}
}
