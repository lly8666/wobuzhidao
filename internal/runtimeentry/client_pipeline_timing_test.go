package runtimeentry

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/runtimeowner"
)

func TestClientPipelineTimingOptInKeepsHandlerErrors(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		name := "off"
		if enabled {
			name = "on"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client := &TunnelClient{cfg: TunnelClientConfig{ObserveTiming: enabled}}
			reads := 0
			lane := &clientLifecycleLane{
				attached: true,
				io: SegmentIO{Read: func() (faketcp.Segment, error) {
					reads++
					if reads == 1 {
						return faketcp.Segment{}, nil
					}
					cancel()
					return faketcp.Segment{}, io.EOF
				}},
			}
			// A missing Runtime is a controlled handler error. Timing must not
			// consume it or change the existing lifecycle error accounting.
			client.clientLaneReadLoop(ctx, lane)
			stats := client.LifecycleStats()
			if stats.RetryableErrors != 1 || stats.LastError != runtimeowner.ErrRuntimeClosed.Error() {
				t.Fatalf("handler error changed: %+v", stats)
			}
			if reads != 2 || lane.failed {
				t.Fatalf("cancelled read/error flow changed: reads=%d failed=%v", reads, lane.failed)
			}
			client.lifecycleTick(time.Now())
			pipeline := client.pipeline.snapshot()
			wantSamples := uint64(0)
			if enabled {
				wantSamples = 1
			}
			for field, value := range map[string]DurationDiagnostic{
				"process": pipeline.Process, "state_lock_wait": pipeline.StateLockWait,
				"handler": pipeline.Handler, "tick": pipeline.Tick,
				"runtime_tick": pipeline.RuntimeTick, "retire": pipeline.Retire, "schedule": pipeline.Schedule,
			} {
				if value.Samples != wantSamples || value.MaxNS > value.TotalNS {
					t.Fatalf("%s counter=%+v want samples=%d", field, value, wantSamples)
				}
			}
			if enabled && (pipeline.Process.TotalNS < pipeline.Handler.TotalNS ||
				pipeline.Tick.TotalNS != pipeline.RuntimeTick.TotalNS+pipeline.Retire.TotalNS+pipeline.Schedule.TotalNS) {
				t.Fatalf("timing boundaries disagree: %+v", pipeline)
			}
			snapshot := client.DiagnosticSnapshot(time.Now())
			if (snapshot.ClientPipeline != nil) != enabled {
				t.Fatalf("opt-in diagnostic presence changed: %+v", snapshot.ClientPipeline)
			}
			encoded, err := json.Marshal(snapshot)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			_, hasField := fields["client_pipeline"]
			if hasField != enabled {
				t.Fatalf("default JSON must omit client_pipeline: enabled=%v JSON=%s", enabled, encoded)
			}
		})
	}
}

func TestClientPipelineTimingClosedConsumerDoesNotHandlePacket(t *testing.T) {
	client := &TunnelClient{cfg: TunnelClientConfig{ObserveTiming: true}, closed: true}
	lane := &clientLifecycleLane{io: SegmentIO{Read: func() (faketcp.Segment, error) {
		return faketcp.Segment{}, nil
	}}}
	// No association/runtime is needed: closed still exits before handling.
	client.clientLaneReadLoop(context.Background(), lane)
	pipeline := client.pipeline.snapshot()
	if pipeline.Process.Samples != 1 || pipeline.StateLockWait.Samples != 1 || pipeline.Handler.Samples != 0 {
		t.Fatalf("closed consumer crossed handler boundary: %+v", pipeline)
	}
}

func TestClientPipelineDurationBucketsRemainBoundedCounters(t *testing.T) {
	var pipeline clientPipelineTiming
	for _, elapsed := range []time.Duration{77 * time.Microsecond, 3 * time.Millisecond, 20 * time.Millisecond} {
		pipeline.handler.observe(elapsed)
	}
	snapshot := pipeline.snapshot().Handler
	if snapshot.Samples != 3 || snapshot.TotalNS != 23077000 || snapshot.MaxNS != 20000000 ||
		snapshot.Over50US != 3 || snapshot.Over1MS != 2 || snapshot.Over10MS != 1 {
		t.Fatalf("duration aggregate=%+v", snapshot)
	}
}
