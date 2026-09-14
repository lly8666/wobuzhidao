#!/usr/bin/env bash
set -euo pipefail
BASE_PRODUCT=${1:?usage: run_measured_transient_300ms_v6.sh BASE_PRODUCT BASE_HELPER CASE RATE_BPS SPIKE_LOSS RCVBUF_EFFECTIVE PRODUCT_REF DUAL_PATCH}
BASE_HELPER=${2:?helper base required}
CASE_ID=${3:?case id required}
TEST_RATE_BPS=${4:?rate required}
SPIKE_LOSS_PCT=${5:?spike required}
RCVBUF_EFFECTIVE=${6:?effective rcvbuf required; 0 keeps product default}
PRODUCT_REF=${7:?product ref required}
DUAL_PATCH=${8:-false}
HELPER_REF=cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d

case "$TEST_RATE_BPS" in 5000000|15000000|20000000) ;; *) echo "unsupported measured rate $TEST_RATE_BPS" >&2; exit 2;; esac
case "$SPIKE_LOSS_PCT" in 20|30) ;; *) echo "unsupported spike $SPIKE_LOSS_PCT" >&2; exit 2;; esac
case "$RCVBUF_EFFECTIVE" in 0|2097152|8388608) ;; *) echo "rcvbuf effective must be 0, 2097152, or 8388608" >&2; exit 2;; esac

PRODUCT_DIR="$RUNNER_TEMP/product-${CASE_ID}"
HELPER_DIR="$RUNNER_TEMP/helper-${CASE_ID}"
rm -rf "$PRODUCT_DIR" "$HELPER_DIR"
cp -a "$BASE_PRODUCT" "$PRODUCT_DIR"
cp -a "$BASE_HELPER" "$HELPER_DIR"
test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$PRODUCT_REF"
test "$(git -C "$HELPER_DIR" rev-parse HEAD)" = "$HELPER_REF"

if [[ "$DUAL_PATCH" == true ]]; then
  python3 "$GITHUB_WORKSPACE/suite/.github/scripts/instrument_fec_dual_horizon5s_diag.py" "$PRODUCT_DIR"
fi
python3 "$GITHUB_WORKSPACE/suite/.github/scripts/instrument_fec_maxblocks640_diag.py" "$PRODUCT_DIR"
python3 "$GITHUB_WORKSPACE/suite/.github/scripts/instrument_server_link_rcvbuf_diag.py" "$PRODUCT_DIR"
python3 "$GITHUB_WORKSPACE/suite/.github/scripts/instrument_server_mux_perf_diag.py" "$PRODUCT_DIR"
python3 "$GITHUB_WORKSPACE/suite/.github/scripts/instrument_game_client_path_diag.py" "$PRODUCT_DIR"
gofmt -w \
  "$PRODUCT_DIR/cmd/wbd-link-server-mux/main.go" \
  "$PRODUCT_DIR/cmd/wbd-game-lane-client/main.go" \
  "$PRODUCT_DIR/cmd/wbd-link-proxy/main.go" \
  "$PRODUCT_DIR/internal/fec/block.go" \
  "$PRODUCT_DIR/internal/linkdata/path.go" 2>/dev/null || true
if [[ "$DUAL_PATCH" == true ]]; then
  gofmt -w "$PRODUCT_DIR/internal/fec/decoder_observe.go" "$PRODUCT_DIR/internal/fec/block_dual_horizon_test.go" "$PRODUCT_DIR/internal/linkdata/fec_observe.go"
fi
(
  cd "$PRODUCT_DIR"
  # Compile the patched sample without executing repository policy tests. Some
  # baseline MTU assertions are intentionally stale and are unrelated to this
  # diagnostic experiment; executing them here would prevent the sample from
  # starting while providing no additional compile-safety signal.
  go test ./cmd/wbd-link-server-mux ./cmd/wbd-game-lane-client ./internal/fec ./internal/linkdata -run '^$'
)

