#!/usr/bin/env bash
set -euo pipefail

: "${WBD_E2E_DIR:=/tmp/wbd-singleflow-v3}"
DIR=$WBD_E2E_DIR
BIN=$DIR/bin
C=wbdv3c$$
R=wbdv3r$$
S=wbdv3s$$
PIDS=()

log() { printf '[singleflow-v3] %s\n' "$*"; }
cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  for p in "${PIDS[@]:-}"; do kill -KILL "$p" 2>/dev/null || true; done
  ip netns del "$C" 2>/dev/null || true
  ip netns del "$R" 2>/dev/null || true
  ip netns del "$S" 2>/dev/null || true
}
trap cleanup EXIT INT TERM

for f in wbd-faketcp wbd-faketcp-mux wbd_dtls_shim; do
  test -x "$BIN/$f" || { echo "missing $BIN/$f" >&2; exit 1; }
done
for f in front.pem front.key dtls.pem dtls.key ca.pem; do
  test -s "$DIR/$f" || { echo "missing $DIR/$f" >&2; exit 1; }
done

rm -f "$DIR"/*.log "$DIR"/*.pcap "$DIR"/ticket.hex "$DIR"/pcap.result
mkdir -p "$DIR/tickets"
chmod 700 "$DIR/tickets"

log 'create client -> NAT router -> server namespaces'
ip netns add "$C"
ip netns add "$R"
ip netns add "$S"
ip link add cr type veth peer name rc
ip link add rs type veth peer name sr
ip link set cr netns "$C"
ip link set rc netns "$R"
ip link set rs netns "$R"
ip link set sr netns "$S"
ip -n "$C" addr add 10.88.1.2/24 dev cr
ip -n "$R" addr add 10.88.1.1/24 dev rc
ip -n "$R" addr add 198.51.100.1/24 dev rs
ip -n "$S" addr add 198.51.100.2/24 dev sr
for ns in "$C" "$R" "$S"; do ip -n "$ns" link set lo up; done
ip -n "$C" link set cr up
ip -n "$R" link set rc up
ip -n "$R" link set rs up
ip -n "$S" link set sr up
ip -n "$C" route add default via 10.88.1.1
ip -n "$S" route add default via 198.51.100.1
ip netns exec "$R" sysctl -q -w net.ipv4.ip_forward=1
ip netns exec "$R" iptables -P FORWARD ACCEPT
ip netns exec "$R" iptables -t nat -A POSTROUTING -s 10.88.1.0/24 -o rs -j MASQUERADE

# Raw FakeTCP owns the public sequence space. Kernel-generated RSTs are not part
# of WBD and are suppressed exactly as in the existing raw-path qualification.
for ns in "$C" "$S"; do
  ip netns exec "$ns" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
done

cat >"$DIR/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47000))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY
ip netns exec "$S" python3 "$DIR/echo.py" >"$DIR/echo.log" 2>&1 &
PIDS+=("$!")

# Capture on the router's public side so assertions see the NAT-visible tuple.
ip netns exec "$R" tcpdump -U -i rs -s 0 -w "$DIR/public.pcap" 'tcp port 443' >"$DIR/tcpdump.log" 2>&1 &
TCPDUMP_PID=$!
PIDS+=("$TCPDUMP_PID")
sleep .2

ROUTE_KEY='v3-route-key-0123456789abcdef'
USER_NAME='v3-test-user'
PASSWORD='v3-test-password'

log 'start the sole public owner: raw mux with in-flow Reality-like TLS'
ip netns exec "$S" "$BIN/wbd-faketcp-mux" server \
  --listen 198.51.100.2:443 \
  --dtls-shim "$BIN/wbd_dtls_shim" \
  --link-target 127.0.0.1:47000 \
  --cert "$DIR/dtls.pem" --key "$DIR/dtls.key" \
  --max-sessions 8 \
  --front-server-name front.test \
  --front-cert "$DIR/front.pem" --front-key "$DIR/front.key" \
  --front-route-key "$ROUTE_KEY" \
  --username "$USER_NAME" --password "$PASSWORD" \
  --ticket-dir "$DIR/tickets" \
  >"$DIR/mux.log" 2>&1 &
MUX_PID=$!
PIDS+=("$MUX_PID")
for _ in $(seq 1 200); do
  grep -q 'READY role=server-mux.*single_flow=1.*reality_like=1' "$DIR/mux.log" && break
  sleep .05
done
grep -q 'READY role=server-mux.*single_flow=1.*reality_like=1' "$DIR/mux.log"

# A raw socket must be the only product owner. No kernel TCP LISTEN is allowed
# on the public port in the V3 composition.
if ip netns exec "$S" ss -ltnH 'sport = :443' | grep -q .; then
  echo 'SINGLEFLOW_E2E_FAIL kernel TCP listener exists on public :443' >&2
  ip netns exec "$S" ss -ltnp >&2 || true
  exit 1
fi
echo 'SINGLEFLOW_PUBLIC_OWNER_PASS owner=raw-mux kernel_listener=0' | tee "$DIR/owner.log"

log 'start one raw association; Reality-like TLS/auth/switch occurs inside it'
ip netns exec "$C" "$BIN/wbd-faketcp" client \
  --local-udp 127.0.0.1:45101 \
  --source 10.88.1.2:41001 \
  --remote 198.51.100.2:443 \
  --shadow-recovery legacy \
  --reality-server-name front.test \
  --reality-route-key "$ROUTE_KEY" \
  --username "$USER_NAME" --password "$PASSWORD" \
  --ticket-out "$DIR/ticket.hex" \
  --verify-server=false --bootstrap-timeout 12s \
  >"$DIR/faketcp.log" 2>&1 &
FAKE_PID=$!
PIDS+=("$FAKE_PID")
for _ in $(seq 1 400); do
  if grep -q 'READY role=client.*single_flow=1.*reality_like=1' "$DIR/faketcp.log" && \
     grep -q 'WBD_SINGLEFLOW_DATAGRAM_READY.*public_flow=reused.*hol=bootstrap-only' "$DIR/faketcp.log" && \
     grep -q 'WBD_SINGLEFLOW_DATAGRAM_READY.*public_flow=reused.*hol=bootstrap-only' "$DIR/mux.log"; then
    break
  fi
  sleep .05
done
grep -q 'WBD_SINGLEFLOW_TLS_SWITCH_REQUEST_SENT' "$DIR/faketcp.log"
grep -q 'WBD_SINGLEFLOW_TLS_SWITCH_ACK_RECEIVED' "$DIR/faketcp.log"
grep -q 'READY role=client.*single_flow=1.*reality_like=1' "$DIR/faketcp.log"
grep -q 'WBD_SINGLEFLOW_REALITY_AUTH_OK.*tls=1.3' "$DIR/mux.log"
grep -q 'WBD_SINGLEFLOW_TLS_SWITCH_REQUEST_RECEIVED' "$DIR/mux.log"
grep -q 'WBD_SINGLEFLOW_TLS_SWITCH_ACK_SENT' "$DIR/mux.log"
grep -q 'WBD_SINGLEFLOW_DATAGRAM_READY.*public_flow=reused.*hol=bootstrap-only' "$DIR/mux.log"
test -s "$DIR/ticket.hex"

log 'start pinned wolfSSL DTLS only after the encrypted in-flow switch barrier'
ip netns exec "$C" "$BIN/wbd_dtls_shim" client 46101 127.0.0.1 45101 "$DIR/ca.pem" wbd.test \
  >"$DIR/dtls.log" 2>&1 &
DTLS_PID=$!
PIDS+=("$DTLS_PID")
for _ in $(seq 1 800); do
  if grep -q 'READY role=client version=DTLSv1.3.*verify=peer-hostname' "$DIR/dtls.log" && \
     grep -q 'READY role=server version=DTLSv1.3' "$DIR/mux.log"; then
    break
  fi
  sleep .05
done
grep -q 'WBD_DTLS_CLIENT_CONNECT_START' "$DIR/dtls.log"
grep -q 'READY role=client version=DTLSv1.3.*verify=peer-hostname' "$DIR/dtls.log"
grep -q 'WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3' "$DIR/mux.log"
grep -q 'READY role=server version=DTLSv1.3' "$DIR/mux.log"

cat >"$DIR/baseline_probe.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(2)
msg=b'V3_BASELINE_ECHO'
s.sendto(msg,('127.0.0.1',46101))
b,_=s.recvfrom(65535)
assert b==msg,(b,msg)
print('SINGLEFLOW_BASELINE_ECHO_PASS')
PY
ip netns exec "$C" python3 "$DIR/baseline_probe.py" | tee "$DIR/baseline.log"

log 'prove steady-state no-HOL: drop the first data PSH, require the later datagram first'
ip netns exec "$R" iptables -N WBD_NOHOL
ip netns exec "$R" iptables -A WBD_NOHOL \
  -s 10.88.1.2 -d 198.51.100.2 -p tcp --dport 443 --tcp-flags PSH PSH \
  -m statistic --mode nth --every 1000000 --packet 0 -j DROP
ip netns exec "$R" iptables -I FORWARD 1 -j WBD_NOHOL
cat >"$DIR/nohol_probe.py" <<'PY'
import socket,time
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0))
first=b'V3_EARLIER_DROPPED'
later=b'V3_LATER_MUST_BYPASS'
t0=time.monotonic()
s.sendto(first,('127.0.0.1',46101))
time.sleep(0.02)
s.sendto(later,('127.0.0.1',46101))
s.settimeout(0.75)
got,_=s.recvfrom(65535)
elapsed=time.monotonic()-t0
if got != later:
    raise SystemExit('HOL_FAIL first-return=%r elapsed=%.3f'%(got,elapsed))
if elapsed >= 0.75:
    raise SystemExit('HOL_FAIL later delivery too slow %.3f'%elapsed)
print('SINGLEFLOW_NO_HOL_BYPASS_PASS elapsed_ms=%d'%(elapsed*1000))
# The earlier datagram should still recover later through shadow retransmission;
# this proves the bypass did not simply abandon reliability forever.
s.settimeout(3.0)
end=time.monotonic()+3.0
while time.monotonic()<end:
    try:
        b,_=s.recvfrom(65535)
    except socket.timeout:
        break
    if b == first:
        print('SINGLEFLOW_EARLIER_RECOVERY_PASS')
        break
else:
    raise SystemExit('unreachable')
if 'b' not in locals() or b != first:
    raise SystemExit('earlier datagram did not recover after bypass')
PY
ip netns exec "$C" python3 "$DIR/nohol_probe.py" | tee "$DIR/nohol.log"
ip netns exec "$R" iptables -nvxL WBD_NOHOL >"$DIR/nohol-iptables.log"
DROPS=$(awk '$3=="DROP" {print $1; exit}' "$DIR/nohol-iptables.log")
test "${DROPS:-0}" -eq 1

# Stop tcpdump cleanly before parsing the pcap. Do not tear down the WBD flow
# first; FIN/RST absence is part of the captured transition assertion.
kill -INT "$TCPDUMP_PID" 2>/dev/null || true
wait "$TCPDUMP_PID" 2>/dev/null || true
PIDS=("${PIDS[@]/$TCPDUMP_PID}")
test -s "$DIR/public.pcap"

cat >"$DIR/analyze_pcap.py" <<'PY'
import hashlib,ipaddress,struct,sys
pcap,ticket_path=sys.argv[1:3]
data=open(pcap,'rb').read()
if len(data)<24: raise SystemExit('pcap too short')
magic=data[:4]
if magic==b'\xd4\xc3\xb2\xa1': endian='<'
elif magic==b'\xa1\xb2\xc3\xd4': endian='>'
else: raise SystemExit('unsupported pcap magic '+magic.hex())
off=24; packets=[]
while off+16<=len(data):
    _,_,incl,_=struct.unpack_from(endian+'IIII',data,off); off+=16
    frame=data[off:off+incl]; off+=incl
    if len(frame)<14 or frame[12:14]!=b'\x08\x00': continue
    ip=frame[14:]
    if len(ip)<20 or ip[9]!=6: continue
    ihl=(ip[0]&15)*4
    if len(ip)<ihl+20: continue
    src=str(ipaddress.IPv4Address(ip[12:16])); dst=str(ipaddress.IPv4Address(ip[16:20]))
    tcp=ip[ihl:]
    sp,dp,seq,ack=struct.unpack_from('!HHII',tcp,0)
    thl=((tcp[12]>>4)&15)*4
    if len(tcp)<thl: continue
    flags=tcp[13]
    payload=tcp[thl:]
    if sp==443 or dp==443:
        packets.append((src,sp,dst,dp,seq,ack,flags,payload))
if not packets: raise SystemExit('no public :443 TCP-shaped packets captured')
client_syn=[p for p in packets if p[3]==443 and p[6]&0x02 and not p[6]&0x10]
if len(client_syn)!=1: raise SystemExit('expected exactly one client SYN, got %d'%len(client_syn))
cs=client_syn[0]; client=(cs[0],cs[1]); server=(cs[2],cs[3])
server_synack=[p for p in packets if (p[0],p[1])==server and (p[2],p[3])==client and p[6]&0x12==0x12]
if len(server_synack)!=1: raise SystemExit('expected exactly one server SYNACK, got %d'%len(server_synack))
for p in packets:
    endpoints={(p[0],p[1]),(p[2],p[3])}
    if endpoints!={client,server}: raise SystemExit('second public tuple observed: %r'%((p[0],p[1],p[2],p[3]),))
    if p[6]&0x01: raise SystemExit('FIN observed on V3 public flow')
    if p[6]&0x04: raise SystemExit('RST observed on V3 public flow')
cp=[p[7] for p in packets if (p[0],p[1])==client and p[7]]
sp=[p[7] for p in packets if (p[0],p[1])==server and p[7]]
if not any(len(p)>=6 and p[0]==22 and p[1]==3 and p[5]==1 for p in cp):
    raise SystemExit('real TLS ClientHello record not found on public flow')
ticket=bytes.fromhex(open(ticket_path).read().strip())
digest=hashlib.sha256(ticket).digest()[:16]
req=b'WBSF'+bytes([1,1])+digest
ack=b'WBSF'+bytes([1,2])+digest
for name,stream in [('client',b''.join(cp)),('server',b''.join(sp))]:
    if req in stream or ack in stream:
        raise SystemExit('plaintext switch frame leaked on public '+name+' payload')
print('SINGLEFLOW_PCAP_PASS client=%s:%d server=%s:%d packets=%d syn=1 synack=1 fin=0 rst=0 tls_clienthello=1 plaintext_switch=0'%(client[0],client[1],server[0],server[1],len(packets)))
PY
python3 "$DIR/analyze_pcap.py" "$DIR/public.pcap" "$DIR/ticket.hex" | tee "$DIR/pcap.result"

grep -q 'SINGLEFLOW_PCAP_PASS' "$DIR/pcap.result"
grep -q 'SINGLEFLOW_NO_HOL_BYPASS_PASS' "$DIR/nohol.log"
grep -q 'SINGLEFLOW_EARLIER_RECOVERY_PASS' "$DIR/nohol.log"
echo 'SINGLEFLOW_REALITYLIKE_E2E_PASS one_public_flow=1 reality_tls13=1 encrypted_switch=1 dtls13=1 steady_no_hol=1'
