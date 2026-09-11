#!/usr/bin/env python3
from pathlib import Path
import sys

root = Path(sys.argv[1]) if len(sys.argv) > 1 else Path('.')

p = root / 'internal/linkdata/path.go'
s = p.read_text()
old = '''\tFECRepairTXPackets     uint64
\tFECRepairTXBytes       uint64

\tWireRXPackets uint64
\tWireRXBytes   uint64
\tInnerRXPackets uint64
\tInnerRXBytes   uint64
'''
new = '''\tFECRepairTXPackets     uint64
\tFECRepairTXBytes       uint64

\tWireRXPackets          uint64
\tWireRXBytes            uint64
\tFECSystematicRXPackets uint64
\tFECSystematicRXBytes   uint64
\tFECRepairRXPackets     uint64
\tFECRepairRXBytes       uint64
\tInnerRXPackets         uint64
\tInnerRXBytes           uint64
'''
if s.count(old) != 1:
    raise SystemExit('PathStats RX field marker drift')
s = s.replace(old, new, 1)
old = '''\tif !p.FECEnabled() {
\t\tif len(wire) > int(p.config.MTU) {
\t\t\treturn nil, fec.ErrPacketTooLarge
\t\t}
\t\tpackets = [][]byte{wire}
\t} else {
\t\tpackets, _, err = p.dec.Add(wire)
'''
new = '''\tif !p.FECEnabled() {
\t\tif len(wire) > int(p.config.MTU) {
\t\t\treturn nil, fec.ErrPacketTooLarge
\t\t}
\t\tpackets = [][]byte{wire}
\t} else {
\t\tif len(wire) >= fec.HeaderSize {
\t\t\tif h, parseErr := fec.ParseBlockHeader(wire[:fec.HeaderSize]); parseErr == nil {
\t\t\t\tif int(h.ShardIndex) < fec.DataShards {
\t\t\t\t\tp.stats.FECSystematicRXPackets++
\t\t\t\t\tp.stats.FECSystematicRXBytes += uint64(len(wire))
\t\t\t\t} else {
\t\t\t\t\tp.stats.FECRepairRXPackets++
\t\t\t\t\tp.stats.FECRepairRXBytes += uint64(len(wire))
\t\t\t\t}
\t\t\t}
\t\t}
\t\tpackets, _, err = p.dec.Add(wire)
'''
if s.count(old) != 1:
    raise SystemExit('Decode FEC marker drift')
s = s.replace(old, new, 1)
p.write_text(s)

p = root / 'cmd/wbd-link-server-mux/main.go'
s = p.read_text()
old = '''\tif ps.active {
\t\tif flush {
\t\t\tif peerKey, wire, err := s.plane.Flush(ps.id); err == nil && peerKey == ps.key {
\t\t\t\t_ = sendWire(s.conn, ps.peer, wire)
\t\t\t}
\t\t}
\t\ts.plane.Remove(ps.id)
\t}
'''
new = '''\tif ps.active {
\t\tif st, err := s.plane.Stats(ps.id); err == nil {
\t\t\tfmt.Printf("WBD_LINK_PATH_STATS tunnel_id_prefix=%s wire_rx=%d wire_rx_bytes=%d systematic_rx=%d systematic_rx_bytes=%d repair_rx=%d repair_rx_bytes=%d inner_rx=%d inner_rx_bytes=%d wire_tx=%d systematic_tx=%d repair_tx=%d\\n", printableSID(ps), st.WireRXPackets, st.WireRXBytes, st.FECSystematicRXPackets, st.FECSystematicRXBytes, st.FECRepairRXPackets, st.FECRepairRXBytes, st.InnerRXPackets, st.InnerRXBytes, st.WireTXPackets, st.FECSystematicTXPackets, st.FECRepairTXPackets)
\t\t}
\t\tif flush {
\t\t\tif peerKey, wire, err := s.plane.Flush(ps.id); err == nil && peerKey == ps.key {
\t\t\t\t_ = sendWire(s.conn, ps.peer, wire)
\t\t\t}
\t\t}
\t\ts.plane.Remove(ps.id)
\t}
'''
if s.count(old) != 1:
    raise SystemExit('removePeer stats marker drift')
s = s.replace(old, new, 1)
p.write_text(s)
