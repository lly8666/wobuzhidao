#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: validate_perlane_tc_diag_d3b.sh PRODUCT_DIR RUNNER_TEMP}
TMP_ROOT=${2:?usage: validate_perlane_tc_diag_d3b.sh PRODUCT_DIR RUNNER_TEMP}
SOURCE_SHA=d3b54dced255cc9becb3e501991f71a034e7438f
WOLFSSL_SOURCE_ARTIFACT_ID=9529710833
ASSET="$TMP_ROOT/wbd-assets-tc"
LOG="$TMP_ROOT/tenm-300oneway-20loss-tc"

cd "$PRODUCT_DIR"
test "$(git rev-parse HEAD)" = "$SOURCE_SHA"
echo "WBD_TCDIAG_SOURCE product_source_sha=$SOURCE_SHA helper_sha=${GITHUB_SHA:-unknown} instrumentation=game_server_counters_only"

mkdir -p "$ASSET" "$LOG" /tmp/wolf-art /tmp/wolf/src /tmp/wolf/build
rm -rf /tmp/wolf-art/* /tmp/wolf/src/* /tmp/wolf/build/*

# Add diagnostic-only Game counters before building. This does not alter wire
# format, scheduling, FEC, recovery, or delivery semantics: it only increments
# four counters while the existing gameSession mutex is already held.
python3 - <<'PY'
from pathlib import Path
p = Path('cmd/wbd-game-lane-server/main.go')
s = p.read_text()
old = '''\tinFirst     uint64
\tinDup       uint64
\toutLogic    uint64
'''
new = '''\tinFirst     uint64
\tinDup       uint64
\tinLane      [gamelane.MaxLanes + 1]uint64
\toutLogic    uint64
'''
if s.count(old) != 1:
    raise SystemExit('gameSession counter marker drift')
s = s.replace(old, new, 1)
old = '''\th, _, err := gamelane.Parse(wire)
\tif err != nil { return err }
'''
new = '''\th, payload, err := gamelane.Parse(wire)
\tif err != nil { return err }
'''
if s.count(old) != 1:
    raise SystemExit('Game Parse marker drift')
s = s.replace(old, new, 1)
old = '''\tif bound := gs.peerLane[peer.String()]; bound != h.LaneID { gs.mu.Unlock(); return fmt.Errorf("peer %s lane changed from %d to %d", peer, bound, h.LaneID) }
\tresult, err := gs.dec.Add(wire)
'''
new = '''\tif bound := gs.peerLane[peer.String()]; bound != h.LaneID { gs.mu.Unlock(); return fmt.Errorf("peer %s lane changed from %d to %d", peer, bound, h.LaneID) }
\tif len(payload) >= 4 && string(payload[:4]) == "WBD1" { gs.inLane[h.LaneID]++ }
\tresult, err := gs.dec.Add(wire)
'''
if s.count(old) != 1:
    raise SystemExit('Game decoder marker drift')
s = s.replace(old, new, 1)
old = '''\tinFirst,inDup,outLogic,outLane,dormantDrop := gs.inFirst,gs.inDup,gs.outLogic,gs.outLane,gs.dormantDrop
\tgs.mu.Unlock(); s.mu.Unlock()
\t_ = gs.service.Close()
\tfmt.Printf("WBD_GAME_LANE_SESSION_CLOSE tunnel_id_prefix=%s reason=%s in_first=%d in_dup=%d out_logical=%d out_lane=%d dormant_drop=%d\\n", tunnelIDPrefix(gs.meta),reason,inFirst,inDup,outLogic,outLane,dormantDrop)
'''
new = '''\tinFirst,inDup,outLogic,outLane,dormantDrop := gs.inFirst,gs.inDup,gs.outLogic,gs.outLane,gs.dormantDrop
\tinLane := gs.inLane
\tgs.mu.Unlock(); s.mu.Unlock()
\t_ = gs.service.Close()
\tfmt.Printf("WBD_GAME_LANE_SESSION_CLOSE tunnel_id_prefix=%s reason=%s in_first=%d in_dup=%d out_logical=%d out_lane=%d dormant_drop=%d in_lane_1=%d in_lane_2=%d in_lane_3=%d in_lane_4=%d\\n", tunnelIDPrefix(gs.meta),reason,inFirst,inDup,outLogic,outLane,dormantDrop,inLane[1],inLane[2],inLane[3],inLane[4])
'''
if s.count(old) != 1:
    raise SystemExit('Game close marker drift')
s = s.replace(old, new, 1)
p.write_text(s)
PY

gofmt -w cmd/wbd-game-lane-server/main.go

# Adapt only hosted test harnesses: 20 s, 10 Mbps, zero rotations, SACK/RACK,
# and a netem queue large enough that configured random loss is the only drop.
python3 - <<'PY'
from pathlib import Path

rotation = Path('scripts/game_lane_rotation_soak.sh')
s = rotation.read_text()
repl = {
    '[[ "$DURATION_SEC" =~ ^[0-9]+$ && "$DURATION_SEC" -ge 500 ]] || { echo "DURATION_SEC must be >=500" >&2; exit 2; }':
        '[[ "$DURATION_SEC" =~ ^[0-9]+$ && "$DURATION_SEC" -ge 20 ]] || { echo "DURATION_SEC must be >=20" >&2; exit 2; }',
    '[[ "$RATE_BPS" == 1000000 ]] || { echo "RATE_BPS must be exactly 1000000" >&2; exit 2; }':
        '[[ "$RATE_BPS" == 10000000 ]] || { echo "RATE_BPS must be exactly 10000000" >&2; exit 2; }',
    '[[ "$ROTATIONS" =~ ^[0-9]+$ && "$ROTATIONS" -ge 1 ]] || { echo "ROTATIONS must be positive" >&2; exit 2; }':
        '[[ "$ROTATIONS" =~ ^[0-9]+$ ]] || { echo "ROTATIONS must be non-negative" >&2; exit 2; }',
    "if summary['loss_ratio'] > 0.001: raise SystemExit(13)":
        "# diagnostic: application loss is measured below",
    "if summary['down_payload_bps'] < rate_bps*0.995: raise SystemExit(14)":
        "# diagnostic: application goodput is measured below",
}
for old, new in repl.items():
    if s.count(old) != 1:
        raise SystemExit('rotation guard drift: ' + old)
    s = s.replace(old, new, 1)
if '--shadow-recovery legacy' not in s:
    raise SystemExit('replacement recovery marker missing')
s = s.replace('--shadow-recovery legacy', '--shadow-recovery sack-rack').replace('recovery=legacy', 'recovery=sack-rack')

# The base bootstrap capture is stopped here by the soak harness. Add only tc
# counters after that point. clsact egress runs before root netem; peer ingress
# runs after netem, so TX/RX deltas are the actual configured wire loss.
marker = 'rm -f "$LOG_DIR/game-lanes.pcap"\n'
block = r'''rm -f "$LOG_DIR/game-lanes.pcap"

sudo ip netns exec "$C" tc qdisc add dev gc0 clsact
sudo ip netns exec "$S" tc qdisc add dev gs0 clsact
for lane in $(seq 1 4); do
  sport=$((41000+lane))
  pref=$((100+lane))
  sudo ip netns exec "$C" tc filter add dev gc0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
  sudo ip netns exec "$S" tc filter add dev gs0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.2 dst_ip 10.89.0.1 src_port "$sport" dst_port "$RAW" action pass
  pref=$((200+lane))
  sudo ip netns exec "$S" tc filter add dev gs0 egress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
  sudo ip netns exec "$C" tc filter add dev gc0 ingress protocol ip pref "$pref" flower ip_proto tcp src_ip 10.89.0.1 dst_ip 10.89.0.2 src_port "$RAW" dst_port "$sport" action pass
done
'''
if s.count(marker) != 1:
    raise SystemExit('tc insertion marker drift')
s = s.replace(marker, block, 1)

# Dump counter state and qdisc state after measured traffic but before namespace
# teardown. No packet payload is copied to userspace.
marker = '# Stop Game endpoints so their final statistics are available in the artifact.\n'
block = r'''sudo ip netns exec "$C" tc -s filter show dev gc0 egress >"$LOG_DIR/tc-gc0-egress.txt"
sudo ip netns exec "$S" tc -s filter show dev gs0 ingress >"$LOG_DIR/tc-gs0-ingress.txt"
sudo ip netns exec "$S" tc -s filter show dev gs0 egress >"$LOG_DIR/tc-gs0-egress.txt"
sudo ip netns exec "$C" tc -s filter show dev gc0 ingress >"$LOG_DIR/tc-gc0-ingress.txt"
sudo ip netns exec "$C" tc -s qdisc show dev gc0 >"$LOG_DIR/tc-gc0-qdisc.txt"
sudo ip netns exec "$S" tc -s qdisc show dev gs0 >"$LOG_DIR/tc-gs0-qdisc.txt"

# Stop Game endpoints so their final statistics are available in the artifact.
'''
if s.count(marker) != 1:
    raise SystemExit('tc dump marker drift')
s = s.replace(marker, block, 1)
rotation.write_text(s)

full = Path('scripts/game_lane_fullstack.sh')
s = full.read_text()
if '--shadow-recovery legacy' not in s:
    raise SystemExit('initial recovery marker missing')
s = s.replace('--shadow-recovery legacy', '--shadow-recovery sack-rack').replace('recovery=legacy', 'recovery=sack-rack')
old = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
new = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --shadow-recovery sack-rack \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
if s.count(old) != 1:
    raise SystemExit('server mux recovery marker drift')
s = s.replace(old, new, 1)
full.write_text(s)

control_path = Path('scripts/game_lane_rotation_soak_control.sh')
control = control_path.read_text()
for dev in ('gc0', 'gs0'):
    matches = [line for line in control.splitlines() if f'tc qdisc replace dev {dev} root netem delay' in line]
    if len(matches) != 1:
        raise SystemExit(f'netem marker drift for {dev}: {matches}')
    old_line = matches[0]
    new_line = old_line.replace('root netem delay', 'root netem limit 24000 delay', 1)
    control = control.replace(old_line, new_line, 1)
control_path.write_text(control)
PY

bash -n scripts/game_lane_fullstack.sh
bash -n scripts/game_lane_rotation_soak.sh
bash -n scripts/game_lane_rotation_soak_control.sh

echo 'WBD_TCDIAG_HARNESS_READY recovery=sack-rack observer=tc-counters game_counter=WBD1-only rate_cap=none one_way_delay_ms=300 loss_pct_each_direction=20 queue_limit_packets=24000'

: "${GH_TOKEN:?GH_TOKEN is required to download pinned wolfSSL source}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
curl -fL -H "Authorization: Bearer ${GH_TOKEN}" -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/${GITHUB_REPOSITORY}/actions/artifacts/${WOLFSSL_SOURCE_ARTIFACT_ID}/zip" \
  -o /tmp/wolfssl-source.zip
unzip -q /tmp/wolfssl-source.zip -d /tmp/wolf-art
echo '4a7ff40a32db0d7a262aaea2d2e674da6708250cba908441c737c981fc84f88b  /tmp/wolf-art/wolfssl-ac01707f-source.tar.gz' | sha256sum -c -
tar -xzf /tmp/wolf-art/wolfssl-ac01707f-source.tar.gz -C /tmp/wolf/src --strip-components=1
(cd /tmp/wolf/src && ./autogen.sh)
(cd /tmp/wolf/build && /tmp/wolf/src/configure --enable-dtls13 --disable-shared --enable-static \
  CFLAGS='-O2 -DWOLFSSL_DTLS_WINDOW_WORDS=128 -DWOLFSSL_MAX_MTU=16384')
make -C /tmp/wolf/build -j2 src/libwolfssl.la
gcc -DWOLFSSL_DTLS_WINDOW_WORDS=128 -DWOLFSSL_MAX_MTU=16384 -O2 -Wall -Wextra -Werror \
  -I/tmp/wolf/build -I/tmp/wolf/src native/dtls/wbd_dtls_shim.c \
  /tmp/wolf/build/src/.libs/libwolfssl.a -lm -o "$ASSET/wbd_dtls_shim"

go test ./internal/fec ./internal/faketcp ./internal/gamelane ./internal/linkdata -count=1
go test ./cmd/wbd-game-lane-server -count=1
go build -trimpath -o "$ASSET/wbd-faketcp" ./cmd/wbd-faketcp
go build -trimpath -o "$ASSET/wbd-faketcp-mux" ./cmd/wbd-faketcp-mux
go build -trimpath -o "$ASSET/wbd-link-proxy" ./cmd/wbd-link-proxy
go build -trimpath -o "$ASSET/wbd-link-server-mux" ./cmd/wbd-link-server-mux
go build -trimpath -o "$ASSET/wbd-game-lane-client" ./cmd/wbd-game-lane-client
go build -trimpath -o "$ASSET/wbd-game-lane-server" ./cmd/wbd-game-lane-server
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=target.example' \
  -keyout "$ASSET/front.key" -out "$ASSET/front.pem" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=wbd-dtls.test' \
  -keyout "$ASSET/dtls.key" -out "$ASSET/dtls.pem" >/dev/null 2>&1
chmod +x "$ASSET"/wbd-* "$ASSET/wbd_dtls_shim"

export GITHUB_WORKSPACE="$PRODUCT_DIR"
export RUNNER_TEMP="$TMP_ROOT"
export LANES=4
export FEC=20:20
export DURATION_SEC=20
export RATE_BPS=10000000
export PAYLOAD_BYTES=1000
export ROTATE_INTERVAL_SEC=10
export ROTATIONS=0
export NETEM_DELAY_MS=300
export NETEM_LOSS_PCT=20
export SOAK_REPLICA=1

echo 'WBD_TCDIAG_NETEM one_way_delay_ms=300 base_rtt_ms=600 loss_pct_each_direction=20 rate_cap=none queue_limit_packets=24000'
set +e
sudo -E bash "$PRODUCT_DIR/scripts/game_lane_rotation_soak_control.sh" "$ASSET" "$LOG"
run_rc=$?
set -e

# Analyze low-overhead counters after namespace teardown. FakeTCP final stats and
# Game final stats have also been flushed by the harness cleanup at this point.
set +e
python3 - "$LOG" "$SOURCE_SHA" <<'PY'
import json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
source_sha = sys.argv[2]
load_path = root / 'load-result.json'
if not load_path.exists():
    raise SystemExit('load-result.json missing')
load = json.loads(load_path.read_text())

def tc_pref_packets(path, wanted):
    text = path.read_text(errors='replace')
    blocks = re.split(r'(?=filter protocol .*? pref \d+ )', text)
    out = {}
    for block in blocks:
        pm = re.search(r'\bpref (\d+)\b', block)
        sm = re.search(r'\bSent \d+ bytes (\d+) pkt\b', block)
        if pm and sm:
            pref = int(pm.group(1))
            if pref in wanted:
                out[pref] = int(sm.group(1))
    missing = sorted(set(wanted) - set(out))
    if missing:
        raise SystemExit(f'missing tc prefs in {path.name}: {missing}; parsed={out}')
    return out

f_tx = tc_pref_packets(root/'tc-gc0-egress.txt', range(101,105))
f_rx = tc_pref_packets(root/'tc-gs0-ingress.txt', range(101,105))
r_tx = tc_pref_packets(root/'tc-gs0-egress.txt', range(201,205))
r_rx = tc_pref_packets(root/'tc-gc0-ingress.txt', range(201,205))

game_server = (root/'game-server.log').read_text(errors='replace')
close_lines = re.findall(r'WBD_GAME_LANE_SESSION_CLOSE[^\n]*', game_server)
if not close_lines:
    raise SystemExit('Game server session-close stats missing')
close = close_lines[-1]
m = re.search(r'out_logical=(\d+).*?in_lane_1=(\d+).*?in_lane_2=(\d+).*?in_lane_3=(\d+).*?in_lane_4=(\d+)', close)
if not m:
    raise SystemExit('instrumented per-lane Game stats missing: ' + close)
reverse_expected = int(m.group(1))
post_forward = {i: int(m.group(i+1)) for i in range(1,5)}

client_game = (root/'game-client.log').read_text(errors='replace')
reverse_rx = {int(a): int(b) for a,b in re.findall(r'WBD_GAME_LANE_CLIENT_LANE_STATS lane=(\d+) tx=\d+ rx=(\d+)', client_game)}
if sorted(reverse_rx) != [1,2,3,4]:
    raise SystemExit(f'client per-lane stats missing: {reverse_rx}')

peak = {}
for lane in range(1,5):
    text = (root/f'faketcp-{lane}.log').read_text(errors='replace')
    stats = re.findall(r'WBD_FAKETCP_STATS (\{[^\n]+\})', text)
    if not stats:
        raise SystemExit(f'FakeTCP stats missing lane {lane}')
    obj = json.loads(stats[-1])
    peak[lane] = int(obj['sender']['PeakPending'])

expected = int(load['sent'])
per_lane = []
for lane in range(1,5):
    fpref, rpref = 100+lane, 200+lane
    ftx, frx = f_tx[fpref], f_rx[fpref]
    rtx, rrx = r_tx[rpref], r_rx[rpref]
    if frx > ftx or rrx > rtx:
        raise SystemExit(f'impossible tc counter ordering lane={lane}: f={ftx}/{frx} r={rtx}/{rrx}')
    pf = post_forward[lane]
    rr = reverse_rx[lane]
    row = {
        'lane': lane,
        'outer_forward_tx': ftx,
        'outer_forward_rx': frx,
        'outer_forward_loss': ((ftx-frx)/ftx if ftx else None),
        'outer_reverse_tx': rtx,
        'outer_reverse_rx': rrx,
        'outer_reverse_loss': ((rtx-rrx)/rtx if rtx else None),
        'post_fec_forward_expected': expected,
        'post_fec_forward_rx_raw_wbd1': pf,
        'post_fec_forward_loss_floor': (max(0, expected-pf)/expected if expected else None),
        'post_fec_reverse_expected': reverse_expected,
        'post_fec_reverse_rx': rr,
        'post_fec_reverse_loss': (max(0, reverse_expected-rr)/reverse_expected if reverse_expected else None),
        'faketcp_peak_pending': peak[lane],
    }
    per_lane.append(row)
    print('WBD_TCDIAG_RESULT ' + json.dumps(row, sort_keys=True))

def qdisc_stats(name):
    text = (root/name).read_text(errors='replace')
    m = re.search(r'Sent \d+ bytes (\d+) pkt \(dropped (\d+), overlimits (\d+)', text)
    if not m:
        return {'packets': None, 'dropped': None, 'overlimits': None}
    return {'packets': int(m.group(1)), 'dropped': int(m.group(2)), 'overlimits': int(m.group(3))}

summary = {
    'product_source_sha': source_sha,
    'observer': 'tc-clsact-counters+game-WBD1-counter',
    'recovery': 'sack-rack',
    'one_way_delay_ms': 300,
    'base_rtt_ms': 600,
    'loss_pct_each_direction': 20,
    'rate_cap': 'none',
    'fec': '20:20',
    'lanes': 4,
    'application_sent': expected,
    'application_received_unique': int(load['received_unique']),
    'application_loss_ratio': float(load['loss_ratio']),
    'up_payload_bps': float(load['up_payload_bps']),
    'down_payload_bps': float(load['down_payload_bps']),
    'bad_payload': int(load['bad_payload']),
    'duplicates': int(load['duplicates']),
    'qdisc_client': qdisc_stats('tc-gc0-qdisc.txt'),
    'qdisc_server': qdisc_stats('tc-gs0-qdisc.txt'),
    'per_lane': per_lane,
}
print('WBD_TCDIAG_SUMMARY ' + json.dumps(summary, sort_keys=True))
(root/'tcdiag-result.json').write_text(json.dumps(summary, indent=2, sort_keys=True)+'\n')
if summary['bad_payload'] != 0:
    raise SystemExit('payload corruption observed')
if summary['duplicates'] != 0:
    raise SystemExit('application duplicate observed')
for row in per_lane:
    if not (row['outer_forward_tx'] > 0 and row['outer_reverse_tx'] > 0):
        raise SystemExit(f'missing outer traffic: {row}')
PY
analysis_rc=$?
set -e

set +e
echo '=== tcdiag result ==='; cat "$LOG/tcdiag-result.json" 2>/dev/null || true
echo '=== load result ==='; cat "$LOG/load-result.json" 2>/dev/null || true
echo '=== qdisc ==='; cat "$LOG/tc-gc0-qdisc.txt" "$LOG/tc-gs0-qdisc.txt" 2>/dev/null || true
echo '=== tc filters ==='; cat "$LOG"/tc-*-{egress,ingress}.txt 2>/dev/null || true
echo '=== Game final stats ==='; grep -E 'WBD_GAME_LANE_(SESSION_CLOSE|CLIENT_LANE_STATS)' "$LOG"/*.log 2>/dev/null | tail -n 50 || true
echo '=== FakeTCP pressure/stats ==='; grep -R -E 'WBD_FAKETCP_(STATS|OUTSTANDING_PRESSURE)' "$LOG" 2>/dev/null | tail -n 200 || true
set -e

if (( analysis_rc != 0 )); then exit "$analysis_rc"; fi
if (( run_rc != 0 )); then exit "$run_rc"; fi
