#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: run_transient_5_30_5_60s.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${PRODUCT_SOURCE_SHA:?PRODUCT_SOURCE_SHA required}"
: "${HELPER_SOURCE_SHA:?HELPER_SOURCE_SHA required}"
: "${GH_TOKEN:?GH_TOKEN required}"

case "$TEST_RATE_BPS" in
  20000000) RATE_LABEL=20m ;;
  30000000) RATE_LABEL=30m ;;
  *) echo "TEST_RATE_BPS must be 20000000 or 30000000" >&2; exit 2 ;;
esac

test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$PRODUCT_SOURCE_SHA"
test "$(git -C "$HELPER_DIR" rev-parse HEAD)" = "$HELPER_SOURCE_SHA"

PATCHED_HELPER="${RUNNER_TEMP:?}/helper-${RATE_LABEL}-60s"
OUT="${RUNNER_TEMP}/transient-${RATE_LABEL}-5-30-5-60s"
CANDIDATE="transient-${RATE_LABEL}-5to30to5-60s"
LOG="${RUNNER_TEMP}/singlelane-${CANDIDATE}"
rm -rf "$PATCHED_HELPER" "$OUT"
cp -a "$HELPER_DIR" "$PATCHED_HELPER"
mkdir -p "$OUT"

# The pinned helper is intentionally 5 Mbps / 45 s / 15 s. Generalize only
# its test-local guards/accounting while keeping product source untouched.
python3 - "$PATCHED_HELPER" "$TEST_RATE_BPS" <<'PY'
from pathlib import Path
import sys
root=Path(sys.argv[1]); rate=int(sys.argv[2])

p=root/'.github/scripts/instrument_singlelane_5m_profile.py'
s=p.read_text()
if s.count('5000000') < 2:
    raise SystemExit('rate-profile helper drift')
s=s.replace('5000000',str(rate)).replace('duration_sec=45','duration_sec=90')
p.write_text(s)

p=root/'.github/scripts/instrument_singlelane_transient_spike.py'
s=p.read_text()
old="if (base_loss, spike_loss, spike_start, spike_duration, duration, rate_bps) != (5, 30, 15, 15, 45, 5000000):"
new=f"if (base_loss, spike_loss, spike_start, spike_duration, duration, rate_bps) != (5, 30, 15, 60, 90, {rate}):"
if s.count(old)!=1: raise SystemExit('transient tuple guard drift')
s=s.replace(old,new,1)
s=s.replace("raise SystemExit('transient spike probe is pinned to 5Mbps, 45s, 5% -> 30% for 15s -> 5%')",
            f"raise SystemExit('transient spike probe is pinned to {rate}bps, 90s, 5% -> 30% for 60s -> 5%')",1)
if s.count("phase_bounds=(15.0,30.0)")!=1: raise SystemExit('phase-bound drift')
s=s.replace("phase_bounds=(15.0,30.0)","phase_bounds=(15.0,75.0)",1)
old='''    sleep 15\n    transient_qdisc_snapshot spike30-end\n    transient_qdisc_set 5\n    echo "WBD_TRANSIENT_NETEM phase=post5 start_sec=30 loss_pct=5"'''
new='''    sleep 60\n    transient_qdisc_snapshot spike30-end\n    transient_qdisc_set 5\n    echo "WBD_TRANSIENT_NETEM phase=post5 start_sec=75 loss_pct=5"'''
if s.count(old)!=1: raise SystemExit('middle-spike sleep marker drift')
s=s.replace(old,new,1)
s=s.replace("print('WBD_TRANSIENT_SPIKE_PATCHED rate_bps=5000000 duration_sec=45 pre_loss=5 spike_loss=30 spike_start=15 spike_duration=15')",
            f"print('WBD_TRANSIENT_SPIKE_PATCHED rate_bps={rate} duration_sec=90 pre_loss=5 spike_loss=30 spike_start=15 spike_duration=60')",1)
p.write_text(s)

