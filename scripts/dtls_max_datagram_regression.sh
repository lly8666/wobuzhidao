#!/usr/bin/env bash
set -euo pipefail

ASSET_DIR=${1:?usage: dtls_max_datagram_regression.sh ASSET_DIR}
SHIM="$ASSET_DIR/wbd_dtls_shim"
CERT="$ASSET_DIR/dtls.pem"
KEY="$ASSET_DIR/dtls.key"

for f in "$SHIM" "$CERT" "$KEY"; do
  test -e "$f" || { echo "missing regression asset: $f" >&2; exit 2; }
done

TMP=${RUNNER_TEMP:-/tmp}/wbd-dtls-max-datagram-$$
mkdir -p "$TMP"
PIDS=()
cleanup() {
  set +e
  for p in "${PIDS[@]:-}"; do kill -TERM "$p" 2>/dev/null || true; done
  for p in "${PIDS[@]:-}"; do wait "$p" 2>/dev/null || true; done
}
trap cleanup EXIT

cat >"$TMP/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',55200))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY
python3 "$TMP/echo.py" >"$TMP/echo.log" 2>&1 & PIDS+=("$!")

"$SHIM" server 55201 127.0.0.1 55200 "$CERT" "$KEY" >"$TMP/server.log" 2>&1 & PIDS+=("$!")
sleep .15
"$SHIM" client 55202 127.0.0.1 55201 none none >"$TMP/client.log" 2>&1 & PIDS+=("$!")

for _ in $(seq 1 600); do
  grep -q 'READY role=client version=DTLSv1.3' "$TMP/client.log" && \
  grep -q 'READY role=server version=DTLSv1.3' "$TMP/server.log" && break
  sleep .05
done
grep -q 'READY role=client version=DTLSv1.3' "$TMP/client.log"
grep -q 'READY role=server version=DTLSv1.3' "$TMP/server.log"

# Protocol-derived maximum for the existing product contract:
# interface C=1360 -> LINK plaintext C+40=1400 -> FEC source frame +56=1456.
# The DTLS transport sits above FakeTCP carrier fragmentation, so wolfSSL must
# accept this logical datagram rather than enforcing a smaller physical UDP MTU.
python3 - <<'PY'
import socket
n=1456
payload=bytes((i*29+7)&0xff for i in range(n))
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',0))
s.settimeout(5)
s.sendto(payload,('127.0.0.1',55202))
got,_=s.recvfrom(65535)
assert got==payload,(len(got),len(payload))
print(f'WBD_DTLS_MAX_DATAGRAM_PASS bytes={n}')
PY

if grep -q 'DTLS trying to send too much in single datagram' "$TMP/client.log" "$TMP/server.log"; then
  echo 'unexpected wolfSSL DTLS MTU rejection' >&2
  exit 1
fi
