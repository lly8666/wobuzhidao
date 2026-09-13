#!/usr/bin/env bash
set -euo pipefail

PRODUCT_DIR=${1:?usage: run_transient_profile_300ms.sh PRODUCT_DIR HELPER_DIR}
HELPER_DIR=${2:?helper checkout required}
: "${TEST_RATE_BPS:?TEST_RATE_BPS required}"
: "${SPIKE_LOSS_PCT:?SPIKE_LOSS_PCT required}"
: "${PRODUCT_SOURCE_SHA:?PRODUCT_SOURCE_SHA required}"
: "${HELPER_SOURCE_SHA:?HELPER_SOURCE_SHA required}"

case "$TEST_RATE_BPS" in
  20000000) RATE_LABEL=20m ;;
  30000000) RATE_LABEL=30m ;;
  *) echo "TEST_RATE_BPS must be 20000000 or 30000000" >&2; exit 2 ;;
esac
case "$SPIKE_LOSS_PCT" in
  20|30) ;;
  *) echo "SPIKE_LOSS_PCT must be 20 or 30" >&2; exit 2 ;;
esac

test "$(git -C "$PRODUCT_DIR" rev-parse HEAD)" = "$PRODUCT_SOURCE_SHA"
test "$(git -C "$HELPER_DIR" rev-parse HEAD)" = "$HELPER_SOURCE_SHA"
echo "WBD_TRANSIENT_SOURCE product_sha=$PRODUCT_SOURCE_SHA helper_sha=$HELPER_SOURCE_SHA rate_bps=$TEST_RATE_BPS one_way_delay_ms=300 loss_model=iid phases=5pct-30s,${SPIKE_LOSS_PCT}pct-60s,5pct-30s"

# Derive the runtime MTU limits from the exact product source before applying
# diagnostics. This keeps the same unified-MTU contract as the earlier probes.
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

# Add diagnostic-only FakeTCP pending-index counters so the 4096 compaction
# threshold can be observed directly. No transport decision reads these fields.
python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/instrument_faketcp_pending4096_diag.py" "$PRODUCT_DIR"
echo "WBD_DIAGNOSTIC_PATCH pending4096_stats=1 behavior_change=none"

PATCHED_HELPER="${RUNNER_TEMP:?}/helper-transient-${RATE_LABEL}-${SPIKE_LOSS_PCT}"
rm -rf "$PATCHED_HELPER"
cp -a "$HELPER_DIR" "$PATCHED_HELPER"

# Keep the realistic traffic mix aligned with the exact unified-path inner MTU.
python3 - "$PATCHED_HELPER/.github/scripts/instrument_singlelane_realistic_burst.py" "$INNER_MTU" <<'PY'
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

# Retarget the helper's profile normalizer from its historical 5M profile to
# the requested 20M/30M rate. It still only changes the test harness guard.
python3 - "$PATCHED_HELPER/.github/scripts/instrument_singlelane_5m_profile.py" "$TEST_RATE_BPS" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); rate=sys.argv[2]; s=p.read_text()
old='new=\'[[ "$RATE_BPS" == 5000000 ]] || { echo "RATE_BPS must be exactly 5000000" >&2; exit 2; }\''
new=f'new=\'[[ "$RATE_BPS" == {rate} ]] || {{ echo "RATE_BPS must be exactly {rate}" >&2; exit 2; }}\''
if s.count(old)!=1: raise SystemExit(f'rate-profile marker drift: {s.count(old)}')
s=s.replace(old,new,1).replace('WBD_SINGLELANE_5M_PROFILE_PATCHED rate_bps=5000000',f'WBD_SINGLELANE_RATE_PROFILE_PATCHED rate_bps={rate}',1)
p.write_text(s)
PY