MEASURED_BASE="$RUNNER_TEMP/measured-base-${CASE_ID}.sh"
python3 - "$GITHUB_WORKSPACE/suite/.github/scripts/run_transient_profile_300ms.sh" "$MEASURED_BASE" <<'PY'
from pathlib import Path
import sys
src,dst=map(Path,sys.argv[1:])
s=src.read_text()
marker='python3 -m py_compile "$PATCHED_HELPER/.github/scripts/instrument_singlelane_5m_profile.py" \\\n  "$PATCHED_HELPER/.github/scripts/instrument_singlelane_transient_spike.py"\n'
insert='python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/instrument_singlelane_measurement_v2.py" "$PATCHED_HELPER" "${GITHUB_WORKSPACE:?}/suite/.github/scripts/paced_udp_load_v2.py"\n'
if s.count(marker)!=1:
    raise SystemExit(f'measurement insertion marker drift: {s.count(marker)}')
s=s.replace(marker,insert+marker,1)
dst.write_text(s)
PY
chmod +x "$MEASURED_BASE"

V3="$RUNNER_TEMP/measured-v3-${CASE_ID}.sh"
python3 - "$GITHUB_WORKSPACE/suite/.github/scripts/run_transient_profile_300ms_v3.sh" "$V3" <<'PY'
from pathlib import Path
import sys
src,dst=map(Path,sys.argv[1:]); s=src.read_text()
old='src="${GITHUB_WORKSPACE:?}/suite/.github/scripts/run_transient_profile_300ms.sh"'
new='src="${WBD_MEASURED_BASE_SCRIPT:?}"'
if s.count(old)!=1: raise SystemExit(f'v3 base marker drift: {s.count(old)}')
dst.write_text(s.replace(old,new,1))
PY
chmod +x "$V3"

case "$TEST_RATE_BPS" in
  5000000) RATE_LABEL=5m ;;
  15000000) RATE_LABEL=15m ;;
  20000000) RATE_LABEL=20m ;;
esac
OUT="$RUNNER_TEMP/transient-${RATE_LABEL}-5-${SPIKE_LOSS_PCT}-5-300ms-120s"
mkdir -p "$OUT"
DIAG_JSONL="$OUT/socket-runtime.jsonl"
rm -f "$DIAG_JSONL" "$DIAG_JSONL.pid"
sudo -E python3 "$GITHUB_WORKSPACE/suite/.github/scripts/runtime_socket_thread_diag.py" "$DIAG_JSONL" --interval 1.0 &
DIAG_LAUNCHER=$!
for _ in $(seq 1 50); do [[ -s "$DIAG_JSONL.pid" ]] && break; sleep .1; done
[[ -s "$DIAG_JSONL.pid" ]] || { echo "runtime diag failed to start" >&2; exit 41; }
DIAG_PID=$(tr -d '\r\n' <"$DIAG_JSONL.pid")

export TEST_RATE_BPS SPIKE_LOSS_PCT
export PRODUCT_SOURCE_SHA="$PRODUCT_REF"
export HELPER_SOURCE_SHA="$HELPER_REF"
export WBD_TEST_VARIANT="$CASE_ID"
export WBD_MEASURED_BASE_SCRIPT="$MEASURED_BASE"
export WBD_LOAD_PHASE_SPEC="pre5:0:30,spike${SPIKE_LOSS_PCT}:30:90,post5:90:120"
export WBD_SERVER_LINK_RCVBUF_EFFECTIVE_BYTES="$RCVBUF_EFFECTIVE"
export WBD_LOAD_DRAIN_SEC=15
set +e
bash "$V3" "$PRODUCT_DIR" "$HELPER_DIR"
RUN_RC=$?
set -e

