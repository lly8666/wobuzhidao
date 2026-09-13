#!/usr/bin/env bash
set -uo pipefail
PRODUCT_DIR=${1:?usage: run_transient_profile_300ms_v2.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}

set +e
bash "${GITHUB_WORKSPACE:?}/suite/.github/scripts/run_transient_profile_300ms.sh" "$PRODUCT_DIR" "$HELPER_DIR"
run_rc=$?
set -e

case "${TEST_RATE_BPS:?}" in
  20000000) rate_label=20m ;;
  30000000) rate_label=30m ;;
  *) exit "$run_rc" ;;
esac
spike=${SPIKE_LOSS_PCT:?}
audit="${RUNNER_TEMP:?}/transient-${rate_label}-5-${spike}-5-300ms-120s/transient-audit.json"

if [[ -f "$audit" ]]; then
python3 - "$audit" "$spike" <<'PY'
import json,sys
p=sys.argv[1]; spike=int(sys.argv[2]); label=f'spike{spike}'
d=json.load(open(p))
raw=d.get('netem') or {}

def snap(phase,side):
    x=((raw.get(phase) or {}).get(side) or {})
    return {'packets':int(x.get('packets',0) or 0),'drops':int(x.get('drops',0) or 0),'overlimits':int(x.get('overlimits',0) or 0)}

def delta(end,start):
    packets=max(0,end['packets']-start['packets'])
    drops=max(0,end['drops']-start['drops'])
    over=max(0,end['overlimits']-start['overlimits'])
    denom=packets+drops
    return {'packets':packets,'drops':drops,'overlimits':over,'loss_ratio':(drops/denom if denom else None)}

zero={'packets':0,'drops':0,'overlimits':0}
corrected={}
for side in ('client','server'):
    a=snap('pre5',side); b=snap(label,side); c=snap('post5',side)
    corrected.setdefault('pre5',{})[side]=delta(a,zero)
    corrected.setdefault(label,{})[side]=delta(b,a)
    corrected.setdefault('post5',{})[side]=delta(c,b)

def ok(v,target):
    if v is None: return False
    lo,hi=((0.035,0.065) if target==5 else ((0.17,0.23) if target==20 else (0.27,0.33)))
    return lo <= float(v) <= hi
phase_ok=all(ok(corrected[ph][side]['loss_ratio'],target)
             for ph,target in [('pre5',5),(label,spike),('post5',5)]
             for side in ('client','server'))
d['netem_raw_cumulative']=raw
d['netem']=corrected
d.setdefault('checks',{})['netem_phase_loss_clean']=phase_ok
checks=d['checks']
d['valid_for_capacity_conclusion']=bool(
    checks.get('transport_frame_integrity_clean') and phase_ok and
    checks.get('unified_mtu_budget_applied') and checks.get('fec_diag_present') and
    checks.get('pending4096_diag_present') and checks.get('host_kernel_pressure_clean'))
with open(p,'w') as f: json.dump(d,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_TRANSIENT_PHASE_CORRECTED '+json.dumps({'spike_loss_pct':spike,'phase_loss':corrected,'phase_loss_clean':phase_ok},sort_keys=True))
print('WBD_TRANSIENT_PROFILE_AUDIT_V2 '+json.dumps(d,sort_keys=True))
PY
else
  echo "WBD_TRANSIENT_PHASE_CORRECT_MISSING audit=$audit" >&2
fi
exit "$run_rc"