# Retarget the transient instrumentation to 30s @5%, 60s @20/30%, 30s @5%.
# netem's "loss random X%" is iid/random loss independently on each direction.
python3 - "$PATCHED_HELPER/.github/scripts/instrument_singlelane_transient_spike.py" "$TEST_RATE_BPS" "$SPIKE_LOSS_PCT" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); rate=int(sys.argv[2]); spike=int(sys.argv[3]); s=p.read_text()
label=f'spike{spike}'
repls={
"if (base_loss, spike_loss, spike_start, spike_duration, duration, rate_bps) != (5, 30, 15, 15, 45, 5000000):":
 f"if (base_loss, spike_loss, spike_start, spike_duration, duration, rate_bps) != (5, {spike}, 30, 60, 120, {rate}):",
"    raise SystemExit('transient spike probe is pinned to 5Mbps, 45s, 5% -> 30% for 15s -> 5%')":
 f"    raise SystemExit('transient spike probe is pinned to {rate}bps, 120s, 5% -> {spike}% for 60s -> 5%')",
"phase_bounds=(15.0,30.0)": "phase_bounds=(30.0,90.0)",
"phase_names=('pre5','spike30','post5')": f"phase_names=('pre5','{label}','post5')",
"def phase_for_tx(t): return 'pre5' if t < phase_bounds[0] else ('spike30' if t < phase_bounds[1] else 'post5')":
 f"def phase_for_tx(t): return 'pre5' if t < phase_bounds[0] else ('{label}' if t < phase_bounds[1] else 'post5')",
"for phase,tag in [('pre5','pre5-end'),('spike30','spike30-end'),('post5','post5-end')]:":
 f"for phase,tag in [('pre5','pre5-end'),('{label}','{label}-end'),('post5','post5-end')]:",
"print('WBD_TRANSIENT_SPIKE_PATCHED rate_bps=5000000 duration_sec=45 pre_loss=5 spike_loss=30 spike_start=15 spike_duration=15')":
 f"print('WBD_TRANSIENT_SPIKE_PATCHED rate_bps={rate} duration_sec=120 pre_loss=5 spike_loss={spike} spike_start=30 spike_duration=60')",
}
for old,new in repls.items():
    if s.count(old)!=1: raise SystemExit(f'transient retarget marker drift {old[:50]!r}: {s.count(old)}')
    s=s.replace(old,new,1)
old='''    sleep 15
    transient_qdisc_snapshot pre5-end
    transient_qdisc_set 30
    echo "WBD_TRANSIENT_NETEM phase=spike30 start_sec=15 loss_pct=30" >>"$LOG_DIR/transient-netem.log"
    sleep 15
    transient_qdisc_snapshot spike30-end
    transient_qdisc_set 5
    echo "WBD_TRANSIENT_NETEM phase=post5 start_sec=30 loss_pct=5" >>"$LOG_DIR/transient-netem.log"
    sleep 15
    transient_qdisc_snapshot post5-end
'''
new=f'''    sleep 30
    transient_qdisc_snapshot pre5-end
    transient_qdisc_set {spike}
    echo "WBD_TRANSIENT_NETEM phase={label} start_sec=30 loss_pct={spike}" >>"$LOG_DIR/transient-netem.log"
    sleep 60
    transient_qdisc_snapshot {label}-end
    transient_qdisc_set 5
    echo "WBD_TRANSIENT_NETEM phase=post5 start_sec=90 loss_pct=5" >>"$LOG_DIR/transient-netem.log"
    sleep 30
    transient_qdisc_snapshot post5-end
'''
if s.count(old)!=1: raise SystemExit(f'transient worker marker drift: {s.count(old)}')
s=s.replace(old,new,1)
p.write_text(s)
PY

# Retarget the helper wrapper's historical 5M/45s pins and result metadata.
python3 - "$PATCHED_HELPER/.github/scripts/run_singlelane_ab_observability.sh" "$TEST_RATE_BPS" "$SPIKE_LOSS_PCT" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); rate=sys.argv[2]; spike=sys.argv[3]; s=p.read_text()
repls={
'[[ "$TEST_RATE_BPS" == 5000000 && "$TEST_DURATION_SEC" == 45 ]] || { echo "transient probe requires 5Mbps/45s" >&2; exit 2; }':
 f'[[ "$TEST_RATE_BPS" == {rate} && "$TEST_DURATION_SEC" == 120 ]] || {{ echo "transient probe requires {rate}bps/120s" >&2; exit 2; }}',
'[[ "$SPIKE_LOSS_PCT" == 30 && "$SPIKE_START_SEC" == 15 && "$SPIKE_DURATION_SEC" == 15 ]] || { echo "transient probe requires 30pct spike from t=15s for 15s" >&2; exit 2; }':
 f'[[ "$SPIKE_LOSS_PCT" == {spike} && "$SPIKE_START_SEC" == 30 && "$SPIKE_DURATION_SEC" == 60 ]] || {{ echo "transient probe requires {spike}pct spike from t=30s for 60s" >&2; exit 2; }}',
" 'spike_loss_pct_each_direction':30,'spike_start_sec':15,'spike_duration_sec':15,":
 f" 'spike_loss_pct_each_direction':{spike},'spike_start_sec':30,'spike_duration_sec':60,",
" 'rate_bps_each_direction':5000000,'duration_sec':45,":
 f" 'rate_bps_each_direction':{rate},'duration_sec':120,",
}
for old,new in repls.items():
    if s.count(old)!=1: raise SystemExit(f'AB wrapper retarget marker drift {old[:60]!r}: {s.count(old)}')
    s=s.replace(old,new,1)
