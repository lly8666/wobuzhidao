#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?usage: game_lane_fullstack.sh ASSET_DIR [LOG_DIR]}
LOG_DIR=${2:-/tmp/wbd-game-lane-fullstack}
mkdir -p "$LOG_DIR" "$LOG_DIR/tickets"
chmod 700 "$LOG_DIR/tickets"

required=(
  wbd-reality-front wbd-faketcp wbd-faketcp-mux wbd-link-proxy wbd-link-server-mux
  wbd-game-lane-client wbd-game-lane-server wbd_dtls_shim front.pem front.key dtls.pem dtls.key
)
for f in "${required[@]}"; do
  test -e "$ASSET_DIR/$f" || { echo "missing asset $ASSET_DIR/$f" >&2; exit 1; }
done

C=wbdglc$$
S=wbdgls$$
PIDS=()
TPID=
cleanup() {
  set +e
  if [[ -n "${TPID:-}" ]]; then sudo kill -INT "$TPID" 2>/dev/null; wait "$TPID" 2>/dev/null; fi
  for p in "${PIDS[@]:-}"; do sudo kill -TERM "$p" 2>/dev/null; kill -TERM "$p" 2>/dev/null; done
  sleep .3
  sudo ip netns del "$C" 2>/dev/null
  sudo ip netns del "$S" 2>/dev/null
}
trap cleanup EXIT

ROUTE_KEY='WBD_REALITY_ROUTE_KEY_0123456789abcdef'
USERNAME='solo'
PASSWORD='shared-password'
TARGET='target.example'
FRONT=40443
RAW=40000
LINK=47000
GAME=49000
ECHO=48000
SESSION_ID=00112233445566778899aabbccddeeff

sudo ip netns add "$C"
sudo ip netns add "$S"
sudo ip link add gc0 type veth peer name gs0
sudo ip link set gc0 netns "$C"
sudo ip link set gs0 netns "$S"
sudo ip -n "$C" addr add 10.89.0.2/24 dev gc0
sudo ip -n "$S" addr add 10.89.0.1/24 dev gs0
for ns in "$C" "$S"; do sudo ip -n "$ns" link set lo up; done
sudo ip -n "$C" link set gc0 up
sudo ip -n "$S" link set gs0 up
sudo ip netns exec "$C" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP

cat >"$LOG_DIR/echo.py" <<'PY'
import pathlib,socket
count=pathlib.Path(__import__('sys').argv[1])
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    with count.open('ab') as f: f.write(b.hex().encode()+b'\n')
    s.sendto(b,a)
PY
: >"$LOG_DIR/echo-count.log"
sudo ip netns exec "$S" python3 "$LOG_DIR/echo.py" "$LOG_DIR/echo-count.log" >"$LOG_DIR/echo.log" 2>&1 &
ECHOPID=$!; PIDS+=("$ECHOPID")

sudo ip netns exec "$S" "$ASSET_DIR/wbd-game-lane-server" \
  -listen 127.0.0.1:${GAME} -service 127.0.0.1:${ECHO} -max-lanes 4 \
  >"$LOG_DIR/game-server.log" 2>&1 &
GSPID=$!; PIDS+=("$GSPID")
for _ in $(seq 1 200); do grep -q 'WBD_GAME_LANE_SERVER_READY' "$LOG_DIR/game-server.log" && break; sleep .05; done
grep -q 'WBD_GAME_LANE_SERVER_READY' "$LOG_DIR/game-server.log"

sudo ip netns exec "$S" "$ASSET_DIR/wbd-reality-front" server \
  -listen 10.89.0.1:${FRONT} -target 127.0.0.1:9 -server-name "$TARGET" \
  -cert "$ASSET_DIR/front.pem" -key "$ASSET_DIR/front.key" -route-key "$ROUTE_KEY" \
  -username "$USERNAME" -password "$PASSWORD" -ticket-dir "$LOG_DIR/tickets" \
  >"$LOG_DIR/front-server.log" 2>&1 &
FRONTPID=$!; PIDS+=("$FRONTPID")
for _ in $(seq 1 300); do grep -q 'WBD_REALITY_FRONT_READY' "$LOG_DIR/front-server.log" && break; sleep .05; done
grep -q 'WBD_REALITY_FRONT_READY' "$LOG_DIR/front-server.log"

