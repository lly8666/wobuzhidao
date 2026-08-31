#!/bin/bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
    echo "usage: $0 ASSET_DIR [LOG_DIR]" >&2
    exit 2
fi
if [[ $(id -u) -ne 0 ]]; then
    echo "openwrt_fullstack_one_shot.sh requires root" >&2
    exit 1
fi

ASSET_DIR=$(readlink -f "$1")
LOG_DIR=${2:-/tmp/wbd-openwrt-fullstack-logs}
mkdir -p "$LOG_DIR"
ROOT=$(cd "$(dirname "$0")/.." && pwd)
TMP=$(mktemp -d /tmp/wbd-openwrt-fullstack.XXXXXX)
TICKET_DIR="$TMP/tickets"
mkdir -m 700 "$TICKET_DIR"

for f in wbd-faketcp wbd-faketcp-mux wbd-link-proxy wbd-link-server-mux wbd-platform-proxy-openwrt wbd-platform-proxy-server wbd_dtls_shim; do
    [[ -x "$ASSET_DIR/$f" ]] || { echo "missing executable $ASSET_DIR/$f" >&2; exit 2; }
done
for f in front.pem front.key dtls.pem dtls.key; do
    [[ -f "$ASSET_DIR/$f" ]] || { echo "missing asset $ASSET_DIR/$f" >&2; exit 2; }
done

C=wbdosc$$
R=wbdosr$$
S=wbdoss$$
T=wbdost$$
PIDS=()
PLATFORM_CLIENT_PID=

cleanup() {
    set +e
    if ip netns list | grep -q "^$R\b"; then
        ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup --port 12345 --underlay4 10.78.0.1 >/dev/null 2>&1 || true
    fi
    for pid in "${PIDS[@]:-}"; do
        [[ -z "$pid" ]] || kill -TERM "$pid" 2>/dev/null || true
    done
    sleep .2
    for ns in "$C" "$R" "$S" "$T"; do
        ip netns pids "$ns" 2>/dev/null | xargs -r kill -TERM 2>/dev/null || true
        ip netns del "$ns" 2>/dev/null || true
    done
    rm -rf "$TMP"
}
trap cleanup EXIT INT TERM

wait_log() {
    local file=$1 pattern=$2 tries=${3:-400}
    for _ in $(seq 1 "$tries"); do
        grep -qE "$pattern" "$file" 2>/dev/null && return 0
        sleep .05
    done
    echo "timeout waiting for $pattern in $file" >&2
    cat "$file" >&2 || true
    return 1
}

validate_tunnel_json() {
    python3 - "$1" <<'PY'
import ipaddress,json,re,sys
p=sys.argv[1]
with open(p,'r',encoding='utf-8') as f: x=json.load(f)
assert re.fullmatch(r'[0-9a-f]{32}', x['tunnel_id']), x
iface=ipaddress.ip_interface(x['address4'])
assert iface.version == 4 and iface.network.prefixlen == 32, x
assert isinstance(x['routes4'], list) and x['routes4'], x
for r in x['routes4']:
    assert ipaddress.ip_network(r, strict=False).version == 4
print('WBD_OPENWRT_TUNNEL_CONFIG_PASS', x['tunnel_id'], x['address4'], ','.join(x['routes4']), flush=True)
PY
}

for tool in ip nft iptables python3 dig; do
    command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 2; }
done

for ns in "$C" "$R" "$S" "$T"; do ip netns add "$ns"; done
for ns in "$C" "$R" "$S" "$T"; do ip -n "$ns" link set lo up; done

ip link add oc$$ type veth peer name or0$$
ip link set oc$$ netns "$C"
ip link set or0$$ netns "$R"
ip link add or1$$ type veth peer name os0$$
ip link set or1$$ netns "$R"
ip link set os0$$ netns "$S"
ip link add os1$$ type veth peer name ot$$
ip link set os1$$ netns "$S"
ip link set ot$$ netns "$T"

ip -n "$C" addr add 10.10.0.2/24 dev oc$$
ip -n "$C" link set oc$$ up
ip -n "$C" route add default via 10.10.0.1

