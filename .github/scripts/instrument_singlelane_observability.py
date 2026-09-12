#!/usr/bin/env python3
import pathlib
import sys

product = pathlib.Path(sys.argv[1])
rotation = product / 'scripts/game_lane_rotation_soak.sh'
s = rotation.read_text()

# End-to-end first-arrival timeliness. WBD1 already carries seq + scheduled TX
# time in bytes [4:20], so this is test-only observation and changes no wire
# protocol or product behavior.
old = "import json, select, socket, struct, sys, time\n"
new = "import json, math, select, socket, struct, sys, time\n"
if s.count(old) != 1:
    raise SystemExit('observability: load import marker drift')
s = s.replace(old, new, 1)

old = "sent=0; unique=set(); dup=0; bad=0; last_rx=start\n"
new = "sent=0; unique=set(); dup=0; bad=0; last_rx=start\nlatency_ms=[]; timely_1s=0; timely_2s=0; timely_5s=0\n"
if s.count(old) != 1:
    raise SystemExit('observability: load state marker drift')
s = s.replace(old, new, 1)

old = """                seq=struct.unpack('!Q',data[4:12])[0]
                if seq in unique: dup+=1
                else: unique.add(seq)
"""
new = """                seq=struct.unpack('!Q',data[4:12])[0]
                tx_rel=struct.unpack('!d',data[12:20])[0]
                if seq in unique:
                    dup+=1
                else:
                    unique.add(seq)
                    age=max(0.0,(last_rx-start)-tx_rel)
                    latency_ms.append(age*1000.0)
                    if age <= 1.0: timely_1s+=1
                    if age <= 2.0: timely_2s+=1
                    if age <= 5.0: timely_5s+=1
"""
if s.count(old) != 1:
    raise SystemExit('observability: receive marker drift')
s = s.replace(old, new, 1)

old = "recv=len(unique); lost=max(0,sent-recv)\nsummary={\n"
new = """recv=len(unique); lost=max(0,sent-recv)
latency_ms.sort()
def pct(p):
    if not latency_ms: return None
    i=max(0,min(len(latency_ms)-1,math.ceil(p*len(latency_ms))-1))
    return latency_ms[i]
summary={
"""
if s.count(old) != 1:
    raise SystemExit('observability: summary marker drift')
s = s.replace(old, new, 1)

old = "  'loss_ratio':(lost/sent if sent else 1.0),\n}\n"
new = """  'loss_ratio':(lost/sent if sent else 1.0),
  'rtt_samples':len(latency_ms),
  'rtt_ms_p50':pct(0.50),'rtt_ms_p95':pct(0.95),'rtt_ms_p99':pct(0.99),
  'rtt_ms_max':(latency_ms[-1] if latency_ms else None),
  'timely_1s':timely_1s,'timely_2s':timely_2s,'timely_5s':timely_5s,
  'timely_1s_ratio':(timely_1s/sent if sent else 0.0),
  'timely_2s_ratio':(timely_2s/sent if sent else 0.0),
  'timely_5s_ratio':(timely_5s/sent if sent else 0.0),
}
"""
if s.count(old) != 1:
    raise SystemExit('observability: summary fields marker drift')
s = s.replace(old, new, 1)

# Lightweight resource accounting during only the offered-load window.
# - iptables jump counters are before netem, so they count attempted outer bytes
#   including packets that the synthetic network later drops.
# - gc0/gs0 tx_bytes are after qdisc and show bytes that actually leave each
#   public veth after netem.
# - /proc CPU sampling includes only the product transport stack, never load.py.
marker = ': >"$LOG_DIR/rotation.log"\n'
if s.count(marker) != 1:
    raise SystemExit('observability: rotation marker drift')
