#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: run_transient_unified_mtu_5_30_5_60s.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${PRODUCT_SOURCE_SHA:?PRODUCT_SOURCE_SHA required}"
: "${HELPER_SOURCE_SHA:?HELPER_SOURCE_SHA required}"

case "$TEST_RATE_BPS" in
  20000000) RATE_LABEL=20m ;;
  30000000) RATE_LABEL=30m ;;
  *) echo "TEST_RATE_BPS must be 20000000 or 30000000" >&2; exit 2 ;;
esac

test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$PRODUCT_SOURCE_SHA"
test "$(git -C "$HELPER_DIR" rev-parse HEAD)" = "$HELPER_SOURCE_SHA"

# Derive the runtime limits from the exact product source. The transient harness
# launches low-level tools directly, so it must explicitly pass the same values
# the product launcher would derive from the operator-visible connection MTU.
BUDGET_GO="$PRODUCT_DIR/mtu_transient_budget_tmp.go"
cleanup_budget() { rm -f "$BUDGET_GO"; }
trap cleanup_budget EXIT
cat >"$BUDGET_GO" <<'GO'
package main
import (
  "fmt"
  "github.com/lly8666/wobuzhidao/internal/pathmtu"
)
func main() {
  b, err := pathmtu.Derive(1500, pathmtu.Features{FEC:true, Game:true})
  if err != nil { panic(err) }
  fmt.Printf("%d %d %d %d %d %d %d\n", b.ConnectionMTU, b.CarrierPayloadMTU, b.DTLSPlaintextMTU, b.LinkPlaintextMTU, b.InnerMTU, b.FECOverhead, b.GameOverhead)
}
GO
read -r CONNECTION_MTU CARRIER_PAYLOAD_MTU DTLS_PLAINTEXT_MTU LINK_PLAINTEXT_MTU INNER_MTU FEC_OVERHEAD GAME_OVERHEAD < <(
  cd "$PRODUCT_DIR" && go run ./mtu_transient_budget_tmp.go
)
rm -f "$BUDGET_GO"
[[ "$CONNECTION_MTU" == 1500 ]]
[[ "$CARRIER_PAYLOAD_MTU" == 1460 ]]
echo "WBD_TRANSIENT_MTU_BUDGET connection=$CONNECTION_MTU carrier=$CARRIER_PAYLOAD_MTU dtls_plain=$DTLS_PLAINTEXT_MTU link_plain=$LINK_PLAINTEXT_MTU inner=$INNER_MTU fec_overhead=$FEC_OVERHEAD game_overhead=$GAME_OVERHEAD"

# The pinned helper's realistic-mix profile predates unified path MTU and has a
# hard-coded 1360-byte top bucket. Patch only the test-local copy so its maximum
# business datagram exactly follows the current product's derived InnerMTU.
PATCHED_BASE="${RUNNER_TEMP:?}/helper-unified-mtu-${RATE_LABEL}"
rm -rf "$PATCHED_BASE"
cp -a "$HELPER_DIR" "$PATCHED_BASE"
python3 - "$PATCHED_BASE/.github/scripts/instrument_singlelane_realistic_burst.py" "$INNER_MTU" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); inner=int(sys.argv[2]); s=p.read_text()
old="size_cycle=([64]*35+[128]*15+[256]*10+[512]*10+[1000]*10+[1200]*5+[1360]*15)"
new=f"size_cycle=([64]*35+[128]*15+[256]*10+[512]*10+[1000]*10+[1200]*5+[{inner}]*15)"
if s.count(old)!=1:
    raise SystemExit(f'realistic-mix max-MTU marker drift: {s.count(old)}')
s=s.replace(old,new,1)
s=s.replace('# 1360B is the configured inner MTU in this harness.',
            f'# {inner}B is the exact unified-path derived inner MTU for connection MTU 1500.',1)
p.write_text(s)
PY
python3 -m py_compile "$PATCHED_BASE/.github/scripts/instrument_singlelane_realistic_burst.py"

export INNER_MTU LINK_PLAINTEXT_MTU
export WBD_EXPECT_CONNECTION_MTU="$CONNECTION_MTU"
export WBD_EXPECT_CARRIER_PAYLOAD_MTU="$CARRIER_PAYLOAD_MTU"
export WBD_EXPECT_DTLS_PLAINTEXT_MTU="$DTLS_PLAINTEXT_MTU"
export WBD_EXPECT_FEC_OVERHEAD="$FEC_OVERHEAD"
export WBD_EXPECT_GAME_OVERHEAD="$GAME_OVERHEAD"

set +e
bash "${GITHUB_WORKSPACE:?}/suite/.github/scripts/run_transient_5_30_5_60s.sh" "$PRODUCT_DIR" "$PATCHED_BASE"
run_rc=$?
set -e

