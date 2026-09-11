#!/usr/bin/env bash
set -euo pipefail

BASE="${GITHUB_WORKSPACE:-$(pwd)}/scripts/game_lane_fullstack.sh"
OUT="${RUNNER_TEMP:-/tmp}/game_lane_dynamic_mtu_smoke_generated.sh"

python3 - "$BASE" "$OUT" <<'PY_PATCH'
import pathlib, sys
src = pathlib.Path(sys.argv[1]).read_text()
marker = 'cat >"$LOG_DIR/probe.py" <<\'PY\'\n'
pos = src.find(marker)
if pos < 0:
    raise SystemExit('dynamic MTU smoke: fullstack tail marker not found')
prefix = src[:pos]

# Weak-network admission needs the same generous bootstrap horizon used by the
# long qualification. This changes only test timing, never product liveness.
prefix = prefix.replace('--bootstrap-timeout 12s', '--bootstrap-timeout 30s')
prefix = prefix.replace('--reality-timeout 12s', '--reality-timeout 30s')

# Keep normal probe accounting small while allowing the 120 s load to echo
# without writing every payload as hex to echo-count.log.
old_echo = "    if not (b.startswith(b'warm') or b.startswith(b'game')):\n        continue\n    with count.open('ab') as f: f.write(b.hex().encode()+b'\\n')\n    s.sendto(b,a)\n"
new_echo = "    if b.startswith(b'WBD1'):\n        s.sendto(b,a)\n        continue\n    if not (b.startswith(b'warm') or b.startswith(b'game')):\n        continue\n    with count.open('ab') as f: f.write(b.hex().encode()+b'\\n')\n    s.sendto(b,a)\n"
if old_echo not in prefix:
    raise SystemExit('dynamic MTU smoke: echo guard marker not found')
prefix = prefix.replace(old_echo, new_echo, 1)

# Apply impairment on both public-veth egress directions before any
# Reality/FakeTCP bootstrap. Loopback control/service traffic stays clean.
anchor = 'sudo ip netns exec "$S" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP\n'
netem = anchor + r'''NETEM_DELAY_MS=${NETEM_DELAY_MS:-300}
NETEM_LOSS_PCT=${NETEM_LOSS_PCT:-20}
sudo ip netns exec "$C" tc qdisc replace dev gc0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
sudo ip netns exec "$S" tc qdisc replace dev gs0 root netem delay "${NETEM_DELAY_MS}ms" loss random "${NETEM_LOSS_PCT}%"
{
  echo "WBD_DYNAMIC_MTU_NETEM_READY delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT} direction=bidirectional"
  sudo ip netns exec "$C" tc qdisc show dev gc0
  sudo ip netns exec "$S" tc qdisc show dev gs0
} | tee "$LOG_DIR/netem.log"
'''
if anchor not in prefix:
    raise SystemExit('dynamic MTU smoke: netem anchor not found')
prefix = prefix.replace(anchor, netem, 1)

