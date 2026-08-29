#!/usr/bin/env bash
set -euo pipefail

ROOT=${1:-/tmp/reconnect}
FAKE=${FAKE:-$ROOT/wbd-faketcp}
MUX=${MUX:-$ROOT/wbd-faketcp-mux}
DTLS=${DTLS:-$ROOT/wbd_dtls_shim}
CA=${CA:-$ROOT/ca.pem}
CERT=${CERT:-$ROOT/dtls.pem}
KEY=${KEY:-$ROOT/dtls.key}

C=wbdrecc$$
R=wbdrecr$$
S=wbdrecs$$

kill_ns() {
  ns=$1
  for _ in $(seq 1 40); do
    pids=$(sudo ip netns pids "$ns" 2>/dev/null || true)
    [ -z "$pids" ] && return 0
    printf '%s\n' "$pids" | xargs -r sudo kill -KILL 2>/dev/null || true
    sleep .025
  done
  echo "namespace $ns still has processes" >&2
  sudo ip netns pids "$ns" >&2 || true
  return 1
}

cleanup() {
  set +e
  kill_ns "$C" >/dev/null 2>&1 || true
  kill_ns "$S" >/dev/null 2>&1 || true
  sudo ip netns del "$C" 2>/dev/null || true
  sudo ip netns del "$R" 2>/dev/null || true
  sudo ip netns del "$S" 2>/dev/null || true
}
trap cleanup EXIT

sudo ip netns add "$C"
sudo ip netns add "$R"
sudo ip netns add "$S"
sudo ip link add c0 type veth peer name rc0
sudo ip link add rs0 type veth peer name s0
sudo ip link set c0 netns "$C"
sudo ip link set rc0 netns "$R"
sudo ip link set rs0 netns "$R"
sudo ip link set s0 netns "$S"
sudo ip -n "$C" addr add 10.94.0.2/24 dev c0
sudo ip -n "$R" addr add 10.94.0.1/24 dev rc0
sudo ip -n "$R" addr add 198.19.0.1/24 dev rs0
sudo ip -n "$S" addr add 198.19.0.2/24 dev s0
for ns in "$C" "$R" "$S"; do sudo ip -n "$ns" link set lo up; done
sudo ip -n "$C" link set c0 up
sudo ip -n "$R" link set rc0 up
sudo ip -n "$R" link set rs0 up
sudo ip -n "$S" link set s0 up
sudo ip -n "$C" route add default via 10.94.0.1
sudo ip -n "$S" route add default via 198.19.0.1
sudo ip netns exec "$R" sysctl -qw net.ipv4.ip_forward=1
sudo ip netns exec "$R" iptables -t nat -A POSTROUTING -s 10.94.0.0/24 -o rs0 -j MASQUERADE
sudo ip netns exec "$R" iptables -A FORWARD -i rc0 -o rs0 -j ACCEPT
sudo ip netns exec "$R" iptables -A FORWARD -i rs0 -o rc0 -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT

# Match production shared-port behavior: a kernel TCP listener and the raw
# FakeTCP mux both observe TCP:443. Kernel-generated RST is always suppressed.
sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --sport 443 --tcp-flags RST RST -j DROP

cat >"$ROOT/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47000))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY

cat >"$ROOT/tcp443.py" <<'PY'
import socket,threading
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('198.19.0.2',443)); s.listen(32)
def one(c):
    try:
        c.settimeout(1)
        b=c.recv(4096)
        if b: c.sendall(b)
    except Exception: pass
    finally: c.close()
while True:
    c,_=s.accept()
    threading.Thread(target=one,args=(c,),daemon=True).start()
PY

sudo ip netns exec "$S" python3 "$ROOT/echo.py" >"$ROOT/echo.log" 2>&1 &
sudo ip netns exec "$S" python3 "$ROOT/tcp443.py" >"$ROOT/tcp443.log" 2>&1 &
sudo ip netns exec "$S" "$MUX" server \
  --listen 198.19.0.2:443 --dtls-shim "$DTLS" \
  --link-target 127.0.0.1:47000 --cert "$CERT" --key "$KEY" \
  --max-sessions 64 >"$ROOT/mux.log" 2>&1 &
for _ in $(seq 1 200); do grep -q 'READY role=server-mux' "$ROOT/mux.log" && break; sleep .05; done
grep -q 'READY role=server-mux' "$ROOT/mux.log"

# Prove ordinary kernel TCP:443 remains functional alongside the raw listener
# before enabling the experiment-only SYNACK isolation gate.
sudo ip netns exec "$C" python3 - <<'PY'
import socket
s=socket.create_connection(('198.19.0.2',443),3)
s.sendall(b'reality-like-setup')
assert s.recv(64)==b'reality-like-setup'
s.close()
PY