OUT="${RUNNER_TEMP}/transient-${RATE_LABEL}-5-30-5-60s"
LOG="${RUNNER_TEMP}/singlelane-transient-${RATE_LABEL}-5to30to5-60s"
AUDIT="$OUT/transient-audit.json"
python3 - "$AUDIT" "$LOG" "$CONNECTION_MTU" "$CARRIER_PAYLOAD_MTU" "$DTLS_PLAINTEXT_MTU" "$LINK_PLAINTEXT_MTU" "$INNER_MTU" "$FEC_OVERHEAD" "$GAME_OVERHEAD" <<'PY'
import glob,json,os,re,sys
path,logdir,connection,carrier,dtls,link,inner,fec_ov,game_ov=sys.argv[1:]
connection,carrier,dtls,link,inner,fec_ov,game_ov=map(int,(connection,carrier,dtls,link,inner,fec_ov,game_ov))
try:
    audit=json.load(open(path))
except Exception:
    audit={}
raw=audit.get('netem') or {}
def sample(phase,side):
    d=((raw.get(phase) or {}).get(side) or {})
    return int(d.get('packets',0) or 0), int(d.get('drops',0) or 0)
def delta(cur,prev):
    p=max(0,cur[0]-prev[0]); d=max(0,cur[1]-prev[1]); den=p+d
    return {'packets':p,'drops':d,'loss_ratio':(d/den if den else None)}
phase_delta={}
for side in ('client','server'):
    pre=sample('pre5',side); spike=sample('spike30',side); post=sample('post5',side)
    phase_delta.setdefault('pre5',{})[side]=delta(pre,(0,0))
    phase_delta.setdefault('spike30',{})[side]=delta(spike,pre)
    phase_delta.setdefault('post5',{})[side]=delta(post,spike)

def in_range(v,lo,hi): return v is not None and lo <= v <= hi
netem_clean=True
for side in ('client','server'):
    netem_clean &= in_range(phase_delta['pre5'][side]['loss_ratio'],0.035,0.065)
    netem_clean &= in_range(phase_delta['spike30'][side]['loss_ratio'],0.27,0.33)
    netem_clean &= in_range(phase_delta['post5'][side]['loss_ratio'],0.035,0.065)

all_text=''
if os.path.isdir(logdir):
    for p in glob.glob(os.path.join(logdir,'*.log')):
        try: all_text+='\n'+open(p,errors='replace').read()
        except OSError: pass
runtime_carrier=sorted(set(int(x) for x in re.findall(r'READY role=(?:client|server(?:-mux)?)[^\n]*carrier_mtu=(\d+)',all_text)))
agg=audit.get('carrier_fragment_aggregate') or {}
payload_budgets=sorted(set(int(x) for x in (agg.get('payload_budgets') or [])))
checks=audit.setdefault('checks',{})
checks['unified_mtu_budget_applied']=link>0 and inner>0 and inner < link < dtls < carrier < connection
checks['carrier_payload_budget_matches']=payload_budgets in ([carrier],[]) 
checks['runtime_carrier_mtu_matches']=runtime_carrier in ([connection],[])
checks['netem_phase_delta_clean']=bool(netem_clean)
audit['mtu_budget']={
    'connection_mtu':connection,'carrier_payload_mtu':carrier,'dtls_plaintext_mtu':dtls,
    'link_plaintext_mtu':link,'inner_mtu':inner,'fec_overhead':fec_ov,'game_overhead':game_ov,
}
audit['runtime_carrier_mtu_values']=runtime_carrier
audit['netem_phase_delta']=phase_delta
# Capacity conclusions are valid only if the transport stayed alive, netem hit
# the intended phases, and the hosted runner/kernel did not itself drop data.
audit['valid_for_capacity_conclusion']=bool(
    checks.get('no_payload_corruption') and checks.get('no_size_or_lane_failure') and
    checks.get('host_kernel_pressure_clean') and checks.get('unified_mtu_budget_applied') and
    checks.get('carrier_payload_budget_matches') and checks.get('runtime_carrier_mtu_matches') and
    checks.get('netem_phase_delta_clean')
)
with open(path,'w') as f:
    json.dump(audit,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_TRANSIENT_UNIFIED_MTU_AUDIT '+json.dumps({
    'mtu_budget':audit['mtu_budget'],'runtime_carrier_mtu_values':runtime_carrier,
    'netem_phase_delta':phase_delta,'carrier_fragment_aggregate':agg,
    'checks':checks,'valid_for_capacity_conclusion':audit['valid_for_capacity_conclusion'],
    'app_loss_ratio':(audit.get('app') or {}).get('app_loss_ratio'),
    'goodput_mbps':(audit.get('app') or {}).get('goodput_mbps'),
    'avg_cpu_cores':(audit.get('resource') or {}).get('avg_cpu_cores'),
    'host_cpu_busy_cores':(audit.get('host_pressure') or {}).get('host_cpu_busy_cores'),
    'host_kernel_anomaly_count':audit.get('host_kernel_anomaly_count'),
    'errors':audit.get('errors'),
},sort_keys=True))
PY

exit "$run_rc"
