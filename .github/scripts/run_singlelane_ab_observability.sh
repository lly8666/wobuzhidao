#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?product checkout required}
TMP_ROOT=${2:?runner temp required}
: "${CANDIDATE:?candidate required}"
: "${SOURCE_SHA:?source sha required}"
: "${LOSS_PCT:?baseline loss required}"
: "${TRAFFIC_PROFILE:=realistic-mix-v1}"
: "${LOSS_MODEL:=transient}"
: "${TEST_RATE_BPS:=5000000}"
: "${TEST_DURATION_SEC:=45}"
: "${SPIKE_LOSS_PCT:=30}"
: "${SPIKE_START_SEC:=15}"
: "${SPIKE_DURATION_SEC:=15}"

[[ "$LOSS_PCT" == 5 ]] || { echo "transient probe baseline loss must be 5" >&2; exit 2; }
[[ "$LOSS_MODEL" == transient ]] || { echo "transient probe LOSS_MODEL must be transient" >&2; exit 2; }
[[ "$TRAFFIC_PROFILE" == realistic-mix-v1 ]] || { echo "transient probe requires realistic-mix-v1" >&2; exit 2; }
[[ "$TEST_RATE_BPS" == 5000000 && "$TEST_DURATION_SEC" == 45 ]] || { echo "transient probe requires 5Mbps/45s" >&2; exit 2; }
[[ "$SPIKE_LOSS_PCT" == 30 && "$SPIKE_START_SEC" == 15 && "$SPIKE_DURATION_SEC" == 15 ]] || { echo "transient probe requires 30pct spike from t=15s for 15s" >&2; exit 2; }

HELPER_DIR=${WBD_HELPER_DIR:-${GITHUB_WORKSPACE:?}/helper}
BASE="$HELPER_DIR/.github/scripts/validate_singlelane_shadow_candidate.sh"
RUNNER="$TMP_ROOT/validate-singlelane-ab-instrumented.sh"
cp "$BASE" "$RUNNER"
python3 - "$RUNNER" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
marker='bash -n scripts/game_lane_fullstack.sh\n'
insert='python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_observability.py" "$PRODUCT_DIR"\nTRAFFIC_PROFILE="$TRAFFIC_PROFILE" LOSS_MODEL=iid LOSS_PCT=20 python3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_realistic_burst.py" "$PRODUCT_DIR"\npython3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_host_pressure.py" "$PRODUCT_DIR"\npython3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_5m_profile.py" "$PRODUCT_DIR"\npython3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_singlelane_transient_spike.py" "$PRODUCT_DIR"\npython3 "${WBD_HELPER_DIR:?}/.github/scripts/instrument_transient_qdisc_change.py" "$PRODUCT_DIR"\n\n'
if s.count(marker) != 1:
    raise SystemExit('A/B wrapper: validator insertion marker drift')
s=s.replace(marker,insert+marker,1)
p.write_text(s)
PY
sed -i "s/loss_pct_each_direction=20/loss_pct_each_direction=${LOSS_PCT}_then_${SPIKE_LOSS_PCT}/" "$RUNNER"
sed -i "s/export NETEM_LOSS_PCT=20/export NETEM_LOSS_PCT=${LOSS_PCT}/" "$RUNNER"
sed -i "s/export DURATION_SEC=20/export DURATION_SEC=${TEST_DURATION_SEC}/" "$RUNNER"
sed -i "s/export RATE_BPS=20000000/export RATE_BPS=${TEST_RATE_BPS}/" "$RUNNER"
sed -i "s/rate_bps=20000000/rate_bps=${TEST_RATE_BPS}/" "$RUNNER"
chmod +x "$RUNNER"

export WBD_HELPER_DIR="$HELPER_DIR"
export TRAFFIC_PROFILE LOSS_MODEL LOSS_PCT TEST_RATE_BPS TEST_DURATION_SEC
export SPIKE_LOSS_PCT SPIKE_START_SEC SPIKE_DURATION_SEC
set +e
bash "$RUNNER" "$PRODUCT_DIR" "$TMP_ROOT"
rc=$?
set -e

