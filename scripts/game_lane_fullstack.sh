#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?usage: game_lane_fullstack.sh ASSET_DIR [LOG_DIR]}
LOG_DIR=${2:-/tmp/wbd-game-lane-fullstack}
LANES=${LANES:-4}
FEC=${FEC:-off}
PROBE_COUNT=${PROBE_COUNT:-20}

case "$LANES" in
  1|2|3|4) ;;
  *) echo "LANES must be 1..4" >&2; exit 2 ;;
esac
case "$FEC" in
  off|20:20) ;;
  *) echo "FEC must be off or 20:20" >&2; exit 2 ;;
esac
if ! [[ "$PROBE_COUNT" =~ ^[1-9][0-9]*$ ]]; then
  echo "PROBE_COUNT must be positive" >&2
  exit 2
fi
EXPECTED_LOGICAL=$((PROBE_COUNT + 1))
EXPECTED_IN_DUP=$((EXPECTED_LOGICAL * (LANES - 1)))
MIN_RETURN_DUP=$((PROBE_COUNT * (LANES - 1)))
MAX_RETURN_DUP=$((EXPECTED_LOGICAL * (LANES - 1)))
MIN_OUT_LANE=$((PROBE_COUNT * LANES + 1))
MAX_OUT_LANE=$((EXPECTED_LOGICAL * LANES))

mkdir -p "$LOG_DIR" "$LOG_DIR/tickets"
chmod 700 "$LOG_DIR/tickets"

required=(
  wbd-faketcp wbd-faketcp-mux wbd-link-proxy wbd-link-server-mux
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
RAW=40000
LINK=47000
GAME=49000
ECHO=48000
INSTALLATION_ID=00112233445566778899aabbccddeeff

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
    # The authenticated Logical Tunnel metadata is a control datagram for the
    # downstream shared service, not one of the measured Game payloads.
    if not (b.startswith(b'warm') or b.startswith(b'game')):
        continue
    with count.open('ab') as f: f.write(b.hex().encode()+b'\n')
    s.sendto(b,a)
PY
: >"$LOG_DIR/echo-count.log"
sudo ip netns exec "$S" python3 "$LOG_DIR/echo.py" "$LOG_DIR/echo-count.log" >"$LOG_DIR/echo.log" 2>&1 &
ECHOPID=$!; PIDS+=("$ECHOPID")

sudo ip netns exec "$S" "$ASSET_DIR/wbd-game-lane-server" \
  -listen 127.0.0.1:${GAME} -service 127.0.0.1:${ECHO} -max-lanes "$LANES" \
  >"$LOG_DIR/game-server.log" 2>&1 &
GSPID=$!; PIDS+=("$GSPID")
for _ in $(seq 1 200); do grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${LANES}" "$LOG_DIR/game-server.log" && break; sleep .05; done
grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${LANES}" "$LOG_DIR/game-server.log"

sudo ip netns exec "$S" "$ASSET_DIR/wbd-link-server-mux" \
  -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} \
  -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 60s -max-sessions 8 \
  >"$LOG_DIR/link-server.log" 2>&1 &
LINKPID=$!; PIDS+=("$LINKPID")
for _ in $(seq 1 200); do grep -q 'WBD_LINK_SERVER_MUX_READY.*logical_tunnel=1.*game_backend=1' "$LOG_DIR/link-server.log" && break; sleep .05; done
grep -q 'WBD_LINK_SERVER_MUX_READY.*logical_tunnel=1.*game_backend=1' "$LOG_DIR/link-server.log"

sudo ip netns exec "$S" "$ASSET_DIR/wbd-faketcp-mux" server \
  --listen 10.89.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \
  --link-target 127.0.0.1:${LINK} --cert "$ASSET_DIR/dtls.pem" --key "$ASSET_DIR/dtls.key" \
  --front-cert "$ASSET_DIR/front.pem" --front-key "$ASSET_DIR/front.key" \
  --server-name "$TARGET" --route-key "$ROUTE_KEY" \
  --username "$USERNAME" --password "$PASSWORD" --ticket-dir "$LOG_DIR/tickets" \
  --fallback-target 127.0.0.1:9 --bootstrap-timeout 12s --max-sessions 8 \
  >"$LOG_DIR/faketcp-mux.log" 2>&1 &
MUXPID=$!; PIDS+=("$MUXPID")
for _ in $(seq 1 300); do grep -q 'READY role=server-mux.*recovery=legacy.*single_flow_bootstrap=true.*logical_tunnel=true' "$LOG_DIR/faketcp-mux.log" && break; sleep .05; done
grep -q 'READY role=server-mux.*recovery=legacy.*single_flow_bootstrap=true.*logical_tunnel=true' "$LOG_DIR/faketcp-mux.log"

