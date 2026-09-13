#!/usr/bin/env python3
import sys
from pathlib import Path

def one(s, old, new, label):
    n=s.count(old)
    if n!=1: raise SystemExit(f"{label}: expected 1, got {n}")
    return s.replace(old,new,1)

root=Path(sys.argv[1]); op=root/'internal/linkdata/fec_observe.go'; pp=root/'internal/linkdata/path.go'
s=op.read_text(); p=pp.read_text()
s=one(s,'\tReconstructMissingSources uint64                      `json:"reconstruct_missing_sources"`\n','\tReconstructMissingSources uint64                      `json:"reconstruct_missing_sources"`\n\tReconstructTotalNanos     uint64                      `json:"reconstruct_total_nanos"`\n\tReconstructMaxNanos       uint64                      `json:"reconstruct_max_nanos"`\n\tDecodeAddCalls            uint64                      `json:"decode_add_calls"`\n\tDecodeAddTotalNanos       uint64                      `json:"decode_add_total_nanos"`\n\tDecodeAddMaxNanos         uint64                      `json:"decode_add_max_nanos"`\n','stats')
s=one(s,'\tmissing uint64\n}\n','\tmissing uint64\n\ttotalNS uint64\n\tmaxNS   uint64\n}\n','codec fields')
old='''func (c *observedDecoderCodec) Reconstruct(shards [][]byte, present []bool) error {
\tmissing := 0
\tfor i := 0; i < fec.DataShards && i < len(present); i++ {
\t\tif !present[i] {
\t\t\tmissing++
\t\t}
\t}
\tc.mu.Lock()
\tc.calls++
\tc.missing += uint64(missing)
\tc.mu.Unlock()
\terr := c.inner.Reconstruct(shards, present)
\tif err == nil {
\t\tc.mu.Lock()
\t\tc.success++
\t\tc.mu.Unlock()
\t}
\treturn err
}
func (c *observedDecoderCodec) snapshot() (uint64, uint64, uint64) {
\tc.mu.Lock()
\tdefer c.mu.Unlock()
\treturn c.calls, c.success, c.missing
}
'''
new='''func (c *observedDecoderCodec) Reconstruct(shards [][]byte, present []bool) error {
\tmissing := 0
\tfor i := 0; i < fec.DataShards && i < len(present); i++ {
\t\tif !present[i] { missing++ }
\t}
\tstarted := time.Now()
\terr := c.inner.Reconstruct(shards, present)
\tns := uint64(time.Since(started))
\tc.mu.Lock()
\tc.calls++
\tc.missing += uint64(missing)
\tc.totalNS += ns
\tif ns > c.maxNS { c.maxNS = ns }
\tif err == nil { c.success++ }
\tc.mu.Unlock()
\treturn err
}
func (c *observedDecoderCodec) snapshot() (uint64, uint64, uint64, uint64, uint64) {
\tc.mu.Lock(); defer c.mu.Unlock()
\treturn c.calls, c.success, c.missing, c.totalNS, c.maxNS
}
'''
s=one(s,old,new,'reconstruct timing')
s=one(s,'\tlastReport         time.Time\n}\n','\tlastReport         time.Time\n\ttimingMu           sync.Mutex\n\tdecodeAddCalls     uint64\n\tdecodeAddTotalNS   uint64\n\tdecodeAddMaxNS     uint64\n}\n','decode fields')
s=one(s,'var fecObserve sync.Map // map[*Path]*fecObserveState\n\n','''var fecObserve sync.Map // map[*Path]*fecObserveState

func observeFECDecodeDuration(p *Path, elapsed time.Duration) {
\tv, ok := fecObserve.Load(p); if !ok { return }
\ts := v.(*fecObserveState); ns := uint64(elapsed)
\ts.timingMu.Lock(); defer s.timingMu.Unlock()
\ts.decodeAddCalls++; s.decodeAddTotalNS += ns
\tif ns > s.decodeAddMaxNS { s.decodeAddMaxNS = ns }
}

''','decode helper')
s=one(s,'\tout.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources = s.codec.snapshot()\n\treturn out\n','''\tout.ReconstructCalls, out.ReconstructSuccess, out.ReconstructMissingSources, out.ReconstructTotalNanos, out.ReconstructMaxNanos = s.codec.snapshot()
\ts.timingMu.Lock()
\tout.DecodeAddCalls, out.DecodeAddTotalNanos, out.DecodeAddMaxNanos = s.decodeAddCalls, s.decodeAddTotalNS, s.decodeAddMaxNS
\ts.timingMu.Unlock()
\treturn out
''','publish timings')
p=one(p,'\t} else {\n\t\tpackets, _, err = p.dec.Add(wire)\n\t\tobserveFECDecoder(p, time.Now())\n','\t} else {\n\t\tstarted := time.Now()\n\t\tpackets, _, err = p.dec.Add(wire)\n\t\tobserveFECDecodeDuration(p, time.Since(started))\n\t\tobserveFECDecoder(p, time.Now())\n','decode timing')
op.write_text(s); pp.write_text(p)
print('WBD_DIAGNOSTIC_PATCH fec_decode_timing=1 reconstruct_timing=1 behavior_change=none')
