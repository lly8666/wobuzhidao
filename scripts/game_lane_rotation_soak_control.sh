#!/usr/bin/env bash
set -euo pipefail

# Adapt the generic hosted soak to exercise the product Game dynamic-membership
# contract during lane replacement. This is intentionally a test-only wrapper:
# it does not modify product code or the established full-stack bootstrap.
SRC="$GITHUB_WORKSPACE/scripts/game_lane_rotation_soak.sh"
OUT="${RUNNER_TEMP:-/tmp}/game_lane_rotation_soak_control_generated.sh"

python3 - "$SRC" "$OUT" <<'PY'
import pathlib, sys
p = pathlib.Path(sys.argv[1])
s = p.read_text()

# The base full-stack harness starts the Game client without its runtime control
# socket. Hosted replacement must use the same dynamic-membership API as the
# product runtime, otherwise killing a LINK proxy is (correctly) classified as
# lane_fail instead of planned client_leave. Apply the requested weak network on
# both public veth egress directions before any Reality/FakeTCP bootstrap.
needle = "tail = r'''DURATION_SEC=${DURATION_SEC:-500}"
insert = r"""prefix = prefix.replace('-session-id \"$SESSION_ID\" >\"$LOG_DIR/game-client.log\"',
                        '-session-id \"$SESSION_ID\" -control 127.0.0.1:47499 >\"$LOG_DIR/game-client.log\"')
netem_anchor = 'sudo ip netns exec \"$S\" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP\n'
netem_block = netem_anchor + r'''NETEM_DELAY_MS=${NETEM_DELAY_MS:-0}
NETEM_LOSS_PCT=${NETEM_LOSS_PCT:-0}
if [[ \"$NETEM_DELAY_MS\" != 0 || \"$NETEM_LOSS_PCT\" != 0 ]]; then
  sudo ip netns exec \"$C\" tc qdisc replace dev gc0 root netem delay \"${NETEM_DELAY_MS}ms\" loss random \"${NETEM_LOSS_PCT}%\"
  sudo ip netns exec \"$S\" tc qdisc replace dev gs0 root netem delay \"${NETEM_DELAY_MS}ms\" loss random \"${NETEM_LOSS_PCT}%\"
  {
    echo \"WBD_HOSTED_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional\"
    sudo ip netns exec \"$C\" tc qdisc show dev gc0
    sudo ip netns exec \"$S\" tc qdisc show dev gs0
  } | tee \"$LOG_DIR/netem.log\"
fi
'''
if netem_anchor not in prefix:
    raise SystemExit('control wrapper: netem insertion point not found')
prefix = prefix.replace(netem_anchor, netem_block, 1)
"""
if needle not in s:
    raise SystemExit('control wrapper: inner patcher insertion point not found')
s = s.replace(needle, insert + needle, 1)

# Preserve the existing transport builder under a private name; a later wrapper
# adds the Game membership rejoin after the replacement transport is ready.
old = 'start_replacement_lane() {'
if old not in s:
    raise SystemExit('control wrapper: replacement function not found')
s = s.replace(old, 'start_replacement_lane_transport() {', 1)

marker = ': >"$LOG_DIR/rotation.log"\n'
if marker not in s:
    raise SystemExit('control wrapper: rotation marker not found')

override = r'''game_control_set() {
  local exclude=${1:-0}
  sudo ip netns exec "$C" python3 - "$exclude" <<'PY_CONTROL'
import json, socket, sys
exclude=int(sys.argv[1])
lanes=[{'id':i,'address':f'127.0.0.1:{47100+i}'} for i in range(1,5) if i != exclude]
cmd={'op':'set','lanes':lanes}
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(8)
s.sendto(json.dumps(cmd,separators=(',',':')).encode(),('127.0.0.1',47499))
raw,_=s.recvfrom(4096)
r=json.loads(raw)
expected=[i for i in range(1,5) if i != exclude]
assert r.get('ok') is True, r
assert sorted(r.get('active',[])) == expected, (r,expected)
print('WBD_HOSTED_GAME_CONTROL_PASS exclude=%d active=%s' % (exclude, ','.join(map(str,expected))))
PY_CONTROL
}

# Planned retirement is 4 -> 3 at the Game layer first. That sends the product
# LaneLeave over the still-working old lane, so server-side client_leave is
# observable before any transport is torn down. The remaining three lanes keep
# carrying the 1 Mbps logical stream during replacement.
retire_lane() {
  local lane=$1
  local old_link=${LINK_PIDS[$lane]} old_dtls=${DTLS_PIDS[$lane]} old_fake=${FAKETCP_PIDS[$lane]}
  local old_sport=${LANE_SPORT[$lane]}
  local before_leave before_link_close before_reset
  before_leave=$(count_marker 'WBD_GAME_LANE_UNBIND.*reason=client_leave' "$LOG_DIR/game-server.log")
  before_link_close=$(count_marker 'WBD_LINK_MUX_SESSION_CLOSE ' "$LOG_DIR/link-server.log")
  before_reset=$(count_marker 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log")

  game_control_set "$lane" >>"$LOG_DIR/rotation.log" 2>&1
  wait_count_gt 'WBD_GAME_LANE_UNBIND.*reason=client_leave' "$LOG_DIR/game-server.log" "$before_leave" 400

  sudo kill -TERM "$old_link" 2>/dev/null || true
  wait "$old_link" 2>/dev/null || true
  drop_pid "$old_link"
  wait_count_gt 'WBD_LINK_MUX_SESSION_CLOSE ' "$LOG_DIR/link-server.log" "$before_link_close" 400

  sudo kill -TERM "$old_dtls" 2>/dev/null || true
  wait "$old_dtls" 2>/dev/null || true
  drop_pid "$old_dtls"

  send_retire_rst "$old_sport"
  wait_count_gt 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log" "$before_reset" 400
  sudo kill -TERM "$old_fake" 2>/dev/null || true
  wait "$old_fake" 2>/dev/null || true
  drop_pid "$old_fake"
}

# Rebuild the complete Reality -> FakeTCP -> DTLS -> LINK transport first, then
# atomically add that logical lane back through the Game control socket. Wait for
# server-side Game bind before declaring the rotation complete.
start_replacement_lane() {
  local lane=$1 gen=$2
  local before_game_bind
  before_game_bind=$(count_marker "WBD_GAME_LANE_BIND.*lane=${lane}" "$LOG_DIR/game-server.log")
  start_replacement_lane_transport "$lane" "$gen"
  game_control_set 0 >>"$LOG_DIR/rotation.log" 2>&1
  wait_count_gt "WBD_GAME_LANE_BIND.*lane=${lane}" "$LOG_DIR/game-server.log" "$before_game_bind" 600
}

'''
s = s.replace(marker, override + marker, 1)

# A planned replacement must never degrade into the client's lane-failure path.
assertion = '[[ "$leaves" -ge "$ROTATIONS" ]]\n'
if assertion not in s:
    raise SystemExit('control wrapper: final assertion point not found')
s = s.replace(assertion, assertion + '[[ $(count_marker \'WBD_GAME_LANE_CLIENT_LANE_FAIL \' "$LOG_DIR/game-client.log") -eq 0 ]]\n', 1)

pathlib.Path(sys.argv[2]).write_text(s)
PY
chmod +x "$OUT"
bash -n "$OUT"
exec "$OUT" "$@"
