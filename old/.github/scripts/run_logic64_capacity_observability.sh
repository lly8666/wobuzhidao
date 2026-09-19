#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?product checkout required}
TMP_ROOT=${2:?runner temp required}
: "${CANDIDATE:?candidate required}"
: "${SOURCE_SHA:?source sha required}"
: "${TEST_RATE_BPS:?test rate required}"
: "${WBD_HELPER_DIR:=${GITHUB_WORKSPACE:?}/helper}"
TEST_DURATION_SEC=${TEST_DURATION_SEC:-45}
LOSS_PCT=${LOSS_PCT:-20}
TRAFFIC_PROFILE=${TRAFFIC_PROFILE:-realistic-mix-v1}
LOSS_MODEL=${LOSS_MODEL:-iid}

[[ "$TEST_RATE_BPS" =~ ^(10000000|15000000|20000000)$ ]] || { echo "TEST_RATE_BPS must be 10/15/20 Mbps" >&2; exit 2; }
[[ "$TEST_DURATION_SEC" == 45 ]] || { echo "capacity probe requires 45s" >&2; exit 2; }
[[ "$LOSS_PCT" == 20 && "$LOSS_MODEL" == iid ]] || { echo "capacity probe requires iid 20pct loss" >&2; exit 2; }
[[ "$TRAFFIC_PROFILE" == realistic-mix-v1 ]] || { echo "capacity probe requires realistic-mix-v1" >&2; exit 2; }

test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$SOURCE_SHA"

BASE="$WBD_HELPER_DIR/.github/scripts/validate_singlelane_shadow_candidate.sh"
RUNNER="$TMP_ROOT/validate-logic64-capacity-${TEST_RATE_BPS}.sh"
cp "$BASE" "$RUNNER"

# Inject observation and traffic-profile patches after the base validator has
# converted the product soak harness to one lane. This mutates only the runner
# checkout's test scripts; FEC/product Go sources stay byte-identical to SOURCE_SHA.
python3 - "$RUNNER" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
marker='bash -n scripts/game_lane_fullstack.sh\n'
insert=r'''python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_observability.py" "$PRODUCT_DIR"
TRAFFIC_PROFILE="$TRAFFIC_PROFILE" LOSS_MODEL="$LOSS_MODEL" LOSS_PCT="$LOSS_PCT" python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_realistic_burst.py" "$PRODUCT_DIR"
python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_host_pressure.py" "$PRODUCT_DIR"
python3 - "$PRODUCT_DIR" "$TEST_RATE_BPS" <<'PY_RATE'
from pathlib import Path
import sys
root=Path(sys.argv[1]); rate=sys.argv[2]
p=root/'scripts/game_lane_rotation_soak.sh'
s=p.read_text()
old='[[ "$RATE_BPS" == 20000000 ]] || { echo "RATE_BPS must be exactly 20000000" >&2; exit 2; }'
new=f'[[ "$RATE_BPS" == {rate} ]] || {{ echo "RATE_BPS must be exactly {rate}" >&2; exit 2; }}'
if s.count(old) != 1:
    raise SystemExit(f'capacity rate guard drift: {s.count(old)}')
p.write_text(s.replace(old,new,1))
print('WBD_LOGIC64_CAPACITY_RATE_PATCHED rate_bps='+rate)
PY_RATE

'''
if s.count(marker) != 1:
    raise SystemExit('capacity wrapper: validator insertion marker drift')
p.write_text(s.replace(marker,insert+marker,1))
PY

sed -i "s/loss_pct_each_direction=20/loss_pct_each_direction=${LOSS_PCT}/" "$RUNNER"
sed -i "s/export NETEM_LOSS_PCT=20/export NETEM_LOSS_PCT=${LOSS_PCT}/" "$RUNNER"
sed -i "s/export DURATION_SEC=20/export DURATION_SEC=${TEST_DURATION_SEC}/" "$RUNNER"
sed -i "s/export RATE_BPS=20000000/export RATE_BPS=${TEST_RATE_BPS}/" "$RUNNER"
sed -i "s/rate_bps=20000000/rate_bps=${TEST_RATE_BPS}/" "$RUNNER"
chmod +x "$RUNNER"

export TRAFFIC_PROFILE LOSS_MODEL LOSS_PCT TEST_RATE_BPS TEST_DURATION_SEC
set +e
bash "$RUNNER" "$PRODUCT_DIR" "$TMP_ROOT"
run_rc=$?
set -e

LOG="$TMP_ROOT/singlelane-$CANDIDATE"
set +e
python3 - "$LOG" "$CANDIDATE" "$SOURCE_SHA" "$TEST_RATE_BPS" "$TEST_DURATION_SEC" <<'PY'
import glob,json,os,sys
root,candidate,source_sha,rate,duration=sys.argv[1:]

def read(name):
    p=os.path.join(root,name)
    return json.load(open(p)) if os.path.exists(p) else {}

