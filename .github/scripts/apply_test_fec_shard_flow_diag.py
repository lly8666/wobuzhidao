#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 2:
    raise SystemExit("usage: apply_test_fec_shard_flow_diag.py PRODUCT_DIR")
root = Path(sys.argv[1])
obs = root / "internal/linkdata/fec_observe.go"
path = root / "internal/linkdata/path.go"
o = obs.read_text()
p = path.read_text()

def repl(text, old, new, label):
    n = text.count(old)
    if n != 1:
        raise SystemExit(f"shard-flow diag {label} marker drift: {n}")
    return text.replace(old, new, 1)

o = repl(o, '''import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
''', '''import (
	"encoding/json"
	"fmt"
	"math/bits"
	"sync"
	"time"
''', 'imports')

o = repl(o, '''	BlockSizeHistogram        [fec.DataShards + 1]uint64 `json:"block_size_histogram"`
}
''', '''	BlockSizeHistogram        [fec.DataShards + 1]uint64 `json:"block_size_histogram"`

	TXSystematicShards        uint64 `json:"tx_systematic_shards"`
	TXParityShards            uint64 `json:"tx_parity_shards"`
	RXSystematicShards        uint64 `json:"rx_systematic_shards"`
	RXParityShards            uint64 `json:"rx_parity_shards"`
	RXUniqueSystematicShards  uint64 `json:"rx_unique_systematic_shards"`
	RXUniqueParityShards      uint64 `json:"rx_unique_parity_shards"`
	RXDuplicateShards         uint64 `json:"rx_duplicate_shards"`
	RXObservedBlocks          int    `json:"rx_observed_blocks"`
	RXHorizonSettledBlocks    int    `json:"rx_horizon_settled_blocks"`
	RXHorizonNoFinalMetadata  int    `json:"rx_horizon_no_final_metadata"`
	RXHorizonUnderRequired    int    `json:"rx_horizon_under_required"`
	RXHorizonMissingRequired  int    `json:"rx_horizon_missing_required"`
}
''', 'stats fields')

o = repl(o, '''	hist               [fec.DataShards + 1]uint64
	lastReport         time.Time
}
''', '''	hist               [fec.DataShards + 1]uint64
	lastReport         time.Time

	flowMu              sync.Mutex
	txSystematic        uint64
	txParity            uint64
	rxSystematic        uint64
	rxParity            uint64
	rxUniqueSystematic  uint64
	rxUniqueParity      uint64
	rxDuplicate         uint64
	rxMasks             map[uint32]uint64
	rxDataCount         map[uint32]uint8
	latestRXBlock       uint32
}
''', 'observer state')

o = repl(o, '''	fecObserve.Store(p, &fecObserveState{codec: o})
''', '''	fecObserve.Store(p, &fecObserveState{
		codec: o,
		rxMasks: make(map[uint32]uint64),
		rxDataCount: make(map[uint32]uint8),
	})
''', 'observer init')

old_wire = '''func observeFECWire(p *Path, h fec.BlockHeader) {
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
'''
new_wire = '''func observeFECWire(p *Path, h fec.BlockHeader) {
	v, ok := fecObserve.Load(p)
	if !ok {
		return
	}
	s := v.(*fecObserveState)
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	if int(h.ShardIndex) < fec.DataShards {
		s.txSystematic++
	} else {
		s.txParity++
	}
	// The first parity shard is emitted exactly once when a block closes.
	if int(h.ShardIndex) == fec.DataShards {
		n := int(h.DataCount)
		if n >= 1 && n <= fec.DataShards {
			s.hist[n]++
		}
	}
}

func observeFECWireRX(p *Path, h fec.BlockHeader) {
	v, ok := fecObserve.Load(p)
	if !ok {
		return
	}
	s := v.(*fecObserveState)
	idx := int(h.ShardIndex)
	if idx < 0 || idx >= fec.TotalShards {
		return
	}
	s.flowMu.Lock()
	defer s.flowMu.Unlock()
	if idx < fec.DataShards {
		s.rxSystematic++
	} else {
		s.rxParity++
		s.rxDataCount[h.BlockID] = h.DataCount
	}
	bit := uint64(1) << uint(idx)
	mask := s.rxMasks[h.BlockID]
	if mask&bit != 0 {
		s.rxDuplicate++
	} else {
		if idx < fec.DataShards {
			s.rxUniqueSystematic++
		} else {
			s.rxUniqueParity++
		}
		s.rxMasks[h.BlockID] = mask | bit
	}
	if h.BlockID > s.latestRXBlock {
		s.latestRXBlock = h.BlockID
	}
}
'''
o = repl(o, old_wire, new_wire, 'wire observers')

o = repl(o, '''	out.PressureRetireEvents = s.retireEvents
	out.BlockSizeHistogram = s.hist
	out.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources = s.codec.snapshot()
	return out
}
''', '''	out.PressureRetireEvents = s.retireEvents
	out.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources = s.codec.snapshot()

	s.flowMu.Lock()
	out.BlockSizeHistogram = s.hist
	out.TXSystematicShards = s.txSystematic
	out.TXParityShards = s.txParity
	out.RXSystematicShards = s.rxSystematic
	out.RXParityShards = s.rxParity
	out.RXUniqueSystematicShards = s.rxUniqueSystematic
	out.RXUniqueParityShards = s.rxUniqueParity
	out.RXDuplicateShards = s.rxDuplicate
	out.RXObservedBlocks = len(s.rxMasks)
	latest := s.latestRXBlock
	horizon := uint32(out.Decoder.MaxBlocks)
	for id, mask := range s.rxMasks {
		if latest <= id || latest-id <= horizon {
			continue
		}
		out.RXHorizonSettledBlocks++
		dataCount, final := s.rxDataCount[id]
		if !final || dataCount == 0 || int(dataCount) > fec.DataShards {
			out.RXHorizonNoFinalMetadata++
			continue
		}
		received := bits.OnesCount64(mask)
		required := int(dataCount)
		if received < required {
			out.RXHorizonUnderRequired++
			out.RXHorizonMissingRequired += required - received
		}
	}
	s.flowMu.Unlock()
	return out
}
''', 'stats snapshot')

p = repl(p, '''	} else {
		packets, _, err = p.dec.Add(wire)
		observeFECDecoder(p, time.Now())
''', '''	} else {
		if len(wire) >= fec.HeaderSize {
			if h, parseErr := fec.ParseBlockHeader(wire[:fec.HeaderSize]); parseErr == nil {
				observeFECWireRX(p, h)
			}
		}
		packets, _, err = p.dec.Add(wire)
		observeFECDecoder(p, time.Now())
''', 'decode RX hook')

obs.write_text(o)
path.write_text(p)
print("WBD_TEST_FEC_SHARD_FLOW_DIAG_PATCHED", obs, path)