# Causal A/B guard: WBD raw SYNACK uses the exact existing 12-byte option
# profile (MSS 1360, SACK-permitted, WS=8). Once the ordinary setup probe has
# passed, allow that raw SYNACK signature and drop the competing kernel SYNACK.
# If dirty reconnect becomes stable, this proves shared-port dual-SYNACK is the
# failure mechanism; production will use a per-flow guard rather than this
# temporary global handoff gate.
WBD_SYN_U32='0>>22&0x3C@20=0x02040550&&0>>22&0x3C@24=0x04020103&&0>>22&0x3C@28=0x03080101'
sudo ip netns exec "$S" iptables -N WBD_SYNACK_GUARD
sudo ip netns exec "$S" iptables -A WBD_SYNACK_GUARD -m u32 --u32 "$WBD_SYN_U32" -j RETURN
sudo ip netns exec "$S" iptables -A WBD_SYNACK_GUARD -j DROP
sudo ip netns exec "$S" iptables -I OUTPUT 1 -p tcp --sport 443 --tcp-flags SYN,ACK SYN,ACK -j WBD_SYNACK_GUARD
sudo ip netns exec "$S" iptables -S WBD_SYNACK_GUARD >"$ROOT/synack-guard-rules.log"

run_phase() {
  phase=$1
  count=$2
  echo "RECONNECT_PHASE_START phase=$phase count=$count" | tee -a "$ROOT/results.log"
  for i in $(seq 1 "$count"); do
    if [ "$phase" = fixed ]; then sport=41001; else sport=$((42000+i)); fi
    local_udp=45101
    plain=46101
    flog="$ROOT/fake-$phase-$i.log"
    dlog="$ROOT/dtls-$phase-$i.log"

    sudo ip netns exec "$C" "$FAKE" client \
      --local-udp 127.0.0.1:$local_udp --source 10.94.0.2:$sport --remote 198.19.0.2:443 \
      >"$flog" 2>&1 &
    fake_ready=0
    for _ in $(seq 1 240); do
      if grep -q 'READY role=client' "$flog"; then fake_ready=1; break; fi
      sleep .05
    done
    if [ "$fake_ready" != 1 ]; then
      echo "RECONNECT_FAIL phase=$phase iter=$i stage=faketcp sport=$sport" | tee -a "$ROOT/results.log"
      cat "$flog" "$ROOT/mux.log"
      return 1
    fi

    sudo ip netns exec "$C" "$DTLS" client "$plain" 127.0.0.1 "$local_udp" "$CA" wbd.test \
      >"$dlog" 2>&1 &
    dtls_ready=0
    for _ in $(seq 1 240); do
      if grep -q 'READY role=client version=DTLSv1.3.*verify=peer-hostname' "$dlog"; then dtls_ready=1; break; fi
      sleep .05
    done
    if [ "$dtls_ready" != 1 ]; then
      echo "RECONNECT_FAIL phase=$phase iter=$i stage=dtls sport=$sport" | tee -a "$ROOT/results.log"
      cat "$flog" "$dlog" "$ROOT/mux.log"
      sudo ip netns exec "$S" iptables -nvx -L WBD_SYNACK_GUARD | tee "$ROOT/synack-guard-counters.log"
      return 1
    fi

    sudo ip netns exec "$C" python3 - "$plain" "$phase-$i" <<'PY'
import socket,sys
port=int(sys.argv[1]); tag=sys.argv[2].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(3)
msg=b'wbd-reconnect-'+tag
s.sendto(msg,('127.0.0.1',port))
data,_=s.recvfrom(65535)
assert data==msg,(data,msg)
PY
    echo "RECONNECT_PASS phase=$phase iter=$i sport=$sport" | tee -a "$ROOT/results.log"

    # Kill real processes inside the namespace, not only the outer sudo wrapper.
    # This models TerminateProcess/crash semantics and intentionally keeps NAT,
    # server association state and conntrack alive between iterations.
    kill_ns "$C"
    sleep .05
  done
}

run_phase fixed 30
run_phase rotate 30

sudo ip netns exec "$S" iptables -nvx -L WBD_SYNACK_GUARD | tee "$ROOT/synack-guard-counters.log"
fixed_pass=$(grep -c '^RECONNECT_PASS phase=fixed' "$ROOT/results.log")
rotate_pass=$(grep -c '^RECONNECT_PASS phase=rotate' "$ROOT/results.log")
reconnects=$(grep -c 'WBD_FAKETCP_MUX_RECONNECT' "$ROOT/mux.log" || true)
accepted_payload=$(grep -c 'WBD_FAKETCP_MUX_RAW_PAYLOAD_RX.*association=accepted' "$ROOT/mux.log" || true)
echo "RECONNECT_SUMMARY fixed=$fixed_pass/30 rotate=$rotate_pass/30 server_reconnect_markers=$reconnects accepted_payload_markers=$accepted_payload" | tee -a "$ROOT/results.log"
[ "$fixed_pass" -eq 30 ]
[ "$rotate_pass" -eq 30 ]
[ "$reconnects" -ge 25 ]
[ "$accepted_payload" -ge 1 ]
echo 'FAKETCP_RECONNECT_STRESS_PASS'