def fec_logs():
    out={}
    for p in sorted(glob.glob(os.path.join(root,'*.log'))):
        samples=[]
        try:
            lines=open(p,errors='replace')
        except OSError:
            continue
        for line in lines:
            marker='WBD_LINK_FEC_DIAG '
            if marker not in line: continue
            try: samples.append(json.loads(line.split(marker,1)[1].strip()))
            except Exception: pass
        if not samples: continue
        max_blocks=max(int(x.get('decoder',{}).get('max_blocks',0) or 0) for x in samples)
        current=[int(x.get('decoder',{}).get('in_flight',0) or 0) for x in samples]
        out[os.path.basename(p)]={
          'samples':len(samples),'max_blocks':max_blocks,
          'max_current_in_flight':max(current or [0]),
          'final_current_in_flight':current[-1] if current else 0,
          'peak_in_flight':max(int(x.get('peak_in_flight',0) or 0) for x in samples),
          'samples_at_max_blocks':sum(1 for v in current if max_blocks and v>=max_blocks),
          'samples_ge_90pct_max':sum(1 for v in current if max_blocks and v*10>=max_blocks*9),
          'peak_retired_incomplete':max(int(x.get('peak_retired_incomplete',0) or 0) for x in samples),
          'peak_retired_missing_sources':max(int(x.get('peak_retired_missing_sources',0) or 0) for x in samples),
          'pressure_retire_events':max(int(x.get('pressure_retire_events',0) or 0) for x in samples),
          'reconstruct_calls':max(int(x.get('reconstruct_calls',0) or 0) for x in samples),
          'reconstruct_success':max(int(x.get('reconstruct_success',0) or 0) for x in samples),
          'final_decoder':samples[-1].get('decoder',{}),
        }
    return out

load=read('load-result.json'); resource=read('resource-metrics.json'); host=read('host-pressure.json')
fec=fec_logs()
if not fec:
    raise SystemExit('missing WBD_LINK_FEC_DIAG samples')
max_blocks=max((v['max_blocks'] for v in fec.values()),default=0)
peak=max((v['peak_in_flight'] for v in fec.values()),default=0)
max_current=max((v['max_current_in_flight'] for v in fec.values()),default=0)
retired=max((v['peak_retired_incomplete'] for v in fec.values()),default=0)
retire_events=max((v['pressure_retire_events'] for v in fec.values()),default=0)
sat=sum(v['samples_at_max_blocks'] for v in fec.values())
near=sum(v['samples_ge_90pct_max'] for v in fec.values())
obj={
 'candidate':candidate,'product_source_sha':source_sha,
 'rate_bps_each_direction':int(rate),'duration_sec':int(duration),
 'one_way_delay_ms':300,'loss_pct_each_direction':20,'loss_model':'iid',
 'traffic_profile':'realistic-mix-v1','fec':'20:20',
 'sent':load.get('sent'),'received_unique':load.get('received_unique'),
 'app_loss_ratio':load.get('loss_ratio'),'byte_loss_ratio':load.get('byte_loss_ratio'),
 'goodput_mbps':((load.get('down_payload_bps') or 0)/1e6),
 'rtt_ms_p99':load.get('rtt_ms_p99'),'bad_payload':load.get('bad_payload'),
 'outer_attempted_mbps_per_direction_mean':resource.get('outer_attempted_mbps_per_direction_mean'),
 'avg_cpu_cores':resource.get('avg_cpu_cores'),'cpu_cores_by_component':resource.get('cpu_cores_by_component'),
 'host_cpu_busy_cores':host.get('host_cpu_busy_cores'),'host_cpu_count':host.get('host_cpu_count'),
 'softnet_dropped_delta':host.get('softnet_dropped_delta'),'softnet_time_squeeze_delta':host.get('softnet_time_squeeze_delta'),
 'client_udp_delta':host.get('client_udp_delta'),'server_udp_delta':host.get('server_udp_delta'),
 'client_netem_delta':host.get('client_netem_delta'),'server_netem_delta':host.get('server_netem_delta'),
 'fec_max_blocks':max_blocks,'fec_peak_in_flight':peak,'fec_max_current_in_flight':max_current,
 'fec_peak_retired_incomplete':retired,'fec_pressure_retire_events':retire_events,
 'fec_samples_at_max_blocks':sat,'fec_samples_ge_90pct_max':near,'fec_by_log':fec,
}
obj['capacity64_pressure'] = bool(max_blocks == 64 and (peak >= 64 or max_current >= 64 or sat > 0))
print('WBD_LOGIC64_CAPACITY_RESULT '+json.dumps(obj,sort_keys=True))
os.makedirs(root,exist_ok=True)
open(os.path.join(root,'logic64-capacity-result.json'),'w').write(json.dumps(obj,sort_keys=True,indent=2)+'\n')
PY
analysis_rc=$?
set -e

if (( analysis_rc != 0 )); then exit "$analysis_rc"; fi
exit "$run_rc"
