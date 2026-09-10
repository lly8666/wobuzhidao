#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?usage: game_lane_realistic_analysis.sh ASSET_DIR [LOG_DIR]}
LOG_DIR=${2:-/tmp/wbd-realistic-analysis}
BASE="$GITHUB_WORKSPACE/scripts/game_lane_fullstack.sh"
PATCHED="${RUNNER_TEMP:-/tmp}/game_lane_realistic_generated.sh"
HELPER_DIR="$GITHUB_WORKSPACE/scripts"

python3 - "$BASE" "$PATCHED" "$HELPER_DIR" <<'PY_PATCHER'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
out = pathlib.Path(sys.argv[2])
helpers = pathlib.Path(sys.argv[3])
marker = 'cat >"$LOG_DIR/probe.py" <<\'PY\'\n'
pos = src.find(marker)
if pos < 0:
    raise SystemExit('realistic analysis: fullstack tail insertion point not found')
prefix = src[:pos]

# Exercise weak-network conditions during the real same-flow bootstrap too,
# rather than applying netem only after a clean startup.
prefix = prefix.replace('--bootstrap-timeout 12s', '--bootstrap-timeout 45s')
prefix = prefix.replace('--reality-timeout 12s', '--reality-timeout 45s')
link_old = '-demo-reality-ticket "$ticket" >"$LOG_DIR/link-${i}.log"'
if link_old not in prefix:
    raise SystemExit('realistic analysis: initial LINK launch marker not found')
prefix = prefix.replace(link_old, '-demo-reality-ticket "$ticket" -keepalive 15s >"$LOG_DIR/link-${i}.log"')

echo_old = "    if not (b.startswith(b'warm') or b.startswith(b'game')):\n"
if echo_old not in prefix:
    raise SystemExit('realistic analysis: echo guard marker not found')
prefix = prefix.replace(echo_old, "    if not (b.startswith(b'warm') or b.startswith(b'game') or b.startswith(b'WBDR')):\n", 1)

# The existing OUTPUT RST suppressors prevent the host kernel from rejecting
# the userspace FakeTCP lineage. Keep those first-match drops, then model a
# restrictive host firewall on the public interface: only the exact WBD raw
# service flow is admitted; unrelated IPv4 traffic on gc0/gs0 is dropped.
anchor = 'sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP\n'
if anchor not in prefix:
    raise SystemExit('realistic analysis: firewall insertion point not found')
block = anchor + r'''NETEM_DELAY_MS=${NETEM_DELAY_MS:-300}
NETEM_LOSS_PCT=${NETEM_LOSS_PCT:-20}
{
  echo "WBD_REALISTIC_ENV_SETUP firewall=public-default-drop exact_faketcp_allow=1 kernel_rst_suppress=1 delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT}"
  sudo ip netns exec "$C" iptables -A INPUT  -i gc0 -p tcp -s 10.89.0.1 -d 10.89.0.2 --sport 40000 -j ACCEPT
  sudo ip netns exec "$C" iptables -A INPUT  -i gc0 -j DROP
  sudo ip netns exec "$C" iptables -A OUTPUT -o gc0 -p tcp -s 10.89.0.2 -d 10.89.0.1 --dport 40000 -j ACCEPT
  sudo ip netns exec "$C" iptables -A OUTPUT -o gc0 -j DROP
  sudo ip netns exec "$S" iptables -A INPUT  -i gs0 -p tcp -s 10.89.0.2 -d 10.89.0.1 --dport 40000 -j ACCEPT
  sudo ip netns exec "$S" iptables -A INPUT  -i gs0 -j DROP
  sudo ip netns exec "$S" iptables -A OUTPUT -o gs0 -p tcp -s 10.89.0.1 -d 10.89.0.2 --sport 40000 -j ACCEPT
  sudo ip netns exec "$S" iptables -A OUTPUT -o gs0 -j DROP
  sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  echo '--- client filter ---'
  sudo ip netns exec "$C" iptables -L -n -v --line-numbers
  echo '--- server filter ---'
  sudo ip netns exec "$S" iptables -L -n -v --line-numbers
  echo '--- client qdisc ---'
  sudo ip netns exec "$C" tc -s qdisc show dev gc0
  echo '--- server qdisc ---'
  sudo ip netns exec "$S" tc -s qdisc show dev gs0
} >"$LOG_DIR/environment-setup.log" 2>&1
'''
prefix = prefix.replace(anchor, block, 1)