# Capture the public wire before any lane starts. Each lane must have exactly one
# FakeTCP SYN lineage; Reality-like TLS bootstrap and DTLS/LINK follow on that
# same association with no second WBD payload SYN.
sudo ip netns exec "$C" tcpdump --immediate-mode -i gc0 -s 0 -U -w "$LOG_DIR/game-lanes.pcap" "tcp port ${RAW}" >"$LOG_DIR/tcpdump.log" 2>&1 &
TPID=$!
for _ in $(seq 1 200); do grep -q 'listening on gc0' "$LOG_DIR/tcpdump.log" && break; sleep .05; done
grep -q 'listening on gc0' "$LOG_DIR/tcpdump.log"

for i in $(seq 1 "$LANES"); do
  fport=$((45100+i)); sport=$((41000+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-faketcp" client \
    --local-udp 127.0.0.1:${fport} --source 10.89.0.2:${sport} --remote 10.89.0.1:${RAW} \
    --shadow-recovery legacy \
    --reality-server-name "$TARGET" --reality-route-key "$ROUTE_KEY" \
    --reality-username "$USERNAME" --reality-password "$PASSWORD" \
    --reality-ticket-out "$LOG_DIR/ticket-${i}.txt" \
    --reality-installation-id "$INSTALLATION_ID" \
    --reality-tunnel-config-out "$LOG_DIR/tunnel-${i}.json" \
    --reality-verify-server=false --reality-timeout 12s \
    >"$LOG_DIR/faketcp-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 700); do
  ok=1
  for i in $(seq 1 "$LANES"); do
    grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$LOG_DIR/faketcp-${i}.log" || ok=0
    grep -q 'READY role=client.*recovery=legacy.*single_flow_bootstrap=true' "$LOG_DIR/faketcp-${i}.log" || ok=0
  done
  [[ $ok -eq 1 ]] && break
  sleep .05
done
for i in $(seq 1 "$LANES"); do
  grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$LOG_DIR/faketcp-${i}.log"
  grep -q 'READY role=client.*recovery=legacy.*single_flow_bootstrap=true' "$LOG_DIR/faketcp-${i}.log"
done

SESSION_ID=$(python3 - "$LOG_DIR" "$LANES" <<'PY'
import json,pathlib,sys
p=pathlib.Path(sys.argv[1]); n=int(sys.argv[2])
tickets=[(p/f'ticket-{i}.txt').read_text().strip() for i in range(1,n+1)]
assert all(len(x)==64 for x in tickets), tickets
assert len(set(tickets))==n, tickets
configs=[json.loads((p/f'tunnel-{i}.json').read_text()) for i in range(1,n+1)]
first=configs[0]
assert len(first['tunnel_id'])==32, first
assert first['address4'].endswith('/32'), first
assert first.get('routes4'), first
for cfg in configs[1:]:
    assert cfg['tunnel_id']==first['tunnel_id'], (first,cfg)
    assert cfg['address4']==first['address4'], (first,cfg)
    assert cfg.get('routes4')==first.get('routes4'), (first,cfg)
print(first['tunnel_id'])
PY
)
python3 - "$LOG_DIR" "$LANES" "$SESSION_ID" <<'PY'
import pathlib,sys
p=pathlib.Path(sys.argv[1]); n=int(sys.argv[2]); sid=sys.argv[3]
t=[(p/f'ticket-{i}.txt').read_text().strip() for i in range(1,n+1)]
print(f'WBD_GAME_LANE_SINGLE_FLOW_TICKETS_PASS unique={len(set(t))} tunnel_id_prefix={sid[:8]}')
PY

test "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' "$LOG_DIR/faketcp-mux.log")" -eq "$LANES"

for i in $(seq 1 "$LANES"); do
  dport=$((46100+i)); fport=$((45100+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd_dtls_shim" client ${dport} 127.0.0.1 ${fport} none none \
    >"$LOG_DIR/dtls-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 900); do
  ok=1
  for i in $(seq 1 "$LANES"); do grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$LOG_DIR/dtls-${i}.log" || ok=0; done
  bound=$(grep -c 'BOUND role=server.*inherited=yes' "$LOG_DIR/faketcp-mux.log" || true)
  [[ $ok -eq 1 && $bound -ge $LANES ]] && break
  sleep .05
done
for i in $(seq 1 "$LANES"); do grep -q 'READY role=client version=DTLSv1.3.*verify=none' "$LOG_DIR/dtls-${i}.log"; done
test "$(grep -c 'BOUND role=server.*inherited=yes' "$LOG_DIR/faketcp-mux.log")" -eq "$LANES"

for i in $(seq 1 "$LANES"); do
  ticket=$(tr -d '\r\n' <"$LOG_DIR/ticket-${i}.txt")
  lport=$((47100+i)); dport=$((46100+i))
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-link-proxy" \
    -mode client -listen 127.0.0.1:${lport} -dtls 127.0.0.1:${dport} -fec "$FEC" -lanes 1 \
    -demo-reality-ticket "$ticket" >"$LOG_DIR/link-${i}.log" 2>&1 &
  pid=$!; PIDS+=("$pid")
done
for _ in $(seq 1 1000); do
  ok=1
  for i in $(seq 1 "$LANES"); do grep -q "WBD_LINK_READY role=client fec=${FEC}" "$LOG_DIR/link-${i}.log" || ok=0; done
  sessions=$(grep -c 'WBD_LINK_MUX_SESSION_READY tunnel_id_prefix=' "$LOG_DIR/link-server.log" || true)
  [[ $ok -eq 1 && $sessions -ge $LANES ]] && break
  sleep .05
done
for i in $(seq 1 "$LANES"); do grep -q "WBD_LINK_READY role=client fec=${FEC}" "$LOG_DIR/link-${i}.log"; done
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY tunnel_id_prefix=' "$LOG_DIR/link-server.log")" -eq "$LANES"
test "$(grep -c 'WBD_LINK_LOGICAL_TUNNEL_BIND tunnel_id_prefix=' "$LOG_DIR/link-server.log")" -eq "$LANES"
python3 - "$LOG_DIR/link-server.log" "$SESSION_ID" "$LANES" <<'PY'
import pathlib,re,sys
text=pathlib.Path(sys.argv[1]).read_text(); sid=sys.argv[2]; lanes=int(sys.argv[3])
prefix=sid[:8]
binds=re.findall(r'WBD_LINK_LOGICAL_TUNNEL_BIND tunnel_id_prefix=([0-9a-f]+).*?active_transports=(\d+).*?max_transports=(\d+)', text)
assert len(binds)==lanes, binds
assert all(p==prefix for p,_,_ in binds), (prefix,binds)
assert [int(a) for _,a,_ in binds]==list(range(1,lanes+1)), binds
assert all(int(m)==4 for _,_,m in binds), binds
print(f'WBD_GAME_LANE_LOGICAL_TUNNEL_BIND_PASS tunnel_id_prefix={prefix} active_transports={lanes} max_transports=4')
PY
if [[ "$FEC" == off ]]; then
  test "$(grep -c 'WBD_LINK_MUX_SESSION_READY.*fec_mode=0 fec=0:0' "$LOG_DIR/link-server.log")" -eq "$LANES"
else
  test "$(grep -c 'WBD_LINK_MUX_SESSION_READY.*fec_mode=1 fec=20:20' "$LOG_DIR/link-server.log")" -eq "$LANES"
fi
test "$(find "$LOG_DIR/tickets" -type f | wc -l)" -eq 0

lane_addrs=()
client_ports=()
for i in $(seq 1 "$LANES"); do
  lane_addrs+=("127.0.0.1:$((47100+i))")
  client_ports+=("$((41000+i))")
done
LANE_ADDRS=$(IFS=,; echo "${lane_addrs[*]}")
CLIENT_PORTS=$(IFS=,; echo "${client_ports[*]}")

sudo ip netns exec "$C" "$ASSET_DIR/wbd-game-lane-client" \
  -listen 127.0.0.1:47500 -lanes "$LANE_ADDRS" \
  -session-id "$SESSION_ID" >"$LOG_DIR/game-client.log" 2>&1 &
GCPID=$!; PIDS+=("$GCPID")
for _ in $(seq 1 300); do grep -q "WBD_GAME_LANE_CLIENT_READY.*lanes=${LANES}" "$LOG_DIR/game-client.log" && break; sleep .05; done
grep -q "WBD_GAME_LANE_CLIENT_READY.*lanes=${LANES}" "$LOG_DIR/game-client.log"
test "$(grep -c 'WBD_GAME_LANE_OUTER lane=' "$LOG_DIR/game-client.log")" -eq "$LANES"

cat >"$LOG_DIR/probe.py" <<'PY'
import socket,sys,time
count=int(sys.argv[1]); prefix=sys.argv[2].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47600)); s.settimeout(4)
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
  [[ $binds -ge $LANES ]] && break
  sleep .05
