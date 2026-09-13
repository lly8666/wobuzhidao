#!/usr/bin/env bash
set -euo pipefail

CASE=${1:?case required}
PRODUCT_DIR=${2:?product dir required}
ASSET_DIR=${3:?asset dir required}
OUT=${4:?output dir required}
REVIEW_DIR=$(cd "$(dirname "$0")/../.." && pwd)
ANALYZER="$REVIEW_DIR/scripts/analyze_mtu_boundary_pcap.py"
mkdir -p "$OUT"

wait_log() {
  local f=$1 pat=$2 loops=${3:-400}
  local i
  for i in $(seq 1 "$loops"); do
    if [[ -f "$f" ]] && grep -Eq "$pat" "$f"; then return 0; fi
    sleep .05
  done
  echo "timeout waiting for $pat in $f" >&2
  tail -n 120 "$f" 2>/dev/null || true
  return 1
}

if [[ "$CASE" == game-off || "$CASE" == game-fec ]]; then
  if [[ "$CASE" == game-off ]]; then
    FEC_MODE=off
    INNER=1388
    LINK=1428
  else
    FEC_MODE=20:20
    INNER=1332
    LINK=1372
  fi
  PATCHED="$OUT/game_lane_fullstack_mtu_boundary.sh"
  python3 - "$PRODUCT_DIR/scripts/game_lane_fullstack.sh" "$PATCHED" <<'PY'
from pathlib import Path
import sys
src,dst=map(Path,sys.argv[1:])
s=src.read_text()
def one(old,new):
    global s
    if s.count(old)!=1:
        raise SystemExit(f"patch marker drift count={s.count(old)} marker={old!r}")
    s=s.replace(old,new,1)
one('sudo ip -n "$C" link set gc0 up\n',
    'sudo ip -n "$C" link set gc0 mtu 1500 up\nsudo ip netns exec "$C" ethtool -K gc0 tso off gso off gro off >/dev/null 2>&1 || true\n')
one('sudo ip -n "$S" link set gs0 up\n',
    'sudo ip -n "$S" link set gs0 mtu 1500 up\nsudo ip netns exec "$S" ethtool -K gs0 tso off gso off gro off >/dev/null 2>&1 || true\n')
one('-fec "$FEC" -lanes 1 \\\n', '-fec "$FEC" -fec-flush-ms 32 -lanes 1 \\\n')
one('count=int(sys.argv[1]); prefix=sys.argv[2].encode()\n',
    'count=int(sys.argv[1]); prefix=sys.argv[2].encode(); target=int(sys.argv[3])\n')
one("    b=prefix+i.to_bytes(2,'big')\n",
    "    b=prefix+i.to_bytes(2,'big')\n    if target:\n        assert target >= len(b),(target,len(b))\n        b += b'X'*(target-len(b))\n")
one('sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" 1 warm >"$LOG_DIR/probe-warm.log" 2>&1\n',
    'sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" 1 warm 0 >"$LOG_DIR/probe-warm.log" 2>&1\n')
