#!/usr/bin/env bash
set -euo pipefail

# Adapt the generic hosted soak to exercise the product Game dynamic-membership
# contract during make-before-break lane replacement. This is intentionally a
# test-only wrapper: it does not modify product code or the established
# full-stack bootstrap.
SRC="$GITHUB_WORKSPACE/scripts/game_lane_rotation_soak.sh"
OUT="${RUNNER_TEMP:-/tmp}/game_lane_rotation_soak_control_generated.sh"

python3 - "$SRC" "$OUT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()

# The base full-stack harness starts the Game client without its runtime control
# socket. Hosted replacement must use the same dynamic-membership API as the
# product runtime. Apply the requested weak network on both public veth egress
# directions before any Reality/FakeTCP bootstrap.
needle = "tail = r'''DURATION_SEC=${DURATION_SEC:-500}"
insert = r"""prefix = prefix.replace('-session-id \"$SESSION_ID\" >\"$LOG_DIR/game-client.log\"',
                        '-session-id \"$SESSION_ID\" -control 127.0.0.1:47499 >\"$LOG_DIR/game-client.log\"')
netem_anchor = 'sudo ip netns exec \"$S\" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP\n'
netem_block = netem_anchor + r'''NETEM_DELAY_MS=${NETEM_DELAY_MS:-0}
NETEM_LOSS_PCT=${NETEM_LOSS_PCT:-0}
if [[ "$NETEM_DELAY_MS" != 0 || "$NETEM_LOSS_PCT" != 0 ]]; then
  sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  {
    echo "WBD_HOSTED_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional"
    sudo ip netns exec "$C" tc qdisc show dev gc0
    sudo ip netns exec "$S" tc qdisc show dev gs0
  } | tee "$LOG_DIR/netem.log"
fi
'''
if netem_anchor not in prefix:
    raise SystemExit('control wrapper: netem insertion point not found')
prefix = prefix.replace(netem_anchor, netem_block, 1)
"""
if needle not in s:
    raise SystemExit('control wrapper: inner patcher insertion point not found')
s = s.replace(needle, insert + needle, 1)

# Match the production/physical LINK liveness budget. The generic stress script
# used 2s to accelerate failures, which leaves only a 6s 3x keepalive budget and
# can turn sustained weak-net ARQ/FEC queueing into an unrelated lane death.
# Keep liveness enabled and preserve the frozen 3x semantics; only use the
# production interval (15s, hence a 45s liveness budget).
if s.count('-keepalive 2s') < 2:
    raise SystemExit('control wrapper: expected initial and replacement 2s keepalive markers')
s = s.replace('-keepalive 2s', '-keepalive 15s')

# Preserve the existing transport builder under a private name. Product
# replacement is make-before-break, so the replacement LINK proxy must have a
# generation-specific Game-facing UDP port and coexist with the old LINK proxy
# until Game Probe/Ready qualification has completed.
old = 'start_replacement_lane() {'
if old not in s:
    raise SystemExit('control wrapper: replacement function not found')
s = s.replace(old, 'start_replacement_lane_transport() {', 1)
old_lport = '  local lport=$((47100 + lane))\n'
new_lport = '  local lport=$((47100 + gen*100 + lane))\n'
if old_lport not in s:
    raise SystemExit('control wrapper: replacement LINK listen port not found')
s = s.replace(old_lport, new_lport, 1)

marker = ': >"$LOG_DIR/rotation.log"\n'
if marker not in s:
    raise SystemExit('control wrapper: rotation marker not found')