p=root/'.github/scripts/run_singlelane_ab_observability.sh'
s=p.read_text()
repls={
  ': "${TEST_DURATION_SEC:=45}"': ': "${TEST_DURATION_SEC:=90}"',
  ': "${SPIKE_DURATION_SEC:=15}"': ': "${SPIKE_DURATION_SEC:=60}"',
  '[[ "$TEST_RATE_BPS" == 5000000 && "$TEST_DURATION_SEC" == 45 ]] || { echo "transient probe requires 5Mbps/45s" >&2; exit 2; }':
    f'[[ "$TEST_RATE_BPS" == {rate} && "$TEST_DURATION_SEC" == 90 ]] || {{ echo "transient probe requires {rate}bps/90s" >&2; exit 2; }}',
  '[[ "$SPIKE_LOSS_PCT" == 30 && "$SPIKE_START_SEC" == 15 && "$SPIKE_DURATION_SEC" == 15 ]] || { echo "transient probe requires 30pct spike from t=15s for 15s" >&2; exit 2; }':
    '[[ "$SPIKE_LOSS_PCT" == 30 && "$SPIKE_START_SEC" == 15 && "$SPIKE_DURATION_SEC" == 60 ]] || { echo "transient probe requires 30pct spike from t=15s for 60s" >&2; exit 2; }',
  "'spike_loss_pct_each_direction':30,'spike_start_sec':15,'spike_duration_sec':15,":
    "'spike_loss_pct_each_direction':30,'spike_start_sec':15,'spike_duration_sec':60,",
  "'rate_bps_each_direction':5000000,'duration_sec':45,":
    f"'rate_bps_each_direction':{rate},'duration_sec':90,",
}
for old,new in repls.items():
    if s.count(old)!=1: raise SystemExit(f'wrapper marker drift: {old!r}: {s.count(old)}')
    s=s.replace(old,new,1)
p.write_text(s)
PY

python3 -m py_compile \
  "$PATCHED_HELPER/.github/scripts/instrument_singlelane_5m_profile.py" \
  "$PATCHED_HELPER/.github/scripts/instrument_singlelane_transient_spike.py"
bash -n "$PATCHED_HELPER/.github/scripts/run_singlelane_ab_observability.sh"

export CANDIDATE SOURCE_SHA="$PRODUCT_SOURCE_SHA"
export LOSS_PCT=5 TRAFFIC_PROFILE=realistic-mix-v1 LOSS_MODEL=transient
export TEST_DURATION_SEC=90 TEST_RATE_BPS
export SPIKE_LOSS_PCT=30 SPIKE_START_SEC=15 SPIKE_DURATION_SEC=60
export WOLFSSL_SOURCE_ARTIFACT_ID=9529710833
export WBD_HELPER_DIR="$PATCHED_HELPER"

set +e
bash "$PATCHED_HELPER/.github/scripts/run_singlelane_ab_observability.sh" "$PRODUCT_DIR" "$RUNNER_TEMP" 2>&1 | tee "$OUT/action.log"
run_rc=${PIPESTATUS[0]}
set -e

if [[ -d "$LOG" ]]; then
  cp -a "$LOG" "$OUT/runtime-log"
fi

python3 - "$OUT" "$LOG" "$TEST_RATE_BPS" "$PRODUCT_SOURCE_SHA" "$HELPER_SOURCE_SHA" "${GITHUB_SHA:-unknown}" <<'PY'
import glob,json,os,re,sys
out,logdir,rate,product_sha,helper_sha,suite_sha=sys.argv[1:]
rate=int(rate)
def read(name):
    p=os.path.join(logdir,name)
    try: return json.load(open(p))
    except Exception: return {}
load=read('load-result.json')
ab=read('ab-transient-result.json')
resource=read('resource-metrics.json')
host=read('host-pressure.json')
netem=read('transient-netem-result.json')
shadow=read('shadow-matrix-result.json')

