#!/bin/sh
set -eu

BIN=${WBD_FAKETCP_BIN:-/tmp/wbd-faketcp}
PORT=${WBD_SHARED_PORT:-52443}
C="wbdspc$$"
S="wbdsps$$"
CPID=
SPID=
TPID=
EPID=

cleanup() {
    set +e
    [ -z "$CPID" ] || kill -TERM "$CPID" 2>/dev/null || true
    [ -z "$SPID" ] || kill -TERM "$SPID" 2>/dev/null || true
    [ -z "$TPID" ] || kill -TERM "$TPID" 2>/dev/null || true
    [ -z "$EPID" ] || kill -TERM "$EPID" 2>/dev/null || true
    sleep .1
    ip netns del "$C" 2>/dev/null || true
    ip netns del "$S" 2>/dev/null || true
    rm -f /tmp/wbd-shared-tcp.py /tmp/wbd-shared-udp.py /tmp/wbd-shared-probe.py \
        /tmp/wbd-shared-client.log /tmp/wbd-shared-server.log /tmp/wbd-shared-tcp.log /tmp/wbd-shared-udp.log
}
trap cleanup EXIT INT TERM HUP

[ "$(id -u)" -eq 0 ] || { echo 'linux_shared_port_smoke.sh requires root' >&2; exit 1; }
[ -x "$BIN" ] || { echo "missing executable WBD_FAKETCP_BIN=$BIN" >&2; exit 1; }
command -v ip >/dev/null
command -v iptables >/dev/null
command -v python3 >/dev/null

ip netns add "$C"
ip netns add "$S"
ip link add wbdspc0 type veth peer name wbdsps0
ip link set wbdspc0 netns "$C"
ip link set wbdsps0 netns "$S"
ip -n "$C" addr add 10.89.0.1/24 dev wbdspc0
ip -n "$S" addr add 10.89.0.2/24 dev wbdsps0
ip -n "$C" link set lo up
ip -n "$S" link set lo up
ip -n "$C" link set wbdspc0 up
ip -n "$S" link set wbdsps0 up

# Product-side raw RST ownership is scoped to the shared public source port.
# The client intentionally keeps its normal kernel behavior: the stray kernel
# SYN-ACK produced by the real TCP listener for a WBD raw SYN may be reset, and
# the WBD raw association must remain unaffected.
ip netns exec "$S" iptables -I OUTPUT -p tcp --sport "$PORT" --tcp-flags RST RST -j DROP

cat >/tmp/wbd-shared-tcp.py <<'PY'
import socket,sys
port=int(sys.argv[1])
s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind(('10.89.0.2',port)); s.listen(16)
while True:
    c,_=s.accept()
    with c:
        b=c.recv(4096)
        if b: c.sendall(b'KERNEL:'+b)
PY

cat >/tmp/wbd-shared-udp.py <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',52500))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b'RAW:'+b,a)
PY

cat >/tmp/wbd-shared-probe.py <<'PY'
import socket,sys,time
kind=sys.argv[1]
if kind=='tcp':
    payload=sys.argv[2].encode()
    s=socket.create_connection(('10.89.0.2',int(sys.argv[3])),timeout=3)
    s.sendall(payload); got=s.recv(4096); s.close()
    want=b'KERNEL:'+payload
    assert got==want,(got,want)
    print('WBD_SHARED_PORT_TCP_PASS',sys.argv[2])
elif kind=='raw':
    payload=b'fake-tcp-shared-port'
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); s.settimeout(5)
    s.sendto(payload,('127.0.0.1',52504)); got,_=s.recvfrom(4096); s.close()
    assert got==b'RAW:'+payload,(got,payload)
    print('WBD_SHARED_PORT_RAW_PASS')
else:
    raise SystemExit(2)
PY

ip netns exec "$S" python3 /tmp/wbd-shared-tcp.py "$PORT" >/tmp/wbd-shared-tcp.log 2>&1 & TPID=$!
ip netns exec "$S" python3 /tmp/wbd-shared-udp.py >/tmp/wbd-shared-udp.log 2>&1 & EPID=$!
ip netns exec "$S" "$BIN" server --listen "10.89.0.2:$PORT" --target-udp 127.0.0.1:52500 >/tmp/wbd-shared-server.log 2>&1 & SPID=$!

# Ordinary kernel TCP must work while the raw receiver is observing the same
# destination port; an ordinary SYN must not create a WBD raw association.
sleep .2
ip netns exec "$C" python3 /tmp/wbd-shared-probe.py tcp before "$PORT"
if grep -q 'READY role=server' /tmp/wbd-shared-server.log; then
    echo 'raw server incorrectly accepted ordinary kernel TCP SYN' >&2
    cat /tmp/wbd-shared-server.log >&2
    exit 1
fi

ip netns exec "$C" "$BIN" client --local-udp 127.0.0.1:52504 --source 10.89.0.1:52510 --remote "10.89.0.2:$PORT" >/tmp/wbd-shared-client.log 2>&1 & CPID=$!
i=0
while [ "$i" -lt 200 ]; do
    if grep -q 'READY role=client' /tmp/wbd-shared-client.log && grep -q 'READY role=server' /tmp/wbd-shared-server.log; then
        break
    fi
    if ! kill -0 "$CPID" 2>/dev/null || ! kill -0 "$SPID" 2>/dev/null; then
        break
    fi
    i=$((i+1)); sleep .05
done
cat /tmp/wbd-shared-client.log
cat /tmp/wbd-shared-server.log
grep -q 'READY role=client' /tmp/wbd-shared-client.log
grep -q 'READY role=server' /tmp/wbd-shared-server.log

ip netns exec "$C" python3 /tmp/wbd-shared-probe.py raw
# The real kernel TCP listener must still be usable after a raw association has
# completed on the same numeric public port.
ip netns exec "$C" python3 /tmp/wbd-shared-probe.py tcp after "$PORT"

echo "WBD_LINUX_SHARED_PORT_PASS port=$PORT kernel_tcp=1 faketcp=1 coexistence=1"