ip -n "$R" addr add 10.10.0.1/24 dev or0$$
ip -n "$R" link set or0$$ up
ip -n "$R" addr add 10.78.0.2/24 dev or1$$
ip -n "$R" link set or1$$ up
ip -n "$R" route add 10.90.0.0/24 via 10.78.0.1
ip netns exec "$R" sysctl -qw net.ipv4.ip_forward=1

ip -n "$S" addr add 10.78.0.1/24 dev os0$$
ip -n "$S" link set os0$$ up
ip -n "$S" addr add 10.90.0.1/24 dev os1$$
ip -n "$S" link set os1$$ up
ip -n "$S" route add 10.10.0.0/24 via 10.78.0.2
ip netns exec "$S" sysctl -qw net.ipv4.ip_forward=1

ip -n "$T" addr add 10.90.0.2/24 dev ot$$
ip -n "$T" link set ot$$ up
ip -n "$T" route add 10.10.0.0/24 via 10.90.0.1

# Raw FakeTCP owns its TCP-shaped packets, so suppress kernel RST generation in
# the two namespaces participating in the one public raw association.
ip netns exec "$R" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP

cat >"$TMP/target_services.py" <<'PY'
import selectors,socket,struct,threading,time
ADDR="10.90.0.2"
REQUIRED="10.90.0.1"

def dns_server():
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((ADDR,53))
    while True:
        data,peer=s.recvfrom(4096)
        if peer[0] != REQUIRED or len(data) < 12 or int.from_bytes(data[4:6],"big") != 1:
            continue
        i=12
        try:
            while data[i] != 0:
                i += 1 + data[i]
            qend=i+5
            question=data[12:qend]
            qtype=int.from_bytes(data[i+1:i+3],"big")
            qclass=int.from_bytes(data[i+3:i+5],"big")
        except (IndexError,ValueError):
            continue
        answer=b""
        ancount=0
        if qtype == 1 and qclass == 1:
            answer=b"\xc0\x0c"+struct.pack("!HHIH",1,1,30,4)+socket.inet_aton("192.0.2.123")
            ancount=1
        header=data[:2]+struct.pack("!HHHHH",0x8180,1,ancount,0,0)
        s.sendto(header+question+answer,peer)

def udp_fullcone():
    sel=selectors.DefaultSelector()
    socks={}
    for port,tag in [(5353,b"A"),(5354,b"B"),(5454,b"CONTROL")]:
        s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((ADDR,port)); s.setblocking(False)
        sel.register(s,selectors.EVENT_READ,(port,tag)); socks[port]=s
    injector=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); injector.bind((ADDR,7777))
    while True:
        for key,_ in sel.select(timeout=1):
            s=key.fileobj; port,tag=key.data
            data,peer=s.recvfrom(65535)
            if peer[0] != REQUIRED:
                continue
            if port == 5454:
                try: mapped=int(data.decode())
                except ValueError: continue
                s.sendto(b"CONTROL_OK",peer)
                time.sleep(.02)
                injector.sendto(b"UNSOLICITED",("10.90.0.1",mapped))
                continue
            reply=tag+b":"+data+b":SEEN="+str(peer[1]).encode()
            s.sendto(reply,peer)

def tcp_server(port,tag,require_proxy):
    s=socket.socket(socket.AF_INET,socket.SOCK_STREAM); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
    s.bind((ADDR,port)); s.listen(32)
    while True:
        c,peer=s.accept()
        try:
            if require_proxy and peer[0] != REQUIRED:
                c.close(); continue
            data=b""
            while True:
                part=c.recv(4096)
                if not part: break
                data += part
                if len(data) > 65536: break
            c.sendall(tag+b":"+data)
            try: c.shutdown(socket.SHUT_WR)
            except OSError: pass
        finally:
            c.close()

for fn,args in [
    (dns_server,()),(udp_fullcone,()),
    (tcp_server,(8080,b"PROXIED",True)),
    (tcp_server,(8082,b"RESTORED",False)),
]:
    threading.Thread(target=fn,args=args,daemon=True).start()
while True: time.sleep(3600)
PY

cat >"$TMP/underlay.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(("10.78.0.1",5555))
while True:
    data,peer=s.recvfrom(65535)
    s.sendto(b"UNDERLAY:"+data+b":SEEN="+peer[0].encode(),peer)