sudo kill -TERM "$DIAG_PID" 2>/dev/null || true
wait "$DIAG_LAUNCHER" 2>/dev/null || true
sudo chmod -R a+rX "$OUT" 2>/dev/null || true
AUDIT="$OUT/transient-audit.json"
if [[ -s "$DIAG_JSONL" ]]; then
  python3 "$GITHUB_WORKSPACE/suite/.github/scripts/summarize_runtime_socket_diag.py" "$DIAG_JSONL" "$OUT/socket-runtime-summary.json" --spike "$SPIKE_LOSS_PCT" --audit "$AUDIT" || true
fi

python3 - "$AUDIT" "$OUT" "$CASE_ID" "$RCVBUF_EFFECTIVE" "$PRODUCT_REF" "$DUAL_PATCH" <<'PY'
import glob,json,os,re,sys
path,outdir,case,rcvbuf,product_ref,dual=sys.argv[1:]
rcvbuf=int(rcvbuf); dual=(dual=='true')
if not os.path.exists(path):
    raise SystemExit(f'audit missing: {path}')
d=json.load(open(path))
logdir=os.path.join(outdir,'logs')
def jread(name):
    p=os.path.join(logdir,name)
    try: return json.load(open(p))
    except Exception: return {}
load=jread('load-result.json')
# Promote the FEC640 test override without changing product defaults.
fec=d.pop('fec64',{}) or d.get('fec640',{}) or {}
max_blocks=int(fec.get('max_blocks',0) or 0)
peak=int(fec.get('peak_in_flight',0) or 0)
cur=int(fec.get('max_current_in_flight',0) or 0)
sat=int(fec.get('samples_at_max_blocks',0) or 0)
fec.pop('capacity64_pressure',None)
fec['configured_max_blocks']=640
fec['capacity640_pressure']=bool(max_blocks==640 and (peak>=640 or cur>=640 or sat>0))
d['fec640']=fec

# Low-rate cumulative path counters. They are diagnostic-only and printed once/sec.
def lines_with(marker):
    rows=[]
    for p in glob.glob(os.path.join(logdir,'**','*.log'),recursive=True):
        try: lines=open(p,errors='replace')
        except OSError: continue
        for line in lines:
            if marker in line: rows.append((p,line.strip()))
    return rows

def kv(line):
    return {k:(float(v) if '.' in v else int(v)) for k,v in re.findall(r'([A-Za-z0-9_]+)=([0-9.]+)',line)}

game_rows=[kv(x) for _,x in lines_with('WBD_GAME_PATH_DIAG ')]
game_final_rows=[kv(x) for _,x in lines_with('WBD_GAME_PATH_FINAL ')]
game_final=(game_final_rows[-1] if game_final_rows else (game_rows[-1] if game_rows else {}))
perf_rows=[]
for _,line in lines_with('WBD_LINK_MUX_PERF_DIAG '):
    r=kv(line)
    m=re.search(r'inbound_hist=\[([^]]+)\].*outbound_hist=\[([^]]+)\]',line)
    if m:
        r['inbound_hist']=[int(x) for x in m.group(1).split()]
        r['outbound_hist']=[int(x) for x in m.group(2).split()]
    perf_rows.append(r)
rcv_rows=[kv(x) for _,x in lines_with('WBD_LINK_RCVBUF_DIAG ')]
load_start_ns=None
try: load_start_ns=int(open(os.path.join(logdir,'load-start-epoch-ns.txt')).read().strip())
except Exception: pass

def final_delta(rows,key):
    if not rows: return None
    return rows[-1].get(key)

