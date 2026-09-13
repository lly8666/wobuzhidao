#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?product checkout required}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${PRODUCT_SOURCE_SHA:?PRODUCT_SOURCE_SHA required}"
: "${HELPER_SOURCE_SHA:?HELPER_SOURCE_SHA required}"
case "$TEST_RATE_BPS" in
  20000000) LABEL=20m ;;
  30000000) LABEL=30m ;;
  *) echo "TEST_RATE_BPS must be 20000000 or 30000000" >&2; exit 2 ;;
esac

test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$PRODUCT_SOURCE_SHA"
test "$(git -C "$HELPER_DIR" rev-parse HEAD)" = "$HELPER_SOURCE_SHA"

# Exact unified-path MTU budget from the exact product source.
TMPGO="$PRODUCT_DIR/size_probe_budget_tmp.go"
trap 'rm -f "$TMPGO"' EXIT
cat >"$TMPGO" <<'GO'
package main
import (
  "fmt"
  "github.com/lly8666/wobuzhidao/internal/pathmtu"
)
func main() {
  b,err:=pathmtu.Derive(1500,pathmtu.Features{FEC:true,Game:true}); if err!=nil { panic(err) }
  fmt.Printf("%d %d %d %d %d\n",b.ConnectionMTU,b.CarrierPayloadMTU,b.DTLSPlaintextMTU,b.LinkPlaintextMTU,b.InnerMTU)
}
GO
read -r CONNECTION CARRIER DTLS LINK INNER < <(cd "$PRODUCT_DIR" && go run ./size_probe_budget_tmp.go)
rm -f "$TMPGO"
echo "WBD_SIZE_PROBE_MTU connection=$CONNECTION carrier=$CARRIER dtls_plain=$DTLS link_plain=$LINK inner=$INNER"
[[ "$CONNECTION" == 1500 && "$CARRIER" == 1460 ]]

# Test-only helper copy: realistic-mix top bucket follows exact InnerMTU.
PATCHED_HELPER="${RUNNER_TEMP:?}/helper-size-probe-${LABEL}"
rm -rf "$PATCHED_HELPER" && cp -a "$HELPER_DIR" "$PATCHED_HELPER"
python3 - "$PATCHED_HELPER/.github/scripts/instrument_singlelane_realistic_burst.py" "$INNER" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); inner=int(sys.argv[2]); s=p.read_text()
old="size_cycle=([64]*35+[128]*15+[256]*10+[512]*10+[1000]*10+[1200]*5+[1360]*15)"
new=f"size_cycle=([64]*35+[128]*15+[256]*10+[512]*10+[1000]*10+[1200]*5+[{inner}]*15)"
if s.count(old)!=1: raise SystemExit(f'mix max marker drift: {s.count(old)}')
p.write_text(s.replace(old,new,1))
PY

# Reuse the known constant-load observer, but on a temporary wrapper pinned to
# exactly 20 seconds, iid 5% loss and the requested 20/30 Mbps source rate.
CAP="$RUNNER_TEMP/run-capacity-size-probe-${LABEL}.sh"
cp "$PRODUCT_DIR/.github/scripts/run_logic64_capacity_observability.sh" "$CAP"
python3 - "$CAP" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
repl={
'[[ "$TEST_RATE_BPS" =~ ^(10000000|15000000|20000000)$ ]] || { echo "TEST_RATE_BPS must be 10/15/20 Mbps" >&2; exit 2; }':
'[[ "$TEST_RATE_BPS" =~ ^(20000000|30000000)$ ]] || { echo "TEST_RATE_BPS must be 20/30 Mbps" >&2; exit 2; }',
'[[ "$TEST_DURATION_SEC" == 45 ]] || { echo "capacity probe requires 45s" >&2; exit 2; }':
'[[ "$TEST_DURATION_SEC" == 20 ]] || { echo "size probe requires 20s" >&2; exit 2; }',
'[[ "$LOSS_PCT" == 20 && "$LOSS_MODEL" == iid ]] || { echo "capacity probe requires iid 20pct loss" >&2; exit 2; }':
'[[ "$LOSS_PCT" == 5 && "$LOSS_MODEL" == iid ]] || { echo "size probe requires iid 5pct loss" >&2; exit 2; }',
"'one_way_delay_ms':300,'loss_pct_each_direction':20,'loss_model':'iid',":
"'one_way_delay_ms':300,'loss_pct_each_direction':5,'loss_model':'iid',",
}
for old,new in repl.items():
    if s.count(old)!=1: raise SystemExit(f'capacity wrapper marker drift: {old[:50]} count={s.count(old)}')
    s=s.replace(old,new,1)
p.write_text(s)
PY
chmod +x "$CAP"

export CANDIDATE="size-probe-${LABEL}-constant5"
export SOURCE_SHA="$PRODUCT_SOURCE_SHA"
export TEST_DURATION_SEC=20
export LOSS_PCT=5
export TRAFFIC_PROFILE=realistic-mix-v1
export LOSS_MODEL=iid
export WBD_HELPER_DIR="$PATCHED_HELPER"
export INNER_MTU="$INNER"
export LINK_PLAINTEXT_MTU="$LINK"
export GH_TOKEN="${GH_TOKEN:?GH_TOKEN required}"
set +e
bash "$CAP" "$PRODUCT_DIR" "$RUNNER_TEMP"
run_rc=$?
set -e

LOG="$RUNNER_TEMP/singlelane-$CANDIDATE"
OUT="$RUNNER_TEMP/size-failure-$LABEL"
mkdir -p "$OUT"
python3 - "$LOG" "$OUT/failure-context.json" "$TEST_RATE_BPS" "$CONNECTION" "$CARRIER" "$DTLS" "$LINK" "$INNER" <<'PY'
import glob,json,os,re,sys
root,out,rate,connection,carrier,dtls,link,inner=sys.argv[1:]
patterns=re.compile(r'(fec: packet too large|linkdata (?:encode|decode)|WBD_LINK_PROXY_FAIL|lane_fail=[1-9]|dormant_drop=[1-9]|connection refused)',re.I)
records=[]
for path in sorted(glob.glob(os.path.join(root,'*.log'))):
    try: lines=open(path,errors='replace').read().splitlines()
    except OSError: continue
    for i,line in enumerate(lines):
        if patterns.search(line):
            lo=max(0,i-3); hi=min(len(lines),i+4)
            records.append({'file':os.path.basename(path),'line_no':i+1,'line':line,
                            'context':[{'line_no':j+1,'line':lines[j]} for j in range(lo,hi)]})
first_size=next((x for x in records if 'packet too large' in x['line'].lower() or 'linkdata encode' in x['line'].lower() or 'linkdata decode' in x['line'].lower()),None)
obj={'rate_bps':int(rate),'mtu':{'connection':int(connection),'carrier':int(carrier),'dtls_plain':int(dtls),'link_plain':int(link),'inner':int(inner)},
     'record_count':len(records),'first_size_failure':first_size,'records':records[:40]}
os.makedirs(os.path.dirname(out),exist_ok=True)
open(out,'w').write(json.dumps(obj,indent=2,sort_keys=True)+'\n')
print('WBD_SIZE_FAILURE_CONTEXT '+json.dumps({'rate_bps':int(rate),'record_count':len(records),'first_size_failure':first_size},sort_keys=True))
for r in records[:20]:
    print(f"WBD_FAILURE_LINE file={r['file']} line={r['line_no']} text={r['line']}")
PY

# Preserve both context and original logs regardless of the expected failing run.
cp -a "$LOG" "$OUT/logs" 2>/dev/null || true
exit "$run_rc"
