package linkdata

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
)

func TestWeak15ImmutablePathIsAdmittedAndEmitsTenRepairShards(t *testing.T) {
	cfg := control.LinkConfig{FECMode: control.FECFixed, Scheduler: control.FECSchedulerTailRS, DataShards: 20, ParityShards: 10, LaneCount: 1, FlushMillis: 8, MTU: 1400}
	p, err := New(cfg, 8)
	if err != nil {
		t.Fatal(err)
	}
	var repair uint64
	now := time.Unix(2, 0)
	for i := 0; i < 20; i++ {
		if _, err := p.Encode([]byte{byte(i + 1)}, now); err != nil {
			t.Fatal(err)
		}
	}
	repair = p.Stats().FECRepairTXPackets
	if repair != 10 {
		t.Fatalf("repair=%d want=10", repair)
	}
}