done
test "$(grep -c 'WBD_GAME_LANE_BIND.*lanes=' "$LOG_DIR/game-server.log")" -eq "$LANES"
test "$(grep -c 'WBD_GAME_LANE_TUNNEL_META_READY' "$LOG_DIR/game-server.log")" -eq "$LANES"
sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" "$PROBE_COUNT" game >"$LOG_DIR/probe.log" 2>&1
grep -q "WBD_GAME_LANE_PROBE_PASS count=${PROBE_COUNT}" "$LOG_DIR/probe.log"

# Every logical Game datagram reaches the downstream exactly once regardless of
# lane/FEC copies. Authenticated TunnelMeta is intentionally excluded by echo.py.
test "$(wc -l <"$LOG_DIR/echo-count.log")" -eq "$EXPECTED_LOGICAL"
sleep .25

sudo kill -TERM "$GCPID" 2>/dev/null || true
wait "$GCPID" || true
PIDS=("${PIDS[@]/$GCPID}")
sudo kill -TERM "$GSPID" 2>/dev/null || true
wait "$GSPID" || true
PIDS=("${PIDS[@]/$GSPID}")

# The first warm datagram is a binding barrier, not a measured fanout sample.
# Its earliest downstream reply may occur after only a subset of the lanes have
# registered, while the remaining copies of that same logical datagram finish
# binding the session. The formal probes start only after all lanes are bound.
# Require all formal probes on every return lane, exact ingress duplication,
# exactly-once logical delivery, and allow only the warm reply fanout to vary.
python3 - "$LOG_DIR/game-client.log" "$LOG_DIR/game-server.log" \
  "$EXPECTED_LOGICAL" "$EXPECTED_IN_DUP" "$PROBE_COUNT" "$LANES" \
  "$MIN_RETURN_DUP" "$MAX_RETURN_DUP" "$MIN_OUT_LANE" "$MAX_OUT_LANE" <<'PY'
