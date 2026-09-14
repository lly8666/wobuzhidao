#!/usr/bin/env python3
from pathlib import Path
import sys

if len(sys.argv) != 3:
    raise SystemExit("usage: instrument_singlelane_measurement_v2.py ROOT_DIR PACED_LOAD_SCRIPT")
root = Path(sys.argv[1])
load_src = Path(sys.argv[2]).read_text()
rotation = root / "scripts/game_lane_rotation_soak.sh"
fullstack = root / "scripts/game_lane_fullstack.sh"

# Patch the scripts that will actually be executed.  When this helper is first
# invoked against the patched-helper copy it also installs a hook into the A/B
# wrapper (below).  That hook invokes this same script again against PRODUCT_DIR
# after the historical realistic/transient instrumentation has run, so those
# later patches cannot overwrite the measured load or isolated LINK buffer flag.
if rotation.exists() and fullstack.exists():
    s = rotation.read_text()
    start = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD\'\n'
    measured_start = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD_V2\'\n'
    if measured_start not in s:
        i = s.find(start)
        if i < 0:
            raise SystemExit("load heredoc start marker missing")
        j = s.find('\nPY_LOAD\n', i + len(start))
        if j < 0:
            raise SystemExit("load heredoc end marker missing")
        replacement = measured_start + load_src.rstrip() + '\nPY_LOAD_V2\n'
        s = s[:i] + replacement + s[j + len('\nPY_LOAD\n'):]

    old = '''sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'''
    new = '''date +%s%N >"$LOG_DIR/load-start-epoch-ns.txt"\nsudo ip netns exec "$C" env \\
  WBD_LOAD_MAX_PAYLOAD="$INNER_MTU" \\
  WBD_LOAD_PHASE_SPEC="${WBD_LOAD_PHASE_SPEC:-}" \\
  WBD_LOAD_DRAIN_SEC="${WBD_LOAD_DRAIN_SEC:-15}" \\
  WBD_LOAD_MAX_SEND_BURST="${WBD_LOAD_MAX_SEND_BURST:-64}" \\
  WBD_LOAD_MAX_RECV_BURST="${WBD_LOAD_MAX_RECV_BURST:-256}" \\
  python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'''
    if new not in s:
        if s.count(old) != 1:
            raise SystemExit(f"load launch marker drift: {s.count(old)}")
        s = s.replace(old, new, 1)
    rotation.write_text(s)

    # The rotation harness takes its process/topology prefix from
    # game_lane_fullstack.sh at runtime, so the isolated server LINK socket knob
    # belongs in that base script.
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
    if new not in f:
        if f.count(old) != 1:
            raise SystemExit(f"fullstack server mux launch marker drift: {f.count(old)}")
        f = f.replace(old, new, 1)
    fullstack.write_text(f)

# The historical A/B wrapper instruments PRODUCT_DIR only after it starts.  A
# patch applied merely to the helper's scripts directory is therefore not enough.
# Install a final instrumentation hook into the wrapper so the measured load and
# selected :47000 buffer setting are applied to the real runtime product scripts
# after all legacy helper patches, immediately before bash -n / execution.
wrapper = root / ".github/scripts/run_singlelane_ab_observability.sh"
wrapper_hooked = False
if wrapper.exists():
    w = wrapper.read_text()
    hook_line = 'python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/instrument_singlelane_measurement_v2.py" "$PRODUCT_DIR" "${GITHUB_WORKSPACE:?}/suite/.github/scripts/paced_udp_load_v2.py"\\n'
    if hook_line not in w:
        marker = 'python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_transient_qdisc_change.py" "$PRODUCT_DIR"\\n\\n'
        if w.count(marker) != 1:
            raise SystemExit(f"A/B runtime measurement hook marker drift: {w.count(marker)}")
        w = w.replace(marker, marker[:-2] + hook_line + '\\n', 1)
        wrapper.write_text(w)
    wrapper_hooked = True

print(
    "WBD_SINGLELANE_MEASUREMENT_V2_PATCHED "
    "actual_tx_timestamp=1 bounded_catchup=1 phase_actual_tx=1 "
    "isolated_link_rcvbuf=1 topology_base=fullstack timeline_epoch=1 "
    f"runtime_product_hook={1 if wrapper_hooked else 0}"
)
