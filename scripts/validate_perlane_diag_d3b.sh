#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: validate_perlane_diag_d3b.sh PRODUCT_DIR RUNNER_TEMP}
TMP_ROOT=${2:?usage: validate_perlane_diag_d3b.sh PRODUCT_DIR RUNNER_TEMP}
SOURCE_SHA=d3b54dced255cc9becb3e501991f71a034e7438f
WOLFSSL_SOURCE_ARTIFACT_ID=9529710833
ASSET="$TMP_ROOT/wbd-assets"
LOG="$TMP_ROOT/tenm-300oneway-20loss"

cd "$PRODUCT_DIR"
test "$(git rev-parse HEAD)" = "$SOURCE_SHA"
echo "WBD_PERLANE_SOURCE product_source_sha=$SOURCE_SHA helper_sha=${GITHUB_SHA:-unknown}"

mkdir -p "$ASSET" "$LOG" /tmp/wolf-art /tmp/wolf/src /tmp/wolf/build
rm -rf /tmp/wolf-art/* /tmp/wolf/src/* /tmp/wolf/build/*
: "${GH_TOKEN:?GH_TOKEN is required to download the pinned wolfSSL source artifact}"
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

# Patch only hosted diagnostic harnesses. Product Go/C/native sources remain byte-for-byte SOURCE_SHA.
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
        "# per-lane diagnostic evaluates after captures",
    "if summary['down_payload_bps'] < rate_bps*0.995: raise SystemExit(14)":
        "# per-lane diagnostic evaluates after captures",
}
for old, new in repl.items():
    if s.count(old) != 1:
        raise SystemExit('rotation guard drift: ' + old)
    s = s.replace(old, new, 1)
if '--shadow-recovery legacy' not in s:
    raise SystemExit('replacement recovery marker missing')
s = s.replace('--shadow-recovery legacy', '--shadow-recovery sack-rack').replace('recovery=legacy', 'recovery=sack-rack')
rotation.write_text(s)

full = Path('scripts/game_lane_fullstack.sh')
s = full.read_text()
if '--shadow-recovery legacy' not in s:
    raise SystemExit('initial recovery marker missing')
s = s.replace('--shadow-recovery legacy', '--shadow-recovery sack-rack').replace('recovery=legacy', 'recovery=sack-rack')

old = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
new = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --shadow-recovery sack-rack \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
if s.count(old) != 1:
    raise SystemExit('server mux marker drift')
s = s.replace(old, new, 1)

marker = "grep -q 'listening on gc0' \"$LOG_DIR/tcpdump.log\"\n"
extra = r'''grep -q 'listening on gc0' "$LOG_DIR/tcpdump.log"

# Sender-side gc0 sees forward attempts before client netem and reverse arrivals after server netem.
# Server-side gs0 sees forward arrivals after client netem and reverse attempts before server netem.
sudo ip netns exec "$S" tcpdump --immediate-mode -i gs0 -s 0 -U -w "$LOG_DIR/game-lanes-server.pcap" "tcp port ${RAW}" >"$LOG_DIR/tcpdump-server.log" 2>&1 &
SPID=$!; PIDS+=("$SPID")
for _ in $(seq 1 200); do grep -q 'listening on gs0' "$LOG_DIR/tcpdump-server.log" && break; sleep .05; done
grep -q 'listening on gs0' "$LOG_DIR/tcpdump-server.log"

# This loopback capture is after LINK/FEC decode and before Game first-arrival dedupe.
sudo ip netns exec "$S" tcpdump --immediate-mode -i lo -s 0 -U -w "$LOG_DIR/game-after-fec-server.pcap" "udp dst port ${GAME}" >"$LOG_DIR/tcpdump-game.log" 2>&1 &
GPID=$!; PIDS+=("$GPID")
for _ in $(seq 1 200); do grep -q 'listening on lo' "$LOG_DIR/tcpdump-game.log" && break; sleep .05; done
grep -q 'listening on lo' "$LOG_DIR/tcpdump-game.log"
'''
if s.count(marker) != 1:
    raise SystemExit('tcpdump marker drift')
s = s.replace(marker, extra, 1)

cleanup = 'cleanup() {\n  set +e\n'
diag = 'cleanup() {\n  set +e\n  if sudo ip netns exec "$C" true 2>/dev/null; then\n    sudo ip netns exec "$C" tc -s qdisc show dev gc0 >"$LOG_DIR/tc-client-qdisc.txt" 2>&1 || true\n  fi\n  if sudo ip netns exec "$S" true 2>/dev/null; then\n    sudo ip netns exec "$S" tc -s qdisc show dev gs0 >"$LOG_DIR/tc-server-qdisc.txt" 2>&1 || true\n  fi\n'
if s.count(cleanup) != 1:
    raise SystemExit('cleanup marker drift')
s = s.replace(cleanup, diag, 1)
full.write_text(s)

control_path = Path('scripts/game_lane_rotation_soak_control.sh')
control = control_path.read_text()
if 'rate 100mbit' in control.lower():
    raise SystemExit('unexpected bandwidth cap in source control wrapper')
for dev in ('gc0', 'gs0'):
    matches = [line for line in control.splitlines() if f'tc qdisc replace dev {dev} root netem delay' in line]
    if len(matches) != 1:
        raise SystemExit(f'netem marker drift for {dev}: matches={len(matches)}')
    old_line = matches[0]
    # Preserve literal escaped quotes because this line lives inside the wrapper's raw Python template.
    new_line = old_line.replace('root netem delay', 'root netem limit \\"${NETEM_LIMIT_PACKETS}\\" delay', 1)
    control = control.replace(old_line, new_line, 1)
control_path.write_text(control)
PY

bash -n scripts/game_lane_fullstack.sh
bash -n scripts/game_lane_rotation_soak.sh
bash -n scripts/game_lane_rotation_soak_control.sh

echo 'WBD_PERLANE_HARNESS_READY recovery=sack-rack rate_cap=none one_way_delay_ms=300 loss_pct_each_direction=20'

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

fec_factor=2
logical_pps=$(( (RATE_BPS + PAYLOAD_BYTES*8 - 1) / (PAYLOAD_BYTES*8) ))
delay_working_set=$(( (logical_pps * LANES * fec_factor * NETEM_DELAY_MS + 999) / 1000 ))
NETEM_LIMIT_PACKETS=$(( delay_working_set * 8 ))
if (( NETEM_LIMIT_PACKETS < 16384 )); then NETEM_LIMIT_PACKETS=16384; fi
export NETEM_LIMIT_PACKETS
echo "WBD_PERLANE_NETEM one_way_delay_ms=$NETEM_DELAY_MS base_rtt_ms=$((NETEM_DELAY_MS*2)) loss_pct_each_direction=$NETEM_LOSS_PCT rate_cap=none delay_working_set_packets=$delay_working_set queue_limit_packets=$NETEM_LIMIT_PACKETS"

set +e
sudo -E bash "$PRODUCT_DIR/scripts/game_lane_rotation_soak_control.sh" "$ASSET" "$LOG"
run_rc=$?
set -e

# Always analyze whatever the short run captured. A transport/app failure is data, not a reason to lose diagnostics.
set +e
python3 - "$LOG" "$SOURCE_SHA" <<'PY'
import json, pathlib, re, subprocess, sys
root = pathlib.Path(sys.argv[1])
source_sha = sys.argv[2]
load_path = root / 'load-result.json'
if not load_path.exists():
    raise SystemExit('load-result.json missing; harness failed before measured load')
load = json.loads(load_path.read_text())

def tcpdump_lines(name):
    path = root / name
    if not path.exists():
        raise SystemExit(f'missing capture {path}')
    p = subprocess.run(['tcpdump', '-nn', '-tt', '-r', str(path)], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, check=True)
    return p.stdout.splitlines()

c_outer = tcpdump_lines('game-lanes.pcap')
s_outer = tcpdump_lines('game-lanes-server.pcap')
game = tcpdump_lines('game-after-fec-server.pcap')
game_log = (root / 'game-server.log').read_text(errors='replace')

peer_by_lane = {}
for m in re.finditer(r'WBD_GAME_LANE_BIND .*?lane=(\d+).*?association_peer=127\.0\.0\.1:(\d+)', game_log):
    peer_by_lane[int(m.group(1))] = int(m.group(2))
if sorted(peer_by_lane) != [1, 2, 3, 4]:
    raise SystemExit(f'could not map all Game lane peers: {peer_by_lane}')

post_fec = {i: 0 for i in range(1, 5)}
for line in game:
    m = re.search(r'IP 127\.0\.0\.1\.(\d+) > 127\.0\.0\.1\.49000: UDP, length (\d+)', line)
    if not m:
        continue
    sport = int(m.group(1)); length = int(m.group(2))
    # Measured WBD1 payload is 1000 B before Game envelope; controls/probes are far smaller.
    if length <= 1000:
        continue
    for lane, peer_port in peer_by_lane.items():
        if sport == peer_port:
            post_fec[lane] += 1
            break

def count(lines, src_ip, src_port, dst_ip, dst_port):
    needle = f'{src_ip}.{src_port} > {dst_ip}.{dst_port}:'
    return sum(needle in line for line in lines)

expected = int(load['sent'])
per_lane = []
for lane in range(1, 5):
    sport = 41000 + lane
    f_tx = count(c_outer, '10.89.0.2', sport, '10.89.0.1', 40000)
    f_rx = count(s_outer, '10.89.0.2', sport, '10.89.0.1', 40000)
    r_tx = count(s_outer, '10.89.0.1', 40000, '10.89.0.2', sport)
    r_rx = count(c_outer, '10.89.0.1', 40000, '10.89.0.2', sport)
    delivered = post_fec[lane]
    row = {
        'lane': lane,
        'outer_forward_tx': f_tx, 'outer_forward_rx': f_rx,
        'outer_forward_loss': ((f_tx-f_rx)/f_tx if f_tx else None),
        'outer_reverse_tx': r_tx, 'outer_reverse_rx': r_rx,
        'outer_reverse_loss': ((r_tx-r_rx)/r_tx if r_tx else None),
        'fec_source_expected': expected,
        'post_fec_game_rx': delivered,
        'post_fec_residual_missing': max(0, expected-delivered),
        'post_fec_loss': (max(0, expected-delivered)/expected if expected else None),
        'game_peer_port': peer_by_lane[lane],
    }
    per_lane.append(row)
    print('WBD_PERLANE_RESULT ' + json.dumps(row, sort_keys=True))

summary = {
    'product_source_sha': source_sha,
    'recovery': 'sack-rack',
    'one_way_delay_ms': 300,
    'base_rtt_ms': 600,
    'loss_pct_each_direction': 20,
    'rate_cap': 'none',
    'fec': '20:20',
    'lanes': 4,
    'application_sent': load['sent'],
    'application_received_unique': load['received_unique'],
    'application_loss_ratio': load['loss_ratio'],
    'down_payload_bps': load['down_payload_bps'],
    'bad_payload': load['bad_payload'],
    'duplicates': load['duplicates'],
    'per_lane': per_lane,
}
print('WBD_PERLANE_SUMMARY ' + json.dumps(summary, sort_keys=True))
(root / 'perlane-result.json').write_text(json.dumps(summary, indent=2, sort_keys=True))
if int(load['bad_payload']) != 0:
    raise SystemExit('payload corruption observed')
for row in per_lane:
    if not (row['outer_forward_tx'] > 0 and row['outer_reverse_tx'] > 0):
        raise SystemExit(f'missing outer traffic for lane: {row}')
PY
analysis_rc=$?
set -e

set +e
echo '=== load-result ==='; cat "$LOG/load-result.json" 2>/dev/null || true
echo '=== per-lane result ==='; cat "$LOG/perlane-result.json" 2>/dev/null || true
echo '=== qdisc client ==='; cat "$LOG/tc-client-qdisc.txt" 2>/dev/null || true
echo '=== qdisc server ==='; cat "$LOG/tc-server-qdisc.txt" 2>/dev/null || true
echo '=== Game lane mapping ==='; grep -E 'WBD_GAME_LANE_(BIND|SESSION_CLOSE|CLIENT_LANE_STATS)' "$LOG"/*.log 2>/dev/null | tail -n 100 || true
echo '=== FakeTCP pressure/stats ==='; grep -R -E 'WBD_FAKETCP_(STATS|OUTSTANDING_PRESSURE)' "$LOG" 2>/dev/null | tail -n 300 || true
set -e

if (( analysis_rc != 0 )); then exit "$analysis_rc"; fi
# For this diagnostic, preserve a nonzero harness result after emitting measurement data.
if (( run_rc != 0 )); then exit "$run_rc"; fi