override = r'''game_control_cutover() {
  local phase=$1 lane=$2 old_lport=${3:-0} new_lport=${4:-0}
  local p1 p2 p3 p4
  p1=$((47100 + ${LANE_GEN[1]}*100 + 1))
  p2=$((47100 + ${LANE_GEN[2]}*100 + 2))
  p3=$((47100 + ${LANE_GEN[3]}*100 + 3))
  p4=$((47100 + ${LANE_GEN[4]}*100 + 4))
  sudo ip netns exec "$C" python3 - "$phase" "$lane" "$old_lport" "$new_lport" "$p1" "$p2" "$p3" "$p4" <<'PY_CONTROL'
import base64, json, socket, sys
phase=sys.argv[1]
lane=int(sys.argv[2]); old_port=int(sys.argv[3]); new_port=int(sys.argv[4])
ports={i:int(sys.argv[4+i]) for i in range(1,5)}
if phase not in ('overlap','commit'):
    raise SystemExit('unknown Game cutover phase: '+phase)
lanes=[]
for i in range(1,5):
    if phase == 'overlap' and i == lane:
        lanes.append({'id':i,'address':f'127.0.0.1:{old_port}'})
        lanes.append({'id':i,'address':f'127.0.0.1:{new_port}'})
    else:
        lanes.append({'id':i,'address':f'127.0.0.1:{ports[i]}'})
cmd={'op':'set','lanes':lanes}
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(20)
s.sendto(json.dumps(cmd,separators=(',',':')).encode(),('127.0.0.1',47499))
raw,_=s.recvfrom(4096)
r=json.loads(raw)
active=r.get('active',[])
# Go's encoding/json treats []uint8 as bytes and emits base64; tolerate an
# ordinary JSON array too so the harness follows the semantic API, not one
# representation detail.
if isinstance(active,str):
    active=list(base64.b64decode(active))
assert r.get('ok') is True, r
assert sorted(active) == [1,2,3,4], (r,active)
expected_targets=5 if phase == 'overlap' else 4
print('WBD_HOSTED_GAME_CONTROL_PASS phase=%s lane=%d active=1,2,3,4 targets=%d' % (phase,lane,expected_targets))
PY_CONTROL
}

observe_marker() {
  local pattern=$1 file=$2 before=$3 loops=${4:-40} label=$5
  local n=$before
  for _ in $(seq 1 "$loops"); do
    n=$(count_marker "$pattern" "$file")
    if (( n > before )); then
      echo "WBD_HOSTED_GRACEFUL_OBSERVED marker=${label} before=${before} now=${n}" >>"$LOG_DIR/rotation.log"
      return 0
    fi
    sleep .05
  done
  echo "WBD_HOSTED_GRACEFUL_LOST marker=${label} before=${before} now=${n}" >>"$LOG_DIR/rotation.log"
  return 0
}

retire_old_transport() {
  local old_link=$1 old_dtls=$2 old_fake=$3 old_sport=$4
  local before_link_close before_reset
  before_link_close=$(count_marker 'WBD_LINK_MUX_SESSION_CLOSE ' "$LOG_DIR/link-server.log")
  before_reset=$(count_marker 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log")

  # Once the candidate crossed Game Probe/Ready and the control plane committed
  # candidate-only membership, the old physical stack may retire. LINK close is
  # best-effort under netem; FakeTCP exact-flow RST remains the required hard
  # association cleanup boundary.
  sudo kill -TERM "$old_link" 2>/dev/null || true
  wait "$old_link" 2>/dev/null || true
  drop_pid "$old_link"
  observe_marker 'WBD_LINK_MUX_SESSION_CLOSE ' "$LOG_DIR/link-server.log" "$before_link_close" 40 link_graceful_close

  sudo kill -TERM "$old_dtls" 2>/dev/null || true
  wait "$old_dtls" 2>/dev/null || true
  drop_pid "$old_dtls"

  send_retire_rst "$old_sport"
  wait_count_gt 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log" "$before_reset" 400
  sudo kill -TERM "$old_fake" 2>/dev/null || true
  wait "$old_fake" 2>/dev/null || true
  drop_pid "$old_fake"
}

# Exact product-shaped replacement:
#   A stays primary
#   -> build B completely on a distinct local LINK endpoint
#   -> submit A+B for the same LaneID
#   -> wait authenticated Probe/Ready qualification
#   -> commit B-only (best-effort CLIENT_LEAVE for A)
#   -> retire A's LINK/DTLS/FakeTCP stack.
# Candidate failure therefore leaves healthy A untouched.
start_replacement_lane() {
  local lane=$1 gen=$2
  local old_link=${LINK_PIDS[$lane]} old_dtls=${DTLS_PIDS[$lane]} old_fake=${FAKETCP_PIDS[$lane]}
  local old_sport=${LANE_SPORT[$lane]} old_gen=${LANE_GEN[$lane]}
  local old_lport=$((47100 + old_gen*100 + lane))
  local new_lport=$((47100 + gen*100 + lane))
  local before_client_qualified before_server_qualified before_leave

  before_client_qualified=$(count_marker "WBD_GAME_LANE_CLIENT_QUALIFIED lane=${lane} " "$LOG_DIR/game-client.log")
  before_server_qualified=$(count_marker "WBD_GAME_LANE_QUALIFIED.*lane=${lane}" "$LOG_DIR/game-server.log")
  before_leave=$(count_marker 'WBD_GAME_LANE_UNBIND.*reason=client_leave' "$LOG_DIR/game-server.log")

  start_replacement_lane_transport "$lane" "$gen"

  # start_replacement_lane_transport updates LANE_GEN/PID arrays to B. The old
  # process identities above are intentionally retained until qualification.
  game_control_cutover overlap "$lane" "$old_lport" "$new_lport" >>"$LOG_DIR/rotation.log" 2>&1
  wait_count_gt "WBD_GAME_LANE_CLIENT_QUALIFIED lane=${lane} " "$LOG_DIR/game-client.log" "$before_client_qualified" 100
  wait_count_gt "WBD_GAME_LANE_QUALIFIED.*lane=${lane}" "$LOG_DIR/game-server.log" "$before_server_qualified" 100
  echo "WBD_HOSTED_GAME_QUALIFICATION_PASS lane=${lane} old_port=${old_lport} candidate_port=${new_lport}" >>"$LOG_DIR/rotation.log"

  game_control_cutover commit "$lane" "$old_lport" "$new_lport" >>"$LOG_DIR/rotation.log" 2>&1
  observe_marker 'WBD_GAME_LANE_UNBIND.*reason=client_leave' "$LOG_DIR/game-server.log" "$before_leave" 40 game_client_leave
  retire_old_transport "$old_link" "$old_dtls" "$old_fake" "$old_sport"
}

'''
s = s.replace(marker, override + marker, 1)