import pathlib,re,sys
client_path,server_path=sys.argv[1],sys.argv[2]
expected=int(sys.argv[3]); expected_in_dup=int(sys.argv[4])
probe=int(sys.argv[5]); lanes=int(sys.argv[6])
min_dup,max_dup=map(int,sys.argv[7:9])
min_out,max_out=map(int,sys.argv[9:11])
client=pathlib.Path(client_path).read_text()
server=pathlib.Path(server_path).read_text()

m=re.search(r'WBD_GAME_LANE_CLIENT_STATS logical_tx=(\d+) delivered=(\d+) duplicate=(\d+) stale=(\d+)',client)
assert m, client
logical_tx,delivered,duplicate,stale=map(int,m.groups())
assert logical_tx==expected,(logical_tx,expected)
assert delivered==expected,(delivered,expected)
assert stale==0,stale
assert min_dup <= duplicate <= max_dup,(duplicate,min_dup,max_dup)

lane_stats={int(l): (int(tx),int(rx)) for l,tx,rx in re.findall(r'WBD_GAME_LANE_CLIENT_LANE_STATS lane=(\d+) tx=(\d+) rx=(\d+)',client)}
assert len(lane_stats)==lanes,lane_stats
for lane in range(1,lanes+1):
    tx,rx=lane_stats[lane]
    assert tx==expected,(lane,tx,expected)
    assert probe <= rx <= expected,(lane,rx,probe,expected)

m=re.search(r'WBD_GAME_LANE_SESSION_CLOSE .*in_first=(\d+) in_dup=(\d+) out_logical=(\d+) out_lane=(\d+)',server)
assert m, server
in_first,in_dup,out_logical,out_lane=map(int,m.groups())
assert in_first==expected,(in_first,expected)
assert in_dup==expected_in_dup,(in_dup,expected_in_dup)
assert out_logical==expected,(out_logical,expected)
assert min_out <= out_lane <= max_out,(out_lane,min_out,max_out)
warm_fanout=out_lane-probe*lanes
assert 1 <= warm_fanout <= lanes,(warm_fanout,lanes)
print(f'WBD_GAME_LANE_STATS_PASS lanes={lanes} formal_probe={probe} warm_return_fanout={warm_fanout} return_dup={duplicate}')
PY

sudo kill -INT "$TPID" 2>/dev/null || true
wait "$TPID" || true
TPID=
python3 "$GITHUB_WORKSPACE/scripts/analyze_game_lane_pcap.py" "$LOG_DIR/game-lanes.pcap" \
  --server-port "$RAW" --client-ports "$CLIENT_PORTS" --out "$LOG_DIR/game-lanes-wire.json"
grep -q "WBD_GAME_LANE_PCAP_PASS flows=${LANES} distinct_5tuple=1 distinct_seq_space=1" \
  <(python3 "$GITHUB_WORKSPACE/scripts/analyze_game_lane_pcap.py" "$LOG_DIR/game-lanes.pcap" --server-port "$RAW" --client-ports "$CLIENT_PORTS")

echo "WBD_GAME_LANE_FULLSTACK_PASS lanes=${LANES} logical_delivery_once=1 outer_flows_distinct=1 per_lane_single_flow=1 shared_logical_tunnel=1 fec=${FEC}"
