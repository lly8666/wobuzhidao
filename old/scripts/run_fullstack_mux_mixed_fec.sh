#!/usr/bin/env bash
set -euo pipefail

: "${FEC_A:?FEC_A required}"
: "${FEC_B:?FEC_B required}"
: "${LABEL:?LABEL required}"

S=wbdmfs$$
A=wbdmfa$$
B=wbdmfb$$
PIDS=''
D=/tmp/mixedfec/${LABEL}
mkdir -p "$D"
mkdir -m 700 "$D/tickets"

cleanup() {
  rc=$?
  trap - EXIT
  set +e
  if [ "$rc" -ne 0 ]; then
    echo "WBD_MIXED_FEC_FAILURE_DIAGNOSTICS rc=$rc fec_a=$FEC_A fec_b=$FEC_B"
    for f in "$D"/*.log; do
      [ -e "$f" ] || continue
      echo "=== $(basename "$f") ==="
      sudo tail -n 260 "$f" 2>/dev/null || tail -n 260 "$f" 2>/dev/null || true
    done
  fi
  sudo chmod -R a+rX "$D" 2>/dev/null || true
  for p in ${PIDS:-}; do sudo kill -TERM "$p" 2>/dev/null || kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  sudo ip netns del "$A" 2>/dev/null || true
  sudo ip netns del "$B" 2>/dev/null || true
  sudo ip netns del "$S" 2>/dev/null || true
  exit "$rc"
}
trap cleanup EXIT

ROUTE_KEY='WBD_REALITY_ROUTE_KEY_0123456789abcdef'
USERNAME='solo'
PASSWORD='shared-password'
TARGET='target.example'
RAW=40000
LINK=47000
ECHO=48000
INSTALL_A='000000000000000000000000000000a1'
INSTALL_B='000000000000000000000000000000b2'

sudo ip netns add "$S"
sudo ip netns add "$A"
sudo ip netns add "$B"
sudo ip link add va type veth peer name sa
sudo ip link add vb type veth peer name sb
sudo ip link set va netns "$A"
sudo ip link set vb netns "$B"
sudo ip link set sa netns "$S"
sudo ip link set sb netns "$S"
sudo ip netns exec "$S" ip link add br0 type bridge
sudo ip netns exec "$S" ip link set sa master br0
sudo ip netns exec "$S" ip link set sb master br0
sudo ip -n "$S" addr add 10.78.0.1/24 dev br0
sudo ip -n "$A" addr add 10.78.0.2/24 dev va
sudo ip -n "$B" addr add 10.78.0.3/24 dev vb
for ns in "$S" "$A" "$B"; do
  sudo ip -n "$ns" link set lo up
  sudo ip netns exec "$ns" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
done
sudo ip -n "$S" link set br0 up
sudo ip -n "$S" link set sa up
sudo ip -n "$S" link set sb up
sudo ip -n "$A" link set va up
sudo ip -n "$B" link set vb up

cat > /tmp/mixedfec/echo.py <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    if b:
        s.sendto(b,a)
PY
sudo ip netns exec "$S" python3 /tmp/mixedfec/echo.py >"$D/echo.log" 2>&1 & PIDS="$PIDS $!"

sudo ip netns exec "$S" /tmp/mixedfec/wbd-link-server-mux \
  -listen 127.0.0.1:${LINK} -service 127.0.0.1:${ECHO} -raw-ip-service 127.0.0.1:${ECHO} \
  -ticket-dir "$D/tickets" -ticket-ttl 60s -idle-timeout 0 -max-sessions 8 >"$D/link-server.log" 2>&1 & LINKPID=$!; PIDS="$PIDS $LINKPID"
for _ in $(seq 1 200); do grep -q 'WBD_LINK_SERVER_MUX_READY' "$D/link-server.log" 2>/dev/null && break; sleep .05; done
grep -q 'WBD_LINK_SERVER_MUX_READY .*idle_timeout=0s' "$D/link-server.log"

sudo ip netns exec "$S" /tmp/mixedfec/wbd-faketcp-mux server \
  --listen 10.78.0.1:${RAW} --dtls-shim /tmp/mixedfec/wbd_dtls_shim --link-target 127.0.0.1:${LINK} \
  --cert /tmp/mixedfec/dtls.pem --key /tmp/mixedfec/dtls.key --max-sessions 8 \
  --front-cert /tmp/mixedfec/front.pem --front-key /tmp/mixedfec/front.key --server-name "$TARGET" \
  --route-key "$ROUTE_KEY" --username "$USERNAME" --password "$PASSWORD" --ticket-dir "$D/tickets" \
  --fallback-target 127.0.0.1:9 --tunnel-pool 10.66.0.0/16 --tunnel-routes4 0.0.0.0/0 >"$D/faketcp-mux.log" 2>&1 & MUXPID=$!; PIDS="$PIDS $MUXPID"
for _ in $(seq 1 300); do grep -q 'READY role=server-mux .*single_flow_bootstrap=true .*logical_tunnel=true' "$D/faketcp-mux.log" 2>/dev/null && break; sleep .05; done
grep -q 'READY role=server-mux .*single_flow_bootstrap=true .*logical_tunnel=true' "$D/faketcp-mux.log"

sudo ip netns exec "$A" /tmp/mixedfec/wbd-faketcp client \
  --local-udp 127.0.0.1:45101 --source 10.78.0.2:41001 --remote 10.78.0.1:${RAW} \
  --reality-server-name "$TARGET" --reality-route-key "$ROUTE_KEY" --reality-username "$USERNAME" --reality-password "$PASSWORD" \
  --reality-ticket-out "$D/ticket-a.txt" --reality-installation-id "$INSTALL_A" --reality-tunnel-config-out "$D/tunnel-a.json" --reality-verify-server=false >"$D/faketcp-a.log" 2>&1 & FCA=$!; PIDS="$PIDS $FCA"
sudo ip netns exec "$B" /tmp/mixedfec/wbd-faketcp client \
  --local-udp 127.0.0.1:45102 --source 10.78.0.3:41002 --remote 10.78.0.1:${RAW} \
  --reality-server-name "$TARGET" --reality-route-key "$ROUTE_KEY" --reality-username "$USERNAME" --reality-password "$PASSWORD" \
  --reality-ticket-out "$D/ticket-b.txt" --reality-installation-id "$INSTALL_B" --reality-tunnel-config-out "$D/tunnel-b.json" --reality-verify-server=false >"$D/faketcp-b.log" 2>&1 & FCB=$!; PIDS="$PIDS $FCB"
for _ in $(seq 1 800); do
  if grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY' "$D/faketcp-a.log" 2>/dev/null && grep -q 'READY role=client .*single_flow_bootstrap=true' "$D/faketcp-a.log" 2>/dev/null && \
     grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY' "$D/faketcp-b.log" 2>/dev/null && grep -q 'READY role=client .*single_flow_bootstrap=true' "$D/faketcp-b.log" 2>/dev/null; then break; fi
  sleep .05
done
grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY' "$D/faketcp-a.log"
grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY' "$D/faketcp-b.log"
TA=$(sudo cat "$D/ticket-a.txt" | tr -d '\r\n')
TB=$(sudo cat "$D/ticket-b.txt" | tr -d '\r\n')
test ${#TA} -eq 64
test ${#TB} -eq 64
test "$TA" != "$TB"
ADDR_A=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["address4"].split("/")[0])' "$D/tunnel-a.json")
ADDR_B=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["address4"].split("/")[0])' "$D/tunnel-b.json")
TUN_A=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["tunnel_id"])' "$D/tunnel-a.json")
TUN_B=$(sudo python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["tunnel_id"])' "$D/tunnel-b.json")
test "$ADDR_A" != "$ADDR_B"
test "$TUN_A" != "$TUN_B"
echo "WBD_MIXED_FEC_V2_LEASES_PASS addr_a=$ADDR_A addr_b=$ADDR_B tunnel_a=${TUN_A:0:8} tunnel_b=${TUN_B:0:8}"

sudo ip netns exec "$A" /tmp/mixedfec/wbd_dtls_shim client 46101 127.0.0.1 45101 none none >"$D/dtls-a.log" 2>&1 & PIDS="$PIDS $!"
sudo ip netns exec "$B" /tmp/mixedfec/wbd_dtls_shim client 46102 127.0.0.1 45102 none none >"$D/dtls-b.log" 2>&1 & PIDS="$PIDS $!"
for _ in $(seq 1 800); do grep -q 'READY role=client version=DTLSv1.3' "$D/dtls-a.log" 2>/dev/null && grep -q 'READY role=client version=DTLSv1.3' "$D/dtls-b.log" 2>/dev/null && break; sleep .05; done
grep -q 'READY role=client version=DTLSv1.3' "$D/dtls-a.log"
grep -q 'READY role=client version=DTLSv1.3' "$D/dtls-b.log"

sudo ip netns exec "$A" /tmp/mixedfec/wbd-link-proxy -mode client -listen 127.0.0.1:47101 -dtls 127.0.0.1:46101 -fec "$FEC_A" -demo-reality-ticket "$TA" >"$D/link-a.log" 2>&1 & LA=$!; PIDS="$PIDS $LA"
sudo ip netns exec "$B" /tmp/mixedfec/wbd-link-proxy -mode client -listen 127.0.0.1:47102 -dtls 127.0.0.1:46102 -fec "$FEC_B" -demo-reality-ticket "$TB" >"$D/link-b.log" 2>&1 & LB=$!; PIDS="$PIDS $LB"
for _ in $(seq 1 800); do
  if grep -q "WBD_LINK_READY role=client fec=${FEC_A} mtu=1360 lanes=1" "$D/link-a.log" 2>/dev/null && grep -q "WBD_LINK_READY role=client fec=${FEC_B} mtu=1360 lanes=1" "$D/link-b.log" 2>/dev/null && test "$(grep -c 'WBD_LINK_MUX_SESSION_READY' "$D/link-server.log" 2>/dev/null || true)" -ge 2; then break; fi
  sleep .05
done
grep -q "WBD_LINK_READY role=client fec=${FEC_A} mtu=1360 lanes=1" "$D/link-a.log"
grep -q "WBD_LINK_READY role=client fec=${FEC_B} mtu=1360 lanes=1" "$D/link-b.log"
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY' "$D/link-server.log")" -eq 2
expected_server() { if [ "$1" = off ]; then echo 'fec_mode=0 fec=0:0 mtu=1360 lanes=1'; else echo "fec_mode=1 fec=20:${1#*:} mtu=1360 lanes=1"; fi; }
EA=$(expected_server "$FEC_A")
EB=$(expected_server "$FEC_B")
grep -q "WBD_LINK_MUX_SESSION_READY .*${EA}" "$D/link-server.log"
grep -q "WBD_LINK_MUX_SESSION_READY .*${EB}" "$D/link-server.log"

cat > /tmp/mixedfec/probe.py <<'PY'
import socket,struct,sys,time
port=int(sys.argv[1]); prefix=sys.argv[2]; source=sys.argv[3]
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0))
s.settimeout(.25)
for i in range(30):
    payload=(f'{prefix}-{i}-'+('x'*480)).encode()
    total=20+len(payload)
    ip=struct.pack('!BBHHHBBH4s4s',0x45,0,total,0,0,64,17,0,socket.inet_aton(source),socket.inet_aton('1.1.1.1'))+payload
    frame=b'WBDP'+bytes((1,1))+struct.pack('!H',len(ip))+ip
    s.sendto(frame,('127.0.0.1',port))
    deadline=time.time()+3.0
    while True:
        left=deadline-time.time()
        if left <= 0: raise SystemExit(f'echo timeout {prefix} {i}')
        s.settimeout(min(.25,left))
        try: reply,_=s.recvfrom(65535)
        except TimeoutError: continue
        if reply == frame: break
print(f'WBD_MIXED_FEC_CLIENT_ECHO_PASS client={prefix} rounds=30 source={source} framing=WBDP1')
PY
sudo ip netns exec "$A" python3 /tmp/mixedfec/probe.py 47101 A "$ADDR_A" >"$D/probe-a.log" 2>&1 & PA=$!
sudo ip netns exec "$B" python3 /tmp/mixedfec/probe.py 47102 B "$ADDR_B" >"$D/probe-b.log" 2>&1 & PB=$!
wait $PA
wait $PB
grep -q 'WBD_MIXED_FEC_CLIENT_ECHO_PASS client=A rounds=30' "$D/probe-a.log"
grep -q 'WBD_MIXED_FEC_CLIENT_ECHO_PASS client=B rounds=30' "$D/probe-b.log"
kill -0 "$LINKPID"
kill -0 "$MUXPID"
kill -0 "$LA"
kill -0 "$LB"
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY' "$D/link-server.log")" -eq 2
grep -q 'WBD_LINK_MUX_BACKEND_READY .*backend=rawip' "$D/link-server.log"
grep -q 'WBD_LINK_RX_FIRST .*backend=rawip' "$D/link-server.log"
echo "WBD_MIXED_FEC_SESSION_ISOLATION_PASS fec_a=${FEC_A} fec_b=${FEC_B} mtu=1360 lanes=1 sessions=2 v2_single_flow=1 framing=WBDP1 client_a=ok client_b=ok server=ok"
