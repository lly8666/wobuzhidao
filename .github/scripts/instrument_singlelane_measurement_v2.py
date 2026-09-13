#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 3:
    raise SystemExit("usage: instrument_singlelane_measurement_v2.py HELPER_DIR PACED_LOAD_SCRIPT")
helper = Path(sys.argv[1])
load_src = Path(sys.argv[2]).read_text()
rotation = helper / "scripts/game_lane_rotation_soak.sh"
fullstack = helper / "scripts/game_lane_fullstack.sh"
s = rotation.read_text()

# The rotation harness owns the measured load tail.
start = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD\'\n'
i = s.find(start)
if i < 0:
    raise SystemExit("load heredoc start marker missing")
j = s.find('\nPY_LOAD\n', i + len(start))
if j < 0:
    raise SystemExit("load heredoc end marker missing")
replacement = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD_V2\'\n' + load_src.rstrip() + '\nPY_LOAD_V2\n'
s = s[:i] + replacement + s[j + len('\nPY_LOAD\n'):]

old = '''sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'''
new = '''date +%s%N >"$LOG_DIR/load-start-epoch-ns.txt"\nsudo ip netns exec "$C" env \\
  WBD_LOAD_MAX_PAYLOAD="$INNER_MTU" \\
  WBD_LOAD_PHASE_SPEC="${WBD_LOAD_PHASE_SPEC:-}" \\
  WBD_LOAD_DRAIN_SEC="${WBD_LOAD_DRAIN_SEC:-15}" \\
  WBD_LOAD_MAX_SEND_BURST="${WBD_LOAD_MAX_SEND_BURST:-64}" \\
  WBD_LOAD_MAX_RECV_BURST="${WBD_LOAD_MAX_RECV_BURST:-256}" \\
  python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'''
if s.count(old) != 1:
    raise SystemExit(f"load launch marker drift: {s.count(old)}")
s = s.replace(old, new, 1)
rotation.write_text(s)

# The rotation harness takes its process/topology prefix from game_lane_fullstack.sh
# at runtime, so the isolated server LINK socket knob belongs in that base script.
f = fullstack.read_text()
old = '''sudo ip netns exec "$S" "$ASSET_DIR/wbd-link-server-mux" \\
  -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} -mtu "$INNER_MTU" \\
  -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 60s -max-sessions 8 \\
  >"$LOG_DIR/link-server.log" 2>&1 &'''
new = '''sudo ip netns exec "$S" "$ASSET_DIR/wbd-link-server-mux" \\
  -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} -mtu "$INNER_MTU" \\
  -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 60s -max-sessions 8 \\
  -diag-rcvbuf-effective-bytes "${WBD_SERVER_LINK_RCVBUF_EFFECTIVE_BYTES:-0}" \\
  >"$LOG_DIR/link-server.log" 2>&1 &'''
if f.count(old) != 1:
    raise SystemExit(f"fullstack server mux launch marker drift: {f.count(old)}")
fullstack.write_text(f.replace(old, new, 1))

print("WBD_SINGLELANE_MEASUREMENT_V2_PATCHED actual_tx_timestamp=1 bounded_catchup=1 phase_actual_tx=1 isolated_link_rcvbuf=1 topology_base=fullstack timeline_epoch=1")