p.write_text(s)
PY

python3 -m py_compile "$PATCHED_HELPER/.github/scripts/instrument_singlelane_5m_profile.py" \
  "$PATCHED_HELPER/.github/scripts/instrument_singlelane_transient_spike.py"

export INNER_MTU LINK_PLAINTEXT_MTU
export WBD_EXPECT_CONNECTION_MTU="$CONNECTION_MTU"
export WBD_EXPECT_CARRIER_PAYLOAD_MTU="$CARRIER_PAYLOAD_MTU"
export WBD_EXPECT_DTLS_PLAINTEXT_MTU="$DTLS_PLAINTEXT_MTU"
export WBD_EXPECT_FEC_OVERHEAD="$FEC_OVERHEAD"
export WBD_EXPECT_GAME_OVERHEAD="$GAME_OVERHEAD"
export WBD_HELPER_DIR="$PATCHED_HELPER"
export CANDIDATE="transient-${RATE_LABEL}-5to${SPIKE_LOSS_PCT}to5-300ms-120s"
export SOURCE_SHA="$PRODUCT_SOURCE_SHA"
export LOSS_PCT=5
export LOSS_MODEL=transient
export TRAFFIC_PROFILE=realistic-mix-v1
export TEST_DURATION_SEC=120
export SPIKE_START_SEC=30
export SPIKE_DURATION_SEC=60

set +e
bash "$PATCHED_HELPER/.github/scripts/run_singlelane_ab_observability.sh" "$PRODUCT_DIR" "$RUNNER_TEMP"
run_rc=$?
set -e

LOG="$RUNNER_TEMP/singlelane-$CANDIDATE"
OUT="$RUNNER_TEMP/transient-${RATE_LABEL}-5-${SPIKE_LOSS_PCT}-5-300ms-120s"
mkdir -p "$OUT"
AUDIT="$OUT/transient-audit.json"

python3 - "$AUDIT" "$LOG" "$TEST_RATE_BPS" "$SPIKE_LOSS_PCT" \
  "$CONNECTION_MTU" "$CARRIER_PAYLOAD_MTU" "$DTLS_PLAINTEXT_MTU" "$LINK_PLAINTEXT_MTU" "$INNER_MTU" <<'PY'
import glob,json,os,re,sys
path,logdir,rate,spike,connection,carrier,dtls,link,inner=sys.argv[1:]
rate=int(rate); spike=int(spike); connection=int(connection); carrier=int(carrier); dtls=int(dtls); link=int(link); inner=int(inner)
def read(name):
    p=os.path.join(logdir,name)
    try: return json.load(open(p))
    except Exception: return {}
load=read('load-result.json'); ab=read('ab-transient-result.json'); resource=read('resource-metrics.json'); host=read('host-pressure.json'); netem=read('transient-netem-result.json'); shadow=read('shadow-matrix-result.json')
texts=[]; files=[]
for p in glob.glob(os.path.join(logdir,'**','*.log'),recursive=True):
    try:
        t=open(p,errors='replace').read(); texts.append(t); files.append((p,t))
    except OSError: pass
all_text='\n'.join(texts)

