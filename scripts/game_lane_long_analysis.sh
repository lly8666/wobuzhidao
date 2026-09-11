#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?usage: game_lane_long_analysis.sh ASSET_DIR [LOG_DIR]}
LOG_DIR=${2:-${RUNNER_TEMP:-/tmp}/wbd-game-lane-long-analysis}
BASE_SCRIPT=${BASE_SCRIPT:?BASE_SCRIPT must point at exact-source game_lane_fullstack.sh}
DURATION_SEC=${DURATION_SEC:-1800}
RATE_BPS=${RATE_BPS:-10000000}
PAYLOAD_BYTES=${PAYLOAD_BYTES:-1000}
NETEM_DELAY_MS=${NETEM_DELAY_MS:-300}
NETEM_LOSS_PCT=${NETEM_LOSS_PCT:-20}
SAMPLE_INTERVAL_SEC=${SAMPLE_INTERVAL_SEC:-5}

case "${LANES:-}" in
  1|4) ;;
  *) echo "analysis requires LANES=1 or LANES=4" >&2; exit 2 ;;
esac
case "${FEC:-}" in
  off|20:20) ;;
  *) echo "analysis requires FEC=off or FEC=20:20" >&2; exit 2 ;;
esac
[[ "$DURATION_SEC" == 1800 ]] || { echo "analysis duration must be exactly 1800 seconds" >&2; exit 2; }
[[ "$RATE_BPS" == 10000000 ]] || { echo "analysis offered load must be exactly 10000000 bps" >&2; exit 2; }
[[ "$PAYLOAD_BYTES" == 1000 ]] || { echo "analysis payload must be exactly 1000 bytes" >&2; exit 2; }

mkdir -p "$LOG_DIR"
PATCHED="${RUNNER_TEMP:-/tmp}/game_lane_long_analysis_generated_${LANES}_${FEC//:/_}.sh"
python3 - "$BASE_SCRIPT" "$PATCHED" <<'PY_PATCH'
import pathlib,sys
src=pathlib.Path(sys.argv[1]).read_text()
marker='cat >"$LOG_DIR/probe.py" <<\'PY\'\n'
pos=src.find(marker)
if pos < 0:
    raise SystemExit('analysis patch: fullstack tail marker not found')
prefix=src[:pos]

prefix=prefix.replace('--bootstrap-timeout 12s','--bootstrap-timeout 30s')
prefix=prefix.replace('--reality-timeout 12s','--reality-timeout 30s')

old_echo='''cat >"$LOG_DIR/echo.py" <<'PY'\nimport pathlib,socket\ncount=pathlib.Path(__import__('sys').argv[1])\ns=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)\ns.bind(('127.0.0.1',48000))\nwhile True:\n    b,a=s.recvfrom(65535)\n    # The authenticated Logical Tunnel metadata is a control datagram for the\n    # downstream shared service, not one of the measured Game payloads.\n    if not (b.startswith(b'warm') or b.startswith(b'game')):\n        continue\n    with count.open('ab') as f: f.write(b.hex().encode()+b'\\n')\n    s.sendto(b,a)\nPY\n'''
new_echo='''cat >"$LOG_DIR/echo.py" <<'PY'\nimport pathlib,signal,socket,sys\nout=pathlib.Path(sys.argv[1]); n=0\ndef finish(*_):\n    out.write_text(str(n)+'\\n')\n    raise SystemExit(0)\nsignal.signal(signal.SIGTERM, finish)\nsignal.signal(signal.SIGINT, finish)\ns=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)\ns.bind(('127.0.0.1',48000))\nwhile True:\n    b,a=s.recvfrom(65535)\n    if not (b.startswith(b'warm') or b.startswith(b'game') or b.startswith(b'WBD1')):\n        continue\n    if b.startswith(b'WBD1'):\n        n += 1\n    s.sendto(b,a)\nPY\n'''
if old_echo not in prefix:
    raise SystemExit('analysis patch: echo block not found')
prefix=prefix.replace(old_echo,new_echo,1)

