#!/usr/bin/env python3
import sys
from pathlib import Path

def one(s, old, new, label):
    n=s.count(old)
    if n!=1: raise SystemExit(f"{label}: expected 1, got {n}")
    return s.replace(old,new,1)

root=Path(sys.argv[1]); p=root/'cmd/wbd-link-server-mux/main.go'; s=p.read_text()
s=one(s,'\t"net"\n\t"os"\n','\t"net"\n\t"net/http"\n\t_ "net/http/pprof"\n\t"os"\n\t"runtime"\n','imports')
s=one(s,'\tflag.Parse()\n\n\ts, err := newServer(c)\n','''\tflag.Parse()

\t// Test-only diagnostics; no socket sizing, FEC or scheduling decision reads these settings.
\truntime.SetMutexProfileFraction(5)
\truntime.SetBlockProfileRate(1_000_000)
\tgo func() {
\t\tfmt.Println("WBD_LINK_PPROF_READY addr=127.0.0.1:6060 mutex_fraction=5 block_rate_ns=1000000")
\t\tif err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
\t\t\tfmt.Fprintf(os.Stderr, "WBD_LINK_PPROF_FAIL err=%v\\n", err)
\t\t}
\t}()

\ts, err := newServer(c)
''','pprof startup')
s=one(s,'func (s *server) Run(ctx context.Context) error {\n\tbuf := make([]byte, 65535)\n\tfor {\n','''func (s *server) Run(ctx context.Context) error {
\tbuf := make([]byte, 65535)
\trxWindowStart := time.Now()
\tvar rxWindowPackets, rxWindowBytes uint64
\tfor {
''','rx state')
s=one(s,'\t\tn, from, err := s.conn.ReadFromUDP(buf)\n\t\tnow := time.Now()\n\t\tif err != nil {\n','''\t\tn, from, err := s.conn.ReadFromUDP(buf)
\t\tnow := time.Now()
\t\tif err == nil {
\t\t\trxWindowPackets++
\t\t\trxWindowBytes += uint64(n)
\t\t}
\t\tif elapsed := now.Sub(rxWindowStart); elapsed >= time.Second {
\t\t\tfmt.Printf("WBD_LINK_RX_DIAG epoch_unix_nano=%d interval_ms=%.3f packets=%d bytes=%d pps=%.3f mbps=%.6f\\n",
\t\t\t\tnow.UnixNano(), float64(elapsed)/float64(time.Millisecond), rxWindowPackets, rxWindowBytes,
\t\t\t\tfloat64(rxWindowPackets)/elapsed.Seconds(), float64(rxWindowBytes)*8/elapsed.Seconds()/1e6)
\t\t\trxWindowStart = now
\t\t\trxWindowPackets, rxWindowBytes = 0, 0
\t\t}
\t\tif err != nil {
''','rx accounting')
p.write_text(s)
print('WBD_DIAGNOSTIC_PATCH link_server_pprof=1 rx_per_second=1 behavior_change=none')