for i in 1 2 3 4; do
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-reality-front" client \
    -addr 10.89.0.1:${FRONT} -server-name "$TARGET" -route-key "$ROUTE_KEY" \
    -username "$USERNAME" -password "$PASSWORD" -verify-server=false \
    -ticket-out "$LOG_DIR/ticket-${i}.txt" >"$LOG_DIR/front-${i}.log" 2>&1
  grep -q 'WBD_REALITY_FRONT_OK' "$LOG_DIR/front-${i}.log"
done
python3 - "$LOG_DIR" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1])
t=[(p/f'ticket-{i}.txt').read_text().strip() for i in range(1,5)]
assert all(len(x)==64 for x in t), t
assert len(set(t))==4, t
print('WBD_GAME_LANE_TICKETS_PASS unique=4')
PY

sudo ip netns exec "$S" "$ASSET_DIR/wbd-link-server-mux" \
  -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} \
  -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 60s -max-sessions 8 \
  >"$LOG_DIR/link-server.log" 2>&1 &
LINKPID=$!; PIDS+=("$LINKPID")
for _ in $(seq 1 200); do grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log" && break; sleep .05; done
grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log"

sudo ip netns exec "$S" "$ASSET_DIR/wbd-faketcp-mux" server \
  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \
  --link-target 127.0.0.1:${LINK} --cert "$ASSET_DIR/dtls.pem" --key "$ASSET_DIR/dtls.key" \
  --max-sessions 8 >"$LOG_DIR/faketcp-mux.log" 2>&1 &
MUXPID=$!; PIDS+=("$MUXPID")
for _ in $(seq 1 300); do grep -q 'READY role=server-mux.*recovery=legacy' "$LOG_DIR/faketcp-mux.log" && break; sleep .05; done
grep -q 'READY role=server-mux.*recovery=legacy' "$LOG_DIR/faketcp-mux.log"

sudo ip netns exec "$C" tcpdump -i gc0 -s 0 -U -w "$LOG_DIR/game-lanes.pcap" "tcp port ${RAW}" >"$LOG_DIR/tcpdump.log" 2>&1 &
TPID=$!
for _ in $(seq 1 200); do grep -q 'listening on gc0' "$LOG_DIR/tcpdump.log" && break; sleep .05; done
grep -q 'listening on gc0' "$LOG_DIR/tcpdump.log"