# Append an analysis-only load phase. No application-loss threshold appears
# here; corruption and duplicate delivery remain correctness failures.
tail = r'''
DURATION_SEC=${DURATION_SEC:-1800}
RATE_BPS=${RATE_BPS:-10000000}
PROFILE_SEED=${PROFILE_SEED:-20260910}
SMOKE_STRICT=${SMOKE_STRICT:-0}

case "$LANES" in 1|4) ;; *) echo "analysis LANES must be 1 or 4" >&2; exit 2;; esac
case "$FEC" in off|20:20) ;; *) echo "analysis FEC must be off or 20:20" >&2; exit 2;; esac
case "$DURATION_SEC" in 120|1800) ;; *) echo "analysis duration must be 120s smoke or 1800s long run" >&2; exit 2;; esac
case "$SMOKE_STRICT" in 0|1) ;; *) echo "analysis SMOKE_STRICT must be 0 or 1" >&2; exit 2;; esac
[[ "$RATE_BPS" == 10000000 ]] || { echo "analysis rate must be exactly 10000000 bps" >&2; exit 2; }
[[ "${NETEM_DELAY_MS}" == 300 && "${NETEM_LOSS_PCT}" == 20 ]] || { echo "analysis weaknet must be 300ms/20pct" >&2; exit 2; }
if [[ "$DURATION_SEC" == 120 && "$SMOKE_STRICT" != 1 ]]; then
  echo "120s analysis must use SMOKE_STRICT=1" >&2
  exit 2
fi

drop_pid() {
  local dead=$1 p
  local kept=()
  for p in "${PIDS[@]:-}"; do
    [[ -n "$p" && "$p" != "$dead" ]] && kept+=("$p")
  done
  PIDS=("${kept[@]:-}")
}

# The short bootstrap pcap proves flow lineage in dedicated gates. Keeping a
# sustained payload capture would create a huge artifact and perturb CPU/I/O,
# so measured-run wire cost is taken from public-interface counters instead.
if [[ -n "${TPID:-}" ]]; then
  old_tpid=$TPID
  sudo kill -INT "$old_tpid" 2>/dev/null || true
  wait "$old_tpid" 2>/dev/null || true
  drop_pid "$old_tpid"
  TPID=
  rm -f "$LOG_DIR/game-lanes.pcap"
fi

# One unmeasured packet forces all configured race lanes to bind before the
# network/resource baselines are taken.
sudo ip netns exec "$C" python3 - <<'PY_WARM'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47601)); s.settimeout(12)
p=b'WBDR-WARM'
s.sendto(p,('127.0.0.1',47500))
got,_=s.recvfrom(65535)
assert got==p,(got,p)
print('WBD_REALISTIC_WARM_PASS')
PY_WARM
for _ in $(seq 1 800); do
  n=$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log" 2>/dev/null || true)
  (( n >= LANES )) && break
  sleep .05
done
[[ "$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log" 2>/dev/null || true)" -eq "$LANES" ]]

net_snapshot() {
  local out=$1
  : >"$out"
  sudo ip netns exec "$C" python3 - "$out" client gc0 <<'PY_NET'
import pathlib,sys
out,prefix,dev=sys.argv[1:]
for line in pathlib.Path('/proc/net/dev').read_text().splitlines():
    if ':' not in line: continue
    name,vals=line.split(':',1)
    if name.strip()!=dev: continue
    v=list(map(int,vals.split()))
    keys=['rx_bytes','rx_packets','rx_errs','rx_dropped','rx_fifo','rx_frame','rx_compressed','rx_multicast','tx_bytes','tx_packets','tx_errs','tx_dropped','tx_fifo','tx_colls','tx_carrier','tx_compressed']
    with open(out,'a') as f:
        for k,x in zip(keys,v): f.write(f'{prefix}_{k}={x}\n')
    break
else: raise SystemExit(f'missing interface {dev}')
PY_NET
  sudo ip netns exec "$S" python3 - "$out" server gs0 <<'PY_NET'
import pathlib,sys
out,prefix,dev=sys.argv[1:]
for line in pathlib.Path('/proc/net/dev').read_text().splitlines():
    if ':' not in line: continue
    name,vals=line.split(':',1)
    if name.strip()!=dev: continue
    v=list(map(int,vals.split()))
    keys=['rx_bytes','rx_packets','rx_errs','rx_dropped','rx_fifo','rx_frame','rx_compressed','rx_multicast','tx_bytes','tx_packets','tx_errs','tx_dropped','tx_fifo','tx_colls','tx_carrier','tx_compressed']
    with open(out,'a') as f:
        for k,x in zip(keys,v): f.write(f'{prefix}_{k}={x}\n')
    break
else: raise SystemExit(f'missing interface {dev}')
PY_NET
}

net_snapshot "$LOG_DIR/net-before.txt"
STOP_FILE="$LOG_DIR/monitor.stop"
rm -f "$STOP_FILE"
python3 "$GITHUB_WORKSPACE/scripts/realistic_process_monitor.py" "$ASSET_DIR" "$LOG_DIR/process.csv" "$STOP_FILE" 2.0 >"$LOG_DIR/monitor.log" 2>&1 &
MON_PID=$!

set +e
sudo ip netns exec "$C" python3 "$GITHUB_WORKSPACE/scripts/realistic_game_load.py" \
  "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PROFILE_SEED" >"$LOG_DIR/load.log" 2>&1
LOAD_RC=$?
set -e
touch "$STOP_FILE"
wait "$MON_PID" || true
net_snapshot "$LOG_DIR/net-after.txt"
cat "$LOG_DIR/load.log"
[[ "$LOAD_RC" -eq 0 ]]

{
  echo '=== CLIENT FILTER FINAL ==='
  sudo ip netns exec "$C" iptables -L -n -v -x --line-numbers
  echo '=== SERVER FILTER FINAL ==='
  sudo ip netns exec "$S" iptables -L -n -v -x --line-numbers
} >"$LOG_DIR/firewall-final.txt" 2>&1
{
  echo '=== CLIENT NETEM FINAL ==='
  sudo ip netns exec "$C" tc -s qdisc show dev gc0
  echo '=== SERVER NETEM FINAL ==='
  sudo ip netns exec "$S" tc -s qdisc show dev gs0
} >"$LOG_DIR/netem-final.txt" 2>&1

# A smoke run is a fail-fast health gate, not a performance qualification run.
# Reject known transport exits even if a few packets were delivered before the
# process died, so a long run cannot start behind a partially-live data plane.
if [[ "$SMOKE_STRICT" == 1 ]]; then
  if grep -q 'DTLS trying to send too much in single datagram' "$LOG_DIR"/dtls-*.log 2>/dev/null; then
    echo 'WBD_REALISTIC_SMOKE_FAIL reason=dtls_datagram_too_large' >&2
    exit 1
  fi
  if grep -q 'WBD_LINK_PROXY_FAIL' "$LOG_DIR"/link-*.log 2>/dev/null; then
    echo 'WBD_REALISTIC_SMOKE_FAIL reason=link_proxy_failure' >&2
    exit 1
  fi
fi

# Ask each client FakeTCP to exit normally enough to emit its lifetime sender
# statistics. Match by exact executable basename + client argv, not broad pgrep.
for p in "${PIDS[@]:-}"; do
  [[ -e "/proc/$p/exe" ]] || continue
  exe=$(sudo readlink -f "/proc/$p/exe" 2>/dev/null || true)
  [[ "${exe##*/}" == wbd-faketcp ]] || continue
  cmd=$(sudo tr '\0' ' ' <"/proc/$p/cmdline" 2>/dev/null || true)
  [[ "$cmd" == *" client "* ]] || continue
  sudo kill -TERM "$p" 2>/dev/null || true
  wait "$p" 2>/dev/null || true
  drop_pid "$p"
done
sleep 1

python3 "$GITHUB_WORKSPACE/scripts/realistic_analysis_summary.py" "$LOG_DIR" "$LANES" "$FEC" "$DURATION_SEC" "$RATE_BPS" | tee "$LOG_DIR/analysis.log"
python3 - "$LOG_DIR/analysis-result.json" "$LANES" "$FEC" "$DURATION_SEC" "$SMOKE_STRICT" <<'PY_CHECK'
import json,sys
p,lanes,fec,duration,smoke=sys.argv[1],int(sys.argv[2]),sys.argv[3],int(sys.argv[4]),int(sys.argv[5])
d=json.load(open(p))
assert d['analysis_only'] is True and d['qualification_authority'] is False,d
assert d['loss_gate']=='disabled_analysis_only',d
assert d['lanes']==lanes and d['fec']==fec,d
assert d['requested_duration_sec']==duration and d['requested_bps_each_direction']==10_000_000,d
assert d['application']['duplicates']==0,d
assert d['application']['bad_payload']==0,d
assert d['client_faketcp_lifetime']['lanes_with_final_stats']==lanes,d
if smoke:
    app=d['application']
    wire=d['public_wire']['interface_delta']
    assert app['sent'] > 0 and app['sent_payload_bytes'] > 0,d
    assert app['received_unique'] > 0 and app['received_payload_bytes'] > 0,d
    assert app['sent_by_size'].get('1320',0) > 0,d
    assert d['client_faketcp_lifetime']['enqueued_bytes'] > 0,d
    for k in ('client_tx_bytes','client_rx_bytes','server_tx_bytes','server_rx_bytes'):
        assert wire.get(k,0) > 0,(k,d)
    print('WBD_REALISTIC_SMOKE_PASS')
print('WBD_REALISTIC_ANALYSIS_STRUCTURE_PASS')
PY_CHECK

echo "WBD_REALISTIC_ANALYSIS_COMPLETE source_sha=d1e2c827183415b32d94082e2ca2a59cefdc544b lanes=${LANES} fec=${FEC} duration_sec=${DURATION_SEC} rate_bps=${RATE_BPS} netem_each_direction=300ms_loss20 firewall=public-default-drop loss_gate=disabled smoke_strict=${SMOKE_STRICT}"
'''
out.write_text(prefix + tail)
PY_PATCHER
chmod +x "$PATCHED"
exec "$PATCHED" "$ASSET_DIR" "$LOG_DIR"
