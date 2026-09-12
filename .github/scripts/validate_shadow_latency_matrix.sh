#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?product checkout required}
TMP_ROOT=${2:?runner temp required}
CANDIDATE=${CANDIDATE:?candidate label required}
SOURCE_SHA=${SOURCE_SHA:?product source sha required}
TEST_RATE_BPS=${TEST_RATE_BPS:-20000000}
TEST_LOSS_PCT=${TEST_LOSS_PCT:-20}
TEST_PAYLOAD_BYTES=${TEST_PAYLOAD_BYTES:-1000}
BASE_DRIVER="$GITHUB_WORKSPACE/helper/.github/scripts/validate_singlelane_shadow_candidate.sh"
DRIVER="$TMP_ROOT/validate-shadow-latency-${CANDIDATE}.sh"
LOG="$TMP_ROOT/singlelane-$CANDIDATE"

# Add low-overhead end-to-end timing to the existing echo probe. The payload
# already carries its monotonic transmit timestamp, so no packet capture or
# extra protocol traffic is required. Deadline ratios use all sent packets as
# the denominator; a packet arriving after a deadline counts as late even if it
# eventually arrives during the post-send drain.
python3 - "$PRODUCT_DIR/scripts/game_lane_rotation_soak.sh" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
s=p.read_text()
old="sent=0; unique=set(); dup=0; bad=0; last_rx=start\n"
new="sent=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; deadline_1s=0; deadline_2s=0\n"
if s.count(old) != 1:
    raise SystemExit('latency patch: load state marker drift')
s=s.replace(old,new,1)
old="""                seq=struct.unpack('!Q',data[4:12])[0]\n                if seq in unique: dup+=1\n                else: unique.add(seq)\n"""
new="""                seq=struct.unpack('!Q',data[4:12])[0]\n                tx_rel=struct.unpack('!d',data[12:20])[0]\n                if seq in unique:\n                    dup+=1\n                else:\n                    unique.add(seq)\n                    rtt=max(0.0,(time.monotonic()-start)-tx_rel)\n                    latency_ms.append(rtt*1000.0)\n                    if rtt <= 1.0: deadline_1s += 1\n                    if rtt <= 2.0: deadline_2s += 1\n"""
if s.count(old) != 1:
    raise SystemExit('latency patch: receive marker drift')
s=s.replace(old,new,1)
old="""elapsed=max(duration,1e-9)\nrecv=len(unique); lost=max(0,sent-recv)\nsummary={\n"""
new="""elapsed=max(duration,1e-9)\nrecv=len(unique); lost=max(0,sent-recv)\nlatency_ms.sort()\ndef pct(q):\n    if not latency_ms: return None\n    pos=(len(latency_ms)-1)*q\n    lo=int(pos); hi=min(lo+1,len(latency_ms)-1); frac=pos-lo\n    return latency_ms[lo]*(1.0-frac)+latency_ms[hi]*frac\nsummary={\n"""
if s.count(old) != 1:
    raise SystemExit('latency patch: summary marker drift')
s=s.replace(old,new,1)
old="""  'loss_ratio':(lost/sent if sent else 1.0),\n}\n"""
new="""  'loss_ratio':(lost/sent if sent else 1.0),\n  'deadline_1s_received':deadline_1s,\n  'deadline_2s_received':deadline_2s,\n  'deadline_1s_ratio':(deadline_1s/sent if sent else 0.0),\n  'deadline_2s_ratio':(deadline_2s/sent if sent else 0.0),\n  'rtt_samples':len(latency_ms),\n  'rtt_p50_ms':pct(0.50),'rtt_p95_ms':pct(0.95),'rtt_p99_ms':pct(0.99),\n  'rtt_max_ms':(latency_ms[-1] if latency_ms else None),\n}\n"""
if s.count(old) != 1:
    raise SystemExit('latency patch: metrics marker drift')
s=s.replace(old,new,1)
p.write_text(s)
PY

cp "$BASE_DRIVER" "$DRIVER"
python3 - "$DRIVER" "$TEST_RATE_BPS" "$TEST_LOSS_PCT" "$TEST_PAYLOAD_BYTES" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); rate=sys.argv[2]; loss=sys.argv[3]; payload=sys.argv[4]
s=p.read_text()
# The base diagnostic intentionally pins one severe point. This wrapper changes
# only test parameters; candidate source code and all transport constants remain
# exact to SOURCE_SHA.
if '20000000' not in s:
    raise SystemExit('matrix wrapper: base 20 Mbps marker missing')
s=s.replace('20000000',rate)
repls={
    'loss_pct_each_direction=20':f'loss_pct_each_direction={loss}',
    'export NETEM_LOSS_PCT=20':f'export NETEM_LOSS_PCT={loss}',
    'export PAYLOAD_BYTES=1000':f'export PAYLOAD_BYTES={payload}',
}
for old,new in repls.items():
    if s.count(old) != 1:
        raise SystemExit('matrix wrapper marker drift: '+old)
    s=s.replace(old,new,1)
p.write_text(s)
PY
chmod +x "$DRIVER"

echo "WBD_LATENCY_MATRIX_CASE candidate=$CANDIDATE source_sha=$SOURCE_SHA rate_bps=$TEST_RATE_BPS loss_pct_each_direction=$TEST_LOSS_PCT payload_bytes=$TEST_PAYLOAD_BYTES deadline_1s=1 deadline_2s=2"

set +e
bash "$DRIVER" "$PRODUCT_DIR" "$TMP_ROOT"
rc=$?
set -e

if [[ -f "$LOG/load-result.json" && -f "$LOG/shadow-matrix-result.json" ]]; then
  python3 - "$LOG/load-result.json" "$LOG/shadow-matrix-result.json" "$CANDIDATE" "$SOURCE_SHA" "$TEST_RATE_BPS" "$TEST_LOSS_PCT" "$TEST_PAYLOAD_BYTES" <<'PY'
import json,sys
load_path,result_path,candidate,sha,rate,loss,payload=sys.argv[1:]
load=json.load(open(load_path)); result=json.load(open(result_path))
for k in ('deadline_1s_received','deadline_2s_received','deadline_1s_ratio','deadline_2s_ratio',
          'rtt_samples','rtt_p50_ms','rtt_p95_ms','rtt_p99_ms','rtt_max_ms'):
    result[k]=load.get(k)
result.update({
    'candidate':candidate,'product_source_sha':sha,
    'test_rate_bps':int(rate),'test_loss_pct_each_direction':float(loss),
    'test_payload_bytes':int(payload),'test_one_way_delay_ms':300,
    'test_fec':'20:20','test_rate_cap':'none',
})
open(result_path,'w').write(json.dumps(result,indent=2,sort_keys=True)+'\n')
print('WBD_LATENCY_MATRIX_RESULT '+json.dumps(result,sort_keys=True))
PY
fi

exit "$rc"
