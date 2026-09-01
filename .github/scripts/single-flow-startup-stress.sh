#!/usr/bin/env bash
set -euo pipefail

ROOT=${SFSTRESS_ROOT:-/tmp/sfstress}
C=sfstressc$$
R=sfstressr$$
S=sfstresss$$
SERVER_PIDS=''
ROUTE_KEY='0123456789abcdef0123456789abcdef'
USERNAME='stress-user'
PASSWORD='stress-password'
INSTALLATION_ID='11223344556677889900aabbccddeeff'
ROUNDS=${SFSTRESS_ROUNDS:-20}

cleanup() {
  set +e
  for f in "$ROOT"/fake.pid "$ROOT"/dtls.pid "$ROOT"/link.pid; do
    if [ -s "$f" ]; then
      pid="$(cat "$f")"
      case "$pid" in (*[!0-9]*|'') ;; (*) sudo kill -TERM "$pid" 2>/dev/null || true ;; esac
    fi
  done
  for p in ${SERVER_PIDS:-}; do sudo kill -TERM "$p" 2>/dev/null || true; done
  sleep .3
  sudo ip netns del "$C" 2>/dev/null || true
  sudo ip netns del "$R" 2>/dev/null || true
  sudo ip netns del "$S" 2>/dev/null || true
}
trap cleanup EXIT

sudo ip netns add "$C"
sudo ip netns add "$R"
sudo ip netns add "$S"
sudo ip link add cr type veth peer name rc
sudo ip link add rs type veth peer name sr
sudo ip link set cr netns "$C"
sudo ip link set rc netns "$R"
sudo ip link set rs netns "$R"
sudo ip link set sr netns "$S"
sudo ip -n "$C" addr add 10.92.0.2/24 dev cr
sudo ip -n "$R" addr add 10.92.0.1/24 dev rc
sudo ip -n "$R" addr add 10.93.0.1/24 dev rs
sudo ip -n "$S" addr add 10.93.0.2/24 dev sr
for ns in "$C" "$R" "$S"; do sudo ip -n "$ns" link set lo up; done
sudo ip -n "$C" link set cr up
sudo ip -n "$R" link set rc up
sudo ip -n "$R" link set rs up
sudo ip -n "$S" link set sr up
sudo ip -n "$C" route add default via 10.92.0.1
sudo ip netns exec "$R" sysctl -qw net.ipv4.ip_forward=1
sudo ip netns exec "$R" iptables -P FORWARD ACCEPT
sudo ip netns exec "$R" iptables -t nat -A POSTROUTING -s 10.92.0.0/24 -o rs -j MASQUERADE
sudo ip netns exec "$C" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP

cat >"$ROOT/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY
sudo ip netns exec "$S" python3 "$ROOT/echo.py" >"$ROOT/echo.log" 2>&1 & ECHO_PID=$!
SERVER_PIDS="$ECHO_PID"

sudo ip netns exec "$S" "$ROOT/wbd-link-server-mux" \
  -listen 127.0.0.1:47000 -service 127.0.0.1:48000 \
  -ticket-dir "$ROOT/tickets" -ticket-ttl 60s -max-sessions 64 \
  >"$ROOT/link-server.log" 2>&1 & LINK_SERVER_PID=$!
SERVER_PIDS="$SERVER_PIDS $LINK_SERVER_PID"
for _ in $(seq 1 200); do grep -q 'WBD_LINK_SERVER_MUX_READY' "$ROOT/link-server.log" && break; sleep .05; done
grep -q 'WBD_LINK_SERVER_MUX_READY' "$ROOT/link-server.log"

sudo ip netns exec "$S" python3 -m http.server 44444 --bind 127.0.0.1 >"$ROOT/fallback.log" 2>&1 & FALLBACK_PID=$!
SERVER_PIDS="$SERVER_PIDS $FALLBACK_PID"

sudo ip netns exec "$S" "$ROOT/wbd-faketcp-mux" server \
  --listen 10.93.0.2:443 --dtls-shim "$ROOT/wbd_dtls_shim" \
  --link-target 127.0.0.1:47000 --cert "$ROOT/dtls.pem" --key "$ROOT/dtls.key" \
  --front-cert "$ROOT/front.pem" --front-key "$ROOT/front.key" \
  --server-name wbd.test --route-key "$ROUTE_KEY" \
  --username "$USERNAME" --password "$PASSWORD" \
  --ticket-dir "$ROOT/tickets" --fallback-target 127.0.0.1:44444 \
  --bootstrap-timeout 12s --max-sessions 64 \
  >"$ROOT/mux.log" 2>&1 & MUX_PID=$!