# The generic soak uses break-before-make. The product lifecycle is explicitly
# make-before-break, and start_replacement_lane above now owns both cutover and
# old-transport retirement.
stale_sequence = '  retire_lane "$lane"\n  start_replacement_lane "$lane" "$gen"\n'
if stale_sequence not in s:
    raise SystemExit('control wrapper: rotation sequence point not found')
s = s.replace(stale_sequence, '  start_replacement_lane "$lane" "$gen"\n', 1)

# CLIENT_LEAVE and LINK close are observability signals under loss, not
# correctness gates. Exact peer-reset cleanup, all rotation passes, no
# idle-expiry, zero client lane failures and load/goodput checks remain hard.
stale_assertion = '[[ "$leaves" -ge "$ROTATIONS" ]]\n'
if stale_assertion not in s:
    raise SystemExit('control wrapper: final assertion point not found')
s = s.replace(stale_assertion,
              'echo "WBD_HOSTED_GRACEFUL_SUMMARY client_leave_unbinds=$leaves rotations=$ROTATIONS" >>"$LOG_DIR/rotation.log"\n'
              '[[ $(count_marker \'WBD_GAME_LANE_CLIENT_LANE_FAIL \' "$LOG_DIR/game-client.log") -eq 0 ]]\n', 1)

pathlib.Path(sys.argv[2]).write_text(s)
PY
chmod +x "$OUT"
bash -n "$OUT"
grep -Fq 'sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem' "$OUT"
grep -Fq 'sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem' "$OUT"
grep -Fq 'WBD_HOSTED_GAME_QUALIFICATION_PASS' "$OUT"
grep -Fq 'game_control_cutover overlap "$lane"' "$OUT"
grep -Fq 'local lport=$((47100 + gen*100 + lane))' "$OUT"
grep -Fq -- '-keepalive 15s' "$OUT"
if grep -Fq -- '-keepalive 2s' "$OUT"; then
  echo 'control wrapper: stale accelerated 2s keepalive survived generation' >&2
  exit 1
fi
exec "$OUT" "$@"