block = r'''cpu_snapshot() {
  local out=$1
  python3 - "$out" <<'PY_CPU'
import json, os, pathlib, sys
names={
 'wbd-faketcp':'faketcp','wbd-faketcp-mux':'faketcp',
 'wbd_dtls_shim':'dtls',
 'wbd-link-proxy':'link','wbd-link-server-mux':'link',
 'wbd-game-lane-client':'game','wbd-game-lane-server':'game',
}
by_exe={}; by_component={}; pids={}
for p in pathlib.Path('/proc').iterdir():
    if not p.name.isdigit(): continue
    try:
        exe=os.path.basename(os.readlink(p/'exe'))
        component=names.get(exe)
        if not component: continue
        raw=(p/'stat').read_text()
        tail=raw[raw.rfind(')')+2:].split()
        ticks=int(tail[11])+int(tail[12])
    except (OSError,ValueError,IndexError):
        continue
    by_exe[exe]=by_exe.get(exe,0)+ticks
    by_component[component]=by_component.get(component,0)+ticks
    pids.setdefault(exe,[]).append(int(p.name))
obj={'hz':os.sysconf(os.sysconf_names['SC_CLK_TCK']),'cpus':os.cpu_count() or 1,
     'total_ticks':sum(by_exe.values()),'by_exe_ticks':by_exe,
     'by_component_ticks':by_component,'pids':pids}
open(sys.argv[1],'w').write(json.dumps(obj,sort_keys=True,indent=2)+'\n')
PY_CPU
}

outer_metrics_start() {
  local cport=${LANE_SPORT[1]}
  for ns in "$C" "$S"; do
    sudo ip netns exec "$ns" iptables -D OUTPUT -p tcp -j WBDACCT 2>/dev/null || true
    sudo ip netns exec "$ns" iptables -F WBDACCT 2>/dev/null || true
    sudo ip netns exec "$ns" iptables -X WBDACCT 2>/dev/null || true
    sudo ip netns exec "$ns" iptables -N WBDACCT
    sudo ip netns exec "$ns" iptables -A WBDACCT -j RETURN
  done
  sudo ip netns exec "$C" iptables -I OUTPUT 1 -p tcp -s 10.89.0.2 -d 10.89.0.1 --sport "$cport" --dport "$RAW" -j WBDACCT
  sudo ip netns exec "$S" iptables -I OUTPUT 1 -p tcp -s 10.89.0.1 -d 10.89.0.2 --sport "$RAW" --dport "$cport" -j WBDACCT
  local ctx stx
  ctx=$(sudo ip netns exec "$C" cat /sys/class/net/gc0/statistics/tx_bytes)
  stx=$(sudo ip netns exec "$S" cat /sys/class/net/gs0/statistics/tx_bytes)
  printf '{"client_tx_bytes":%s,"server_tx_bytes":%s}\n' "$ctx" "$stx" >"$LOG_DIR/outer-start.json"
}

outer_metrics_finish() {
  local cvals svals ctx stx
  cvals=$(sudo ip netns exec "$C" iptables -nvxL WBDACCT | awk '$3=="RETURN" {print $1" "$2; exit}')
  svals=$(sudo ip netns exec "$S" iptables -nvxL WBDACCT | awk '$3=="RETURN" {print $1" "$2; exit}')
  ctx=$(sudo ip netns exec "$C" cat /sys/class/net/gc0/statistics/tx_bytes)
  stx=$(sudo ip netns exec "$S" cat /sys/class/net/gs0/statistics/tx_bytes)
  set -- $cvals; local cpkts=${1:-0} cbytes=${2:-0}
  set -- $svals; local spkts=${1:-0} sbytes=${2:-0}
  printf '{"client_attempted_packets":%s,"client_attempted_bytes":%s,"server_attempted_packets":%s,"server_attempted_bytes":%s,"client_tx_bytes":%s,"server_tx_bytes":%s}\n' \
    "$cpkts" "$cbytes" "$spkts" "$sbytes" "$ctx" "$stx" >"$LOG_DIR/outer-end.json"
}

resource_metrics_finish() {
  python3 - "$LOG_DIR" "$DURATION_SEC" "$RATE_BPS" <<'PY_RESOURCE'
import json, os, sys
root=sys.argv[1]; duration=float(sys.argv[2]); rate=float(sys.argv[3])
cs=json.load(open(os.path.join(root,'cpu-start.json')))
ce=json.load(open(os.path.join(root,'cpu-end.json')))
os0=json.load(open(os.path.join(root,'outer-start.json')))
oe=json.load(open(os.path.join(root,'outer-end.json')))
hz=float(cs['hz'])
components=sorted(set(cs.get('by_component_ticks',{}))|set(ce.get('by_component_ticks',{})))
by_component={}
for k in components:
    dt=max(0,int(ce.get('by_component_ticks',{}).get(k,0))-int(cs.get('by_component_ticks',{}).get(k,0)))
    by_component[k]=dt/hz/duration
cpu_ticks=max(0,int(ce['total_ticks'])-int(cs['total_ticks']))
cpu_cores=cpu_ticks/hz/duration
attempted=int(oe['client_attempted_bytes'])+int(oe['server_attempted_bytes'])
attempted_pkts=int(oe['client_attempted_packets'])+int(oe['server_attempted_packets'])
post=max(0,int(oe['client_tx_bytes'])-int(os0['client_tx_bytes']))+max(0,int(oe['server_tx_bytes'])-int(os0['server_tx_bytes']))
obj={
 'window_sec':duration,
 'avg_cpu_cores':cpu_cores,
 'avg_cpu_percent_one_core':cpu_cores*100.0,
 'avg_cpu_percent_machine':cpu_cores/max(1,int(cs.get('cpus',1)))*100.0,
 'cpu_cores_by_component':by_component,
 'outer_attempted_packets_aggregate':attempted_pkts,
 'outer_attempted_bytes_aggregate':attempted,
 'outer_attempted_bps_aggregate':attempted*8.0/duration,
 'outer_attempted_mbps_aggregate':attempted*8.0/duration/1e6,
 'outer_attempted_mbps_per_direction_mean':attempted*8.0/duration/2e6,
 'outer_post_netem_bytes_aggregate':post,
 'outer_post_netem_bps_aggregate':post*8.0/duration,
 'outer_post_netem_mbps_aggregate':post*8.0/duration/1e6,
 'outer_attempted_to_offered_inner_aggregate_ratio':(attempted*8.0/duration)/(2.0*rate),
}
open(os.path.join(root,'resource-metrics.json'),'w').write(json.dumps(obj,sort_keys=True,indent=2)+'\n')
print('WBD_SINGLELANE_RESOURCE_RESULT '+json.dumps(obj,sort_keys=True))
PY_RESOURCE
}

'''
s = s.replace(marker, block + marker, 1)