tail = r'''DURATION_SEC=${DURATION_SEC:-120}
RATE_BPS=${RATE_BPS:-10000000}
SERVER_INNER_MTU=${SERVER_INNER_MTU:-1360}
USER_INNER_MTU=${USER_INNER_MTU:?USER_INNER_MTU is required}
EXPECTED_LINK_MTU=$((USER_INNER_MTU + 40))
MAX_GAME_PAYLOAD=$((EXPECTED_LINK_MTU - 32))

[[ "$DURATION_SEC" == 120 ]] || { echo "smoke duration must be exactly 120 seconds" >&2; exit 2; }
[[ "$RATE_BPS" == 10000000 ]] || { echo "smoke rate must be exactly 10000000 bps" >&2; exit 2; }
[[ "$LINK_PLAINTEXT_MTU" == "$EXPECTED_LINK_MTU" ]] || {
  echo "LINK_PLAINTEXT_MTU=$LINK_PLAINTEXT_MTU want USER_INNER_MTU+40=$EXPECTED_LINK_MTU" >&2
  exit 2
}
[[ "$INNER_MTU" == "$SERVER_INNER_MTU" ]] || {
  echo "server INNER_MTU=$INNER_MTU want SERVER_INNER_MTU=$SERVER_INNER_MTU" >&2
  exit 2
}

# The shared server deliberately stays at its 1360 compatibility/default value.
# A non-default client must still establish LINK with its own C+40 proposal.
for i in $(seq 1 "$LANES"); do
  grep -q "WBD_LINK_READY role=client fec=${FEC} mtu=${EXPECTED_LINK_MTU}" "$LOG_DIR/link-${i}.log"
done
test "$(grep -c "WBD_LINK_MUX_SESSION_READY.*mtu=${EXPECTED_LINK_MTU}" "$LOG_DIR/link-server.log" || true)" -eq "$LANES"

echo "WBD_DYNAMIC_MTU_NEGOTIATION_PASS user_inner_mtu=${USER_INNER_MTU} link_plaintext_mtu=${EXPECTED_LINK_MTU} server_default_inner_mtu=${SERVER_INNER_MTU} lanes=${LANES} fec=${FEC}"

cat >"$LOG_DIR/smoke.py" <<'PY_LOAD'
import json, select, socket, struct, sys, time, zlib
out_path=sys.argv[1]
duration=float(sys.argv[2]); rate_bps=int(sys.argv[3]); max_payload=int(sys.argv[4])
if max_payload < 64:
    raise SystemExit('max payload too small')
# Deterministic mixed-size distribution: 35/25/20/15/5 percent. The largest
# item is exactly LINK-GameHeader, i.e. user inner C + WBDP(8) in this harness.
sizes=[128,256,512,1000,max_payload]
weights=[35,25,20,15,5]
if max_payload < 1000:
    sizes=[max(64,min(x,max_payload)) for x in sizes]
cycle=[]
for size,w in zip(sizes,weights): cycle.extend([size]*w)

s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47601)); s.setblocking(False)
start=time.monotonic(); deadline=start+duration
sent=0; sent_bytes=0; unique=set(); dup=0; bad=0; last_rx=start

def make_packet(seq,size,elapsed):
    # 4 magic + 8 seq + 8 send timestamp + 2 total length + 4 CRC = 26-byte header.
    hdr=bytearray(b'WBD1'+struct.pack('!QdH',seq,elapsed,size))
    body_len=size-(len(hdr)+4)
    if body_len < 0: raise RuntimeError((size,len(hdr)))
    body=bytes(((seq*31+i*17)&0xff) for i in range(body_len))
    pre=bytes(hdr)+body
    return pre+struct.pack('!I',zlib.crc32(pre)&0xffffffff)

def check_packet(data):
    if len(data)<26 or data[:4]!=b'WBD1': return None
    seq,stamp,size=struct.unpack('!QdH',data[4:22])
    if size!=len(data): return None
    want=struct.unpack('!I',data[-4:])[0]
    if zlib.crc32(data[:-4])&0xffffffff != want: return None
    return seq

# One exact maximum-size preflight while tcpdump is still active. It proves the
# C-dependent LINK ceiling is usable before the long packet capture is stopped.
pre=make_packet((1<<63),max_payload,0.0)
s.sendto(pre,('127.0.0.1',47500))
pre_deadline=time.monotonic()+70.0
while True:
    left=pre_deadline-time.monotonic()
    if left<=0: raise RuntimeError('maximum-size preflight timed out')
    r,_,_=select.select([s],[],[],min(.1,left))
    if not r: continue
    data,_=s.recvfrom(65535)
    if data==pre: break
print(f'WBD_DYNAMIC_MTU_MAX_PAYLOAD_PASS bytes={max_payload}')

# Tell the wrapper that the bounded pcap can now be stopped.
open(out_path+'.preflight','w').write('ok\n')

next_tx=time.monotonic()
while True:
    now=time.monotonic()
    while now >= next_tx and next_tx < deadline:
        size=cycle[sent % len(cycle)]
        p=make_packet(sent,size,next_tx-start)
        s.sendto(p,('127.0.0.1',47500))
        sent+=1; sent_bytes+=len(p)
        next_tx=start+(sent_bytes*8.0/rate_bps)
        now=time.monotonic()
    r,_,_=select.select([s],[],[],.002)
    if r:
        while True:
            try: data,_=s.recvfrom(65535)
            except BlockingIOError: break
            seq=check_packet(data)
            if seq is None:
                bad+=1; continue
            if seq >= (1<<63):
                continue
            if seq in unique: dup+=1
            else: unique.add(seq)
            last_rx=time.monotonic()
    now=time.monotonic()
    if now >= deadline and (len(unique)==sent or now >= deadline+65.0): break

elapsed=max(duration,1e-9)
loss=sent-len(unique)
loss_pct=(100.0*loss/sent) if sent else 100.0
goodput_bps=sum(cycle[i % len(cycle)] for i in unique)*8.0/elapsed if unique else 0.0
result={
    'requested_duration_sec':duration,
    'requested_bps_each_direction':rate_bps,
    'max_payload':max_payload,
    'sent':sent,
    'received_unique':len(unique),
    'loss':loss,
    'loss_pct':loss_pct,
    'duplicates':dup,
    'bad_payload':bad,
    'goodput_bps':goodput_bps,
}
open(out_path,'w').write(json.dumps(result,sort_keys=True)+'\n')
print('WBD_DYNAMIC_MTU_LOAD_RESULT '+json.dumps(result,sort_keys=True))
if sent == 0 or bad != 0 or dup != 0 or loss_pct > 0.1:
    raise SystemExit(1)
PY_LOAD

# Run load generator in background so the wrapper can stop the packet capture as
# soon as the one exact maximum-size preflight has crossed the full stack.
sudo ip netns exec "$C" python3 "$LOG_DIR/smoke.py" "$LOG_DIR/load-result.json" \
  "$DURATION_SEC" "$RATE_BPS" "$MAX_GAME_PAYLOAD" >"$LOG_DIR/load.log" 2>&1 &
LOADPID=$!
PIDS+=("$LOADPID")
for _ in $(seq 1 1400); do
  [[ -f "$LOG_DIR/load-result.json.preflight" ]] && break
  kill -0 "$LOADPID" 2>/dev/null || { cat "$LOG_DIR/load.log" >&2; exit 1; }
  sleep .05
done
[[ -f "$LOG_DIR/load-result.json.preflight" ]]

sudo kill -INT "$TPID" 2>/dev/null || true
wait "$TPID" 2>/dev/null || true
TPID=

# Parse the bounded Ethernet pcap directly and assert the real public IPv4 total
# length never exceeds the underlay MTU. Also require at least one substantial
# data packet, so an empty/handshake-only capture cannot pass this check.
python3 - "$LOG_DIR/game-lanes.pcap" "$RAW" <<'PY_PCAP'
import struct,sys
p=sys.argv[1]; server=int(sys.argv[2]); b=open(p,'rb').read()
fmts={b'\xd4\xc3\xb2\xa1':'<',b'\xa1\xb2\xc3\xd4':'>',b'\x4d\x3c\xb2\xa1':'<',b'\xa1\xb2\x3c\x4d':'>'}
e=fmts.get(b[:4]); assert e is not None
network=struct.unpack_from(e+'IHHIIII',b,0)[-1]; assert network==1,network
off=24; totals=[]; payloads=[]
while off+16<=len(b):
    _sec,_frac,incl,_orig=struct.unpack_from(e+'IIII',b,off); off+=16
    f=b[off:off+incl]; off+=incl
    if len(f)<54 or struct.unpack_from('!H',f,12)[0]!=0x0800: continue
    ip=f[14:]; ihl=(ip[0]&15)*4
    if ip[0]>>4!=4 or ip[9]!=6 or len(ip)<ihl+20: continue
    total=struct.unpack_from('!H',ip,2)[0]; tcp=ip[ihl:total]
    sp,dp=struct.unpack_from('!HH',tcp,0)
    if sp!=server and dp!=server: continue
    doff=(tcp[12]>>4)*4
    totals.append(total); payloads.append(max(0,len(tcp)-doff))
assert totals and max(payloads)>1000,(len(totals),max(payloads,default=0))
assert max(totals)<=1500,max(totals)
print(f'WBD_DYNAMIC_MTU_OUTER_MTU_PASS packets={len(totals)} max_outer_ipv4={max(totals)} max_faketcp_payload={max(payloads)}')
PY_PCAP

wait "$LOADPID"
PIDS=("${PIDS[@]/$LOADPID}")
cat "$LOG_DIR/load.log"
python3 - "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$MAX_GAME_PAYLOAD" <<'PY_ASSERT'
import json,sys
d=json.load(open(sys.argv[1]))
assert d['requested_duration_sec']==float(sys.argv[2]),d
assert d['requested_bps_each_direction']==int(sys.argv[3]),d
assert d['max_payload']==int(sys.argv[4]),d
assert d['bad_payload']==0 and d['duplicates']==0,d
assert d['loss_pct']<=0.1,d
print('WBD_DYNAMIC_MTU_SMOKE_PASS '+json.dumps(d,sort_keys=True))
PY_ASSERT

echo "WBD_DYNAMIC_MTU_2M_PASS user_inner_mtu=${USER_INNER_MTU} link_plaintext_mtu=${EXPECTED_LINK_MTU} server_default_inner_mtu=${SERVER_INNER_MTU} lanes=${LANES} fec=${FEC} duration_sec=${DURATION_SEC} bps_each_direction=${RATE_BPS} delay_ms=${NETEM_DELAY_MS} loss_pct=${NETEM_LOSS_PCT}"
'''
pathlib.Path(sys.argv[2]).write_text(prefix + tail)
PY_PATCH

chmod +x "$OUT"
bash -n "$OUT"
grep -Fq 'WBD_DYNAMIC_MTU_NEGOTIATION_PASS' "$OUT"
grep -Fq 'DURATION_SEC=${DURATION_SEC:-120}' "$OUT"
grep -Fq 'RATE_BPS=${RATE_BPS:-10000000}' "$OUT"
grep -Fq 'loss random "${NETEM_LOSS_PCT}%"' "$OUT"
grep -Fq 'assert max(totals)<=1500' "$OUT"
exec "$OUT" "$@"