# FEC 64-block pressure, using the same fields as the historical capacity audit.
fec_logs=[]
for p,t in files:
    samples=[]
    for line in t.splitlines():
        if 'WBD_LINK_FEC_DIAG ' not in line: continue
        raw=line.split('WBD_LINK_FEC_DIAG ',1)[1].strip()
        try: samples.append(json.loads(raw))
        except Exception: pass
    if not samples: continue
    max_blocks=max(int((x.get('decoder') or {}).get('max_blocks',0) or 0) for x in samples)
    cur=[int((x.get('decoder') or {}).get('in_flight',0) or 0) for x in samples]
    fec_logs.append({
      'log':os.path.basename(p),'samples':len(samples),'max_blocks':max_blocks,
      'peak_in_flight':max(int(x.get('peak_in_flight',0) or 0) for x in samples),
      'max_current_in_flight':max(cur or [0]),'final_current_in_flight':cur[-1] if cur else 0,
      'samples_at_max_blocks':sum(1 for v in cur if max_blocks and v>=max_blocks),
      'samples_ge_90pct_max':sum(1 for v in cur if max_blocks and v*10>=max_blocks*9),
      'peak_retired':max(int(x.get('peak_retired',0) or 0) for x in samples),
      'peak_retired_incomplete':max(int(x.get('peak_retired_incomplete',0) or 0) for x in samples),
      'peak_retired_missing_sources':max(int(x.get('peak_retired_missing_sources',0) or 0) for x in samples),
      'pressure_retire_events':max(int(x.get('pressure_retire_events',0) or 0) for x in samples),
      'reconstruct_calls':max(int(x.get('reconstruct_calls',0) or 0) for x in samples),
      'reconstruct_success':max(int(x.get('reconstruct_success',0) or 0) for x in samples),
      'final_decoder':samples[-1].get('decoder') or {},
    })
fec_max_blocks=max([x['max_blocks'] for x in fec_logs] or [0]); fec_peak=max([x['peak_in_flight'] for x in fec_logs] or [0]); fec_cur=max([x['max_current_in_flight'] for x in fec_logs] or [0]); fec_sat=sum(x['samples_at_max_blocks'] for x in fec_logs); fec_90=sum(x['samples_ge_90pct_max'] for x in fec_logs)
capacity64_pressure=(fec_max_blocks==64 and (fec_peak>=64 or fec_cur>=64 or fec_sat>0))

# FakeTCP's 4096 sparse pending-index threshold. The diagnostic fields are
# cumulative/high-water counters added without changing transport decisions.
pending_logs=[]
for p,t in files:
    rows=[]
    for line in t.splitlines():
        for marker in ('WBD_FAKETCP_STATS ','WBD_FAKETCP_MUX_SHADOW_STATS '):
            if marker not in line: continue
            raw=line.split(marker,1)[1].strip()
            try: obj=json.loads(raw)
            except Exception: continue
            if isinstance(obj,dict):
                sender=obj.get('sender') if isinstance(obj.get('sender'),dict) else obj
                if isinstance(sender,dict) and any(k in sender for k in ('PendingHead','Pending4096Compactions','MaxPendingBackingRecords')):
                    rows.append(sender)
    if not rows: continue
    pending_logs.append({
      'log':os.path.basename(p),'samples':len(rows),
      'max_pending_head':max(int(x.get('MaxPendingHead',x.get('PendingHead',0)) or 0) for x in rows),
      'max_pending_backing_records':max(int(x.get('MaxPendingBackingRecords',x.get('PendingBackingRecords',0)) or 0) for x in rows),
      'max_pending_byseq_records':max(int(x.get('PendingBySeqRecords',0) or 0) for x in rows),
      'pending_compactions':max(int(x.get('PendingCompactions',0) or 0) for x in rows),
      'pending_4096_compactions':max(int(x.get('Pending4096Compactions',0) or 0) for x in rows),
      'final_pending_head':int(rows[-1].get('PendingHead',0) or 0),
      'final_pending_backing_records':int(rows[-1].get('PendingBackingRecords',0) or 0),
    })
p4096_total=sum(x['pending_4096_compactions'] for x in pending_logs); pmax_head=max([x['max_pending_head'] for x in pending_logs] or [0]); pmax_back=max([x['max_pending_backing_records'] for x in pending_logs] or [0])

phase_label=f'spike{spike}'
def loss_ok(v,target):
    if v is None: return False
    lo,hi=((0.035,0.065) if target==5 else ((0.17,0.23) if target==20 else (0.27,0.33)))
    return lo <= float(v) <= hi
netem_ok=True
for ph,target in [('pre5',5),(phase_label,spike),('post5',5)]:
    d=netem.get(ph) or {}
    for side in ('client','server'):
        netem_ok = netem_ok and loss_ok((d.get(side) or {}).get('loss_ratio'),target)