old = '''sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &
LOAD_PID=$!; PIDS+=("$LOAD_PID")
load_start=$(date +%s)
'''
new = '''cpu_snapshot "$LOG_DIR/cpu-start.json"
outer_metrics_start
sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &
LOAD_PID=$!; PIDS+=("$LOAD_PID")
load_start=$(date +%s)
(
  sleep "$DURATION_SEC"
  cpu_snapshot "$LOG_DIR/cpu-end.json"
  outer_metrics_finish
  resource_metrics_finish
) >"$LOG_DIR/resource-metrics.log" 2>&1 &
METRICS_PID=$!; PIDS+=("$METRICS_PID")
'''
if s.count(old) != 1:
    raise SystemExit('observability: load launch marker drift')
s = s.replace(old, new, 1)

old = '''wait "$LOAD_PID"
drop_pid "$LOAD_PID"
cat "$LOG_DIR/load.log"
cat "$LOG_DIR/load-result.json"
'''
new = '''wait "$LOAD_PID"
drop_pid "$LOAD_PID"
wait "$METRICS_PID"
drop_pid "$METRICS_PID"
cat "$LOG_DIR/load.log"
cat "$LOG_DIR/load-result.json"
cat "$LOG_DIR/resource-metrics.log"
cat "$LOG_DIR/resource-metrics.json"
'''
if s.count(old) != 1:
    raise SystemExit('observability: load completion marker drift')
s = s.replace(old, new, 1)

rotation.write_text(s)
print('WBD_SINGLELANE_OBSERVABILITY_PATCHED '+str(rotation))
