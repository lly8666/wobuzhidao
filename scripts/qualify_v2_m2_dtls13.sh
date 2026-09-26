#!/usr/bin/env bash
set -euo pipefail

EXPECTED_SOURCE_SHA=4a7ff40a32db0d7a262aaea2d2e674da6708250cba908441c737c981fc84f88b
EXPECTED_COMMIT=ac01707f552c611fbd135cc723b2682b3e7f80f2
EXPECTED_TAG=v5.9.2-stable
SOURCE_TGZ=${1:?usage: qualify_v2_m2_dtls13.sh SOURCE_TGZ OUT_DIR}
OUT=${2:?usage: qualify_v2_m2_dtls13.sh SOURCE_TGZ OUT_DIR}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
PROBE_SRC="$ROOT/native/dtls/dtls13_probe.c"
mkdir -p "$OUT"
OUT=$(cd "$OUT" && pwd)

actual=$(sha256sum "$SOURCE_TGZ" | awk '{print $1}')
[ "$actual" = "$EXPECTED_SOURCE_SHA" ] || { echo "source sha mismatch: $actual" >&2; exit 2; }
rm -rf "$OUT/src" "$OUT/build" "$OUT/install" "$OUT/certs"
mkdir -p "$OUT/src" "$OUT/build" "$OUT/install" "$OUT/certs"
tar -xzf "$SOURCE_TGZ" -C "$OUT/src" --strip-components=1

gcc_line=$(gcc --version | sed -n '1p')
autoconf_line=$(autoconf --version | sed -n '1p')
automake_line=$(automake --version | sed -n '1p')
libtool_line=$(libtoolize --version | sed -n '1p')
(
  cd "$OUT/src"
  ./autogen.sh
) >"$OUT/autogen.log" 2>&1
(
  cd "$OUT/build"
  "$OUT/src/configure" --enable-dtls13 --disable-shared --enable-static --prefix="$OUT/install"
) >"$OUT/configure.log" 2>&1
make -C "$OUT/build" -j2 >"$OUT/make.log" 2>&1
make -C "$OUT/build" install >"$OUT/install.log" 2>&1

LIB="$OUT/install/lib/libwolfssl.a"
INC="$OUT/install/include"
lib_sha=$(sha256sum "$LIB" | awk '{print $1}')
grep -q '#define WOLFSSL_DTLS13' "$INC/wolfssl/options.h"
grep -q '#define WOLFSSL_TLS13' "$INC/wolfssl/options.h"
! grep -Eq '^#define (WOLFSSL_EARLY_DATA|HAVE_EARLY_DATA|OPENSSL_EXTRA)' "$INC/wolfssl/options.h"
nm -g "$LIB" > "$OUT/libwolfssl.symbols"
grep -q 'wolfDTLSv1_3_client_method' "$OUT/libwolfssl.symbols"
grep -q 'wolfDTLSv1_3_server_method' "$OUT/libwolfssl.symbols"
grep -q 'wolfSSL_check_domain_name' "$OUT/libwolfssl.symbols"

gcc -O2 -Wall -Wextra -Werror -I"$INC" "$PROBE_SRC" "$LIB" -lm -o "$OUT/dtls13_probe"
probe_sha=$(sha256sum "$OUT/dtls13_probe" | awk '{print $1}')

C="$OUT/certs"
cat > "$C/server.ext" <<'EXT'
basicConstraints=critical,CA:FALSE
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
subjectAltName=DNS:wbd.test
EXT
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 30 -subj '/CN=WBD Test Root CA' -keyout "$C/ca.key" -out "$C/ca.pem" >/dev/null 2>&1
openssl req -newkey rsa:2048 -nodes -sha256 -subj '/CN=wbd.test' -keyout "$C/server.key" -out "$C/server.csr" >/dev/null 2>&1
openssl x509 -req -in "$C/server.csr" -CA "$C/ca.pem" -CAkey "$C/ca.key" -CAcreateserial -days 30 -sha256 -extfile "$C/server.ext" -out "$C/server.pem" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 30 -subj '/CN=WBD Wrong Root CA' -keyout "$C/wrong-ca.key" -out "$C/wrong-ca.pem" >/dev/null 2>&1
openssl verify -CAfile "$C/ca.pem" "$C/server.pem" >"$OUT/openssl-verify.log"

