#!/bin/bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
    echo "usage: $0 WBD_PLATFORM_PROXY_OPENWRT_BIN WBD_PLATFORM_PROXY_SERVER_BIN" >&2
    exit 2
fi
if [[ $(id -u) -ne 0 ]]; then
    echo "openwrt_udp_hairpin_netns.sh requires root" >&2
    exit 1
fi

CLIENT_BIN=$(readlink -f "$1")
SERVER_BIN=$(readlink -f "$2")
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d /tmp/wbd-openwrt-hairpin.XXXXXX)
C=wbdhc$$
R=wbdhr$$
S=wbdhs$$
T=wbdht$$
PIDS=()

cleanup() {
    set +e
    if ip netns list | grep -q "^$R\b"; then
        ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup --port 12345 --underlay4 10.78.0.1 >/dev/null 2>&1 || true
    fi
    for pid in "${PIDS[@]:-}"; do
        [[ -z "$pid" ]] || kill -TERM "$pid" 2>/dev/null || true
    done
    for ns in "$C" "$R" "$S" "$T"; do
        ip netns pids "$ns" 2>/dev/null | xargs -r kill -TERM 2>/dev/null || true
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

for ns in "$C" "$R" "$S" "$T"; do ip netns add "$ns"; done
for ns in "$C" "$R" "$S" "$T"; do ip -n "$ns" link set lo up; done

ip link add hc$$ type veth peer name hr0$$
ip link set hc$$ netns "$C"
ip link set hr0$$ netns "$R"
ip link add hr1$$ type veth peer name hs0$$
ip link set hr1$$ netns "$R"
ip link set hs0$$ netns "$S"
ip link add hs1$$ type veth peer name ht$$
ip link set hs1$$ netns "$S"
ip link set ht$$ netns "$T"

ip -n "$C" addr add 10.10.0.2/24 dev hc$$
ip -n "$C" link set hc$$ up
ip -n "$C" route add default via 10.10.0.1

ip -n "$R" addr add 10.10.0.1/24 dev hr0$$
ip -n "$R" link set hr0$$ up
ip -n "$R" addr add 10.78.0.2/24 dev hr1$$
ip -n "$R" link set hr1$$ up
ip -n "$R" route add 10.90.0.0/24 via 10.78.0.1
ip netns exec "$R" sysctl -qw net.ipv4.ip_forward=1

ip -n "$S" addr add 10.78.0.1/24 dev hs0$$
ip -n "$S" link set hs0$$ up
ip -n "$S" addr add 10.90.0.1/24 dev hs1$$
ip -n "$S" link set hs1$$ up
ip netns exec "$S" sysctl -qw net.ipv4.ip_forward=1

ip -n "$T" addr add 10.90.0.2/24 dev ht$$
ip -n "$T" link set ht$$ up

cat >"$TMP/target.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(("10.90.0.2",5353))
while True:
    data,peer=s.recvfrom(65535)
    s.sendto(b"ECHO:"+data+b":SEEN="+str(peer[1]).encode(),peer)
PY
cat >"$TMP/underlay.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(("10.78.0.1",5555))
while True:
    data,peer=s.recvfrom(65535)
    s.sendto(b"UNDERLAY:"+data+b":SEEN="+peer[0].encode(),peer)
PY

ip netns exec "$T" python3 -u "$TMP/target.py" >"$TMP/target.log" 2>&1 & PIDS+=("$!")
ip netns exec "$S" python3 -u "$TMP/underlay.py" >"$TMP/underlay.log" 2>&1 & PIDS+=("$!")
ip netns exec "$S" "$SERVER_BIN" -listen 10.78.0.1:20001 -udp-idle 20s >"$TMP/server.log" 2>&1 & SERVER_PID=$!; PIDS+=("$SERVER_PID")
ip netns exec "$R" "$CLIENT_BIN" -port 12345 -wbd 10.78.0.1:20001 -udp-idle 20s -ipv6=false >"$TMP/client.log" 2>&1 & CLIENT_PID=$!; PIDS+=("$CLIENT_PID")

for _ in $(seq 1 80); do
    if grep -q WBD_PLATFORM_PROXY_SERVER_READY "$TMP/server.log" && grep -q WBD_PLATFORM_PROXY_OPENWRT_READY "$TMP/client.log"; then break; fi
    kill -0 "$SERVER_PID" 2>/dev/null || { cat "$TMP/server.log" >&2; exit 1; }
    kill -0 "$CLIENT_PID" 2>/dev/null || { cat "$TMP/client.log" >&2; exit 1; }
    sleep .1
done
grep -q WBD_PLATFORM_PROXY_SERVER_READY "$TMP/server.log"
grep -q WBD_PLATFORM_PROXY_OPENWRT_READY "$TMP/client.log"

ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" apply --mode global --port 12345 --underlay4 10.78.0.1

ip netns exec "$C" python3 - <<'PY'
import socket

def mapped(sock,label):
    sock.sendto(label,("10.90.0.2",5353))
    data,peer=sock.recvfrom(65535)
    assert peer == ("10.90.0.2",5353),peer
    assert data.startswith(b"ECHO:"+label+b":SEEN="),data
    return int(data.rsplit(b":SEEN=",1)[1])

a=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
b=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
a.bind(("10.10.0.2",0)); b.bind(("10.10.0.2",0))
a.settimeout(5); b.settimeout(5)
pa=mapped(a,b"prime-a")
pb=mapped(b,b"prime-b")
assert pa and pb and pa != pb,(pa,pb)

# A reaches B through B's external server-side mapping endpoint.
a.sendto(b"HAIRPIN_A_TO_B",("10.90.0.1",pb))
data,peer=b.recvfrom(65535)
assert data == b"HAIRPIN_A_TO_B",data
assert peer == ("10.90.0.1",pa),(peer,pa)

# B replies to the source endpoint it observed; the reverse loopback must land A.
b.sendto(b"HAIRPIN_B_TO_A",peer)
data,peer=a.recvfrom(65535)
assert data == b"HAIRPIN_B_TO_A",data
assert peer == ("10.90.0.1",pb),(peer,pb)
print(f"WBD_OPENWRT_UDP_HAIRPIN_PASS a={pa} b={pb}",flush=True)

# The WBD underlay remains excluded from capture and keeps its direct source.
u=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); u.settimeout(4)
u.sendto(b"escape",("10.78.0.1",5555))
data,peer=u.recvfrom(65535)
assert peer == ("10.78.0.1",5555),peer
assert data == b"UNDERLAY:escape:SEEN=10.10.0.2",data
print("WBD_OPENWRT_UDP_HAIRPIN_UNDERLAY_ESCAPE_PASS",flush=True)
PY

ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup --port 12345 --underlay4 10.78.0.1
if ip netns exec "$R" nft list table inet wbd >/dev/null 2>&1; then
    echo "WBD nft table survived hairpin cleanup" >&2
    exit 1
fi
if ip netns exec "$R" ip rule show | grep -q '^1066:'; then
    echo "WBD policy rule survived hairpin cleanup" >&2
    exit 1
fi
echo "WBD_OPENWRT_UDP_HAIRPIN_CLEANUP_PASS"
