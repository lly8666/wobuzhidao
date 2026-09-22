package qualificationdiag

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"runtime"
	"time"
)

var ErrInvalidConfig = errors.New("qualificationdiag: invalid config")

type RuntimeStats struct {
	NumCPU        int     `json:"num_cpu"`
	GOMAXPROCS    int     `json:"gomaxprocs"`
	Goroutines    int     `json:"goroutines"`
	HeapAlloc     uint64  `json:"heap_alloc"`
	HeapInuse     uint64  `json:"heap_inuse"`
	HeapObjects   uint64  `json:"heap_objects"`
	TotalAlloc    uint64  `json:"total_alloc"`
	Mallocs       uint64  `json:"mallocs"`
	Frees         uint64  `json:"frees"`
	NextGC        uint64  `json:"next_gc"`
	NumGC         uint32  `json:"num_gc"`
	PauseTotalNs  uint64  `json:"pause_total_ns"`
	GCCPUFraction float64 `json:"gc_cpu_fraction"`
}

type Sample struct {
	Schema  int          `json:"schema"`
	UnixNS  int64        `json:"unix_ns"`
	Runtime RuntimeStats `json:"runtime"`
	Product any          `json:"product"`
}

func Run(ctx context.Context, path string, interval time.Duration, snapshot func(time.Time) any) error {
	if ctx == nil || path == "" || interval <= 0 || snapshot == nil {
		return ErrInvalidConfig
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriterSize(f, 64<<10)
	defer w.Flush()
	enc := json.NewEncoder(w)

	write := func(now time.Time) error {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)
		sample := Sample{
			Schema: 1,
			UnixNS: now.UnixNano(),
			Runtime: RuntimeStats{
				NumCPU: runtime.NumCPU(),
				GOMAXPROCS: runtime.GOMAXPROCS(0),
				Goroutines: runtime.NumGoroutine(),
				HeapAlloc: mem.HeapAlloc,
				HeapInuse: mem.HeapInuse,
				HeapObjects: mem.HeapObjects,
				TotalAlloc: mem.TotalAlloc,
				Mallocs: mem.Mallocs,
				Frees: mem.Frees,
				NextGC: mem.NextGC,
				NumGC: mem.NumGC,
				PauseTotalNs: mem.PauseTotalNs,
				GCCPUFraction: mem.GCCPUFraction,
			},
			Product: snapshot(now),
		}
		if err := enc.Encode(sample); err != nil {
			return err
		}
		return w.Flush()
	}

	if err := write(time.Now()); err != nil {
		return err
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case now := <-ticker.C:
			if err := write(now); err != nil {
				return err
			}
		}
	}
}
