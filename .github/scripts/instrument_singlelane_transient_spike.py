#!/usr/bin/env python3
import os
import pathlib
import sys

product = pathlib.Path(sys.argv[1])
base_loss = int(os.environ.get('LOSS_PCT', '5'))
spike_loss = int(os.environ.get('SPIKE_LOSS_PCT', '30'))
spike_start = int(os.environ.get('SPIKE_START_SEC', '15'))
spike_duration = int(os.environ.get('SPIKE_DURATION_SEC', '15'))
duration = int(os.environ.get('TEST_DURATION_SEC', '45'))
rate_bps = int(os.environ.get('TEST_RATE_BPS', '5000000'))

if (base_loss, spike_loss, spike_start, spike_duration, duration, rate_bps) != (5, 30, 15, 15, 45, 5000000):
    raise SystemExit('transient spike probe is pinned to 5Mbps, 45s, 5% -> 30% for 15s -> 5%')

rotation = product / 'scripts/game_lane_rotation_soak.sh'
s = rotation.read_text()

# Per-phase application delivery/timeliness keyed by scheduled TX time. This is
# test-only accounting; WBD1 wire bytes are unchanged.
old = "sent=0; sent_bytes=0; recv_bytes=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; timely_1s=0; timely_2s=0; timely_5s=0\n"
new = "sent=0; sent_bytes=0; recv_bytes=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; timely_1s=0; timely_2s=0; timely_5s=0\nphase_bounds=(15.0,30.0)\nphase_names=('pre5','spike30','post5')\nphase_sent={k:0 for k in phase_names}; phase_sent_bytes={k:0 for k in phase_names}\nphase_recv={k:0 for k in phase_names}; phase_recv_bytes={k:0 for k in phase_names}\nphase_latency={k:[] for k in phase_names}; phase_timely_1s={k:0 for k in phase_names}; phase_timely_2s={k:0 for k in phase_names}\ndef phase_for_tx(t): return 'pre5' if t < phase_bounds[0] else ('spike30' if t < phase_bounds[1] else 'post5')\n"
if s.count(old) != 1:
    raise SystemExit(f'transient phase state marker drift: {s.count(old)}')
s = s.replace(old, new, 1)

old = """        s.sendto(payload,('127.0.0.1',47500))
        sent+=1; sent_bytes+=size
        next_tx=start+(sent_bytes*8.0/rate_bps)
"""
new = """        s.sendto(payload,('127.0.0.1',47500))
        ph=phase_for_tx(next_tx-start)
        phase_sent[ph]+=1; phase_sent_bytes[ph]+=size
        sent+=1; sent_bytes+=size
        next_tx=start+(sent_bytes*8.0/rate_bps)
"""
if s.count(old) != 1:
    raise SystemExit(f'transient send marker drift: {s.count(old)}')
s = s.replace(old, new, 1)

old = """                    unique.add(seq); recv_bytes+=len(data)
                    age=max(0.0,(last_rx-start)-tx_rel)
                    latency_ms.append(age*1000.0)
                    if age <= 1.0: timely_1s+=1
                    if age <= 2.0: timely_2s+=1
                    if age <= 5.0: timely_5s+=1
"""
new = """                    unique.add(seq); recv_bytes+=len(data)
                    age=max(0.0,(last_rx-start)-tx_rel)
                    age_ms=age*1000.0
                    latency_ms.append(age_ms)
                    ph=phase_for_tx(tx_rel)
                    phase_recv[ph]+=1; phase_recv_bytes[ph]+=len(data); phase_latency[ph].append(age_ms)
                    if age <= 1.0: timely_1s+=1; phase_timely_1s[ph]+=1
                    if age <= 2.0: timely_2s+=1; phase_timely_2s[ph]+=1
                    if age <= 5.0: timely_5s+=1
"""
if s.count(old) != 1:
    raise SystemExit(f'transient receive marker drift: {s.count(old)}')
s = s.replace(old, new, 1)

old = "recv=len(unique); lost=max(0,sent-recv)\nlatency_ms.sort()\n"
new = """recv=len(unique); lost=max(0,sent-recv)
latency_ms.sort()
def phase_pct(xs,p):
    if not xs: return None
    xs=sorted(xs); i=max(0,min(len(xs)-1,math.ceil(p*len(xs))-1)); return xs[i]
phase_metrics={}
for ph in phase_names:
    ps=phase_sent[ph]; pr=phase_recv[ph]; pbytes=phase_sent_bytes[ph]; rbytes=phase_recv_bytes[ph]
    phase_metrics[ph]={
      'sent':ps,'received_unique':pr,'loss_ratio':((ps-pr)/ps if ps else None),
      'sent_payload_bytes':pbytes,'received_payload_bytes':rbytes,
      'byte_loss_ratio':((pbytes-rbytes)/pbytes if pbytes else None),
      'rtt_ms_p50':phase_pct(phase_latency[ph],0.50),'rtt_ms_p95':phase_pct(phase_latency[ph],0.95),
      'rtt_ms_p99':phase_pct(phase_latency[ph],0.99),'rtt_ms_max':(max(phase_latency[ph]) if phase_latency[ph] else None),
      'timely_1s_ratio':(phase_timely_1s[ph]/ps if ps else None),
      'timely_2s_ratio':(phase_timely_2s[ph]/ps if ps else None),
    }
latency_ms.sort()
"""
if s.count(old) != 1:
    raise SystemExit(f'transient summary prelude marker drift: {s.count(old)}')
