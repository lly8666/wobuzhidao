#!/usr/bin/env bash
# This harness must remain LF-only because Windows Actions executes it under WSL.
# Formal matrix trigger: Npcap preflight now preserves Windows PowerShell modules.
set -euo pipefail
ASSET_DIR=${1:?asset dir}
LOG_DIR=${2:?log dir}
INNER_MTU=${3:?inner mtu}
CONNECTION_MTU=${4:?connection mtu}
MAX_LANES=${5:?max lanes}
LOSS_PCT=${6:-15}
LINK_IDLE_TIMEOUT=${7:-180s}
SHARED_TUN_IDLE_TIMEOUT=${8:-180s}
STOP_FILE="$LOG_DIR/stop.server"
RAW=40000
LINK=47000
GAME=49000
GATEWAY=49100
ECHO_PORT=48000
ECHO_IP=198.18.0.1
TUN_IF=wbdg0
OUTER_CHAIN=WBD_SOAK_OUTER_METRICS
PIDS=()
mkdir -p "$LOG_DIR" "$LOG_DIR/tickets"
rm -f "$STOP_FILE" "$LOG_DIR/server-ready.json" "$LOG_DIR/shared-tun-firewall.state"
chmod 700 "$LOG_DIR/tickets"
chmod +x "$ASSET_DIR"/wbd-* "$ASSET_DIR/wbd_dtls_shim" "$ASSET_DIR/linux_shared_tun_firewall.sh"
IFACE=$(ip route show default | awk 'NR==1 {print $5}')
SERVER_IP=$(ip -4 addr show dev "$IFACE" | awk '/inet / {sub(/\/.*/,"",$2); print $2; exit}')
[[ -n "$SERVER_IP" ]] || { echo 'no WSL IPv4' >&2; exit 1; }
cleanup() {
  set +e
  tc qdisc del dev "$IFACE" root 2>/dev/null || true
  iptables -D OUTPUT -p tcp --sport "$RAW" --tcp-flags RST RST -j DROP 2>/dev/null || true
  iptables -t mangle -D OUTPUT -j "$OUTER_CHAIN" 2>/dev/null || true
  iptables -t mangle -F "$OUTER_CHAIN" 2>/dev/null || true
  iptables -t mangle -X "$OUTER_CHAIN" 2>/dev/null || true
  for p in "${PIDS[@]:-}"; do kill -TERM "$p" 2>/dev/null || true; done
  sleep .5
  for p in "${PIDS[@]:-}"; do kill -KILL "$p" 2>/dev/null || true; done
  "$ASSET_DIR/linux_shared_tun_firewall.sh" cleanup --backend iptables --state "$LOG_DIR/shared-tun-firewall.state" --lease-prefix 10.66.0.0/16 --tun-if "$TUN_IF" >/dev/null 2>&1 || true
  ip link del "$TUN_IF" 2>/dev/null || true
  ip addr del "$ECHO_IP/32" dev lo 2>/dev/null || true
}
trap cleanup EXIT
ip addr replace "$ECHO_IP/32" dev lo
cat >"$LOG_DIR/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('198.18.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    if b:
        s.sendto(b,a)
PY
python3 "$LOG_DIR/echo.py" >"$LOG_DIR/echo.log" 2>&1 & PIDS+=("$!")
"$ASSET_DIR/wbd-ip-gateway-shared" -listen 127.0.0.1:${GATEWAY} -firewall-helper "$ASSET_DIR/linux_shared_tun_firewall.sh" -backend iptables -firewall-state "$LOG_DIR/shared-tun-firewall.state" -lease-prefix 10.66.0.0/16 -tun-if "$TUN_IF" -mtu "$INNER_MTU" -idle-timeout "$SHARED_TUN_IDLE_TIMEOUT" -max-sessions 16 >"$LOG_DIR/shared-tun-gateway.log" 2>&1 & PIDS+=("$!")
for _ in $(seq 1 200); do grep -q 'WBD_SHARED_TUN_GATEWAY_READY' "$LOG_DIR/shared-tun-gateway.log" 2>/dev/null && break; sleep .1; done
grep -q 'WBD_SHARED_TUN_GATEWAY_READY' "$LOG_DIR/shared-tun-gateway.log"

# A raw FakeTCP listener has no kernel TCP socket on $RAW. Linux would therefore
# synthesize a TCP RST for the peer's valid raw handshake/data packets unless we
# suppress only that kernel-generated reset shape. The product mux owns this
# five-tuple in userspace; cleanup removes the guard symmetrically.
iptables -I OUTPUT 1 -p tcp --sport "$RAW" --tcp-flags RST RST -j DROP

