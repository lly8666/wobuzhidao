package windowsruntime

import (
	"testing"
	"time"
)

func TestIdleTimeoutZeroNeverExpiresPayloadObservation(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	observation := newPayloadIdleObservation(base)
	if observation.expired(base.Add(24*time.Hour), 0) {
		t.Fatal("idle_timeout=0 unexpectedly expired payload observation")
	}
}

func TestIdleTimeoutZeroIsValidAndNegativeIsRejected(t *testing.T) {
	if err := validateIdleTimeoutSeconds(0); err != nil {
		t.Fatalf("idle_timeout=0 must disable idle teardown: %v", err)
	}
	if err := validateIdleTimeoutSeconds(1); err != nil {
		t.Fatalf("positive idle timeout must remain a finite valid lease: %v", err)
	}
	if err := validateIdleTimeoutSeconds(-1); err == nil {
		t.Fatal("negative idle timeout unexpectedly accepted")
	}
}
