#!/usr/bin/env bash
set -euo pipefail
PRODUCT_DIR=${1:?product checkout}
TMP_ROOT=${2:?temporary directory}
HERE=$(cd "$(dirname "$0")" && pwd)
PROFILE=${PROFILE:-$(cat "$HERE/profile.txt")}
export SOURCE_SHA=484e29463bb83c503ee3baf669c23b013dd44d6c
export CANDIDATE="isolated-$PROFILE"
LOG="$TMP_ROOT/singlelane-$CANDIDATE"
mkdir -p "$LOG"
case "$PROFILE" in
  direct80) rate=80000000; loss=0 ;;
  a20-loss20) rate=20000000; loss=20 ;;
  a80-loss0) rate=80000000; loss=0 ;;
  a80-loss2) rate=80000000; loss=2 ;;
  *) echo 'Unknown profile' >&2; exit 2 ;;
esac
export DIAG_HERE="$HERE"
python3 - "$LOG/provenance.json" "$PROFILE" "$rate" "$loss" <<'PY'
import json,os,platform,subprocess,sys
p,profile,rate,loss=sys.argv[1:]
d=dict(profile=profile, rate_bps=int(rate), loss_pct_each_direction=int(loss),
       product_sha=os.environ['SOURCE_SHA'], helper_sha=os.environ.get('GITHUB_SHA'),
       run_id=os.environ.get('GITHUB_RUN_ID'), run_attempt=os.environ.get('GITHUB_RUN_ATTEMPT'),
       platform=platform.platform(), cpu_count=os.cpu_count(),
       delay_ms_each_direction=0 if profile.startswith('direct') else 300,
       duration_sec=20, payload_bytes=1000, fec=None if profile.startswith('direct') else '20:20',
       note='Diagnostic execution success is not a product loss/latency acceptance result')
d['cpu_info']=subprocess.run(['lscpu'],capture_output=True,text=True).stdout
open(p,'w').write(json.dumps(d,indent=2))
PY
if [[ "$PROFILE" == direct80 ]]; then
  python3 "$HERE/direct.py" "$LOG" "$rate"
  exit
fi
test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$SOURCE_SHA"
test -z "$(git -C "$PRODUCT_DIR" status --porcelain)"
# Add a final generated-script hook, leaving all product Go/C source untouched.
python3 - "$PRODUCT_DIR" "$HERE" "$TMP_ROOT/isolated-driver.sh" "$rate" "$loss" <<'PY'
from pathlib import Path
import sys
product,here,driver,rate,loss=map(str,sys.argv[1:])
p=Path(product)/'scripts/game_lane_rotation_soak.sh'
s=p.read_text()
anchor='pathlib.Path(sys.argv[2]).write_text(prefix + tail)'
assert s.count(anchor)==1
s=s.replace(anchor,anchor+'\nimport os, subprocess\nsubprocess.run([sys.executable, os.environ["DIAG_HERE"]+"/patch_generated.py", sys.argv[2]], check=True)')
p.write_text(s)
s=(Path(here).parent/'validate_singlelane_shadow_candidate.sh').read_text()
s=s.replace('20000000',rate)
assert s.count('export NETEM_LOSS_PCT=20')==1
s=s.replace('export NETEM_LOSS_PCT=20','export NETEM_LOSS_PCT='+loss)
s=s.replace('loss_pct_each_direction=20', 'loss_pct_each_direction='+loss)
# Diagnostics must preserve the harness patch and binary/source provenance.
anchor='set +e\nsudo -E bash'
assert s.count(anchor)==1
s=s.replace(anchor,'git diff -- scripts >"$LOG/harness.patch"\nsha256sum "$ASSET"/wbd-* "$ASSET/wbd_dtls_shim" >"$LOG/binary-sha256.txt"\n'+anchor)
Path(driver).write_text(s)
PY
bash "$TMP_ROOT/isolated-driver.sh" "$PRODUCT_DIR" "$TMP_ROOT"