old='sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" "$PROBE_COUNT" game >"$LOG_DIR/probe.log" 2>&1\n'
new='''sudo ip netns exec "$C" tcpdump --immediate-mode -U -i gc0 -s 0 -w "$LOG_DIR/mtu-boundary.pcap" "tcp port ${RAW}" >"$LOG_DIR/mtu-tcpdump.log" 2>&1 &
MTPID=$!
PIDS+=("$MTPID")
for _ in $(seq 1 200); do grep -q 'listening on gc0' "$LOG_DIR/mtu-tcpdump.log" && break; sleep .05; done
grep -q 'listening on gc0' "$LOG_DIR/mtu-tcpdump.log"
sudo ip netns exec "$C" python3 "$LOG_DIR/probe.py" "$PROBE_COUNT" game "$PROBE_PAYLOAD_SIZE" >"$LOG_DIR/probe.log" 2>&1
sleep .25
sudo kill -INT "$MTPID" 2>/dev/null || true
wait "$MTPID" 2>/dev/null || true
'''
one(old,new)
dst.write_text(s)
PY
  chmod +x "$PATCHED"
  env \
    GITHUB_WORKSPACE="$PRODUCT_DIR" \
    LANES=1 FEC="$FEC_MODE" PROBE_COUNT=3 PROBE_PAYLOAD_SIZE="$INNER" \
    INNER_MTU="$INNER" LINK_PLAINTEXT_MTU="$LINK" WBD_DTLS_TRACE=1 \
    bash "$PATCHED" "$ASSET_DIR" "$OUT"
  grep -Eq 'WRITE role=client datagram=[0-9]+ bytes=1428' "$OUT/dtls-1.log"
  if grep -R -E 'encode_input_exceeds_link_mtu|WBD_LINK_PROXY_FAIL|packet too large' "$OUT"/*.log; then
    echo "unexpected MTU/path size failure" >&2
    exit 1
  fi
  python3 "$ANALYZER" "$OUT/mtu-boundary.pcap" \
    --protocol tcp --max-ip-total 1500 --max-transport-payload 1460 \
    --out "$OUT/mtu-boundary.json"
  printf '%s\n' "case=$CASE carrier_mtu=1500 fec=$FEC_MODE game=on inner=$INNER link=$LINK dtls_plaintext_boundary=1428 product_sha=66612d348fd52852ffe6c03fcbb52807a3a54883" \
    >"$OUT/provenance.txt"
  echo "WBD_MTU_WIRE_CASE_PASS case=$CASE inner=$INNER link=$LINK dtls_plaintext=1428 carrier=1500"
  exit 0
fi

case "$CASE" in
  dtls) APP=1428; LINK_MODE=none; FEC_MODE=off ;;
  link-off) APP=1428; LINK_MODE=link; FEC_MODE=off ;;
  link-fec) APP=1372; LINK_MODE=link; FEC_MODE=20:20 ;;
  *) echo "unknown case: $CASE" >&2; exit 2 ;;
esac

C="wbdmtuc$$"
S="wbdmtus$$"
PIDS=()
CAP_PID=
cleanup() {
  set +e
  if [[ -n "${CAP_PID:-}" ]]; then sudo kill -INT "$CAP_PID" 2>/dev/null || true; fi
  for p in "${PIDS[@]:-}"; do sudo kill -TERM "$p" 2>/dev/null || true; kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  sudo ip netns del "$C" 2>/dev/null || true
  sudo ip netns del "$S" 2>/dev/null || true
  sudo chmod -R a+rX "$OUT" 2>/dev/null || true
}
trap cleanup EXIT

sudo ip netns add "$C"
sudo ip netns add "$S"
sudo ip link add mc0 type veth peer name ms0
sudo ip link set mc0 netns "$C"
sudo ip link set ms0 netns "$S"
sudo ip -n "$C" addr add 10.77.0.2/24 dev mc0
sudo ip -n "$S" addr add 10.77.0.1/24 dev ms0
sudo ip -n "$C" link set lo up
sudo ip -n "$S" link set lo up
sudo ip -n "$C" link set mc0 mtu 1500 up
sudo ip -n "$S" link set ms0 mtu 1500 up
sudo ip netns exec "$C" ethtool -K mc0 tso off gso off gro off >/dev/null 2>&1 || true
sudo ip netns exec "$S" ethtool -K ms0 tso off gso off gro off >/dev/null 2>&1 || true

cat >"$OUT/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',48000))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY
sudo ip netns exec "$S" python3 "$OUT/echo.py" >"$OUT/echo.log" 2>&1 &
PIDS+=("$!")

if [[ "$LINK_MODE" == link ]]; then
  sudo ip netns exec "$S" "$ASSET_DIR/wbd-link-proxy" \
    -mode server -listen 127.0.0.1:47000 -service 127.0.0.1:48000 \
    >"$OUT/link-server.log" 2>&1 &
  PIDS+=("$!")
  DTLS_TARGET=47000
else
  DTLS_TARGET=48000
fi

sudo ip netns exec "$S" env WBD_DTLS_TRACE=1 "$ASSET_DIR/wbd_dtls_shim" \
  server 46000 127.0.0.1 "$DTLS_TARGET" "$ASSET_DIR/dtls.pem" "$ASSET_DIR/dtls.key" \
  >"$OUT/dtls-server.log" 2>&1 &
PIDS+=("$!")
sleep .10
sudo ip netns exec "$C" env WBD_DTLS_TRACE=1 "$ASSET_DIR/wbd_dtls_shim" \
  client 46100 10.77.0.1 46000 none none \
  >"$OUT/dtls-client.log" 2>&1 &
PIDS+=("$!")
wait_log "$OUT/dtls-client.log" 'READY role=client version=DTLSv1.3' 1000
wait_log "$OUT/dtls-server.log" 'READY role=server version=DTLSv1.3' 1000

if [[ "$LINK_MODE" == link ]]; then
  LINK_MTU=1428
  [[ "$FEC_MODE" == 20:20 ]] && LINK_MTU=1372
  sudo ip netns exec "$C" "$ASSET_DIR/wbd-link-proxy" \
    -mode client -listen 127.0.0.1:47100 -dtls 127.0.0.1:46100 \
    -fec "$FEC_MODE" -fec-flush-ms 32 -mtu "$LINK_MTU" -lanes 1 \
    >"$OUT/link-client.log" 2>&1 &
  PIDS+=("$!")
  wait_log "$OUT/link-client.log" 'WBD_LINK_READY role=client' 600
  wait_log "$OUT/link-server.log" 'WBD_LINK_READY role=server' 600
  APP_PORT=47100
else
  LINK_MTU=0
  APP_PORT=46100
fi

sudo ip netns exec "$C" tcpdump --immediate-mode -U -i mc0 -s 0 \
  -w "$OUT/mtu-boundary.pcap" 'udp and port 46000' >"$OUT/tcpdump.log" 2>&1 &
CAP_PID=$!
wait_log "$OUT/tcpdump.log" 'listening on mc0' 200

sudo ip netns exec "$C" python3 - "$APP" "$APP_PORT" <<'PY' >"$OUT/probe.log"
import socket,sys,time
n=int(sys.argv[1]); port=int(sys.argv[2])
head=b'WBD_MTU_BOUNDARY_'
payload=head+b'X'*(n-len(head))
assert len(payload)==n
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0)); s.settimeout(.5)
deadline=time.time()+8
while time.time()<deadline:
    s.sendto(payload,('127.0.0.1',port))
    try:
        got,_=s.recvfrom(65535)
    except socket.timeout:
        continue
    assert got==payload,(len(got),len(payload))
    print(f'WBD_MTU_BOUNDARY_ECHO_PASS bytes={len(got)} port={port}')
    break
else:
    raise SystemExit('boundary echo timeout')
PY
sleep .40
sudo kill -INT "$CAP_PID" 2>/dev/null || true
wait "$CAP_PID" 2>/dev/null || true
CAP_PID=

grep -Eq 'WRITE role=client datagram=[0-9]+ bytes=1428' "$OUT/dtls-client.log"
if [[ "$LINK_MODE" == link ]]; then
  if grep -R -E 'encode_input_exceeds_link_mtu|WBD_LINK_PROXY_FAIL|packet too large' "$OUT"/link-*.log; then
    echo "unexpected LINK MTU failure" >&2
    exit 1
  fi
fi
python3 "$ANALYZER" "$OUT/mtu-boundary.pcap" \
  --protocol udp --max-ip-total 1500 --max-transport-payload 1460 \
  --out "$OUT/mtu-boundary.json"
printf '%s\n' "case=$CASE carrier_mtu=1500 fec=$FEC_MODE game=off app=$APP link=$LINK_MTU dtls_plaintext_boundary=1428 product_sha=66612d348fd52852ffe6c03fcbb52807a3a54883" \
  >"$OUT/provenance.txt"
echo "WBD_MTU_WIRE_CASE_PASS case=$CASE app=$APP link=$LINK_MTU dtls_plaintext=1428 carrier=1500"
