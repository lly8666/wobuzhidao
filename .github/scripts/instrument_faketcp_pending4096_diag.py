#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 2:
    raise SystemExit('usage: instrument_faketcp_pending4096_diag.py PRODUCT_DIR')
root = Path(sys.argv[1])
p = root / 'internal/faketcp/arq.go'
s = p.read_text()

def repl(old, new, label):
    global s
    n = s.count(old)
    if n != 1:
        raise SystemExit(f'pending4096 diag {label} marker drift: {n}')
    s = s.replace(old, new, 1)

repl('''\tShadowRetransmitBytes uint64
}\n''', '''\tShadowRetransmitBytes uint64

\t// Diagnostic-only visibility into the sparse pending index. These counters
\t// do not participate in admission, ACK, retransmission, or compaction.
\tPendingBackingRecords    int
\tPendingHead              int
\tPendingBySeqRecords      int
\tMaxPendingBackingRecords int
\tMaxPendingHead           int
\tPendingCompactions       uint64
\tPending4096Compactions   uint64
}\n''', 'stats fields')

repl('''\trepairRemainder   uint64
\tfreeSlabs         [][]byte
\tstats             SenderStats
}\n''', '''\trepairRemainder   uint64
\tfreeSlabs         [][]byte
\tstats             SenderStats

\t// Diagnostic-only high-water marks/counters for the 4096 sparse-index
\t// compaction threshold in advanceHead.
\tmaxPendingBacking      int
\tmaxPendingHead         int
\tpendingCompactions     uint64
\tpending4096Compactions uint64
}\n''', 'sender fields')

repl('''func (s *Sender) Stats() SenderStats {
\tstats := s.stats
\tstats.RepairCreditBytes = s.repairCredit
\treturn stats
}\n''', '''func (s *Sender) Stats() SenderStats {
\tstats := s.stats
\tstats.RepairCreditBytes = s.repairCredit
\tstats.PendingBackingRecords = len(s.pending)
\tstats.PendingHead = s.head
\tstats.PendingBySeqRecords = len(s.bySeq)
\tstats.MaxPendingBackingRecords = s.maxPendingBacking
\tstats.MaxPendingHead = s.maxPendingHead
\tstats.PendingCompactions = s.pendingCompactions
\tstats.Pending4096Compactions = s.pending4096Compactions
\treturn stats
}\n''', 'stats snapshot')

repl('''\ts.pending = append(s.pending, p)
\ts.bySeq[p.Seq] = p
''', '''\ts.pending = append(s.pending, p)
\tif len(s.pending) > s.maxPendingBacking {
\t\ts.maxPendingBacking = len(s.pending)
\t}
\ts.bySeq[p.Seq] = p
''', 'enqueue high water')

repl('''func (s *Sender) advanceHead() {
\tfor s.head < len(s.pending) && s.pending[s.head] == nil {
\t\ts.head++
\t}
\tif s.head >= 4096 && s.head*2 >= len(s.pending) {
''', '''func (s *Sender) advanceHead() {
\tfor s.head < len(s.pending) && s.pending[s.head] == nil {
\t\ts.head++
\t}
\tif s.head > s.maxPendingHead {
\t\ts.maxPendingHead = s.head
\t}
\tif s.head >= 4096 && s.head*2 >= len(s.pending) {
\t\ts.pendingCompactions++
\t\ts.pending4096Compactions++
''', '4096 compaction counter')

repl('''\tif len(s.pending) >= 2*MaxSteadyStateTrackedRecords && len(s.bySeq)*2 <= len(s.pending) {
\t\tn := 0
''', '''\tif len(s.pending) >= 2*MaxSteadyStateTrackedRecords && len(s.bySeq)*2 <= len(s.pending) {
\t\ts.pendingCompactions++
\t\tn := 0
''', 'sparse compaction counter')

p.write_text(s)
print('WBD_TEST_FAKETCP_PENDING4096_DIAG_PATCHED', p)