PY

ip netns exec "$T" python3 -u "$TMP/target_services.py" >"$LOG_DIR/target.log" 2>&1 & TARGET_PID=$!; PIDS+=("$TARGET_PID")
ip netns exec "$S" python3 -u "$TMP/underlay.py" >"$LOG_DIR/underlay.log" 2>&1 & UNDERLAY_PID=$!; PIDS+=("$UNDERLAY_PID")
sleep .1

# Server application boundary: platform server is the service consumed by the
# Logical-Tunnel-aware LINK mux.
ip netns exec "$S" "$ASSET_DIR/wbd-platform-proxy-server" -listen 127.0.0.1:49000 -udp-idle 30s -tcp-idle 30s >"$LOG_DIR/platform-server.log" 2>&1 & PLATFORM_SERVER_PID=$!; PIDS+=("$PLATFORM_SERVER_PID")
wait_log "$LOG_DIR/platform-server.log" 'WBD_PLATFORM_PROXY_SERVER_READY.*udp_fullcone=1.*tcp=1'

ROUTE_KEY='WBD_REALITY_ROUTE_KEY_0123456789abcdef'
USERNAME='solo'
PASSWORD='shared-password'
SERVER_NAME='target.example'
RAW=40000
LINK=47000
INSTALLATION_ID='00112233445566778899aabbccddeeff'
TICKET_FILE="$TMP/ticket.txt"
TUNNEL_FILE="$TMP/tunnel.json"

tip="127.0.0.1:${LINK}"
ip netns exec "$S" "$ASSET_DIR/wbd-link-server-mux" \
    -listen "$tip" -service 127.0.0.1:49000 \
    -ticket-dir "$TICKET_DIR" -ticket-ttl 60s -max-sessions 4 \
    >"$LOG_DIR/link-server.log" 2>&1 & LINK_PID=$!; PIDS+=("$LINK_PID")
wait_log "$LOG_DIR/link-server.log" 'WBD_LINK_SERVER_MUX_READY.*logical_tunnel=1'

# ADR-0014: one public FakeTCP association owns the flow from SYN onward. Real
# TLS 1.3 / Reality-like admission is the bounded first phase of this same raw
# association; there is no preliminary ordinary-TCP Reality connection.
ip netns exec "$S" "$ASSET_DIR/wbd-faketcp-mux" server \
    --listen 10.78.0.1:${RAW} --dtls-shim "$ASSET_DIR/wbd_dtls_shim" \
    --link-target "$tip" --cert "$ASSET_DIR/dtls.pem" --key "$ASSET_DIR/dtls.key" \
    --front-cert "$ASSET_DIR/front.pem" --front-key "$ASSET_DIR/front.key" \
    --server-name "$SERVER_NAME" --route-key "$ROUTE_KEY" \
    --username "$USERNAME" --password "$PASSWORD" --ticket-dir "$TICKET_DIR" \
    --bootstrap-timeout 12s --fallback-target 127.0.0.1:9 --max-sessions 4 \
    >"$LOG_DIR/faketcp-mux.log" 2>&1 & MUX_PID=$!; PIDS+=("$MUX_PID")
wait_log "$LOG_DIR/faketcp-mux.log" 'READY role=server-mux.*single_flow_bootstrap=true.*logical_tunnel=true' 500

ip netns exec "$R" "$ASSET_DIR/wbd-faketcp" client \
    --local-udp 127.0.0.1:45101 --source 10.78.0.2:41001 --remote 10.78.0.1:${RAW} \
    --shadow-recovery legacy \
    --reality-server-name "$SERVER_NAME" --reality-route-key "$ROUTE_KEY" \
    --reality-username "$USERNAME" --reality-password "$PASSWORD" \
    --reality-ticket-out "$TICKET_FILE" --reality-installation-id "$INSTALLATION_ID" \
    --reality-tunnel-config-out "$TUNNEL_FILE" --reality-verify-server=false \
    --reality-timeout 12s >"$LOG_DIR/faketcp-client.log" 2>&1 & FAKETCP_PID=$!; PIDS+=("$FAKETCP_PID")
