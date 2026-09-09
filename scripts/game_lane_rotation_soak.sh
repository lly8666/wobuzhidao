#!/usr/bin/env bash
set -euo pipefail

BASE="${GITHUB_WORKSPACE:-$(pwd)}/scripts/game_lane_fullstack.sh"
PATCHED="${RUNNER_TEMP:-/tmp}/game_lane_rotation_soak_generated.sh"
python3 - "$BASE" "$PATCHED" <<'PY_PATCHER'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
marker = 'cat >"$LOG_DIR/probe.py" <<\'PY\'\n'
pos = src.find(marker)
if pos < 0:
    raise SystemExit('soak tail insertion point not found')
# Give all initial transports the same keepalive used by replacements and a
# generous bootstrap budget. The generated script otherwise preserves the
# established full-stack setup verbatim.
prefix = src[:pos]
prefix = prefix.replace('--bootstrap-timeout 12s', '--bootstrap-timeout 30s')
prefix = prefix.replace('--reality-timeout 12s', '--reality-timeout 30s')
prefix = prefix.replace('-demo-reality-ticket "$ticket" >"$LOG_DIR/link-${i}.log"',
                        '-demo-reality-ticket "$ticket" -keepalive 2s >"$LOG_DIR/link-${i}.log"')
tail = r'''DURATION_SEC=${DURATION_SEC:-500}
RATE_BPS=${RATE_BPS:-1000000}
ROTATE_INTERVAL_SEC=${ROTATE_INTERVAL_SEC:-25}
ROTATIONS=${ROTATIONS:-16}
PAYLOAD_BYTES=${PAYLOAD_BYTES:-1000}

[[ "$LANES" == 4 ]] || { echo "soak requires LANES=4" >&2; exit 2; }
[[ "$DURATION_SEC" =~ ^[0-9]+$ && "$DURATION_SEC" -ge 500 ]] || { echo "DURATION_SEC must be >=500" >&2; exit 2; }
[[ "$RATE_BPS" == 1000000 ]] || { echo "RATE_BPS must be exactly 1000000" >&2; exit 2; }
[[ "$ROTATIONS" =~ ^[0-9]+$ && "$ROTATIONS" -ge 1 ]] || { echo "ROTATIONS must be positive" >&2; exit 2; }
[[ "$ROTATE_INTERVAL_SEC" =~ ^[0-9]+$ && "$ROTATE_INTERVAL_SEC" -ge 1 ]] || { echo "ROTATE_INTERVAL_SEC must be positive" >&2; exit 2; }
[[ "$PAYLOAD_BYTES" =~ ^[0-9]+$ && "$PAYLOAD_BYTES" -ge 64 ]] || { echo "PAYLOAD_BYTES must be >=64" >&2; exit 2; }

# The base harness starts tcpdump before bootstrap to prove single-flow lineage.
# That proof is covered by its own workflow; stop it before the 500 s soak so the
# qualification artifact stays small and does not retain full payloads.
sudo kill -INT "$TPID" 2>/dev/null || true
wait "$TPID" 2>/dev/null || true
TPID=
rm -f "$LOG_DIR/game-lanes.pcap"

count_marker() {
  local pattern=$1 file=$2
  grep -c "$pattern" "$file" 2>/dev/null || true
}

wait_count_gt() {
  local pattern=$1 file=$2 before=$3 loops=${4:-400}
  local n=0
  for _ in $(seq 1 "$loops"); do
    n=$(count_marker "$pattern" "$file")
    if (( n > before )); then return 0; fi
    sleep .05
  done
  echo "marker did not advance: pattern=$pattern file=$file before=$before now=$n" >&2
  return 1
}

pid_one() {
  local pattern=$1
  mapfile -t hits < <(pgrep -f "$pattern" || true)
  if [[ ${#hits[@]} -ne 1 ]]; then
    echo "expected one pid for pattern=$pattern; got ${#hits[@]}: ${hits[*]:-}" >&2
    return 1
  fi
  printf '%s\n' "${hits[0]}"
}

# Capture the initial process ids by their stable per-lane endpoints. New
# generations keep the Game/LINK service port fixed while replacing every
# transport below it.
declare -a LINK_PIDS DTLS_PIDS FAKETCP_PIDS LANE_GEN LANE_SPORT
for i in $(seq 1 4); do
  LINK_PIDS[$i]=$(pid_one "wbd-link-proxy.*-listen 127.0.0.1:$((47100+i))")
  DTLS_PIDS[$i]=$(pid_one "wbd_dtls_shim client $((46100+i)) 127.0.0.1 $((45100+i))")
  FAKETCP_PIDS[$i]=$(pid_one "wbd-faketcp client.*--source 10.89.0.2:$((41000+i))")
  LANE_GEN[$i]=0
  LANE_SPORT[$i]=$((41000+i))
done

# Make sure the initial server-side bindings are fully visible before load.
for _ in $(seq 1 400); do
  link_binds=$(count_marker 'WBD_LINK_LOGICAL_TUNNEL_BIND ' "$LOG_DIR/link-server.log")
  game_binds=$(count_marker 'WBD_GAME_LANE_BIND ' "$LOG_DIR/game-server.log")
  if (( link_binds >= 4 && game_binds >= 4 )); then break; fi
  sleep .05
done
(( $(count_marker 'WBD_LINK_LOGICAL_TUNNEL_BIND ' "$LOG_DIR/link-server.log") >= 4 ))
(( $(count_marker 'WBD_GAME_LANE_BIND ' "$LOG_DIR/game-server.log") >= 4 ))

cat >"$LOG_DIR/send_rst.py" <<'PY_RST'
import socket, struct, sys, time
src, sport, dst, dport = sys.argv[1], int(sys.argv[2]), sys.argv[3], int(sys.argv[4])

def csum(buf):
    if len(buf) & 1: buf += b'\0'
    total = sum((buf[i] << 8) + buf[i+1] for i in range(0, len(buf), 2))
    while total >> 16:
        total = (total & 0xffff) + (total >> 16)
    return (~total) & 0xffff

srcb, dstb = socket.inet_aton(src), socket.inet_aton(dst)
tcp = struct.pack('!HHLLBBHHH', sport, dport, 0, 0, 5 << 4, 0x04, 0, 0, 0)
tcp_sum = csum(srcb + dstb + struct.pack('!BBH', 0, socket.IPPROTO_TCP, len(tcp)) + tcp)
tcp = struct.pack('!HHLLBBH', sport, dport, 0, 0, 5 << 4, 0x04, 0) + struct.pack('!H', tcp_sum) + struct.pack('!H', 0)
ver_ihl = (4 << 4) | 5
ip = struct.pack('!BBHHHBBH4s4s', ver_ihl, 0, 20 + len(tcp), int(time.time_ns()) & 0xffff, 0, 64, socket.IPPROTO_TCP, 0, srcb, dstb)
ip_sum = csum(ip)
ip = struct.pack('!BBHHHBBH4s4s', ver_ihl, 0, 20 + len(tcp), int(time.time_ns()) & 0xffff, 0, 64, socket.IPPROTO_TCP, ip_sum, srcb, dstb)
s = socket.socket(socket.AF_INET, socket.SOCK_RAW, socket.IPPROTO_RAW)
s.setsockopt(socket.IPPROTO_IP, socket.IP_HDRINCL, 1)
for _ in range(3):
    s.sendto(ip + tcp, (dst, dport))
    time.sleep(.02)
print(f'WBD_HOSTED_RETIRE_RST_TX source_port={sport} remote_port={dport} copies=3')
PY_RST

send_retire_rst() {
  local sport=$1
  # Keep the namespace-wide kernel-RST suppressor in place. Insert a narrow
  # first-match exception only for the deliberate old exact-flow retirement RST.
  sudo ip netns exec "$C" iptables -I OUTPUT 1 -p tcp --sport "$sport" --dport "$RAW" --tcp-flags RST RST -j ACCEPT
  sudo ip netns exec "$C" python3 "$LOG_DIR/send_rst.py" 10.89.0.2 "$sport" 10.89.0.1 "$RAW" >>"$LOG_DIR/rotation.log" 2>&1
  sudo ip netns exec "$C" iptables -D OUTPUT 1
}

start_replacement_lane() {
  local lane=$1 gen=$2
  local fport=$((45100 + gen*100 + lane))
  local dport=$((46100 + gen*100 + lane))
  local sport=$((41000 + gen*100 + lane))
  local lport=$((47100 + lane))
  local ticket_file="$LOG_DIR/ticket-${lane}-g${gen}.txt"
  local tunnel_file="$LOG_DIR/tunnel-${lane}-g${gen}.json"
  local fake_log="$LOG_DIR/lane-${lane}-g${gen}-faketcp.log"
  local dtls_log="$LOG_DIR/lane-${lane}-g${gen}-dtls.log"
  local link_log="$LOG_DIR/lane-${lane}-g${gen}-link.log"
  local before_link_bind
  before_link_bind=$(count_marker 'WBD_LINK_LOGICAL_TUNNEL_BIND ' "$LOG_DIR/link-server.log")

  sudo ip netns exec "$C" "$ASSET_DIR/wbd-faketcp" client \
    --local-udp 127.0.0.1:${fport} --source 10.89.0.2:${sport} --remote 10.89.0.1:${RAW} \
    --shadow-recovery legacy \
    --reality-server-name "$TARGET" --reality-route-key "$ROUTE_KEY" \
    --reality-username "$USERNAME" --reality-password "$PASSWORD" \
    --reality-ticket-out "$ticket_file" \
    --reality-installation-id "$INSTALLATION_ID" \
    --reality-tunnel-config-out "$tunnel_file" \
    --reality-verify-server=false --reality-timeout 30s \
    >"$fake_log" 2>&1 &
  local fpid=$!; PIDS+=("$fpid")
  for _ in $(seq 1 700); do
    grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$fake_log" && \
    grep -q 'READY role=client.*single_flow_bootstrap=true' "$fake_log" && break
    kill -0 "$fpid" 2>/dev/null || { tail -n 100 "$fake_log" >&2; return 1; }
    sleep .05
  done
  grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$fake_log"
  grep -q 'READY role=client.*single_flow_bootstrap=true' "$fake_log"

  local replacement_tunnel
  replacement_tunnel=$(python3 - "$tunnel_file" <<'PY_TUN'
import json,sys
print(json.load(open(sys.argv[1]))['tunnel_id'])
PY_TUN
)
  [[ "$replacement_tunnel" == "$SESSION_ID" ]] || { echo "replacement tunnel changed lane=$lane gen=$gen" >&2; return 1; }

  sudo ip netns exec "$C" "$ASSET_DIR/wbd_dtls_shim" client ${dport} 127.0.0.1 ${fport} none none >"$dtls_log" 2>&1 &
  local dpid=$!; PIDS+=("$dpid")
  for _ in $(seq 1 900); do
    grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$dtls_log" && break
    kill -0 "$dpid" 2>/dev/null || { tail -n 100 "$dtls_log" >&2; return 1; }
    sleep .05
  done
  grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$dtls_log"

  local ticket
  ticket=$(tr -d '\r\n' <"$ticket_file")
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-link-proxy" \
    -mode client -listen 127.0.0.1:${lport} -dtls 127.0.0.1:${dport} \
    -fec "$FEC" -lanes 1 -mtu "$LINK_PLAINTEXT_MTU" -keepalive 2s \
    -demo-reality-ticket "$ticket" >"$link_log" 2>&1 &
  local lpid=$!; PIDS+=("$lpid")
  for _ in $(seq 1 1000); do
    grep -q "WBD_LINK_READY role=client fec=${FEC}" "$link_log" && break
    kill -0 "$lpid" 2>/dev/null || { tail -n 100 "$link_log" >&2; return 1; }
    sleep .05
  done
  grep -q "WBD_LINK_READY role=client fec=${FEC}" "$link_log"
  wait_count_gt 'WBD_LINK_LOGICAL_TUNNEL_BIND ' "$LOG_DIR/link-server.log" "$before_link_bind" 500

  FAKETCP_PIDS[$lane]=$fpid
  DTLS_PIDS[$lane]=$dpid
  LINK_PIDS[$lane]=$lpid
  LANE_GEN[$lane]=$gen
  LANE_SPORT[$lane]=$sport
}

retire_lane() {
  local lane=$1
  local old_link=${LINK_PIDS[$lane]} old_dtls=${DTLS_PIDS[$lane]} old_fake=${FAKETCP_PIDS[$lane]}
  local old_sport=${LANE_SPORT[$lane]}
  local before_leave before_reset
  before_leave=$(count_marker 'WBD_GAME_LANE_UNBIND reason=client_leave' "$LOG_DIR/game-server.log")
  before_reset=$(count_marker 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log")

  sudo kill -TERM "$old_link" 2>/dev/null || true
  wait "$old_link" 2>/dev/null || true
  wait_count_gt 'WBD_GAME_LANE_UNBIND reason=client_leave' "$LOG_DIR/game-server.log" "$before_leave" 400

  sudo kill -TERM "$old_dtls" 2>/dev/null || true
  wait "$old_dtls" 2>/dev/null || true
  send_retire_rst "$old_sport"
  wait_count_gt 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log" "$before_reset" 200
  sudo kill -TERM "$old_fake" 2>/dev/null || true
  wait "$old_fake" 2>/dev/null || true
}

cat >"$LOG_DIR/load.py" <<'PY_LOAD'
import json, select, socket, struct, sys, time
out_path=sys.argv[1]
duration=float(sys.argv[2]); rate_bps=int(sys.argv[3]); payload_bytes=int(sys.argv[4])
pps=rate_bps/(payload_bytes*8.0)
interval=1.0/pps
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47600))
s.setblocking(False)
start=time.monotonic(); deadline=start+duration; next_tx=start
sent=0; unique=set(); dup=0; bad=0; last_rx=start
pad=b'Z'*(payload_bytes-20)
while True:
    now=time.monotonic()
    while now >= next_tx and next_tx < deadline:
        payload=b'WBD1'+struct.pack('!Qd',sent,next_tx-start)+pad
        if len(payload)!=payload_bytes: raise RuntimeError(len(payload))
        s.sendto(payload,('127.0.0.1',47500)); sent+=1; next_tx+=interval
        now=time.monotonic()
    r,_,_=select.select([s],[],[],0.002)
    if r:
        try: data,_=s.recvfrom(65535)
        except BlockingIOError: data=b''
        if data:
            last_rx=time.monotonic()
            if len(data)!=payload_bytes or not data.startswith(b'WBD1'):
                bad+=1
            else:
                seq=struct.unpack('!Q',data[4:12])[0]
                if seq in unique: dup+=1
                else: unique.add(seq)
    if now >= deadline and (now-last_rx >= 2.0 or now >= deadline+5.0):
        break
elapsed=max(duration,1e-9)
recv=len(unique); lost=max(0,sent-recv)
summary={
  'duration_sec':duration,'rate_target_bps':rate_bps,'payload_bytes':payload_bytes,
  'sent':sent,'received_unique':recv,'lost':lost,'duplicates':dup,'bad_payload':bad,
  'up_payload_bps':sent*payload_bytes*8/elapsed,
  'down_payload_bps':recv*payload_bytes*8/elapsed,
  'loss_ratio':(lost/sent if sent else 1.0),
}
open(out_path,'w').write(json.dumps(summary,sort_keys=True,indent=2)+'\n')
print('WBD_SOAK_LOAD_RESULT '+json.dumps(summary,sort_keys=True))
if bad != 0 or dup != 0: raise SystemExit(10)
if sent < int(duration*pps*0.995): raise SystemExit(11)
if not (rate_bps*0.995 <= summary['up_payload_bps'] <= rate_bps*1.005): raise SystemExit(12)
if summary['loss_ratio'] > 0.001: raise SystemExit(13)
if summary['down_payload_bps'] < rate_bps*0.995: raise SystemExit(14)
PY_LOAD

: >"$LOG_DIR/rotation.log"
sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &
LOAD_PID=$!; PIDS+=("$LOAD_PID")
load_start=$(date +%s)

for r in $(seq 1 "$ROTATIONS"); do
  target=$((load_start + r*ROTATE_INTERVAL_SEC))
  while (( $(date +%s) < target )); do
    kill -0 "$LOAD_PID" 2>/dev/null || { cat "$LOG_DIR/load.log" >&2; exit 1; }
    sleep 1
  done
  lane=$(( (r-1)%4 + 1 ))
  gen=$(( ${LANE_GEN[$lane]} + 1 ))
  echo "WBD_SOAK_ROTATION_START rotation=$r lane=$lane old_gen=${LANE_GEN[$lane]} new_gen=$gen fec=$FEC" | tee -a "$LOG_DIR/rotation.log"
  retire_lane "$lane"
  start_replacement_lane "$lane" "$gen"
  echo "WBD_SOAK_ROTATION_PASS rotation=$r lane=$lane generation=$gen fec=$FEC" | tee -a "$LOG_DIR/rotation.log"
done

wait "$LOAD_PID"
PIDS=("${PIDS[@]/$LOAD_PID}")
cat "$LOG_DIR/load.log"
cat "$LOG_DIR/load-result.json"

peer_resets=$(count_marker 'WBD_FAKETCP_MUX_PEER_RESET ' "$LOG_DIR/faketcp-mux.log")
idle_expire=$(count_marker 'WBD_FAKETCP_MUX_SESSION_EXPIRE reason=no_client_rx' "$LOG_DIR/faketcp-mux.log")
leaves=$(count_marker 'WBD_GAME_LANE_UNBIND reason=client_leave' "$LOG_DIR/game-server.log")
rot_ok=$(count_marker 'WBD_SOAK_ROTATION_PASS ' "$LOG_DIR/rotation.log")
[[ "$rot_ok" -eq "$ROTATIONS" ]]
[[ "$peer_resets" -eq "$ROTATIONS" ]]
[[ "$idle_expire" -eq 0 ]]
[[ "$leaves" -ge "$ROTATIONS" ]]

python3 - "$LOG_DIR/load-result.json" "$FEC" "$ROTATIONS" "$peer_resets" "$leaves" "$idle_expire" "$DURATION_SEC" "$RATE_BPS" <<'PY_SUM'
import json,sys
p,fec=sys.argv[1],sys.argv[2]
rot,resets,leaves,idle,duration,rate=map(int,sys.argv[3:])
d=json.load(open(p))
d.update({'fec':fec,'lanes':4,'rotations':rot,'peer_resets':resets,'client_leave_unbinds':leaves,
          'no_client_rx_expire':idle,'requested_duration_sec':duration,'requested_bps_each_direction':rate})
open(p,'w').write(json.dumps(d,sort_keys=True,indent=2)+'\n')
print('WBD_GAME_LANE_ROTATION_SOAK_PASS '+json.dumps(d,sort_keys=True))
PY_SUM

# Stop Game endpoints so their final statistics are available in the artifact.
sudo kill -TERM "$GCPID" 2>/dev/null || true
wait "$GCPID" 2>/dev/null || true
PIDS=("${PIDS[@]/$GCPID}")
sudo kill -TERM "$GSPID" 2>/dev/null || true
wait "$GSPID" 2>/dev/null || true
PIDS=("${PIDS[@]/$GSPID}")
'''
pathlib.Path(sys.argv[2]).write_text(prefix + tail)
PY_PATCHER
chmod +x "$PATCHED"
exec "$PATCHED" "$@"