SERVER_PIDS="$SERVER_PIDS $MUX_PID"
for _ in $(seq 1 200); do grep -q 'READY role=server-mux.*single_flow_bootstrap=true' "$ROOT/mux.log" && break; sleep .05; done
grep -q 'READY role=server-mux.*single_flow_bootstrap=true' "$ROOT/mux.log"

cat >"$ROOT/probe.py" <<'PY'
import socket,sys,time
marker=sys.argv[1].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(.20)
for i in range(3):
    want=marker+i.to_bytes(2,'big')
    end=time.time()+3
    while time.time()<end:
        s.sendto(want,('127.0.0.1',47101))
        try: got,_=s.recvfrom(65535)
        except socket.timeout: continue
        if got == want: break
    else: raise SystemExit('echo timeout at %d'%i)
print('STRESS_ECHO_PASS marker='+sys.argv[1])
PY

assert_pid_file() {
  local f="$1" pid
  test -s "$f"
  pid="$(cat "$f")"
  case "$pid" in (*[!0-9]*|'') echo "invalid child pid file $f: $pid" >&2; exit 1 ;; esac
  sudo kill -0 "$pid"
}

validate_tunnel_config() {
  local cfg="$1"
  # The client runs as root inside a network namespace and intentionally writes
  # authenticated ticket/tunnel state with restrictive permissions. Validate
  # that protected state as root rather than weakening product file modes for CI.
  sudo python3 - "$cfg" "$ROOT/expected-tunnel.txt" <<'PY'
import json,sys,pathlib
cfg=json.load(open(sys.argv[1],encoding='utf-8'))
tid=cfg.get('tunnel_id','')
addr=cfg.get('address4','')
routes=cfg.get('routes4') or []
if len(tid) != 32 or not addr.endswith('/32') or '0.0.0.0/0' not in routes:
    raise SystemExit('invalid authenticated tunnel config: %r' % cfg)
p=pathlib.Path(sys.argv[2])
value=tid+' '+addr
if p.exists():
    if p.read_text().strip() != value:
        raise SystemExit('logical tunnel lease changed across reconnects: %s != %s' % (p.read_text().strip(), value))
else:
    p.write_text(value+'\n')
print('STRESS_TUNNEL_CONFIG_PASS '+value)
PY
}

# Fail before the expensive reconnect loop if runner-owned harness diagnostics
# cannot be created. Product-owned protected files are checked separately above.
touch "$ROOT/.runner-write-check"
rm -f "$ROOT/.runner-write-check"