wait_log "$LOG_DIR/faketcp-client.log" 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' 900
wait_log "$LOG_DIR/faketcp-client.log" 'READY role=client.*recovery=legacy.*single_flow_bootstrap=true' 200
wait_log "$LOG_DIR/faketcp-mux.log" 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' 200

TICKET=$(tr -d '\r\n' <"$TICKET_FILE")
test ${#TICKET} -eq 64
case "$TICKET" in *[!0-9a-fA-F]*) echo 'single-flow ticket is not hex' >&2; exit 1;; esac
validate_tunnel_json "$TUNNEL_FILE"
TUNNEL_PREFIX=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1],encoding="utf-8"))["tunnel_id"][:8])' "$TUNNEL_FILE")
echo 'WBD_OPENWRT_SINGLE_FLOW_BOOTSTRAP_PASS same_flow=1 logical_tunnel=1'

# The bootstrap barrier keeps the same FakeTCP process/4-tuple alive; pinned
# wolfSSL DTLS starts on its existing local UDP endpoint, so sustained payload
# never falls back to an ordinary kernel-TCP stream/HOL path.
ip netns exec "$R" "$ASSET_DIR/wbd_dtls_shim" client 46101 127.0.0.1 45101 none none \
    >"$LOG_DIR/dtls-client.log" 2>&1 & DTLS_PID=$!; PIDS+=("$DTLS_PID")
wait_log "$LOG_DIR/dtls-client.log" 'READY role=client version=DTLSv1.3.*verify=none' 900
wait_log "$LOG_DIR/faketcp-mux.log" 'BOUND role=server.*inherited=yes' 900
wait_log "$LOG_DIR/faketcp-mux.log" 'WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3' 200

ip netns exec "$R" "$ASSET_DIR/wbd-link-proxy" \
    -mode client -listen 127.0.0.1:47101 -dtls 127.0.0.1:46101 -fec off \
    -demo-reality-ticket "$TICKET" >"$LOG_DIR/link-client.log" 2>&1 & LINK_CLIENT_PID=$!; PIDS+=("$LINK_CLIENT_PID")
wait_log "$LOG_DIR/link-client.log" 'WBD_LINK_READY role=client' 900
wait_log "$LOG_DIR/link-server.log" "WBD_LINK_MUX_SESSION_READY tunnel_id_prefix=${TUNNEL_PREFIX}.*lanes=1" 900
test "$(find "$TICKET_DIR" -type f | wc -l)" -eq 0

ip netns exec "$R" "$ASSET_DIR/wbd-platform-proxy-openwrt" \
    -port 12345 -wbd 127.0.0.1:47101 -udp-idle 30s -tcp-idle 30s -ipv6=false \
    >"$LOG_DIR/platform-client.log" 2>&1 & PLATFORM_CLIENT_PID=$!; PIDS+=("$PLATFORM_CLIENT_PID")
wait_log "$LOG_DIR/platform-client.log" 'WBD_PLATFORM_PROXY_OPENWRT_READY.*udp_fullcone=1.*tcp=1'

# Only now install capture. 10.78.0.1 carries the one WBD public FakeTCP
# association and must be exempt before any target traffic is redirected.
ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" apply \
    --mode global --port 12345 --underlay4 10.78.0.1

# Real DNS protocol through TPROXY -> platform -> frozen WBD -> platform server.
DNS_OUT=$(ip netns exec "$C" dig +short +time=2 +tries=2 +noedns @10.90.0.2 example.test A | tr -d '\r')
test "$DNS_OUT" = '192.0.2.123'
echo 'WBD_OPENWRT_FULLSTACK_DNS_PASS'

# Full-cone through the real WBD association: one internal socket uses two
# destinations but must keep one server-side mapping port (EIM). A separate
# control mapping asks a never-contacted endpoint 10.90.0.2:7777 to send to the
# first mapping; that packet must arrive on the original internal socket (EIF).
ip netns exec "$C" python3 - <<'PY'
import socket

def seen(data,prefix):
    assert data.startswith(prefix),(data,prefix)
    return int(data.rsplit(b":SEEN=",1)[1])

