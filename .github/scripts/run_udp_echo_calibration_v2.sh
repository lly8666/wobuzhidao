#!/usr/bin/env bash
set -euo pipefail
RATE_BPS=${1:?usage: run_udp_echo_calibration_v2.sh RATE_BPS CASE_ID MAX_PAYLOAD}
CASE_ID=${2:?case id required}
MAX_PAYLOAD=${3:?max payload required}
DURATION_SEC=${CAL_DURATION_SEC:-30}
OUT="${RUNNER_TEMP:?}/udp-echo-cal-${CASE_ID}"
rm -rf "$OUT"; mkdir -p "$OUT"
C="wbdcalc$$"; S="wbdcals$$"
PIDS=()
cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do sudo kill -TERM "$p" 2>/dev/null || true; kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  sudo ip netns del "$C" 2>/dev/null || true
  sudo ip netns del "$S" 2>/dev/null || true
}
trap cleanup EXIT

sudo ip netns add "$C"
sudo ip netns add "$S"
sudo ip link add calgc0 type veth peer name calgs0
sudo ip link set calgc0 netns "$C"
sudo ip link set calgs0 netns "$S"
sudo ip -n "$C" addr add 10.90.0.2/24 dev calgc0
sudo ip -n "$S" addr add 10.90.0.1/24 dev calgs0
sudo ip -n "$C" link set lo up; sudo ip -n "$S" link set lo up
sudo ip -n "$C" link set calgc0 up; sudo ip -n "$S" link set calgs0 up
sudo ip netns exec "$C" cat /proc/net/snmp >"$OUT/client-snmp-start.txt"
sudo ip netns exec "$S" cat /proc/net/snmp >"$OUT/server-snmp-start.txt"

cat >"$OUT/echo.py" <<'PY'
import json,signal,socket,sys,time
out=sys.argv[1]
stop=False
def sig(*_):
    global stop; stop=True
signal.signal(signal.SIGTERM,sig); signal.signal(signal.SIGINT,sig)
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF,4<<20)
s.setsockopt(socket.SOL_SOCKET,socket.SO_SNDBUF,4<<20)
s.bind(('10.90.0.1',47500)); s.settimeout(.2)
start_epoch_ns=time.time_ns(); packets=0; rx_bytes=0; tx_bytes=0; send_errors=0; last_print=time.monotonic()
while not stop:
    try: b,a=s.recvfrom(65535)
    except socket.timeout: b=None
    if b is not None:
        packets+=1; rx_bytes+=len(b)
        try:
            n=s.sendto(b,a); tx_bytes+=n
        except OSError:
            send_errors+=1
    now=time.monotonic()
    if now-last_print>=1:
        print('WBD_UDP_ECHO_DIAG '+json.dumps({'epoch_ns':time.time_ns(),'packets':packets,'rx_bytes':rx_bytes,'tx_bytes':tx_bytes,'send_errors':send_errors},sort_keys=True),flush=True)
        last_print=now
res={'start_epoch_ns':start_epoch_ns,'end_epoch_ns':time.time_ns(),'packets':packets,'rx_bytes':rx_bytes,'tx_bytes':tx_bytes,'send_errors':send_errors,'effective_rcvbuf_bytes':s.getsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF),'effective_sndbuf_bytes':s.getsockopt(socket.SOL_SOCKET,socket.SO_SNDBUF)}
open(out,'w').write(json.dumps(res,indent=2,sort_keys=True)+'\n')
print('WBD_UDP_ECHO_RESULT '+json.dumps(res,sort_keys=True),flush=True)
PY
sudo ip netns exec "$S" python3 "$OUT/echo.py" "$OUT/echo-result.json" >"$OUT/echo.log" 2>&1 &
EPID=$!; PIDS+=("$EPID")
sleep .3

