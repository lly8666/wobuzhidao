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
ECHO_APP_A_PID=
ECHO_APP_B_PID=
ECHO_BYPASS_PID=
INJECTOR_PID=

cleanup() {
    set +e
    [[ -z "$CLIENT_PID" ]] || kill "$CLIENT_PID" 2>/dev/null || true
    [[ -z "$SERVER_PID" ]] || kill "$SERVER_PID" 2>/dev/null || true
    [[ -z "$ECHO_APP_A_PID" ]] || kill "$ECHO_APP_A_PID" 2>/dev/null || true
    [[ -z "$ECHO_APP_B_PID" ]] || kill "$ECHO_APP_B_PID" 2>/dev/null || true
    [[ -z "$ECHO_BYPASS_PID" ]] || kill "$ECHO_BYPASS_PID" 2>/dev/null || true
    [[ -z "$INJECTOR_PID" ]] || kill "$INJECTOR_PID" 2>/dev/null || true
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

for addr in 10.20.0.2 10.20.0.3 10.20.0.4 10.20.0.5; do
    ip -n "$NS_SERVER" addr add "$addr/24" dev wbds$$
done
ip -n "$NS_SERVER" link set wbds$$ up
ip -n "$NS_SERVER" route add 10.10.0.0/24 via 10.20.0.1

cat >"$TMP/echo.py" <<'PYEOF'
import socket,sys
addr=sys.argv[1]
port=int(sys.argv[2])
tag=sys.argv[3].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind((addr,port))
while True:
    data,peer=s.recvfrom(65535)
    reply=tag+b":"+data+b":SEEN="+str(peer[1]).encode()
    s.sendto(reply,peer)
PYEOF

cat >"$TMP/injector.py" <<'PYEOF'
import socket
control=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
control.bind(("10.20.0.3",5454))
tx=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
tx.bind(("10.20.0.5",7777))
while True:
    data,_=control.recvfrom(128)
    port=int(data.decode())
    tx.sendto(b"UNSOLICITED",("10.20.0.1",port))
PYEOF

ip netns exec "$NS_SERVER" python3 -u "$TMP/echo.py" 10.20.0.2 5353 APPA >"$TMP/echo-app-a.log" 2>&1 &
ECHO_APP_A_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/echo.py" 10.20.0.4 5354 APPB >"$TMP/echo-app-b.log" 2>&1 &
ECHO_APP_B_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/echo.py" 10.20.0.3 5353 BYPASS >"$TMP/echo-bypass.log" 2>&1 &
ECHO_BYPASS_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/injector.py" >"$TMP/injector.log" 2>&1 &
INJECTOR_PID=$!

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

# One internal UDP source talks to two different external endpoints. Both
# endpoints must observe the same server-side mapped source port (EIM). Then a
# third endpoint that the internal source never contacted sends directly to the
# mapped port; that packet must return with its real source endpoint (EIF).
ip netns exec "$NS_CLIENT" python3 - <<'PY'
import socket

def seen_port(data, prefix):
    assert data.startswith(prefix), (data,prefix)
    marker=b":SEEN="
    assert marker in data, data
    return int(data.rsplit(marker,1)[1])

s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(("10.10.0.2",0))
s.settimeout(3)

s.sendto(b"cone-a",("10.20.0.2",5353))
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.2",5353), peer
mapped_a=seen_port(data,b"APPA:cone-a")

s.sendto(b"cone-b",("10.20.0.4",5354))
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.4",5354), peer
mapped_b=seen_port(data,b"APPB:cone-b")
assert mapped_a == mapped_b and mapped_a != 0, (mapped_a,mapped_b)
print(f"WBD_OPENWRT_UDP_FULLCONE_EIM_PASS mapped_port={mapped_a}", flush=True)

# 10.20.0.3 is the configured underlay escape, so this control datagram bypasses
# TPROXY. The injector then sends from unseen 10.20.0.5:7777 directly to the
# server-side mapping port discovered above.
s.sendto(str(mapped_a).encode(),("10.20.0.3",5454))
data,peer=s.recvfrom(65535)
assert peer == ("10.20.0.5",7777), peer
assert data == b"UNSOLICITED", data
print("WBD_OPENWRT_UDP_FULLCONE_EIF_PASS", flush=True)
PY

echo "WBD_OPENWRT_UDP_TPROXY_CAPTURE_PASS"

kill "$CLIENT_PID"
wait "$CLIENT_PID" 2>/dev/null || true
CLIENT_PID=

udp_expect_prefix() {
    local dst=$1
    local port=$2
    local payload=$3
    local prefix=$4
    ip netns exec "$NS_CLIENT" python3 - "$dst" "$port" "$payload" "$prefix" <<'PY'
import socket,sys
host=sys.argv[1]; port=int(sys.argv[2]); payload=sys.argv[3].encode(); prefix=sys.argv[4].encode()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.settimeout(3)
s.sendto(payload,(host,port))
data,peer=s.recvfrom(65535)
assert peer == (host,port), (peer,(host,port))
assert data.startswith(prefix), (data,prefix)
PY
}

# The underlay destination is excluded before the TPROXY rule, so it must keep
# working even with the transparent adapter stopped.
udp_expect_prefix 10.20.0.3 5353 underlay BYPASS:underlay
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
udp_expect_prefix 10.20.0.2 5353 restored APPA:restored
echo "WBD_OPENWRT_UDP_CLEANUP_PASS"