run_case() {
  local name=$1 port=$2 ca=$3 host=$4 expect=$5
  set +e
  "$OUT/dtls13_probe" server "$port" "$C/server.pem" "$C/server.key" >"$OUT/$name.server.log" 2>&1 &
  local sp=$!
  sleep .15
  "$OUT/dtls13_probe" client "$port" "$ca" "$host" >"$OUT/$name.client.log" 2>&1
  local cr=$?
  wait "$sp"; local sr=$?
  set -e
  printf '%s,%s,%s,%s\n' "$name" "$expect" "$cr" "$sr" >> "$OUT/cases.csv"
  if [ "$expect" = pass ]; then
    [ "$cr" -eq 0 ] && [ "$sr" -eq 0 ]
    grep -q 'version=DTLSv1.3' "$OUT/$name.client.log"
    grep -q 'APP_OK reply=wbd-ack' "$OUT/$name.client.log"
  else
    [ "$cr" -ne 0 ]
  fi
}
printf 'case,expected,client_rc,server_rc\n' > "$OUT/cases.csv"
run_case valid 46131 "$C/ca.pem" wbd.test pass
run_case hostname_mismatch 46132 "$C/ca.pem" wrong.test fail
run_case untrusted_ca 46133 "$C/wrong-ca.pem" wbd.test fail
grep -q 'peer subject name mismatch' "$OUT/hostname_mismatch.client.log"
grep -q 'ASN no signer' "$OUT/untrusted_ca.client.log"

python3 - "$OUT" "$EXPECTED_SOURCE_SHA" "$lib_sha" "$probe_sha" "$gcc_line" "$autoconf_line" "$automake_line" "$libtool_line" <<'PY'
import json,sys,pathlib
out=pathlib.Path(sys.argv[1])
receipt={
 "schema":"wbd-v2-m2a-dtls13-qualification/v1",
 "source":{"implementation":"wolfSSL/wolfssl","tag":"v5.9.2-stable","commit":"ac01707f552c611fbd135cc723b2682b3e7f80f2","git_archive_sha256":sys.argv[2]},
 "build":{"flags":["--enable-dtls13","--disable-shared","--enable-static"],"libwolfssl_a_sha256":sys.argv[3],"probe_sha256":sys.argv[4],"gcc":sys.argv[5],"autoconf":sys.argv[6],"automake":sys.argv[7],"libtool":sys.argv[8],"dtls13":True,"early_data":False,"openssl_extra":False},
 "identity":{"valid_hostname":"wbd.test","native_api":"wolfSSL_check_domain_name","ca_verify":"wolfSSL_CTX_load_verify_locations + WOLFSSL_VERIFY_PEER"},
 "cases":{"valid":"pass; DTLSv1.3 + application datagram","hostname_mismatch":"rejected: peer subject name mismatch","untrusted_ca":"rejected: ASN no signer"},
 "zero_rtt":"disabled by build; no early-data macro",
 "result":"pass"
}
(out/'receipt.json').write_text(json.dumps(receipt,indent=2)+"\n")
PY
sha256sum "$OUT/receipt.json" "$OUT/cases.csv" "$OUT/dtls13_probe" "$LIB" > "$OUT/SHA256SUMS"
cat "$OUT/cases.csv"
cat "$OUT/valid.client.log"
cat "$OUT/hostname_mismatch.client.log"
cat "$OUT/untrusted_ca.client.log"
cat "$OUT/receipt.json"