all_text=''
carrier=[]
fec_markers=[]
if os.path.isdir(logdir):
  for path in sorted(glob.glob(os.path.join(logdir,'*.log'))):
    try: text=open(path,errors='replace').read()
    except OSError: continue
    all_text+='\n'+text
    for m in re.finditer(r'WBD_CARRIER_FRAGMENT_STATS\s+(\{[^\n]+\})',text):
      try:
        obj=json.loads(m.group(1)); obj['file']=os.path.basename(path); carrier.append(obj)
      except Exception: pass
    for line in text.splitlines():
      if 'WBD_FEC_' in line or 'rx_horizon_' in line or 'reconstruct_' in line:
        fec_markers.append({'file':os.path.basename(path),'line':line[:2000]})

patterns={
 'link_proxy_fail':r'WBD_LINK_PROXY_FAIL',
 'fec_packet_too_large':r'fec: packet too large',
 'connection_refused':r'connection refused',
 'lane_fail_nonzero':r'lane_fail=[1-9][0-9]*',
 'dormant_drop_nonzero':r'dormant_drop=[1-9][0-9]*',
}
errors={k:len(re.findall(v,all_text,re.I)) for k,v in patterns.items()}
carrier_agg={
 'samples':len(carrier),
 'datagrams':sum(int(x.get('datagrams',0) or 0) for x in carrier),
 'fragmented_datagrams':sum(int(x.get('fragmented_datagrams',0) or 0) for x in carrier),
 'frames':sum(int(x.get('frames',0) or 0) for x in carrier),
 'input_bytes':sum(int(x.get('input_bytes',0) or 0) for x in carrier),
 'payload_budgets':sorted(set(int(x.get('payload_budget',0) or 0) for x in carrier)),
}
carrier_agg['fragmented_ratio']=(carrier_agg['fragmented_datagrams']/carrier_agg['datagrams'] if carrier_agg['datagrams'] else None)

client_udp=host.get('client_udp_delta') or {}
server_udp=host.get('server_udp_delta') or {}
def udp_bad(d):
    return sum(int(d.get(k,0) or 0) for k in ('InErrors','RcvbufErrors','SndbufErrors'))
host_anomaly=(int(host.get('softnet_dropped_delta',0) or 0)+int(host.get('softnet_time_squeeze_delta',0) or 0)+udp_bad(client_udp)+udp_bad(server_udp))
checks={
 'result_present':bool(ab or load),
 'rate_matches':int((ab or {}).get('rate_bps_each_direction', rate) or rate)==rate,
 'phase_metrics_present':bool((ab or {}).get('phase_metrics') or load.get('phase_metrics')),
 'netem_phase_metrics_present':all(k in netem for k in ('pre5','spike30','post5')),
 'no_payload_corruption':int((ab or {}).get('bad_payload',load.get('bad_payload',0)) or 0)==0,
 'no_size_or_lane_failure':sum(errors.values())==0,
 'host_kernel_pressure_clean':host_anomaly==0,
}
audit={
 'product_source_sha':product_sha,'helper_source_sha':helper_sha,'suite_overlay_sha':suite_sha,
 'rate_bps_each_direction':rate,'duration_sec':90,
 'loss_profile':{'pre5_sec':15,'spike30_sec':60,'post5_sec':15},
 'app':ab or load,'resource':resource,'host_pressure':host,'netem':netem,'shadow':shadow,
 'carrier_fragment_aggregate':carrier_agg,'carrier_fragment_samples':carrier,
 'fec_marker_count':len(fec_markers),'fec_markers_tail':fec_markers[-120:],
 'errors':errors,'host_kernel_anomaly_count':host_anomaly,'checks':checks,
 'clean_transport':all(checks.values()),
}
with open(os.path.join(out,'transient-audit.json'),'w') as f:
    json.dump(audit,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_TRANSIENT_60S_AUDIT '+json.dumps(audit,sort_keys=True))
PY

# Do not turn expected high-loss application loss into a synthetic pass/fail gate.
# The underlying harness may still fail on corruption or recovery invariants.
if (( run_rc != 0 )); then
  exit "$run_rc"
fi