sudo ip netns exec "$C" env \
  WBD_LOAD_BIND=10.90.0.2:47600 \
  WBD_LOAD_DST=10.90.0.1:47500 \
  WBD_LOAD_MAX_PAYLOAD="$MAX_PAYLOAD" \
  WBD_LOAD_PHASE_SPEC="all:0:${DURATION_SEC}" \
  WBD_LOAD_DRAIN_SEC=5 \
  python3 "${GITHUB_WORKSPACE:?}/suite/.github/scripts/paced_udp_load_v2.py" \
  "$OUT/load-result.json" "$DURATION_SEC" "$RATE_BPS" 1000 >"$OUT/load.log" 2>&1

sudo kill -TERM "$EPID" 2>/dev/null || true
wait "$EPID" 2>/dev/null || true
PIDS=()
sudo ip netns exec "$C" cat /proc/net/snmp >"$OUT/client-snmp-end.txt"
sudo ip netns exec "$S" cat /proc/net/snmp >"$OUT/server-snmp-end.txt"
sudo ip netns exec "$C" ss -uapnem >"$OUT/client-sockets-final.txt" || true
sudo ip netns exec "$S" ss -uapnem >"$OUT/server-sockets-final.txt" || true
sudo chmod -R a+rX "$OUT" || true

python3 - "$OUT" "$CASE_ID" "$RATE_BPS" "$DURATION_SEC" <<'PY'
import json,pathlib,sys
root=pathlib.Path(sys.argv[1]); case=sys.argv[2]; rate=int(sys.argv[3]); duration=float(sys.argv[4])
load=json.load(open(root/'load-result.json')); echo=json.load(open(root/'echo-result.json'))
def udp(path):
    lines=path.read_text().splitlines()
    for i,line in enumerate(lines):
        if line.startswith('Udp:') and i+1<len(lines) and lines[i+1].startswith('Udp:'):
            keys=line.split()[1:]; vals=lines[i+1].split()[1:]
            return {k:int(v) for k,v in zip(keys,vals)}
    return {}
def delta(a,b):
    return {k:b.get(k,0)-a.get(k,0) for k in set(a)|set(b)}
cu=delta(udp(root/'client-snmp-start.txt'),udp(root/'client-snmp-end.txt'))
su=delta(udp(root/'server-snmp-start.txt'),udp(root/'server-snmp-end.txt'))
local_err=sum(max(0,d.get(k,0)) for d in (cu,su) for k in ('InErrors','RcvbufErrors','SndbufErrors'))
valid=bool(load.get('load_valid_for_capacity') and local_err==0 and echo.get('send_errors',0)==0 and load.get('received_payload_bytes')==load.get('sent_payload_bytes'))
out={'case_id':case,'target_bps':rate,'duration_sec':duration,'actual_injection_bps':load.get('actual_injection_bps'),'actual_injection_ratio':load.get('actual_injection_ratio'),'load_valid_for_capacity':load.get('load_valid_for_capacity'),'received_payload_bytes':load.get('received_payload_bytes'),'sent_payload_bytes':load.get('sent_payload_bytes'),'echo_received_bytes':echo.get('rx_bytes'),'echo_packets':echo.get('packets'),'echo_send_errors':echo.get('send_errors'),'load_socket_buffers':load.get('socket_buffers'),'echo_socket_buffers':{'effective_rcvbuf_bytes':echo.get('effective_rcvbuf_bytes'),'effective_sndbuf_bytes':echo.get('effective_sndbuf_bytes')},'client_udp_delta':cu,'server_udp_delta':su,'send_lag_ms_p50':load.get('send_lag_ms_p50'),'send_lag_ms_p99':load.get('send_lag_ms_p99'),'send_lag_ms_max':load.get('send_lag_ms_max'),'send_lag_sustained_increase':load.get('send_lag_sustained_increase'),'per_second':load.get('per_second'),'size_sent_counts':load.get('size_sent_counts'),'calibration_valid':valid}
json.dump(out,open(root/'calibration-summary.json','w'),indent=2,sort_keys=True); open(root/'calibration-summary.json','a').write('\n')
print('WBD_UDP_CALIBRATION_SUMMARY '+json.dumps(out,sort_keys=True))
PY
cat "$OUT/calibration-summary.json"
