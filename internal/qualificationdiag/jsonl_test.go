package qualificationdiag

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestObserverReportsSnapshotDelayWithoutLosingScheduledTimestamp(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	path := filepath.Join(t.TempDir(), "observer.jsonl")
	var scheduled time.Time
	err := Run(ctx, path, time.Hour, func(now time.Time) any {
		scheduled = now
		time.Sleep(20 * time.Millisecond)
		cancel()
		return "collected"
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var sample Sample
	if err := json.Unmarshal(data, &sample); err != nil {
		t.Fatal(err)
	}
	if sample.UnixNS != scheduled.UnixNano() || sample.ObservedUnixNS < sample.UnixNS || sample.MemoryCollectNS < 0 || sample.ProductCollectNS < int64(15*time.Millisecond) || sample.Product != "collected" {
		t.Fatalf("observer failed to account for delayed snapshot: %+v", sample)
	}
}
