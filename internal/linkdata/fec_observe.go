package linkdata

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

// FECObserveStats is diagnostic-only state for the live LINK-embedded FEC path.
// It is read-only with respect to admission, reconstruction and wire behavior.
type FECObserveStats struct {
	Decoder                   fec.DecoderPressureStats   `json:"decoder"`
	PeakInFlight              int                        `json:"peak_in_flight"`
	PeakRetired               int                        `json:"peak_retired"`
	PeakRetiredIncomplete     int                        `json:"peak_retired_incomplete"`
	PeakRetiredMissingSources int                        `json:"peak_retired_missing_sources"`
	PressureRetireEvents      uint64                     `json:"pressure_retire_events"`
	PressureDetailScans       uint64                     `json:"pressure_detail_scans"`
	ReconstructCalls          uint64                     `json:"reconstruct_calls"`
	ReconstructSuccess        uint64                     `json:"reconstruct_success"`
	ReconstructMissingSources uint64                     `json:"reconstruct_missing_sources"`
	BlockSizeHistogram        [fec.DataShards + 1]uint64 `json:"block_size_histogram"`
}

type observedDecoderCodec struct {
	inner   fec.Codec
	mu      sync.Mutex
	calls   uint64
	success uint64
	missing uint64
}

func (c *observedDecoderCodec) Encode(shards [][]byte) error { return c.inner.Encode(shards) }
func (c *observedDecoderCodec) Reconstruct(shards [][]byte, present []bool) error {
	missing := 0
	for i := 0; i < fec.DataShards && i < len(present); i++ {
		if !present[i] {
			missing++
		}
	}
	c.mu.Lock()
	c.calls++
	c.missing += uint64(missing)
	c.mu.Unlock()
	err := c.inner.Reconstruct(shards, present)
	if err == nil {
		c.mu.Lock()
		c.success++
		c.mu.Unlock()
	}
	return err
}
func (c *observedDecoderCodec) snapshot() (uint64, uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.success, c.missing
}

type fecObserveState struct {
	codec                  *observedDecoderCodec
	peakInFlight           int
	peakRetired            int
	peakRetiredIncomplete  int
	peakRetiredMissing     int
	lastRetired            int
	retireEvents           uint64
	pressureDetailScans    uint64
	hist                    [fec.DataShards + 1]uint64
	lastReport              time.Time
}

var fecObserve sync.Map // map[*Path]*fecObserveState

func installFECObserver(p *Path, codec fec.Codec) fec.Codec {
	o := &observedDecoderCodec{inner: codec}
	fecObserve.Store(p, &fecObserveState{codec: o})
	return o
}

func observeFECWire(p *Path, h fec.BlockHeader) {
	v, ok := fecObserve.Load(p)
	if !ok {
		return
	}
	s := v.(*fecObserveState)
	// The first parity shard is emitted exactly once when a block closes.
	if int(h.ShardIndex) == fec.DataShards {
		n := int(h.DataCount)
		if n >= 1 && n <= fec.DataShards {
			s.hist[n]++
		}
	}
}

func observeFECDecoder(p *Path, now time.Time) {
	v, ok := fecObserve.Load(p)
	if !ok || p.dec == nil {
		return
	}
	s := v.(*fecObserveState)

	// The hot path only samples constant-time occupancy counts. The old code
	// walked every heavy and retired block for every received FEC datagram.
	c := p.dec.PressureCounts()
	if c.InFlight > s.peakInFlight {
		s.peakInFlight = c.InFlight
	}
	if c.Retired > s.peakRetired {
		s.peakRetired = c.Retired
	}

	retiredIncreased := c.Retired > s.lastRetired
	if retiredIncreased {
		s.retireEvents += uint64(c.Retired - s.lastRetired)
	}
	s.lastRetired = c.Retired

	reportDue := s.lastReport.IsZero() || now.Sub(s.lastReport) >= time.Second
	if !retiredIncreased && !reportDue {
		return
	}

	// Detailed missing-source accounting is needed when compact state grows so
	// its peak remains exact for normal pre-cap operation, and at the one-second
	// report cadence. It is no longer a per-datagram scan.
	d := p.dec.PressureStats()
	s.pressureDetailScans++
	if d.RetiredIncomplete > s.peakRetiredIncomplete {
		s.peakRetiredIncomplete = d.RetiredIncomplete
	}
	if d.RetiredMissingSources > s.peakRetiredMissing {
		s.peakRetiredMissing = d.RetiredMissingSources
	}

	if reportDue {
		s.lastReport = now
		b, _ := json.Marshal(p.fecObserveStats(d, s))
		fmt.Printf("WBD_LINK_FEC_DIAG %s\n", b)
	}
}

func (p *Path) fecObserveStats(d fec.DecoderPressureStats, s *fecObserveState) FECObserveStats {
	out := FECObserveStats{Decoder: d}
	if s == nil {
		return out
	}
	out.PeakInFlight = s.peakInFlight
	out.PeakRetired = s.peakRetired
	out.PeakRetiredIncomplete = s.peakRetiredIncomplete
	out.PeakRetiredMissingSources = s.peakRetiredMissing
	out.PressureRetireEvents = s.retireEvents
	out.PressureDetailScans = s.pressureDetailScans
	out.BlockSizeHistogram = s.hist
	out.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources = s.codec.snapshot()
	return out
}

func (p *Path) FECObserveStats() FECObserveStats {
	out := FECObserveStats{}
	if p == nil || p.dec == nil {
		return out
	}
	d := p.dec.PressureStats()
	v, ok := fecObserve.Load(p)
	if !ok {
		out.Decoder = d
		return out
	}
	s := v.(*fecObserveState)
	s.pressureDetailScans++
	if d.RetiredIncomplete > s.peakRetiredIncomplete {
		s.peakRetiredIncomplete = d.RetiredIncomplete
	}
	if d.RetiredMissingSources > s.peakRetiredMissing {
		s.peakRetiredMissing = d.RetiredMissingSources
	}
	return p.fecObserveStats(d, s)
}
