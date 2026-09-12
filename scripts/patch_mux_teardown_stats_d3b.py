#!/usr/bin/env python3
from pathlib import Path
import sys

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path('.')
p = root / 'cmd/wbd-faketcp-mux/main_linux.go'
s = p.read_text()
old = '''\tif worker != nil {
\t\t_ = worker.Stop()
\t}
\ts.table.Remove(flow)
'''
new = '''\tif worker != nil {
\t\t_ = worker.Stop()
\t}
\tss := sess.assoc.SenderStats()
\trs := sess.assoc.ReceiverStats()
\tfmt.Printf("WBD_FAKETCP_MUX_STATS client=%d server=%d sender_enqueued=%d sender_acked=%d sender_sacked=%d sender_retired_sacked=%d sender_fast_retx=%d sender_rto_retx=%d sender_peak_pending=%d receiver_delivered=%d receiver_duplicates=%d receiver_out_of_order=%d receiver_late_below_ack=%d receiver_below_next_drops=%d receiver_buffered_duplicates=%d receiver_forgiven_gaps=%d receiver_forgiven_bytes=%d receiver_peak_buffered_oo=%d\\n", flow.ClientPort, flow.ServerPort, ss.Enqueued, ss.Acked, ss.SACKed, ss.RetiredSACKed, ss.FastRetransmits, ss.RTOTransmits, ss.PeakPending, rs.Delivered, rs.Duplicates, rs.OutOfOrder, rs.LateBelowACK, rs.BelowNextDrops, rs.BufferedDuplicates, rs.ForgivenGaps, rs.ForgivenBytes, rs.PeakBufferedOO)
\ts.table.Remove(flow)
'''
if s.count(old) != 1:
    raise SystemExit('server mux teardown marker drift')
s = s.replace(old, new, 1)
p.write_text(s)