LOG="$TMP_ROOT/singlelane-$CANDIDATE"
python3 - "$LOG" "$CANDIDATE" "$SOURCE_SHA" "$LOSS_PCT" "$TRAFFIC_PROFILE" "$LOSS_MODEL" <<'PY'
import json, os, sys
root,candidate,source_sha,loss,profile,loss_model=sys.argv[1:]
def read(name):
    p=os.path.join(root,name)
    return json.load(open(p)) if os.path.exists(p) else {}
load=read('load-result.json')
resource=read('resource-metrics.json')
shadow=read('shadow-matrix-result.json')
host=read('host-pressure.json')
transient=read('transient-netem-result.json')
out={
 'candidate':candidate,'product_source_sha':source_sha,'baseline_loss_pct_each_direction':int(loss),
 'spike_loss_pct_each_direction':30,'spike_start_sec':15,'spike_duration_sec':15,
 'rate_bps_each_direction':5000000,'duration_sec':45,
 'traffic_profile':profile,'loss_model':loss_model,
 'sent':load.get('sent'),'received_unique':load.get('received_unique'),
 'app_loss_ratio':load.get('loss_ratio'),'byte_loss_ratio':load.get('byte_loss_ratio'),
 'avg_payload_bytes':load.get('avg_payload_bytes'),'sent_payload_bytes':load.get('sent_payload_bytes'),
 'received_payload_bytes':load.get('received_payload_bytes'),
 'goodput_mbps':((load.get('down_payload_bps') or 0)/1e6),
 'phase_metrics':load.get('phase_metrics'),
 'duplicates':load.get('duplicates'),'bad_payload':load.get('bad_payload'),
 'rtt_samples':load.get('rtt_samples'),'rtt_ms_p50':load.get('rtt_ms_p50'),
 'rtt_ms_p95':load.get('rtt_ms_p95'),'rtt_ms_p99':load.get('rtt_ms_p99'),
 'rtt_ms_max':load.get('rtt_ms_max'),
 'timely_1s_ratio':load.get('timely_1s_ratio'),'timely_2s_ratio':load.get('timely_2s_ratio'),
 'timely_5s_ratio':load.get('timely_5s_ratio'),
 'outer_attempted_mbps_aggregate':resource.get('outer_attempted_mbps_aggregate'),
 'outer_attempted_mbps_per_direction_mean':resource.get('outer_attempted_mbps_per_direction_mean'),
 'outer_post_netem_mbps_aggregate':resource.get('outer_post_netem_mbps_aggregate'),
 'outer_attempted_to_offered_inner_aggregate_ratio':resource.get('outer_attempted_to_offered_inner_aggregate_ratio'),
 'avg_cpu_cores':resource.get('avg_cpu_cores'),
 'avg_cpu_percent_machine':resource.get('avg_cpu_percent_machine'),
 'cpu_cores_by_component':resource.get('cpu_cores_by_component'),
 'host_cpu_busy_cores':host.get('host_cpu_busy_cores'),
 'host_cpu_busy_fraction':host.get('host_cpu_busy_fraction'),
 'host_cpu_count':host.get('host_cpu_count'),
 'softnet_dropped_delta':host.get('softnet_dropped_delta'),
 'softnet_time_squeeze_delta':host.get('softnet_time_squeeze_delta'),
 'client_udp_delta':host.get('client_udp_delta'),
 'server_udp_delta':host.get('server_udp_delta'),
 'transient_netem':transient,
 'repair_to_fresh_bytes':shadow.get('repair_to_fresh_bytes'),
 'peak_pending':shadow.get('peak_pending'),'peak_buffered_oo':shadow.get('peak_buffered_oo'),
 'repair_evicted':shadow.get('repair_evicted'),'forgiven_gaps':shadow.get('forgiven_gaps'),
 'fresh_blocked_by_repair':shadow.get('fresh_blocked_by_repair'),
}
print('WBD_AB_TRANSIENT_RESULT '+json.dumps(out,sort_keys=True))
if root and os.path.isdir(root):
    open(os.path.join(root,'ab-transient-result.json'),'w').write(json.dumps(out,sort_keys=True,indent=2)+'\n')
PY

exit "$rc"