for round in $(seq 1 "$ROUNDS"); do
  port=$((49151 + round))
  sudo rm -f "$ROOT/client.ticket" "$ROOT/client-tunnel.json"
  rm -f "$ROOT/fake.log" "$ROOT/dtls.log" "$ROOT/link.log" "$ROOT/probe.log" \
        "$ROOT/fake.pid" "$ROOT/dtls.pid" "$ROOT/link.pid"

  sudo ip netns exec "$C" sh -c "echo \$\$ >'$ROOT/fake.pid'; exec '$ROOT/wbd-faketcp' client \
    --local-udp 127.0.0.1:45101 --source 10.92.0.2:${port} --remote 10.93.0.2:443 \
    --shadow-recovery legacy --reality-server-name wbd.test --reality-route-key '$ROUTE_KEY' \
    --reality-username '$USERNAME' --reality-password '$PASSWORD' \
    --reality-ticket-out '$ROOT/client.ticket' --reality-installation-id '$INSTALLATION_ID' \
    --reality-tunnel-config-out '$ROOT/client-tunnel.json' --reality-verify-server=false --reality-timeout 12s" \
    >"$ROOT/fake.log" 2>&1 &
  for _ in $(seq 1 500); do
    if grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$ROOT/fake.log" && \
       grep -q 'READY role=client.*single_flow_bootstrap=true' "$ROOT/fake.log"; then break; fi
    sleep .05
  done
  grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' "$ROOT/fake.log"
  grep -q 'READY role=client.*single_flow_bootstrap=true' "$ROOT/fake.log"
  assert_pid_file "$ROOT/fake.pid"
  sudo test -s "$ROOT/client.ticket"
  sudo test -s "$ROOT/client-tunnel.json"
  ticket="$(sudo cat "$ROOT/client.ticket" | tr -d '\r\n')"
  test ${#ticket} -eq 64
  validate_tunnel_config "$ROOT/client-tunnel.json"
  sudo rm -f "$ROOT/client.ticket" "$ROOT/client-tunnel.json"

  sudo ip netns exec "$C" sh -c "echo \$\$ >'$ROOT/dtls.pid'; exec '$ROOT/wbd_dtls_shim' client 46101 127.0.0.1 45101 none none" \
    >"$ROOT/dtls.log" 2>&1 &
  for _ in $(seq 1 800); do grep -q 'READY role=client version=DTLSv1.3' "$ROOT/dtls.log" && break; sleep .05; done
  grep -q 'READY role=client version=DTLSv1.3' "$ROOT/dtls.log"
  assert_pid_file "$ROOT/dtls.pid"

  sudo ip netns exec "$C" sh -c "echo \$\$ >'$ROOT/link.pid'; exec '$ROOT/wbd-link-proxy' -mode client \
    -listen 127.0.0.1:47101 -dtls 127.0.0.1:46101 -fec off -mtu 1400 -lanes 1 -demo-reality-ticket '$ticket'" \
    >"$ROOT/link.log" 2>&1 &
  for _ in $(seq 1 800); do grep -q 'WBD_LINK_READY role=client' "$ROOT/link.log" && break; sleep .05; done
  grep -q 'WBD_LINK_READY role=client' "$ROOT/link.log"
  assert_pid_file "$ROOT/link.pid"

  sudo ip netns exec "$C" python3 "$ROOT/probe.py" "ROUND_${round}_" >"$ROOT/probe.log" 2>&1
  grep -q 'STRESS_ECHO_PASS' "$ROOT/probe.log"
  test "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY remote=.*same_flow=1' "$ROOT/mux.log" || true)" -ge "$round"
  test "$(grep -c 'WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3' "$ROOT/mux.log" || true)" -ge "$round"
  test "$(grep -c 'WBD_LINK_MUX_SESSION_READY account=stress-user' "$ROOT/link-server.log" || true)" -ge "$round"

  for f in "$ROOT/link.pid" "$ROOT/dtls.pid" "$ROOT/fake.pid"; do sudo kill -KILL "$(cat "$f")"; done
  for _ in $(seq 1 150); do
    alive=0
    for f in "$ROOT/link.pid" "$ROOT/dtls.pid" "$ROOT/fake.pid"; do
      pid="$(cat "$f")"; if sudo kill -0 "$pid" 2>/dev/null; then alive=1; fi
    done
    [ "$alive" -eq 0 ] && break
    sleep .02
  done
  for f in "$ROOT/link.pid" "$ROOT/dtls.pid" "$ROOT/fake.pid"; do
    pid="$(cat "$f")"
    if sudo kill -0 "$pid" 2>/dev/null; then echo "dirty-exit cleanup failed: child pid $pid is still alive" >&2; exit 1; fi
  done
  if sudo ip netns exec "$C" ss -H -lun | grep -Eq '127\.0\.0\.1:(45101|46101|47101)([[:space:]]|$)'; then
    echo 'dirty-exit cleanup failed: client UDP endpoint still bound' >&2
    sudo ip netns exec "$C" ss -lunp >&2 || true
    exit 1
  fi
  echo "SINGLE_FLOW_STARTUP_STRESS_ROUND_PASS round=${round} source_port=${port} installation=${INSTALLATION_ID}"
done

test "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY remote=.*same_flow=1' "$ROOT/mux.log")" -eq "$ROUNDS"
test "$(grep -c 'WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3' "$ROOT/mux.log")" -eq "$ROUNDS"
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY account=stress-user' "$ROOT/link-server.log")" -eq "$ROUNDS"
kill -0 "$MUX_PID"
kill -0 "$LINK_SERVER_PID"
echo "SINGLE_FLOW_STARTUP_STRESS_PASS rounds=${ROUNDS} nat=1 dirty_exit=1 full_stack=1 logical_tunnel=1"