for i in 1 2 3 4; do
  fport=$((45100+i)); sport=$((41000+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-faketcp" client \
    --local-udp 127.0.0.1:${fport} --source 10.89.0.2:${sport} --remote 10.89.0.1:${RAW} \
    >"$LOG_DIR/faketcp-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 500); do
  ok=1
  for i in 1 2 3 4; do grep -q 'READY role=client.*recovery=legacy' "$LOG_DIR/faketcp-${i}.log" || ok=0; done
  [[ $ok -eq 1 ]] && break
  sleep .05
done
for i in 1 2 3 4; do grep -q 'READY role=client.*recovery=legacy' "$LOG_DIR/faketcp-${i}.log"; done

for i in 1 2 3 4; do
  dport=$((46100+i)); fport=$((45100+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd_dtls_shim" client ${dport} 127.0.0.1 ${fport} none none \
    >"$LOG_DIR/dtls-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 900); do
  ok=1
  for i in 1 2 3 4; do grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$LOG_DIR/dtls-${i}.log" || ok=0; done
  bound=$(grep -c 'BOUND role=server.*inherited=yes' "$LOG_DIR/faketcp-mux.log" || true)
  [[ $ok -eq 1 && $bound -ge 4 ]] && break
  sleep .05
done
for i in 1 2 3 4; do grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$LOG_DIR/dtls-${i}.log"; done
test "$(grep -c 'BOUND role=server.*inherited=yes' "$LOG_DIR/faketcp-mux.log")" -eq 4

for i in 1 2 3 4; do
  ticket=$(tr -d '\r\n' <"$LOG_DIR/ticket-${i}.txt")
  lport=$((47100+i)); dport=$((46100+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-link-proxy" \
    -mode client -listen 127.0.0.1:${lport} -dtls 127.0.0.1:${dport} -fec off \
    -demo-reality-ticket "$ticket" >"$LOG_DIR/link-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 1000); do
  ok=1
  for i in 1 2 3 4; do grep -q 'WBD_LINK_READY role=client' "$LOG_DIR/link-${i}.log" || ok=0; done
  sessions=$(grep -c 'WBD_LINK_MUX_SESSION_READY account=solo' "$LOG_DIR/link-server.log" || true)
  [[ $ok -eq 1 && $sessions -ge 4 ]] && break
  sleep .05
done
for i in 1 2 3 4; do grep -q 'WBD_LINK_READY role=client' "$LOG_DIR/link-${i}.log"; done
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY account=solo' "$LOG_DIR/link-server.log")" -eq 4
test "$(find "$LOG_DIR/tickets" -type f | wc -l)" -eq 0

sudo ip netns exec "$C" "$ASSET_DIR/wbd-game-lane-client" \
  -listen 127.0.0.1:47500 \
  -lanes 127.0.0.1:47101,127.0.0.1:47102,127.0.0.1:47103,127.0.0.1:47104 \
  -session-id "$SESSION_ID" >"$LOG_DIR/game-client.log" 2>&1 &
GCPID=$!; PIDS+=("$GCPID")
for _ in $(seq 1 300); do grep -q 'WBD_GAME_LANE_CLIENT_READY.*lanes=4' "$LOG_DIR/game-client.log" && break; sleep .05; done
grep -q 'WBD_GAME_LANE_CLIENT_READY.*lanes=4' "$LOG_DIR/game-client.log"
test "$(grep -c 'WBD_GAME_LANE_OUTER lane=' "$LOG_DIR/game-client.log")" -eq 4

cat >"$LOG_DIR/probe.py" <<'PY'
import socket,sys,time
count=int(sys.argv[1]); prefix=sys.argv[2].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(2)
for i in range(count):
    b=prefix+i.to_bytes(2,'big')
    s.sendto(b,('127.0.0.1',47500))
    got,_=s.recvfrom(65535)
    assert got==b,(i,got,b)
    time.sleep(.01)
print(f'WBD_GAME_LANE_PROBE_PASS count={count} prefix={prefix.decode()}')
PY
sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" 1 warm >"$LOG_DIR/probe-warm.log" 2>&1
for _ in $(seq 1 300); do
  binds=$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log" || true)
  [[ $binds -ge 4 ]] && break
  sleep .05
done
test "$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log")" -eq 4
sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" 10 game >"$LOG_DIR/probe.log" 2>&1
grep -q 'WBD_GAME_LANE_PROBE_PASS count=10' "$LOG_DIR/probe.log"

# One warmup + ten measured logical datagrams must reach downstream exactly once each.
test "$(wc -l <"$LOG_DIR/echo-count.log")" -eq 11

sudo kill -TERM "$GCPID" 2>/dev/null || true
wait "$GCPID" || true
PIDS=("${PIDS[@]/$GCPID}")
sudo kill -TERM "$GSPID" 2>/dev/null || true
wait "$GSPID" || true
PIDS=("${PIDS[@]/$GSPID}")
grep -q 'WBD_GAME_LANE_CLIENT_STATS logical_tx=11 delivered=11' "$LOG_DIR/game-client.log"
grep -Eq 'WBD_GAME_LANE_SESSION_CLOSE .*in_first=11 .*in_dup=[1-9][0-9]* .*out_logical=11 .*out_lane=[1-9][0-9]*' "$LOG_DIR/game-server.log"

sudo kill -INT "$TPID" 2>/dev/null || true
wait "$TPID" || true
TPID=
python3 "$GITHUB_WORKSPACE/scripts/analyze_game_lane_pcap.py" "$LOG_DIR/game-lanes.pcap" \
  --server-port "$RAW" --client-ports 41001,41002,41003,41004 --out "$LOG_DIR/game-lanes-wire.json"
grep -q 'WBD_GAME_LANE_PCAP_PASS flows=4 distinct_5tuple=1 distinct_seq_space=1' <(python3 "$GITHUB_WORKSPACE/scripts/analyze_game_lane_pcap.py" "$LOG_DIR/game-lanes.pcap" --server-port "$RAW" --client-ports 41001,41002,41003,41004)

echo 'WBD_GAME_LANE_FULLSTACK_PASS lanes=4 logical_delivery_once=1 outer_flows_distinct=1 fec=off'