s = s.replace(old, new, 1)

old = "  'byte_loss_ratio':(max(0,sent_bytes-recv_bytes)/sent_bytes if sent_bytes else 1.0),\n"
new = "  'byte_loss_ratio':(max(0,sent_bytes-recv_bytes)/sent_bytes if sent_bytes else 1.0),\n  'phase_metrics':phase_metrics,\n"
if s.count(old) != 1:
    raise SystemExit(f'transient summary field marker drift: {s.count(old)}')
s = s.replace(old, new, 1)

# During the offered-load window reset qdisc to 5%, switch to 30% at t=15s,
# then back to 5% at t=30s. qdisc is replaced at each boundary so each phase's
# counters start from zero and can be reported independently.
marker = 'cpu_snapshot "$LOG_DIR/cpu-start.json"\nhost_pressure_snapshot start\nouter_metrics_start\n'
if s.count(marker) != 1:
    raise SystemExit(f'transient load-start marker drift: {s.count(marker)}')
block = r'''transient_qdisc_snapshot() {
  local tag=$1
  sudo ip netns exec "$C" tc -s -j qdisc show dev gc0 >"$LOG_DIR/transient-${tag}-client-qdisc.json"
  sudo ip netns exec "$S" tc -s -j qdisc show dev gs0 >"$LOG_DIR/transient-${tag}-server-qdisc.json"
}
transient_qdisc_set() {
  local loss=$1
  sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"
  sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem limit 24000 delay "${NETEM_DELAY_MS}ms" loss random "${loss}%"
}
transient_netem_start() {
  transient_qdisc_set 5
  echo "WBD_TRANSIENT_NETEM phase=pre5 start_sec=0 loss_pct=5" | tee -a "$LOG_DIR/transient-netem.log"
  (
    sleep 15
    transient_qdisc_snapshot pre5-end
    transient_qdisc_set 30
    echo "WBD_TRANSIENT_NETEM phase=spike30 start_sec=15 loss_pct=30" >>"$LOG_DIR/transient-netem.log"
    sleep 15
    transient_qdisc_snapshot spike30-end
    transient_qdisc_set 5
    echo "WBD_TRANSIENT_NETEM phase=post5 start_sec=30 loss_pct=5" >>"$LOG_DIR/transient-netem.log"
    sleep 15
    transient_qdisc_snapshot post5-end
    python3 - "$LOG_DIR" <<'PY_TRANSIENT'
import json, os, sys
root=sys.argv[1]
def q(tag,ns):
    arr=json.load(open(os.path.join(root,f'transient-{tag}-{ns}-qdisc.json')))
    for x in arr:
        if x.get('kind')=='netem':
            packets=int(x.get('packets',0)); drops=int(x.get('drops',0)); denom=packets+drops
            return {'packets':packets,'drops':drops,'overlimits':int(x.get('overlimits',0)),
                    'loss_ratio':(drops/denom if denom else None)}
    return {}
out={}
for phase,tag in [('pre5','pre5-end'),('spike30','spike30-end'),('post5','post5-end')]:
    out[phase]={'client':q(tag,'client'),'server':q(tag,'server')}
open(os.path.join(root,'transient-netem-result.json'),'w').write(json.dumps(out,sort_keys=True,indent=2)+'\n')
print('WBD_TRANSIENT_NETEM_RESULT '+json.dumps(out,sort_keys=True))
PY_TRANSIENT
  ) >"$LOG_DIR/transient-netem-worker.log" 2>&1 &
  TRANSIENT_NETEM_PID=$!; PIDS+=("$TRANSIENT_NETEM_PID")
}

'''
s=s.replace(marker, block+marker+'transient_netem_start\n', 1)

old = 'wait "$METRICS_PID"\ndrop_pid "$METRICS_PID"\ncat "$LOG_DIR/load.log"\n'
new = 'wait "$METRICS_PID"\ndrop_pid "$METRICS_PID"\nwait "$TRANSIENT_NETEM_PID"\ndrop_pid "$TRANSIENT_NETEM_PID"\ncat "$LOG_DIR/load.log"\n'
if s.count(old) != 1:
    raise SystemExit(f'transient completion marker drift: {s.count(old)}')
s=s.replace(old,new,1)

old = 'cat "$LOG_DIR/resource-metrics.json"\n'
new = 'cat "$LOG_DIR/resource-metrics.json"\ncat "$LOG_DIR/transient-netem.log"\ncat "$LOG_DIR/transient-netem-result.json"\n'
if s.count(old) != 1:
    raise SystemExit(f'transient result marker drift: {s.count(old)}')
s=s.replace(old,new,1)

rotation.write_text(s)
print('WBD_TRANSIENT_SPIKE_PATCHED rate_bps=5000000 duration_sec=45 pre_loss=5 spike_loss=30 spike_start=15 spike_duration=15')