s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(("10.10.0.2",0)); s.settimeout(6)
s.sendto(b"cone-a",("10.90.0.2",5353))
data,peer=s.recvfrom(65535)
assert peer == ("10.90.0.2",5353),peer
pa=seen(data,b"A:cone-a")
s.sendto(b"cone-b",("10.90.0.2",5354))
data,peer=s.recvfrom(65535)
assert peer == ("10.90.0.2",5354),peer
pb=seen(data,b"B:cone-b")
assert pa == pb and pa != 0,(pa,pb)
print(f"WBD_OPENWRT_FULLSTACK_FULLCONE_EIM_PASS mapped_port={pa}",flush=True)
ctrl=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
ctrl.bind(("10.10.0.2",0)); ctrl.settimeout(4)
ctrl.sendto(str(pa).encode(),("10.90.0.2",5454))
ack,peer=ctrl.recvfrom(65535)
assert peer == ("10.90.0.2",5454) and ack == b"CONTROL_OK",(peer,ack)
data,peer=s.recvfrom(65535)
assert peer == ("10.90.0.2",7777),peer
assert data == b"UNSOLICITED",data
print("WBD_OPENWRT_FULLSTACK_FULLCONE_EIF_PASS",flush=True)
PY

echo 'WBD_OPENWRT_FULLSTACK_UDP_PASS'

# Target port 8080 refuses the direct client source 10.10.0.2. Success therefore
# proves the request was intercepted and re-originated by the platform server in
# the server namespace before reaching 10.90.0.2.
ip netns exec "$C" python3 - <<'PY'
import socket
s=socket.create_connection(("10.90.0.2",8080),timeout=5)
assert s.getpeername() == ("10.90.0.2",8080)
s.sendall(b"tcp-through-frozen-wbd")
s.shutdown(socket.SHUT_WR); s.settimeout(8)
out=b""
while True:
    p=s.recv(4096)
    if not p: break
    out += p
assert out == b"PROXIED:tcp-through-frozen-wbd",out
print("WBD_OPENWRT_FULLSTACK_TCP_PASS",flush=True)
PY

# The configured underlay address must bypass capture. The underlay echo reports
# its observed source; direct routing preserves 10.10.0.2, whereas accidental
# platform relay would re-originate from the server namespace.
ip netns exec "$C" python3 - <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.settimeout(4)
s.sendto(b"escape",("10.78.0.1",5555))
data,peer=s.recvfrom(65535)
assert peer == ("10.78.0.1",5555),peer
assert data == b"UNDERLAY:escape:SEEN=10.10.0.2",data
print("WBD_OPENWRT_FULLSTACK_UNDERLAY_ESCAPE_PASS",flush=True)
PY

kill -0 "$MUX_PID"
kill -0 "$FAKETCP_PID"
kill -0 "$DTLS_PID"
kill -0 "$LINK_PID"
kill -0 "$LINK_CLIENT_PID"

# Stop the transparent adapter, remove only WBD-owned nft/policy state, and prove
# ordinary routed TCP returns without any platform process serving the flow.
kill -TERM "$PLATFORM_CLIENT_PID"
wait "$PLATFORM_CLIENT_PID" 2>/dev/null || true
PLATFORM_CLIENT_PID=
ip netns exec "$R" sh "$ROOT/scripts/openwrt_tproxy.sh" cleanup --port 12345 --underlay4 10.78.0.1
if ip netns exec "$R" nft list table inet wbd >/dev/null 2>&1; then
    echo 'WBD nft table survived cleanup' >&2; exit 1
fi
if ip netns exec "$R" ip rule show | grep -q '^1066:'; then
    echo 'WBD policy rule survived cleanup' >&2; exit 1
fi
ip netns exec "$C" python3 - <<'PY'
import socket
s=socket.create_connection(("10.90.0.2",8082),timeout=4)
s.sendall(b"plain-after-cleanup"); s.shutdown(socket.SHUT_WR)
out=b""
while True:
    p=s.recv(4096)
    if not p: break
    out += p
assert out == b"RESTORED:plain-after-cleanup",out
print("WBD_OPENWRT_FULLSTACK_CLEANUP_PASS",flush=True)
PY

echo 'WBD_OPENWRT_FULLSTACK_ONE_SHOT_PASS fec=off single_flow=1 logical_tunnel=1'
