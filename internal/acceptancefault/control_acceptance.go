//go:build lifecycleacceptance

package acceptancefault

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

type rule struct {
	Kind  string `json:"kind"`
	Lane  uint8  `json:"lane"`
	Count int    `json:"count"`
}

type control struct {
	Epoch uint64 `json:"epoch"`
	Rules []rule `json:"rules"`
}

var state struct {
	sync.Mutex
	epoch     uint64
	remaining []int
}

func Consume(kind string, lane uint8) bool {
	path := os.Getenv("WBD_LIFECYCLE_ACCEPTANCE_CONTROL")
	if path == "" {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var cfg control
	if json.Unmarshal(data, &cfg) != nil || cfg.Epoch == 0 {
		return false
	}

	state.Lock()
	if state.epoch != cfg.Epoch || len(state.remaining) != len(cfg.Rules) {
		state.epoch = cfg.Epoch
		state.remaining = make([]int, len(cfg.Rules))
		for i, r := range cfg.Rules {
			if r.Count > 0 {
				state.remaining[i] = r.Count
			}
		}
	}
	matched := -1
	for i, r := range cfg.Rules {
		if state.remaining[i] > 0 && r.Kind == kind && (r.Lane == 0 || r.Lane == lane) {
			state.remaining[i]--
			matched = i
			break
		}
	}
	state.Unlock()
	if matched < 0 {
		return false
	}
	appendEvent(kind, lane, cfg.Epoch)
	return true
}

func appendEvent(kind string, lane uint8, epoch uint64) {
	path := os.Getenv("WBD_LIFECYCLE_ACCEPTANCE_EVENTS")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = json.NewEncoder(f).Encode(struct {
		UnixNS int64  `json:"unix_ns"`
		Kind   string `json:"kind"`
		Lane   uint8  `json:"lane"`
		Epoch  uint64 `json:"epoch"`
	}{UnixNS: time.Now().UnixNano(), Kind: kind, Lane: lane, Epoch: epoch})
}
