#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?product checkout required}
TMP_ROOT=${2:?runner temp required}
CANDIDATE=${CANDIDATE:?candidate label required}
SOURCE_SHA=${SOURCE_SHA:?product source sha required}
WOLFSSL_SOURCE_ARTIFACT_ID=${WOLFSSL_SOURCE_ARTIFACT_ID:-9529710833}
ASSET="$TMP_ROOT/wbd-assets-$CANDIDATE"
LOG="$TMP_ROOT/singlelane-$CANDIDATE"

cd "$PRODUCT_DIR"
test "$(git rev-parse HEAD)" = "$SOURCE_SHA"
echo "WBD_SHADOW_MATRIX_SOURCE candidate=$CANDIDATE product_source_sha=$SOURCE_SHA helper_sha=${GITHUB_SHA:-unknown}"

mkdir -p "$ASSET" "$LOG" /tmp/wolf-art /tmp/wolf/src /tmp/wolf/build
rm -rf /tmp/wolf-art/* /tmp/wolf/src/* /tmp/wolf/build/*
: "${GH_TOKEN:?GH_TOKEN required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY required}"
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

# Candidate-local tests: never borrow a green result from another SHA.
go test ./internal/faketcp ./cmd/wbd-faketcp ./cmd/wbd-faketcp-mux ./internal/fec ./internal/gamelane ./internal/linkdata -count=1
for spec in \
  'wbd-faketcp ./cmd/wbd-faketcp' \
  'wbd-faketcp-mux ./cmd/wbd-faketcp-mux' \
  'wbd-link-proxy ./cmd/wbd-link-proxy' \
  'wbd-link-server-mux ./cmd/wbd-link-server-mux' \
  'wbd-game-lane-client ./cmd/wbd-game-lane-client' \
  'wbd-game-lane-server ./cmd/wbd-game-lane-server'; do
  set -- $spec
  go build -trimpath -o "$ASSET/$1" "$2"
done
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=target.example' \
  -keyout "$ASSET/front.key" -out "$ASSET/front.pem" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=wbd-dtls.test' \
  -keyout "$ASSET/dtls.key" -out "$ASSET/dtls.pem" >/dev/null 2>&1
printf '%s\n' "$SOURCE_SHA" >"$ASSET/SOURCE_SHA.txt"
chmod +x "$ASSET"/wbd-* "$ASSET/wbd_dtls_shim"

# Test-only patch: one lane, 20 s send window, 20 Mbps inner, SACK/RACK.
python3 - <<'PY'
from pathlib import Path
rotation = Path('scripts/game_lane_rotation_soak.sh')
s = rotation.read_text()
repl = {
    '[[ "$LANES" == 4 ]] || { echo "soak requires LANES=4" >&2; exit 2; }':
        '[[ "$LANES" == 1 ]] || { echo "single-lane probe requires LANES=1" >&2; exit 2; }',
    '[[ "$DURATION_SEC" =~ ^[0-9]+$ && "$DURATION_SEC" -ge 500 ]] || { echo "DURATION_SEC must be >=500" >&2; exit 2; }':
        '[[ "$DURATION_SEC" =~ ^[0-9]+$ && "$DURATION_SEC" -ge 20 ]] || { echo "DURATION_SEC must be >=20" >&2; exit 2; }',
    '[[ "$RATE_BPS" == 1000000 ]] || { echo "RATE_BPS must be exactly 1000000" >&2; exit 2; }':
        '[[ "$RATE_BPS" == 20000000 ]] || { echo "RATE_BPS must be exactly 20000000" >&2; exit 2; }',
    '[[ "$ROTATIONS" =~ ^[0-9]+$ && "$ROTATIONS" -ge 1 ]] || { echo "ROTATIONS must be positive" >&2; exit 2; }':
        '[[ "$ROTATIONS" =~ ^[0-9]+$ ]] || { echo "ROTATIONS must be non-negative" >&2; exit 2; }',
    "if summary['loss_ratio'] > 0.001: raise SystemExit(13)":
        '# shadow candidate diagnostic compares completeness externally',
    "if summary['down_payload_bps'] < rate_bps*0.995: raise SystemExit(14)":
        '# shadow candidate diagnostic compares goodput externally',
    "d.update({'fec':fec,'lanes':4,": "d.update({'fec':fec,'lanes':1,",
}
for old,new in repl.items():
    if s.count(old) != 1:
        raise SystemExit('rotation guard drift: '+old)
    s = s.replace(old,new,1)
s = s.replace('for i in $(seq 1 4); do','for i in $(seq 1 "$LANES"); do')
s = s.replace('if (( link_binds >= 4 )); then','if (( link_binds >= LANES )); then')
s = s.replace('if (( game_binds >= 4 )); then','if (( game_binds >= LANES )); then')
s = s.replace('>= 4 ))','>= LANES ))')
s = s.replace('(r-1)%4 + 1','(r-1)%LANES + 1')
if '--shadow-recovery legacy' not in s:
    raise SystemExit('replacement recovery marker missing')
s = s.replace('--shadow-recovery legacy','--shadow-recovery sack-rack').replace('recovery=legacy','recovery=sack-rack')
rotation.write_text(s)

full = Path('scripts/game_lane_fullstack.sh')
s = full.read_text()
if '--shadow-recovery legacy' not in s:
    raise SystemExit('initial recovery marker missing')
s = s.replace('--shadow-recovery legacy','--shadow-recovery sack-rack').replace('recovery=legacy','recovery=sack-rack')
old = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
new = '"$ASSET_DIR/wbd-faketcp-mux" server \\\n  --shadow-recovery sack-rack \\\n  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \\\n'
if s.count(old) != 1:
    raise SystemExit('server mux recovery marker drift')
full.write_text(s.replace(old,new,1))

# No bandwidth cap. Give netem enough queue headroom that delay itself does not create tail-drop.
control = Path('scripts/game_lane_rotation_soak_control.sh')
s = control.read_text()
for dev in ('gc0','gs0'):
    matches = [line for line in s.splitlines() if f'tc qdisc replace dev {dev} root netem delay' in line]
    if len(matches) != 1:
        raise SystemExit(f'netem marker drift for {dev}: {matches}')
    old_line = matches[0]
    new_line = old_line.replace('root netem delay', 'root netem limit 24000 delay', 1)
    s = s.replace(old_line,new_line,1)
if 'rate 100mbit' in s.lower():
    raise SystemExit('rate cap unexpectedly present after no-cap patch')
control.write_text(s)
PY

bash -n scripts/game_lane_fullstack.sh
bash -n scripts/game_lane_rotation_soak.sh
bash -n scripts/game_lane_rotation_soak_control.sh

echo "WBD_SHADOW_MATRIX_HARNESS candidate=$CANDIDATE lanes=1 rate_bps=20000000 one_way_delay_ms=300 loss_pct_each_direction=20 fec=20:20 rate_cap=none"
export GITHUB_WORKSPACE="$PRODUCT_DIR"
export RUNNER_TEMP="$TMP_ROOT"
export LANES=1
export FEC=20:20
export DURATION_SEC=20
export RATE_BPS=20000000
export PAYLOAD_BYTES=1000
export ROTATE_INTERVAL_SEC=10
export ROTATIONS=0
export NETEM_DELAY_MS=300
export NETEM_LOSS_PCT=20
export SOAK_REPLICA=1

set +e
sudo -E bash "$PRODUCT_DIR/scripts/game_lane_rotation_soak_control.sh" "$ASSET" "$LOG"
run_rc=$?
set -e
sudo chmod -R a+rX "$LOG" 2>/dev/null || true

python3 - "$LOG" "$CANDIDATE" "$SOURCE_SHA" <<'PY'
import glob,json,os,sys
root,candidate,source_sha=sys.argv[1:]
path=os.path.join(root,'load-result.json')
if not os.path.exists(path):
    raise SystemExit('missing load-result.json')
load=json.load(open(path))
client=[]; server=[]
for p in glob.glob(root+'/*.log'):
    for line in open(p,errors='replace'):
        if 'WBD_FAKETCP_STATS ' in line:
            raw=line.split('WBD_FAKETCP_STATS ',1)[1].strip()
            try: client.append(json.loads(raw))
            except Exception: pass
        if 'WBD_FAKETCP_MUX_SHADOW_STATS ' in line:
            raw=line.split('WBD_FAKETCP_MUX_SHADOW_STATS ',1)[1].strip()
            try: server.append(json.loads(raw))
            except Exception: pass
senders=[x.get('sender',{}) for x in client if x.get('role')=='client' and x.get('sender')]
receivers=[x.get('receiver',{}) for x in client if x.get('role')=='client' and x.get('receiver')]
if not senders:
    raise SystemExit('missing client sender stats')

def total(field, xs): return sum(int(x.get(field,0) or 0) for x in xs)
def maximum(field, xs): return max([int(x.get(field,0) or 0) for x in xs] or [0])
fresh=total('FreshAdmittedBytes',senders) or total('EnqueuedBytes',senders)
shadow=total('ShadowRetransmitBytes',senders) or total('RetransmitBytes',senders)
result={
  'candidate':candidate,'product_source_sha':source_sha,
  'sent':load.get('sent'),'received_unique':load.get('received_unique'),
  'loss_ratio':load.get('loss_ratio'),'down_payload_bps':load.get('down_payload_bps'),
  'bad_payload':load.get('bad_payload'),'duplicates':load.get('duplicates'),
  'fresh_bytes':fresh,'shadow_retransmit_bytes':shadow,
  'repair_to_fresh_bytes':(shadow/fresh if fresh else None),
  'peak_pending':maximum('PeakPending',senders),
  'fresh_admitted':total('FreshAdmitted',senders),
  'fresh_blocked_by_repair':total('FreshBlockedByRepair',senders),
  'repair_evicted':total('RepairEvicted',senders),
  'repair_evicted_bytes':total('RepairEvictedBytes',senders),
  'repair_metadata_evicted':total('RepairMetadataEvicted',senders),
  'repair_credit_bytes':total('RepairCreditBytes',senders),
  'repair_budget_spent':total('RepairBudgetSpent',senders),
  'forgiven_gaps':total('ForgivenGaps',receivers),
  'late_below_ack':total('LateBelowACK',receivers),
  'below_next_drops':total('BelowNextDrops',receivers),
  'peak_buffered_oo':maximum('PeakBufferedOO',receivers),
  'pressure_soft_limit':maximum('PressureSoftLimit',receivers),
  'pressure_rate':maximum('PressureRate',receivers),
  'pressure_srtt_ms':maximum('PressureSRTTMillis',receivers),
  'soft_forgiven_gaps':total('SoftForgivenGaps',receivers),
  'emergency_forgiven_gaps':total('EmergencyForgivenGaps',receivers),
  'server_shadow_stats_count':len(server),
}
print('WBD_SHADOW_MATRIX_RESULT '+json.dumps(result,sort_keys=True))
open(os.path.join(root,'shadow-matrix-result.json'),'w').write(json.dumps(result,indent=2,sort_keys=True))
open(os.path.join(root,'shadow-client-stats.json'),'w').write(json.dumps(client,indent=2,sort_keys=True))
open(os.path.join(root,'shadow-server-stats.json'),'w').write(json.dumps(server,indent=2,sort_keys=True))
if load.get('bad_payload',0):
    raise SystemExit('payload corruption observed')
if result['fresh_blocked_by_repair']:
    raise SystemExit('fresh was blocked by repair pressure')
if result['repair_to_fresh_bytes'] is not None and result['repair_to_fresh_bytes'] > 0.25:
    raise SystemExit('scheduled shadow retransmit bytes exceeded diagnostic 25% guard')
PY
analysis_rc=$?

set +e
echo '=== candidate result ==='; cat "$LOG/shadow-matrix-result.json" 2>/dev/null || true
echo '=== load ==='; cat "$LOG/load-result.json" 2>/dev/null || true
echo '=== qdisc/netem ==='; cat "$LOG/netem.log" 2>/dev/null || true
echo '=== client stats markers ==='; grep -R 'WBD_FAKETCP_STATS\|OUTSTANDING_PRESSURE' "$LOG" 2>/dev/null | tail -n 120 || true
echo '=== server shadow stats markers ==='; grep -R 'WBD_FAKETCP_MUX_SHADOW_STATS' "$LOG" 2>/dev/null | tail -n 80 || true
set -e

if (( analysis_rc != 0 )); then exit "$analysis_rc"; fi
if (( run_rc != 0 )); then exit "$run_rc"; fi