# Count only the FakeTCP carrier before netem, then apply the requested random
# loss only to server egress from the FakeTCP public source port. Unrelated WSL
# traffic is excluded from formal outer packet/byte loss accounting.
iptables -t mangle -N "$OUTER_CHAIN"
iptables -t mangle -A "$OUTER_CHAIN" -p tcp --sport "$RAW" -j RETURN
iptables -t mangle -A OUTPUT -j "$OUTER_CHAIN"
tc qdisc replace dev "$IFACE" root handle 1: prio bands 3
tc qdisc replace dev "$IFACE" parent 1:3 handle 30: netem loss random "${LOSS_PCT}%"
tc filter replace dev "$IFACE" protocol ip parent 1:0 prio 1 u32 \
  match ip protocol 6 0xff \
  match ip sport "$RAW" 0xffff \
  flowid 1:3

"$ASSET_DIR/wbd-game-lane-server" -listen 127.0.0.1:${GAME} -service 127.0.0.1:${GATEWAY} -max-lanes "$MAX_LANES" >"$LOG_DIR/game-server.log" 2>&1 & PIDS+=("$!")
"$ASSET_DIR/wbd-link-server-mux" -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} -mtu "$INNER_MTU" -connection-mtu "$CONNECTION_MTU" -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 120s -setup-timeout 30s -idle-timeout "$LINK_IDLE_TIMEOUT" -max-sessions 16 >"$LOG_DIR/link-server.log" 2>&1 & PIDS+=("$!")
"$ASSET_DIR/wbd-faketcp-mux" server --listen "${SERVER_IP}:${RAW}" --dtls-shim "$ASSET_DIR/wbd_dtls_shim" --link-target 127.0.0.1:${LINK} --cert "$ASSET_DIR/dtls.pem" --key "$ASSET_DIR/dtls.key" --front-cert "$ASSET_DIR/front.pem" --front-key "$ASSET_DIR/front.key" --server-name target.example --route-key WBD_REALITY_ROUTE_KEY_0123456789abcdef --username solo --password shared-password --ticket-dir "$LOG_DIR/tickets" --fallback-target 127.0.0.1:9 --bootstrap-timeout 30s --max-sessions 16 >"$LOG_DIR/faketcp-mux.log" 2>&1 & PIDS+=("$!")
for _ in $(seq 1 600); do
  grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${MAX_LANES}" "$LOG_DIR/game-server.log" 2>/dev/null && grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log" 2>/dev/null && grep -q 'READY role=server-mux' "$LOG_DIR/faketcp-mux.log" 2>/dev/null && break
  sleep .1
done
grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${MAX_LANES}" "$LOG_DIR/game-server.log"
grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log"
grep -q 'READY role=server-mux' "$LOG_DIR/faketcp-mux.log"
printf '{"server_ip":"%s","inner_mtu":%s,"connection_mtu":%s,"max_lanes":%s,"loss_pct":%s,"link_idle_timeout":"%s","shared_tun_idle_timeout":"%s","echo_ip":"%s","echo_port":%s,"tun_if":"%s"}\n' "$SERVER_IP" "$INNER_MTU" "$CONNECTION_MTU" "$MAX_LANES" "$LOSS_PCT" "$LINK_IDLE_TIMEOUT" "$SHARED_TUN_IDLE_TIMEOUT" "$ECHO_IP" "$ECHO_PORT" "$TUN_IF" >"$LOG_DIR/server-ready.json"
echo "WBD_WSL_WINTUN_SOAK_READY server_ip=$SERVER_IP tun=$TUN_IF echo=$ECHO_IP:$ECHO_PORT loss_pct=$LOSS_PCT link_idle_timeout=$LINK_IDLE_TIMEOUT shared_tun_idle_timeout=$SHARED_TUN_IDLE_TIMEOUT rst_guard=1"
while [[ ! -e "$STOP_FILE" ]]; do for p in "${PIDS[@]}"; do kill -0 "$p" 2>/dev/null || { echo "server child died pid=$p" >&2; exit 1; }; done; sleep 1; done
{
  echo '=== outer pre-netem ==='; iptables -t mangle -L "$OUTER_CHAIN" -nvx
  echo '=== rst guard ==='; iptables -nvx -L OUTPUT | grep -E 'DROP.*tcp.*spt:40000.*flags:0x04/0x04' || true
  echo '=== tc netem ==='; tc -s qdisc show dev "$IFACE"
  echo '=== shared tun ==='; ip -s link show dev "$TUN_IF"
  echo '=== shared route ==='; ip route show 10.66.0.0/16
} >"$LOG_DIR/network-end.log" 2>&1
echo "WBD_WSL_WINTUN_SOAK_PASS server_ip=$SERVER_IP inner_mtu=$INNER_MTU max_lanes=$MAX_LANES loss_pct=$LOSS_PCT link_idle_timeout=$LINK_IDLE_TIMEOUT shared_tun_idle_timeout=$SHARED_TUN_IDLE_TIMEOUT rst_guard=1"