anchor='sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP\n'
netem=anchor+r'''if [[ "$NETEM_DELAY_MS" != 0 || "$NETEM_LOSS_PCT" != 0 ]]; then
  sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
  {
    echo "WBD_ANALYSIS_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional"
    sudo ip netns exec "$C" tc -s qdisc show dev gc0
    sudo ip netns exec "$S" tc -s qdisc show dev gs0
  } >"$LOG_DIR/netem-start.log"
fi
'''
if anchor not in prefix:
    raise SystemExit('analysis patch: netem anchor not found')
prefix=prefix.replace(anchor,netem,1)

tail=r'''if [[ -n "${TPID:-}" ]]; then
  sudo kill -INT "$TPID" 2>/dev/null || true
  wait "$TPID" 2>/dev/null || true
  TPID=
fi
rm -f "$LOG_DIR/game-lanes.pcap"

cat >"$LOG_DIR/warm.py" <<'PY_WARM'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47600)); s.settimeout(30)
b=b'warm-analysis'
s.sendto(b,('127.0.0.1',47500))
got,_=s.recvfrom(65535)
assert got==b,(got,b)
print('WBD_ANALYSIS_WARM_PASS')
PY_WARM
sudo ip netns exec "$C" python3 "$LOG_DIR/warm.py" >"$LOG_DIR/warm.log" 2>&1

for _ in $(seq 1 600); do
  binds=$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log" 2>/dev/null || true)
  [[ "$binds" -ge "$LANES" ]] && break
  sleep .05
done
[[ $(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log" 2>/dev/null || true) -ge "$LANES" ]]

read_counter() {
  local ns=$1 dev=$2 key=$3
  sudo ip netns exec "$ns" cat "/sys/class/net/$dev/statistics/$key"
}
python3 - "$LOG_DIR/wire-start.json" \
  "$(read_counter "$C" gc0 tx_bytes)" "$(read_counter "$C" gc0 rx_bytes)" \
  "$(read_counter "$S" gs0 tx_bytes)" "$(read_counter "$S" gs0 rx_bytes)" <<'PY_WIRE_START'
import json,sys
p=sys.argv[1]; vals=list(map(int,sys.argv[2:]))
d={'client_tx_bytes':vals[0],'client_rx_bytes':vals[1],'server_tx_bytes':vals[2],'server_rx_bytes':vals[3]}
open(p,'w').write(json.dumps(d,sort_keys=True,indent=2)+'\n')
PY_WIRE_START

cat >"$LOG_DIR/monitor.py" <<'PY_MON'
import csv,json,os,pathlib,sys,time
out_csv,out_json,stop_path,asset_dir,interval=sys.argv[1:]
interval=float(interval); asset_dir=os.path.realpath(asset_dir)
allowed={'wbd-faketcp','wbd-faketcp-mux','wbd-link-proxy','wbd-link-server-mux','wbd-game-lane-client','wbd-game-lane-server','wbd_dtls_shim'}
hz=os.sysconf(os.sysconf_names['SC_CLK_TCK'])
prev={}; totals=[]; groups={}; start=time.monotonic()

def discover():
    ans=[]
    for p in pathlib.Path('/proc').iterdir():
        if not p.name.isdigit(): continue
        try:
            exe=os.path.realpath(os.readlink(p/'exe'))
        except Exception:
            continue
        name=os.path.basename(exe)
        if name not in allowed or not exe.startswith(asset_dir.rstrip('/')+'/'):
            continue
        try:
            fields=(p/'stat').read_text().split(); ticks=int(fields[13])+int(fields[14])
            rss=0
            for line in (p/'status').read_text().splitlines():
                if line.startswith('VmRSS:'):
                    rss=int(line.split()[1]); break
            ans.append((int(p.name),name,ticks,rss))
        except Exception:
            continue
    return ans

with open(out_csv,'w',newline='') as f:
    w=csv.writer(f); w.writerow(['elapsed_sec','pid','exe','cpu_pct','rss_kb'])
    while True:
        now=time.monotonic(); elapsed=now-start; rows=discover(); by={}; total_cpu=0.0; total_rss=0
        for pid,name,ticks,rss in rows:
            old=prev.get(pid); cpu=0.0
            if old is not None:
                ot,ow=old; dt=max(now-ow,1e-9); cpu=max(0.0,(ticks-ot)/hz/dt*100.0)
            prev[pid]=(ticks,now)
            w.writerow([f'{elapsed:.3f}',pid,name,f'{cpu:.3f}',rss])
            c,r=by.get(name,(0.0,0)); by[name]=(c+cpu,r+rss)
            total_cpu+=cpu; total_rss+=rss
        f.flush()
        totals.append((total_cpu,total_rss))
        for name,(cpu,rss) in by.items(): groups.setdefault(name,[]).append((cpu,rss))
        if os.path.exists(stop_path): break
        time.sleep(interval)

def stats(samples):
    if not samples: return {'samples':0,'avg_cpu_pct':0.0,'peak_cpu_pct':0.0,'avg_rss_mb':0.0,'peak_rss_mb':0.0}
    cp=[x[0] for x in samples]; rs=[x[1]/1024.0 for x in samples]
    return {'samples':len(samples),'avg_cpu_pct':sum(cp)/len(cp),'peak_cpu_pct':max(cp),'avg_rss_mb':sum(rs)/len(rs),'peak_rss_mb':max(rs)}
summary={'sample_interval_sec':interval,'total':stats(totals),'per_executable':{k:stats(v) for k,v in sorted(groups.items())}}
open(out_json,'w').write(json.dumps(summary,sort_keys=True,indent=2)+'\n')
PY_MON
rm -f "$LOG_DIR/monitor.stop"
python3 "$LOG_DIR/monitor.py" "$LOG_DIR/process-samples.csv" "$LOG_DIR/process-summary.json" "$LOG_DIR/monitor.stop" "$ASSET_DIR" "$SAMPLE_INTERVAL_SEC" >"$LOG_DIR/monitor.log" 2>&1 &
MONPID=$!

cat >"$LOG_DIR/load.py" <<'PY_LOAD'
from array import array
import json,math,select,socket,struct,sys,time
out_path=sys.argv[1]; duration=float(sys.argv[2]); rate_bps=int(sys.argv[3]); payload_bytes=int(sys.argv[4])
pps=rate_bps/(payload_bytes*8.0); interval=1.0/pps
expected=int(math.ceil(duration*pps))+4096
seen=bytearray(expected)
hist=array('I',[0])*300002; rtt_count=0; rtt_max_ms=0.0
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',47600)); s.setblocking(False)
start=time.monotonic(); deadline=start+duration; next_tx=start
sent=0; unique=0; dup=0; bad=0; last_rx=start
pad=b'Z'*(payload_bytes-20)
while True:
    now=time.monotonic()
    while now >= next_tx and next_tx < deadline:
        send_ns=time.monotonic_ns()
        payload=b'WBD1'+struct.pack('!QQ',sent,send_ns)+pad
        if len(payload)!=payload_bytes: raise RuntimeError(len(payload))
        s.sendto(payload,('127.0.0.1',47500)); sent+=1; next_tx+=interval; now=time.monotonic()
    r,_,_=select.select([s],[],[],0.001)
    if r:
        while True:
            try: data,_=s.recvfrom(65535)
            except BlockingIOError: break
            now_ns=time.monotonic_ns(); last_rx=time.monotonic()
            if len(data)!=payload_bytes or not data.startswith(b'WBD1'):
                bad+=1; continue
            seq,send_ns=struct.unpack('!QQ',data[4:20])
            if seq >= len(seen):
                bad+=1; continue
            if seen[seq]:
                dup+=1; continue
            seen[seq]=1; unique+=1
            rtt_ms=max(0.0,(now_ns-send_ns)/1e6); rtt_max_ms=max(rtt_max_ms,rtt_ms); rtt_count+=1
            b=min(int(rtt_ms),300001); hist[b]+=1
    if now >= deadline:
        if unique == sent or now >= deadline+65.0: break

def pct(q):
    if rtt_count == 0: return None
    target=max(1,math.ceil(rtt_count*q)); acc=0
    for i,n in enumerate(hist):
        acc+=n
        if acc>=target: return float(i if i<=300000 else 300001)
    return None
elapsed=max(duration,1e-9); lost=max(0,sent-unique)
summary={'requested_duration_sec':int(duration),'actual_send_duration_sec':duration,'rate_target_bps':rate_bps,'payload_bytes':payload_bytes,
         'sent':sent,'received_unique':unique,'lost':lost,'duplicates':dup,'bad_payload':bad,
         'up_payload_bps':sent*payload_bytes*8/elapsed,'down_payload_bps':unique*payload_bytes*8/elapsed,
         'loss_ratio':(lost/sent if sent else 1.0),'drain_budget_sec':65,
         'rtt_samples':rtt_count,'rtt_p50_ms':pct(.50),'rtt_p95_ms':pct(.95),'rtt_p99_ms':pct(.99),'rtt_p999_ms':pct(.999),'rtt_max_ms':rtt_max_ms,
         'last_rx_after_start_sec':max(0.0,last_rx-start)}
open(out_path,'w').write(json.dumps(summary,sort_keys=True,indent=2)+'\n')
print('WBD_LONG_ANALYSIS_LOAD_RESULT '+json.dumps(summary,sort_keys=True))
PY_LOAD

ANALYSIS_PIDS=("${PIDS[@]}")
sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &
LOAD_PID=$!
health_fail=0
while kill -0 "$LOAD_PID" 2>/dev/null; do
  for p in "${ANALYSIS_PIDS[@]}"; do
    if ! kill -0 "$p" 2>/dev/null; then
      echo "analysis product/support process exited early pid=$p" >&2
      health_fail=1
      sudo kill -TERM "$LOAD_PID" 2>/dev/null || true
      break
    fi
  done
  (( health_fail == 0 )) || break
  sleep 5
done
wait "$LOAD_PID" || load_rc=$?
load_rc=${load_rc:-0}
if (( health_fail != 0 || load_rc != 0 )); then
  echo "analysis load infrastructure failure health_fail=$health_fail load_rc=$load_rc" >&2
  exit 20
fi
cat "$LOG_DIR/load.log"

touch "$LOG_DIR/monitor.stop"
wait "$MONPID"

python3 - "$LOG_DIR/wire-end.json" \
  "$(read_counter "$C" gc0 tx_bytes)" "$(read_counter "$C" gc0 rx_bytes)" \
  "$(read_counter "$S" gs0 tx_bytes)" "$(read_counter "$S" gs0 rx_bytes)" <<'PY_WIRE_END'
import json,sys
p=sys.argv[1]; vals=list(map(int,sys.argv[2:]))
d={'client_tx_bytes':vals[0],'client_rx_bytes':vals[1],'server_tx_bytes':vals[2],'server_rx_bytes':vals[3]}
open(p,'w').write(json.dumps(d,sort_keys=True,indent=2)+'\n')
PY_WIRE_END
{
  sudo ip netns exec "$C" tc -s qdisc show dev gc0
  sudo ip netns exec "$S" tc -s qdisc show dev gs0
} >"$LOG_DIR/netem-end.log" 2>&1 || true

sudo kill -TERM "$ECHOPID" 2>/dev/null || true
wait "$ECHOPID" 2>/dev/null || true
for p in "${PIDS[@]}"; do
  [[ "$p" == "$ECHOPID" ]] && continue
  sudo kill -TERM "$p" 2>/dev/null || kill -TERM "$p" 2>/dev/null || true
done
for p in "${PIDS[@]}"; do
  [[ "$p" == "$ECHOPID" ]] && continue
  wait "$p" 2>/dev/null || true
done
sleep 1
PIDS=()

python3 - "$LOG_DIR" "$LANES" "$FEC" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" <<'PY_SUM'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]); lanes=int(sys.argv[2]); fec=sys.argv[3]; duration=int(sys.argv[4]); rate=int(sys.argv[5]); payload=int(sys.argv[6])
load=json.load(open(root/'load-result.json')); cpu=json.load(open(root/'process-summary.json'))
ws=json.load(open(root/'wire-start.json')); we=json.load(open(root/'wire-end.json'))
wire={k:we[k]-ws[k] for k in ws}
echo_count=int((root/'echo-count.log').read_text().strip() or '0')
client_stats=[]
for path in sorted(root.glob('faketcp-[0-9]*.log')):
    for line in path.read_text(errors='replace').splitlines():
        if line.startswith('WBD_FAKETCP_STATS '):
            client_stats.append(json.loads(line.split(' ',1)[1]))
if len(client_stats)!=lanes:
    raise SystemExit(f'expected {lanes} client WBD_FAKETCP_STATS, got {len(client_stats)}')
enq=sum(int(x['sender']['EnqueuedBytes']) for x in client_stats)
retx=sum(int(x['sender']['RetransmitBytes']) for x in client_stats)
fast=sum(int(x['sender']['FastRetransmits']) for x in client_stats)
rto=sum(int(x['sender']['RTOTransmits']) for x in client_stats)
peaks=[int(x['sender']['PeakPending']) for x in client_stats]
useful_up=load['sent']*payload; useful_down_generated=echo_count*payload; useful_down_delivered=load['received_unique']*payload
def ratio(a,b): return (a/b if b else None)
summary={'analysis_only':True,'loss_is_not_a_hard_gate':True,'lanes':lanes,'fec':fec,'requested_duration_sec':duration,'requested_bps_each_direction':rate,
         'load':load,'cpu_memory':cpu,'echo_wbd1_count':echo_count,'wire_bytes':wire,
         'client_arq':{'enqueued_bytes':enq,'retransmit_bytes':retx,'retransmit_overhead_ratio':ratio(retx,enq),'fast_retransmits':fast,'rto_transmits':rto,'peak_pending_per_lane':peaks},
         'wire_amplification':{'client_tx_over_useful_up':ratio(wire['client_tx_bytes'],useful_up),'server_rx_over_useful_up':ratio(wire['server_rx_bytes'],useful_up),
                               'server_tx_over_generated_down':ratio(wire['server_tx_bytes'],useful_down_generated),'client_rx_over_delivered_down':ratio(wire['client_rx_bytes'],useful_down_delivered),
                               'aggregate_attempted_over_logical':ratio(wire['client_tx_bytes']+wire['server_tx_bytes'],useful_up+useful_down_generated)}}
open(root/'analysis-summary.json','w').write(json.dumps(summary,sort_keys=True,indent=2)+'\n')
print('WBD_LONG_ANALYSIS_COMPLETE '+json.dumps({'lanes':lanes,'fec':fec,'loss_ratio':load['loss_ratio'],'rtt_p50_ms':load['rtt_p50_ms'],'rtt_p95_ms':load['rtt_p95_ms'],'rtt_p99_ms':load['rtt_p99_ms'],'client_retx_ratio':summary['client_arq']['retransmit_overhead_ratio'],'wire_amp':summary['wire_amplification']['aggregate_attempted_over_logical'],'cpu_avg':cpu['total']['avg_cpu_pct'],'cpu_peak':cpu['total']['peak_cpu_pct'],'rss_avg_mb':cpu['total']['avg_rss_mb'],'rss_peak_mb':cpu['total']['peak_rss_mb']},sort_keys=True))
PY_SUM

cat "$LOG_DIR/analysis-summary.json"
'''
pathlib.Path(sys.argv[2]).write_text(prefix+tail)
PY_PATCH
chmod +x "$PATCHED"
bash -n "$PATCHED"
exec "$PATCHED" "$ASSET_DIR" "$LOG_DIR"