d['test_variant']=case
d['product_source_sha']=product_ref
d['dual_horizon_patch']=dual
d['load_measurement']={
  'target_bps':load.get('rate_target_bps'),
  'actual_injection_bps':load.get('actual_injection_bps'),
  'actual_injection_ratio':load.get('actual_injection_ratio'),
  'load_valid_for_capacity':load.get('load_valid_for_capacity'),
  'load_invalid_reason':load.get('load_invalid_reason'),
  'send_failures':load.get('send_failures'),'skipped_send_slots':load.get('skipped_send_slots'),
  'send_lag_ms_p50':load.get('send_lag_ms_p50'),'send_lag_ms_p99':load.get('send_lag_ms_p99'),'send_lag_ms_max':load.get('send_lag_ms_max'),
  'send_lag_sustained_increase':load.get('send_lag_sustained_increase'),
  'phase_metrics':load.get('phase_metrics'),'per_second':load.get('per_second'),
  'size_sent_counts':load.get('size_sent_counts'),'size_received_counts':load.get('size_received_counts'),
  'sent_payload_bytes':load.get('sent_payload_bytes'),'received_payload_bytes':load.get('received_payload_bytes'),
  'socket_buffers':load.get('socket_buffers'),'load_start_epoch_ns':load_start_ns,
}
d['game_path_diag']={
  'samples':len(game_rows),'final_samples':len(game_final_rows),'final':game_final,
  'app_rx_bytes':game_final.get('app_rx_bytes'),'lane_tx_bytes':game_final.get('lane_tx_bytes'),
  'lane_rx_bytes':game_final.get('lane_rx_bytes'),'app_tx_bytes':game_final.get('app_tx_bytes'),
  'lane_write_err':game_final.get('lane_write_err'),'app_write_err':game_final.get('app_write_err'),
}
d['link_mux_perf_diag']={
  'samples':len(perf_rows),'rows':perf_rows,
  'max_read_gap_us':max([int(x.get('read_gap_max_us_interval',0)) for x in perf_rows] or [0]),
  'max_inbound_us':max([int(x.get('inbound_max_us_interval',0)) for x in perf_rows] or [0]),
  'max_outbound_us':max([int(x.get('outbound_max_us_interval',0)) for x in perf_rows] or [0]),
  'final_service_write_bytes':final_delta(perf_rows,'service_write_bytes'),
  'final_service_read_bytes':final_delta(perf_rows,'service_read_bytes'),
}
d['server_link_rcvbuf']={
  'requested_effective_bytes':rcvbuf,
  'observed_marker':(rcv_rows[-1] if rcv_rows else {}),
  'exact_marker_match':bool(rcvbuf==0 or (rcv_rows and int(rcv_rows[-1].get('effective_bytes',-1))==rcvbuf)),
}
checks=d.setdefault('checks',{})
checks['fec_max_blocks_640_applied']=(max_blocks==640)
checks['load_injection_valid_99_101']=bool(load.get('load_valid_for_capacity'))
checks['server_link_rcvbuf_exact']=d['server_link_rcvbuf']['exact_marker_match']
d['sample_validity']={
  'load_valid':bool(load.get('load_valid_for_capacity')),
  'netem_valid':bool(checks.get('netem_phase_loss_clean')),
  'transport_integrity_valid':bool(checks.get('transport_frame_integrity_clean')),
  'rcvbuf_exact':d['server_link_rcvbuf']['exact_marker_match'],
  'valid_for_buffer_capacity_compare':bool(load.get('load_valid_for_capacity') and checks.get('netem_phase_loss_clean') and checks.get('transport_frame_integrity_clean') and d['server_link_rcvbuf']['exact_marker_match']),
}
d['diagnostic_scope']={
  'product_behavior_changes':['test_only_fec_max_blocks_640','selected_server_link_socket_rcvbuf_only'] + (['fec_dual_horizon_candidate'] if dual else []),
  'observation_only':['actual_tx_load_accounting','game_path_counters','link_mux_read_gap_and_timing','socket_queue_and_drop_sampling','faketcp_pending4096_counters'],
  'fairness_enabled':False,'pending4096_behavior_changed':False,
}
with open(path,'w') as f: json.dump(d,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_MEASURED_TRANSIENT_AUDIT '+json.dumps({'case':case,'validity':d['sample_validity'],'load':d['load_measurement'],'game':d['game_path_diag'],'perf':{k:v for k,v in d['link_mux_perf_diag'].items() if k!='rows'},'rcvbuf':d['server_link_rcvbuf']},sort_keys=True))
PY

cat "$AUDIT"
exit "$RUN_RC"
