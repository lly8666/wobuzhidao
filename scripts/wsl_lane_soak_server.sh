#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?asset dir}
LOG_DIR=${2:?log dir}
INNER_MTU=${3:?inner mtu}
MAX_LANES=${4:?max lanes}
LOSS_PCT=${5:-15}
STOP_FILE="$LOG_DIR/stop.server"
RAW=40000
LINK=47000
GAME=49000
ECHO=48000
ROUTE_KEY='WBD_REALITY_ROUTE_KEY_0123456789abcdef'
USERNAME='solo'
PASSWORD='shared-password'
TARGET='target.example'
PIDS=()

mkdir -p "$LOG_DIR" "$LOG_DIR/tickets"
rm -f "$STOP_FILE" "$LOG_DIR/server-ready.json"
chmod 700 "$LOG_DIR/tickets"
chmod +x "$ASSET_DIR"/wbd-* "$ASSET_DIR/wbd_dtls_shim"
IFACE=$(ip route show default | awk 'NR==1 {print $5}')
SERVER_IP=$(ip -4 addr show dev "$IFACE" | awk '/inet / {sub(/\/.*/,"",$2); print $2; exit}')
[[ -n "$SERVER_IP" ]] || { echo 'no WSL IPv4' >&2; exit 1; }
ORIGINAL_INPUT_POLICY=$(iptables -S INPUT | awk 'NR==1 {print $3}')
[[ "$ORIGINAL_INPUT_POLICY" == ACCEPT || "$ORIGINAL_INPUT_POLICY" == DROP ]] || ORIGINAL_INPUT_POLICY=ACCEPT

cleanup() {
  set +e
  iptables -P INPUT "$ORIGINAL_INPUT_POLICY" 2>/dev/null || true
  iptables -D OUTPUT -p tcp --sport "$RAW" --tcp-flags RST RST -j DROP 2>/dev/null || true
  iptables -D INPUT -p tcp --dport "$RAW" -m statistic --mode random --probability "0.$LOSS_PCT" -j DROP 2>/dev/null || true
  iptables -D INPUT -p tcp --dport "$RAW" -j ACCEPT 2>/dev/null || true
  iptables -D INPUT -i lo -j ACCEPT 2>/dev/null || true
  iptables -D INPUT -p icmp -j ACCEPT 2>/dev/null || true
  tc qdisc del dev "$IFACE" root 2>/dev/null || true
  for p in "${PIDS[@]:-}"; do kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  for p in "${PIDS[@]:-}"; do kill -KILL "$p" 2>/dev/null || true; done
}
trap cleanup EXIT

# Test-scoped stateful perimeter: deny unsolicited ingress by default, permit
# loopback/ICMP plus the one public FakeTCP port, and inject 15% random loss on
# that explicit public rule before acceptance.
iptables -I INPUT 1 -i lo -j ACCEPT
iptables -I INPUT 2 -p icmp -j ACCEPT
iptables -I INPUT 3 -p tcp --dport "$RAW" -m statistic --mode random --probability "0.$LOSS_PCT" -j DROP
iptables -I INPUT 4 -p tcp --dport "$RAW" -j ACCEPT
iptables -P INPUT DROP
iptables -I OUTPUT 1 -p tcp --sport "$RAW" --tcp-flags RST RST -j DROP
tc qdisc replace dev "$IFACE" root netem loss random "${LOSS_PCT}%"
{
  echo "WBD_WSL_FIREWALL_READY iface=$IFACE raw_port=$RAW input_policy=DROP ingress_loss_pct=$LOSS_PCT egress_loss_pct=$LOSS_PCT"
  iptables -L INPUT -n -v -x
  tc -s qdisc show dev "$IFACE"
} >"$LOG_DIR/network-start.log" 2>&1

cat >"$LOG_DIR/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    if b.startswith(b'WBD1') or b.startswith(b'warm') or b.startswith(b'game'):
        s.sendto(b,a)
PY
python3 "$LOG_DIR/echo.py" >"$LOG_DIR/echo.log" 2>&1 & PIDS+=("$!")

"$ASSET_DIR/wbd-game-lane-server" -listen 127.0.0.1:${GAME} -service 127.0.0.1:${ECHO} -max-lanes "$MAX_LANES" >"$LOG_DIR/game-server.log" 2>&1 & PIDS+=("$!")
"$ASSET_DIR/wbd-link-server-mux" -listen 127.0.0.1:${LINK} -service 127.0.0.1:${GAME} -mtu "$INNER_MTU" -ticket-dir "$LOG_DIR/tickets" -ticket-ttl 120s -setup-timeout 30s -idle-timeout 180s -max-sessions 16 >"$LOG_DIR/link-server.log" 2>&1 & PIDS+=("$!")
"$ASSET_DIR/wbd-faketcp-mux" server --listen "${SERVER_IP}:${RAW}" --dtls-shim "$ASSET_DIR/wbd_dtls_shim" --link-target 127.0.0.1:${LINK} --cert "$ASSET_DIR/dtls.pem" --key "$ASSET_DIR/dtls.key" --front-cert "$ASSET_DIR/front.pem" --front-key "$ASSET_DIR/front.key" --server-name "$TARGET" --route-key "$ROUTE_KEY" --username "$USERNAME" --password "$PASSWORD" --ticket-dir "$LOG_DIR/tickets" --fallback-target 127.0.0.1:9 --bootstrap-timeout 30s --max-sessions 16 >"$LOG_DIR/faketcp-mux.log" 2>&1 & PIDS+=("$!")

for _ in $(seq 1 600); do
  grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${MAX_LANES}" "$LOG_DIR/game-server.log" 2>/dev/null && \
  grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log" 2>/dev/null && \
  grep -q 'READY role=server-mux' "$LOG_DIR/faketcp-mux.log" 2>/dev/null && break
  sleep .1
done
grep -q "WBD_GAME_LANE_SERVER_READY.*max_lanes=${MAX_LANES}" "$LOG_DIR/game-server.log"
grep -q 'WBD_LINK_SERVER_MUX_READY' "$LOG_DIR/link-server.log"
grep -q 'READY role=server-mux' "$LOG_DIR/faketcp-mux.log"
printf '{"server_ip":"%s","interface":"%s","inner_mtu":%s,"max_lanes":%s,"loss_pct":%s}\n' "$SERVER_IP" "$IFACE" "$INNER_MTU" "$MAX_LANES" "$LOSS_PCT" >"$LOG_DIR/server-ready.json"

while [[ ! -e "$STOP_FILE" ]]; do
  for p in "${PIDS[@]}"; do kill -0 "$p" 2>/dev/null || { echo "server child died pid=$p" >&2; exit 1; }; done
  sleep 1
done

{
  echo '=== iptables input ==='; iptables -L INPUT -n -v -x
  echo '=== tc netem ==='; tc -s qdisc show dev "$IFACE"
} >"$LOG_DIR/network-end.log" 2>&1

echo "WBD_WSL_SERVER_SOAK_PASS server_ip=$SERVER_IP inner_mtu=$INNER_MTU max_lanes=$MAX_LANES loss_pct=$LOSS_PCT"
