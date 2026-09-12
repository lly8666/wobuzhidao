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
	Decoder                   fec.DecoderPressureStats    `json:"decoder"`
	PeakInFlight              int                         `json:"peak_in_flight"`
	PeakRetired               int                         `json:"peak_retired"`
	PeakRetiredIncomplete     int                         `json:"peak_retired_incomplete"`
	PeakRetiredMissingSources int                         `json:"peak_retired_missing_sources"`
	PressureRetireEvents      uint64                      `json:"pressure_retire_events"`
	ReconstructCalls          uint64                      `json:"reconstruct_calls"`
	ReconstructSuccess        uint64                      `json:"reconstruct_success"`
	ReconstructMissingSources uint64                      `json:"reconstruct_missing_sources"`
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
	codec              *observedDecoderCodec
	peakInFlight       int
	peakRetired        int
	peakRetiredIncomplete int
	peakRetiredMissing int
	lastRetired        int
	retireEvents       uint64
	hist               [fec.DataShards + 1]uint64
	lastReport         time.Time
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
	d := p.dec.PressureStats()
	if d.InFlight > s.peakInFlight {
		s.peakInFlight = d.InFlight
	}
	if d.Retired > s.peakRetired {
		s.peakRetired = d.Retired
	}
	if d.RetiredIncomplete > s.peakRetiredIncomplete {
		s.peakRetiredIncomplete = d.RetiredIncomplete
	}
	if d.RetiredMissingSources > s.peakRetiredMissing {
		s.peakRetiredMissing = d.RetiredMissingSources
	}
	if d.Retired > s.lastRetired {
		s.retireEvents += uint64(d.Retired - s.lastRetired)
	}
	s.lastRetired = d.Retired
	if s.lastReport.IsZero() || now.Sub(s.lastReport) >= time.Second {
		s.lastReport = now
		b, _ := json.Marshal(p.FECObserveStats())
		fmt.Printf("WBD_LINK_FEC_DIAG %s\n", b)
	}
}

func (p *Path) FECObserveStats() FECObserveStats {
	out := FECObserveStats{}
	if p == nil || p.dec == nil {
		return out
	}
	out.Decoder = p.dec.PressureStats()
	v, ok := fecObserve.Load(p)
	if !ok {
		return out
	}
	s := v.(*fecObserveState)
	out.PeakInFlight = s.peakInFlight
	out.PeakRetired = s.peakRetired
	out.PeakRetiredIncomplete = s.peakRetiredIncomplete
	out.PeakRetiredMissingSources = s.peakRetiredMissing
	out.PressureRetireEvents = s.retireEvents
	out.BlockSizeHistogram = s.hist
	out.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources = s.codec.snapshot()
	return out
}
