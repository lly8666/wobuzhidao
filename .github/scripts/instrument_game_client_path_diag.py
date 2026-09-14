#!/usr/bin/env python3
from pathlib import Path
import sys

product = Path(sys.argv[1])
p = product / "cmd/wbd-game-lane-client/main.go"
s = p.read_text()

old = '''\tlogicalTX   uint64\n\tdelivered   uint64\n\tduplicate   uint64\n\tstale       uint64\n\tlaneFail    uint64\n\tdormantDrop uint64\n}'''
new = '''\tlogicalTX   uint64\n\tdelivered   uint64\n\tduplicate   uint64\n\tstale       uint64\n\tlaneFail    uint64\n\tdormantDrop uint64\n\n\tdiagAppRXPackets uint64\n\tdiagAppRXBytes   uint64\n\tdiagLaneTXBytes  uint64\n\tdiagLaneRXBytes  uint64\n\tdiagLaneWriteErr uint64\n\tdiagAppTXPackets uint64\n\tdiagAppTXBytes   uint64\n\tdiagAppWriteErr  uint64\n}'''
if s.count(old) != 1:
    raise SystemExit(f"client struct marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\tgo func() { errCh <- c.appLoop() }()\n\tif c.control != nil { go func() { errCh <- c.controlLoop() }() }\n'''
new = '''\tgo func() { errCh <- c.appLoop() }()\n\tif c.control != nil { go func() { errCh <- c.controlLoop() }() }\n\tgo c.diagPathLoop()\n'''
if s.count(old) != 1:
    raise SystemExit(f"goroutine marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tif !c.acceptPeer(from) { continue }\n\t\tc.activity.mark(time.Now())'''
new = '''\t\tif !c.acceptPeer(from) { continue }\n\t\tif n >= 4 && string(buf[:4]) == "WBD1" {\n\t\t\tatomic.AddUint64(&c.diagAppRXPackets, 1)\n\t\t\tatomic.AddUint64(&c.diagAppRXBytes, uint64(n))\n\t\t}\n\t\tc.activity.mark(time.Now())'''
if s.count(old) != 1:
    raise SystemExit(f"app rx marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\t\tfor _, lane := range targets {\n\t\t\t\tif _, err := lane.conn.Write(copy.Wire); err != nil { c.failLane(lane, fmt.Errorf("write: %w", err)); continue }\n\t\t\t\tatomic.AddUint64(&lane.tx, 1)\n\t\t\t}'''
new = '''\t\t\tfor _, lane := range targets {\n\t\t\t\tif _, err := lane.conn.Write(copy.Wire); err != nil {\n\t\t\t\t\tatomic.AddUint64(&c.diagLaneWriteErr, 1)\n\t\t\t\t\tc.failLane(lane, fmt.Errorf("write: %w", err)); continue\n\t\t\t\t}\n\t\t\t\tatomic.AddUint64(&lane.tx, 1)\n\t\t\t\tatomic.AddUint64(&c.diagLaneTXBytes, uint64(len(copy.Wire)))\n\t\t\t}'''
if s.count(old) != 1:
    raise SystemExit(f"lane tx marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tatomic.AddUint64(&lane.rx, 1)\n\t\tif handled, controlErr := c.handleLaneMembershipControl(lane, buf[:n]); handled {'''
new = '''\t\tatomic.AddUint64(&lane.rx, 1)\n\t\tatomic.AddUint64(&c.diagLaneRXBytes, uint64(n))\n\t\tif handled, controlErr := c.handleLaneMembershipControl(lane, buf[:n]); handled {'''
if s.count(old) != 1:
    raise SystemExit(f"lane rx marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

# Patch the application delivery write and accounting with independent, stable
# anchors.  Keeping the write line and delivered counter as separate markers
# avoids coupling this diagnostic patch to gofmt/adjacent-line drift while
# still requiring each baseline semantic anchor to occur exactly once.
old = '''\t\tif _, err := c.app.WriteToUDP(result.Payload, peer); err != nil { return err }'''
new = '''\t\tif _, err := c.app.WriteToUDP(result.Payload, peer); err != nil {\n\t\t\tatomic.AddUint64(&c.diagAppWriteErr, 1)\n\t\t\treturn err\n\t\t}\n\t\tif len(result.Payload) >= 4 && string(result.Payload[:4]) == "WBD1" {\n\t\t\tatomic.AddUint64(&c.diagAppTXPackets, 1)\n\t\t\tatomic.AddUint64(&c.diagAppTXBytes, uint64(len(result.Payload)))\n\t\t}'''
if s.count(old) != 1:
    raise SystemExit(f"app write marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

old = '''\t\tatomic.AddUint64(&c.delivered, 1)'''
if s.count(old) != 1:
    raise SystemExit(f"delivered marker drift: {s.count(old)}")
# The delivered counter remains in place; the exact-once check above protects
# the intended delivery site without making the write patch depend on adjacency.

insert_before = '''func (c *client) appLoop() error {'''
diag = '''func (c *client) diagPathLoop() {\n\tt := time.NewTicker(time.Second)\n\tdefer t.Stop()\n\tfor now := range t.C {\n\t\tfmt.Printf("WBD_GAME_PATH_DIAG epoch_ns=%d app_rx_packets=%d app_rx_bytes=%d lane_tx_bytes=%d lane_rx_bytes=%d lane_write_err=%d app_tx_packets=%d app_tx_bytes=%d app_write_err=%d\\n",\n\t\t\tnow.UnixNano(),\n\t\t\tatomic.LoadUint64(&c.diagAppRXPackets), atomic.LoadUint64(&c.diagAppRXBytes),\n\t\t\tatomic.LoadUint64(&c.diagLaneTXBytes), atomic.LoadUint64(&c.diagLaneRXBytes),\n\t\t\tatomic.LoadUint64(&c.diagLaneWriteErr), atomic.LoadUint64(&c.diagAppTXPackets),\n\t\t\tatomic.LoadUint64(&c.diagAppTXBytes), atomic.LoadUint64(&c.diagAppWriteErr))\n\t}\n}\n\n'''
if s.count(insert_before) != 1:
    raise SystemExit(f"diag insertion marker drift: {s.count(insert_before)}")
s = s.replace(insert_before, diag + insert_before, 1)

old = '''\tfmt.Printf("WBD_GAME_LANE_CLIENT_STATS logical_tx=%d delivered=%d duplicate=%d stale=%d lane_fail=%d dormant_drop=%d\\n",\n\t\tatomic.LoadUint64(&c.logicalTX), atomic.LoadUint64(&c.delivered), atomic.LoadUint64(&c.duplicate), atomic.LoadUint64(&c.stale), atomic.LoadUint64(&c.laneFail), atomic.LoadUint64(&c.dormantDrop))'''
new = '''\tfmt.Printf("WBD_GAME_LANE_CLIENT_STATS logical_tx=%d delivered=%d duplicate=%d stale=%d lane_fail=%d dormant_drop=%d\\n",\n\t\tatomic.LoadUint64(&c.logicalTX), atomic.LoadUint64(&c.delivered), atomic.LoadUint64(&c.duplicate), atomic.LoadUint64(&c.stale), atomic.LoadUint64(&c.laneFail), atomic.LoadUint64(&c.dormantDrop))\n\tfmt.Printf("WBD_GAME_PATH_FINAL app_rx_packets=%d app_rx_bytes=%d lane_tx_bytes=%d lane_rx_bytes=%d lane_write_err=%d app_tx_packets=%d app_tx_bytes=%d app_write_err=%d\\n",\n\t\tatomic.LoadUint64(&c.diagAppRXPackets), atomic.LoadUint64(&c.diagAppRXBytes),\n\t\tatomic.LoadUint64(&c.diagLaneTXBytes), atomic.LoadUint64(&c.diagLaneRXBytes),\n\t\tatomic.LoadUint64(&c.diagLaneWriteErr), atomic.LoadUint64(&c.diagAppTXPackets),\n\t\tatomic.LoadUint64(&c.diagAppTXBytes), atomic.LoadUint64(&c.diagAppWriteErr))'''
if s.count(old) != 1:
    raise SystemExit(f"final stats marker drift: {s.count(old)}")
s = s.replace(old, new, 1)

p.write_text(s)
print("WBD_GAME_CLIENT_PATH_DIAG_PATCHED interval_sec=1 per_packet_logging=0 final_exact=1 stable_delivery_anchor=1")
