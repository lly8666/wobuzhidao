#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 WBD_PLATFORM_PROXY_OPENWRT_BIN WBD_PLATFORM_PROXY_SERVER_BIN" >&2
    exit 2
fi
if [[ $(id -u) -ne 0 ]]; then
    echo "openwrt_udp_tproxy_netns.sh requires root" >&2
    exit 1
fi

CLIENT_BIN=$(readlink -f "$1")
SERVER_BIN=$(readlink -f "$2")
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d /tmp/wbd-openwrt-udp.XXXXXX)
NS_CLIENT=wbd-udp-client-$$
NS_PROXY=wbd-udp-proxy-$$
NS_SERVER=wbd-udp-server-$$
CLIENT_PID=
SERVER_PID=
ECHO_APP_PID=
ECHO_BYPASS_PID=

cleanup() {
    set +e
    [[ -z "$CLIENT_PID" ]] || kill "$CLIENT_PID" 2>/dev/null || true
    [[ -z "$SERVER_PID" ]] || kill "$SERVER_PID" 2>/dev/null || true
    [[ -z "$ECHO_APP_PID" ]] || kill "$ECHO_APP_PID" 2>/dev/null || true
    [[ -z "$ECHO_BYPASS_PID" ]] || kill "$ECHO_BYPASS_PID" 2>/dev/null || true
    ip netns exec "$NS_PROXY" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup --port 12345 --underlay4 10.20.0.3 >/dev/null 2>&1 || true
    for ns in "$NS_CLIENT" "$NS_PROXY" "$NS_SERVER"; do
        ip netns pids "$ns" 2>/dev/null | xargs -r kill 2>/dev/null || true
        ip netns del "$ns" 2>/dev/null || true
    done
    rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

for tool in ip nft python3; do
    command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 2; }
done
[[ -x "$CLIENT_BIN" ]] || { echo "client binary not executable: $CLIENT_BIN" >&2; exit 2; }
[[ -x "$SERVER_BIN" ]] || { echo "server binary not executable: $SERVER_BIN" >&2; exit 2; }

ip netns add "$NS_CLIENT"
ip netns add "$NS_PROXY"
ip netns add "$NS_SERVER"
for ns in "$NS_CLIENT" "$NS_PROXY" "$NS_SERVER"; do
    ip -n "$ns" link set lo up
done

ip link add wbdc$$ type veth peer name wbdpc$$
ip link set wbdc$$ netns "$NS_CLIENT"
ip link set wbdpc$$ netns "$NS_PROXY"
ip link add wbdps$$ type veth peer name wbds$$
ip link set wbdps$$ netns "$NS_PROXY"
ip link set wbds$$ netns "$NS_SERVER"

ip -n "$NS_CLIENT" addr add 10.10.0.2/24 dev wbdc$$
ip -n "$NS_CLIENT" link set wbdc$$ up
ip -n "$NS_CLIENT" route add default via 10.10.0.1

ip -n "$NS_PROXY" addr add 10.10.0.1/24 dev wbdpc$$
ip -n "$NS_PROXY" link set wbdpc$$ up
ip -n "$NS_PROXY" addr add 10.20.0.1/24 dev wbdps$$
ip -n "$NS_PROXY" link set wbdps$$ up
ip netns exec "$NS_PROXY" sysctl -qw net.ipv4.ip_forward=1

ip -n "$NS_SERVER" addr add 10.20.0.2/24 dev wbds$$
ip -n "$NS_SERVER" addr add 10.20.0.3/24 dev wbds$$
ip -n "$NS_SERVER" link set wbds$$ up
ip -n "$NS_SERVER" route add 10.10.0.0/24 via 10.20.0.1

cat >"$TMP/echo.py" <<'PYEOF'
import socket,sys
addr=sys.argv[1]
tag=sys.argv[2].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind((addr,5353))
while True:
    data,peer=s.recvfrom(65535)
    s.sendto(tag+b":"+data,peer)
PYEOF

ip netns exec "$NS_SERVER" python3 -u "$TMP/echo.py" 10.20.0.2 APP >"$TMP/echo-app.log" 2>&1 &
ECHO_APP_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/echo.py" 10.20.0.3 BYPASS >"$TMP/echo-bypass.log" 2>&1 &
ECHO_BYPASS_PID=$!

ip netns exec "$NS_PROXY" "$SERVER_BIN" -listen 127.0.0.1:20001 -udp-idle 10s >"$TMP/platform-server.log" 2>&1 &
SERVER_PID=$!
ip netns exec "$NS_PROXY" "$CLIENT_BIN" -port 12345 -wbd 127.0.0.1:20001 -udp-idle 10s -ipv6=false >"$TMP/platform-client.log" 2>&1 &
CLIENT_PID=$!

for _ in $(seq 1 50); do
    if grep -q WBD_PLATFORM_PROXY_SERVER_READY "$TMP/platform-server.log" && grep -q WBD_PLATFORM_PROXY_OPENWRT_READY "$TMP/platform-client.log"; then
        break
    fi
    kill -0 "$SERVER_PID" 2>/dev/null || { cat "$TMP/platform-server.log" >&2; exit 1; }
    kill -0 "$CLIENT_PID" 2>/dev/null || { cat "$TMP/platform-client.log" >&2; exit 1; }
    sleep 0.1
done
grep -q WBD_PLATFORM_PROXY_SERVER_READY "$TMP/platform-server.log"
grep -q WBD_PLATFORM_PROXY_OPENWRT_READY "$TMP/platform-client.log"

ip netns exec "$NS_PROXY" sh "$ROOT/scripts/openwrt_tproxy.sh" apply \
    --mode global --port 12345 --underlay4 10.20.0.3

udp_expect() {
    local dst=$1
    local payload=$2
    local want=$3
    ip netns exec "$NS_CLIENT" python3 - "$dst" "$payload" "$want" <<'PY'
import socket,sys
host=sys.argv[1]; payload=sys.argv[2].encode(); want=sys.argv[3].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.settimeout(3)
s.sendto(payload,(host,5353))
data,peer=s.recvfrom(65535)
assert peer == (host,5353), (peer,(host,5353))
assert data == want, (data,want)
PY
}

udp_expect 10.20.0.2 captured APP:captured

echo "WBD_OPENWRT_UDP_TPROXY_CAPTURE_PASS"

kill "$CLIENT_PID"
wait "$CLIENT_PID" 2>/dev/null || true
CLIENT_PID=

# The underlay destination is excluded before the TPROXY rule, so it must keep
# working even with the transparent adapter stopped.
udp_expect 10.20.0.3 underlay BYPASS:underlay
echo "WBD_OPENWRT_UDP_UNDERLAY_ESCAPE_PASS"

ip netns exec "$NS_PROXY" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup \
    --port 12345 --underlay4 10.20.0.3
if ip netns exec "$NS_PROXY" nft list table inet wbd >/dev/null 2>&1; then
    echo "WBD nft table survived cleanup" >&2
    exit 1
fi
if ip netns exec "$NS_PROXY" ip rule show | grep -q '^1066:'; then
    echo "WBD policy rule survived cleanup" >&2
    exit 1
fi

# With capture removed and the adapter still stopped, the original application
# destination must return to ordinary routed UDP behavior.
udp_expect 10.20.0.2 restored APP:restored
echo "WBD_OPENWRT_UDP_CLEANUP_PASS"
