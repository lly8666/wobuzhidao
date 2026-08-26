#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 WBD_PLATFORM_PROXY_OPENWRT_BIN WBD_PLATFORM_PROXY_SERVER_BIN" >&2
    exit 2
fi
if [[ $(id -u) -ne 0 ]]; then
    echo "openwrt_tcp_tproxy_netns.sh requires root" >&2
    exit 1
fi

CLIENT_BIN=$(readlink -f "$1")
SERVER_BIN=$(readlink -f "$2")
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d /tmp/wbd-openwrt-tcp.XXXXXX)
NS_CLIENT=wbd-tcp-client-$$
NS_PROXY=wbd-tcp-proxy-$$
NS_SERVER=wbd-tcp-server-$$
CLIENT_PID=
SERVER_PID=
TARGET_PID=
UNDERLAY_PID=
RESTORE_PID=

cleanup() {
    set +e
    for pid in "$CLIENT_PID" "$SERVER_PID" "$TARGET_PID" "$UNDERLAY_PID" "$RESTORE_PID"; do
        [[ -z "$pid" ]] || kill "$pid" 2>/dev/null || true
    done
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

ip link add tcc$$ type veth peer name tcp$$
ip link set tcc$$ netns "$NS_CLIENT"
ip link set tcp$$ netns "$NS_PROXY"
ip link add tps$$ type veth peer name tss$$
ip link set tps$$ netns "$NS_PROXY"
ip link set tss$$ netns "$NS_SERVER"

ip -n "$NS_CLIENT" addr add 10.10.0.2/24 dev tcc$$
ip -n "$NS_CLIENT" link set tcc$$ up
ip -n "$NS_CLIENT" route add default via 10.10.0.1

ip -n "$NS_PROXY" addr add 10.10.0.1/24 dev tcp$$
ip -n "$NS_PROXY" link set tcp$$ up
ip -n "$NS_PROXY" addr add 10.20.0.1/24 dev tps$$
ip -n "$NS_PROXY" link set tps$$ up
ip netns exec "$NS_PROXY" sysctl -qw net.ipv4.ip_forward=1

ip -n "$NS_SERVER" addr add 10.20.0.2/24 dev tss$$
ip -n "$NS_SERVER" addr add 10.20.0.3/24 dev tss$$
ip -n "$NS_SERVER" link set tss$$ up
ip -n "$NS_SERVER" route add 10.10.0.0/24 via 10.20.0.1

cat >"$TMP/tcp_server.py" <<'PYEOF'
import socket,sys
addr=sys.argv[1]
port=int(sys.argv[2])
tag=sys.argv[3].encode()
required=sys.argv[4]
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind((addr,port))
s.listen(16)
while True:
    c,peer=s.accept()
    try:
        if required != "-" and peer[0] != required:
            c.close()
            continue
        data=b""
        while True:
            part=c.recv(4096)
            if not part:
                break
            data += part
            if len(data) > 65536:
                break
        c.sendall(tag+b":"+data)
        try:
            c.shutdown(socket.SHUT_WR)
        except OSError:
            pass
    finally:
        c.close()
PYEOF

ip netns exec "$NS_SERVER" python3 -u "$TMP/tcp_server.py" 10.20.0.2 8080 PROXIED 10.20.0.1 >"$TMP/target.log" 2>&1 &
TARGET_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/tcp_server.py" 10.20.0.3 8081 BYPASS - >"$TMP/underlay.log" 2>&1 &
UNDERLAY_PID=$!
ip netns exec "$NS_SERVER" python3 -u "$TMP/tcp_server.py" 10.20.0.2 8082 RESTORED - >"$TMP/restore.log" 2>&1 &
RESTORE_PID=$!

ip netns exec "$NS_PROXY" "$SERVER_BIN" -listen 127.0.0.1:20001 -udp-idle 10s -tcp-idle 10s >"$TMP/platform-server.log" 2>&1 &
SERVER_PID=$!
ip netns exec "$NS_PROXY" "$CLIENT_BIN" -port 12345 -wbd 127.0.0.1:20001 -udp-idle 10s -tcp-idle 10s -ipv6=false >"$TMP/platform-client.log" 2>&1 &
CLIENT_PID=$!

for _ in $(seq 1 50); do
    if grep -q WBD_PLATFORM_PROXY_SERVER_READY "$TMP/platform-server.log" && grep -q WBD_PLATFORM_PROXY_OPENWRT_READY "$TMP/platform-client.log"; then
        break
    fi
    kill -0 "$SERVER_PID" 2>/dev/null || { cat "$TMP/platform-server.log" >&2; exit 1; }
    kill -0 "$CLIENT_PID" 2>/dev/null || { cat "$TMP/platform-client.log" >&2; exit 1; }
    sleep 0.1
done
grep -q 'tcp=1' "$TMP/platform-server.log"
grep -q 'tcp=1' "$TMP/platform-client.log"

ip netns exec "$NS_PROXY" sh "$ROOT/scripts/openwrt_tproxy.sh" apply \
    --mode global --port 12345 --underlay4 10.20.0.3

ip netns exec "$NS_CLIENT" python3 - <<'PY'
import socket
s=socket.create_connection(("10.20.0.2",8080),timeout=3)
s.sendall(b"tcp-through-wbdp")
s.shutdown(socket.SHUT_WR)
s.settimeout(5)
out=b""
while True:
    part=s.recv(4096)
    if not part:
        break
    out += part
assert out == b"PROXIED:tcp-through-wbdp", out
assert s.getpeername() == ("10.20.0.2",8080), s.getpeername()
print("WBD_OPENWRT_TCP_TPROXY_CAPTURE_PASS", flush=True)
PY

kill "$CLIENT_PID"
wait "$CLIENT_PID" 2>/dev/null || true
CLIENT_PID=

# Underlay destination is exempt before the TPROXY rule, so it must still work
# while the transparent adapter is stopped.
ip netns exec "$NS_CLIENT" python3 - <<'PY'
import socket
s=socket.create_connection(("10.20.0.3",8081),timeout=3)
s.sendall(b"underlay")
s.shutdown(socket.SHUT_WR)
out=b""
while True:
    part=s.recv(4096)
    if not part: break
    out += part
assert out == b"BYPASS:underlay", out
print("WBD_OPENWRT_TCP_UNDERLAY_ESCAPE_PASS", flush=True)
PY

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

# Capture is gone and the adapter remains stopped: ordinary routed TCP must be
# restored to a normal destination.
ip netns exec "$NS_CLIENT" python3 - <<'PY'
import socket
s=socket.create_connection(("10.20.0.2",8082),timeout=3)
s.sendall(b"plain")
s.shutdown(socket.SHUT_WR)
out=b""
while True:
    part=s.recv(4096)
    if not part: break
    out += part
assert out == b"RESTORED:plain", out
print("WBD_OPENWRT_TCP_CLEANUP_PASS", flush=True)
PY