patterns={
 'fec_packet_too_large':r'fec: packet too large',
 'fec_header_mismatch':r'fec: inconsistent block header',
 'fec_short_datagram':r'fec: shard datagram too short',
 'link_proxy_fail':r'WBD_LINK_PROXY_FAIL',
 'dtls_unexpected_eof':r'unexpected EOF',
 'buffer_too_small':r'provided buffer is too small',
 'lane_fail_nonzero':r'lane_fail=[1-9][0-9]*',
}
errors={k:len(re.findall(v,all_text,re.I)) for k,v in patterns.items()}
transport_clean=all(v==0 for v in errors.values()) and int(load.get('bad_payload',0) or 0)==0
host_anomaly=(int(host.get('softnet_dropped_delta',0) or 0)+int(host.get('softnet_time_squeeze_delta',0) or 0))
for side in ('client_udp_delta','server_udp_delta'):
    d=host.get(side) or {}
    host_anomaly += sum(int(d.get(k,0) or 0) for k in ('InErrors','RcvbufErrors','SndbufErrors'))

checks={
 'rate_matches':int(ab.get('rate_bps_each_direction',0) or 0)==rate,
 'duration_120s':int(ab.get('duration_sec',0) or 0)==120,
 'random_iid_profile':True,
 'netem_phase_loss_clean':bool(netem_ok),
 'phase_metrics_present':all(k in (load.get('phase_metrics') or {}) for k in ('pre5',phase_label,'post5')),
 'no_payload_corruption':int(load.get('bad_payload',0) or 0)==0,
 'transport_frame_integrity_clean':bool(transport_clean),
 'host_kernel_pressure_clean':host_anomaly==0,
 'unified_mtu_budget_applied':inner>0 and inner<link<dtls<carrier<connection,
 'fec_diag_present':bool(fec_logs),
 'pending4096_diag_present':bool(pending_logs),
}
out={
 'rate_bps_each_direction':rate,'one_way_delay_ms':300,'loss_model':'iid/random',
 'phases':[{'loss_pct':5,'duration_sec':30},{'loss_pct':spike,'duration_sec':60},{'loss_pct':5,'duration_sec':30}],
 'mtu_budget':{'connection_mtu':connection,'carrier_payload_mtu':carrier,'dtls_plaintext_mtu':dtls,'link_plaintext_mtu':link,'inner_mtu':inner},
 'app':{'sent':load.get('sent'),'received_unique':load.get('received_unique'),'loss_ratio':load.get('loss_ratio'),'byte_loss_ratio':load.get('byte_loss_ratio'),'goodput_mbps':((load.get('down_payload_bps') or 0)/1e6),'phase_metrics':load.get('phase_metrics'),'rtt_ms_p50':load.get('rtt_ms_p50'),'rtt_ms_p95':load.get('rtt_ms_p95'),'rtt_ms_p99':load.get('rtt_ms_p99'),'rtt_ms_max':load.get('rtt_ms_max'),'timely_1s_ratio':load.get('timely_1s_ratio'),'timely_2s_ratio':load.get('timely_2s_ratio'),'timely_5s_ratio':load.get('timely_5s_ratio')},
 'resource':resource,'host_pressure':host,'netem':netem,'shadow':shadow,
 'fec64':{'max_blocks':fec_max_blocks,'peak_in_flight':fec_peak,'max_current_in_flight':fec_cur,'samples_at_max_blocks':fec_sat,'samples_ge_90pct_max':fec_90,'capacity64_pressure':capacity64_pressure,'logs':fec_logs},
 'pending4096':{'max_head':pmax_head,'max_backing_records':pmax_back,'compactions_4096_total':p4096_total,'hit_4096_threshold':bool(p4096_total or pmax_head>=4096),'logs':pending_logs},
 'transport_error_counts':errors,'host_kernel_anomaly_count':host_anomaly,'checks':checks,
 'valid_for_capacity_conclusion':bool(transport_clean and netem_ok and checks['unified_mtu_budget_applied'] and checks['fec_diag_present'] and checks['pending4096_diag_present'] and host_anomaly==0),
}
with open(path,'w') as f: json.dump(out,f,indent=2,sort_keys=True); f.write('\n')
print('WBD_TRANSIENT_PROFILE_AUDIT '+json.dumps(out,sort_keys=True))
PY

cp -a "$LOG" "$OUT/logs" 2>/dev/null || true
cat "$AUDIT"
exit "$run_rc"
