#!/usr/bin/env bash
set -euo pipefail

: "${WBD_TEST_MTU:?WBD_TEST_MTU required}"
: "${GH_TOKEN:?GH_TOKEN required}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY required}"
MTU="$WBD_TEST_MTU"
[[ "$MTU" =~ ^[0-9]+$ ]] || { echo "bad MTU: $MTU" >&2; exit 2; }
ROOT=$(pwd)
TMP=${RUNNER_TEMP:-/tmp}/wbd-mtu-${MTU}
ASSET="$TMP/assets"
LOG="$TMP/log"
mkdir -p "$ASSET" "$LOG"

cleanup_tmp_tests() {
  rm -f "$ROOT/mtu_action_budget_tmp.go" \
        "$ROOT/internal/windowsruntime/mtu_action_test.go" \
        "$ROOT/internal/faketcp/mtu_action_test.go"
}
trap cleanup_tmp_tests EXIT

cat > "$ROOT/mtu_action_budget_tmp.go" <<'GO'
package main
import (
  "fmt"
  "os"
  "strconv"
  "github.com/lly8666/wobuzhidao/internal/pathmtu"
)
func main() {
  n, err := strconv.Atoi(os.Getenv("WBD_TEST_MTU")); if err != nil { panic(err) }
  b, err := pathmtu.Derive(n, pathmtu.Features{FEC:true, Game:true}); if err != nil { panic(err) }
  fmt.Printf("%d %d %d %d %d\n", b.ConnectionMTU, b.CarrierPayloadMTU, b.DTLSPlaintextMTU, b.LinkPlaintextMTU, b.InnerMTU)
}
GO
read -r CONNECTION CARRIER DTLS LINK INNER < <(go run "$ROOT/mtu_action_budget_tmp.go")
[[ "$CONNECTION" == "$MTU" ]]
echo "WBD_MTU_BUDGET connection=$CONNECTION carrier=$CARRIER dtls_plain=$DTLS link_plain=$LINK inner=$INNER"

cat > "$ROOT/internal/windowsruntime/mtu_action_test.go" <<'GO'
package windowsruntime
import (
  "os"
  "strconv"
  "strings"
  "testing"
)
func TestActionConfiguredMTUFlowsThroughWindowsPlan(t *testing.T) {
  mtu, err := strconv.Atoi(os.Getenv("WBD_TEST_MTU")); if err != nil { t.Fatal(err) }
  p := testProfile(); p.MTU = mtu
  b, err := gameConnectionMTUBudget(mtu, p.FEC); if err != nil { t.Fatal(err) }
  plan, err := BuildPlan(p, testUnderlay(), strings.Repeat("ab", 32)); if err != nil { t.Fatal(err) }
  if !argPair(plan.Link.Args, "-mtu", strconv.Itoa(b.LinkPlaintextMTU)) { t.Fatalf("LINK args=%v budget=%+v", plan.Link.Args, b) }
  if !argPair(plan.TUN.Args, "-mtu", strconv.Itoa(b.InnerMTU)) { t.Fatalf("TUN args=%v budget=%+v", plan.TUN.Args, b) }
  if !argPair(plan.RouteApply.Args, "-MTU", strconv.Itoa(b.InnerMTU)) { t.Fatalf("route args=%v budget=%+v", plan.RouteApply.Args, b) }
}
GO

cat > "$ROOT/internal/faketcp/mtu_action_test.go" <<'GO'
package faketcp
import (
  "os"
  "strconv"
  "testing"
  "github.com/lly8666/wobuzhidao/internal/pathmtu"
)
func TestActionConfiguredLogicalMTUIsSafeOnPhysical1500Carrier(t *testing.T) {
  mtu, err := strconv.Atoi(os.Getenv("WBD_TEST_MTU")); if err != nil { t.Fatal(err) }
  b, err := pathmtu.Derive(mtu, pathmtu.Features{FEC:true, Game:true}); if err != nil { t.Fatal(err) }
  f, err := NewCarrierFragmenter(1500); if err != nil { t.Fatal(err) }
  frames, err := f.Fragment(make([]byte, b.MaxDTLSDatagram())); if err != nil { t.Fatal(err) }
  for i, frame := range frames { if len(frame) > 1460 { t.Fatalf("frame %d len=%d > physical carrier payload 1460", i, len(frame)) } }
  if b.MaxDTLSDatagram() > 1460 && len(frames) < 2 { t.Fatalf("logical datagram=%d was not carrier-fragmented", b.MaxDTLSDatagram()) }
  if b.MaxDTLSDatagram() <= 1460 && len(frames) != 1 { t.Fatalf("logical datagram=%d unexpectedly fragmented into %d", b.MaxDTLSDatagram(), len(frames)) }
}
GO

gofmt -w "$ROOT/internal/windowsruntime/mtu_action_test.go" "$ROOT/internal/faketcp/mtu_action_test.go"
go test ./internal/pathmtu ./internal/windowsruntime ./internal/control ./internal/faketcp ./internal/fec ./internal/gamelane -count=1

rm -rf /tmp/wolf-art /tmp/wolf/src /tmp/wolf/build
mkdir -p /tmp/wolf-art /tmp/wolf/src /tmp/wolf/build
curl -fL -H "Authorization: Bearer ${GH_TOKEN}" -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/${GITHUB_REPOSITORY}/actions/artifacts/9529710833/zip" -o /tmp/wolfssl-source.zip
unzip -q /tmp/wolfssl-source.zip -d /tmp/wolf-art
echo '4a7ff40a32db0d7a262aaea2d2e674da6708250cba908441c737c981fc84f88b  /tmp/wolf-art/wolfssl-ac01707f-source.tar.gz' | sha256sum -c -
tar -xzf /tmp/wolf-art/wolfssl-ac01707f-source.tar.gz -C /tmp/wolf/src --strip-components=1
(cd /tmp/wolf/src && ./autogen.sh)
(cd /tmp/wolf/build && /tmp/wolf/src/configure --enable-dtls13 --disable-shared --enable-static CFLAGS='-O2 -DWOLFSSL_DTLS_WINDOW_WORDS=128 -DWOLFSSL_MAX_MTU=16384')
make -C /tmp/wolf/build -j2 src/libwolfssl.la
gcc -DWOLFSSL_DTLS_WINDOW_WORDS=128 -DWOLFSSL_MAX_MTU=16384 -O2 -Wall -Wextra -Werror \
  -I/tmp/wolf/build -I/tmp/wolf/src native/dtls/wbd_dtls_shim.c /tmp/wolf/build/src/.libs/libwolfssl.a -lm -o "$ASSET/wbd_dtls_shim"
for spec in \
  'wbd-faketcp ./cmd/wbd-faketcp' \
  'wbd-faketcp-mux ./cmd/wbd-faketcp-mux' \
  'wbd-link-proxy ./cmd/wbd-link-proxy' \
  'wbd-link-server-mux ./cmd/wbd-link-server-mux' \
  'wbd-game-lane-client ./cmd/wbd-game-lane-client' \
  'wbd-game-lane-server ./cmd/wbd-game-lane-server'; do
  set -- $spec
  go build -trimpath -o "$ASSET/$1" "$2"
done
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=target.example' -keyout "$ASSET/front.key" -out "$ASSET/front.pem" >/dev/null 2>&1
openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 1 -subj '/CN=wbd-dtls.test' -keyout "$ASSET/dtls.key" -out "$ASSET/dtls.pem" >/dev/null 2>&1
chmod +x "$ASSET"/wbd-* "$ASSET/wbd_dtls_shim"

# Test-only: make every formal Game probe an exact maximum-size inner datagram.
python3 - "$ROOT/scripts/game_lane_fullstack.sh" "$INNER" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); inner=int(sys.argv[2]); s=p.read_text()
old="    b=prefix+i.to_bytes(2,'big')"
new=old+f"\n    b=b+b'x'*max(0,{inner}-len(b))"
if s.count(old) != 1:
    raise SystemExit(f'probe payload marker drift: {s.count(old)}')
p.write_text(s.replace(old,new,1))
PY
bash -n "$ROOT/scripts/game_lane_fullstack.sh"

LANES=1 FEC=20:20 PROBE_COUNT=3 INNER_MTU="$INNER" LINK_PLAINTEXT_MTU="$LINK" \
  sudo -E bash "$ROOT/scripts/game_lane_fullstack.sh" "$ASSET" "$LOG"

grep -q 'WBD_GAME_LANE_PROBE_PASS count=3' "$LOG/probe.log"
grep -q 'READY role=client.*carrier_mtu=1500' "$LOG/faketcp-1.log"

python3 - "$LOG/game-lanes.pcap" "$MTU" "$INNER" <<'PY'
import struct,sys
path,configured,inner=sys.argv[1],int(sys.argv[2]),int(sys.argv[3])
b=open(path,'rb').read()
if len(b)<24: raise SystemExit('pcap too short')
magic=b[:4]
if magic in (b'\xd4\xc3\xb2\xa1', b'M<\xb2\xa1'): endian='<'
elif magic in (b'\xa1\xb2\xc3\xd4', b'\xa1\xb2<M'): endian='>'
else: raise SystemExit(f'unknown pcap magic {magic.hex()}')
linktype=struct.unpack(endian+'I',b[20:24])[0]
if linktype != 1: raise SystemExit(f'expected ethernet pcap, linktype={linktype}')
o=24; packets=0; ipv4=0; fragments=0; max_ip=0
while o+16 <= len(b):
    _,_,incl,_=struct.unpack(endian+'IIII',b[o:o+16]); o+=16
    frame=b[o:o+incl]; o+=incl; packets+=1
    if len(frame)<14: continue
    eth=struct.unpack('!H',frame[12:14])[0]; ipoff=14
    while eth in (0x8100,0x88a8) and len(frame)>=ipoff+4:
        eth=struct.unpack('!H',frame[ipoff+2:ipoff+4])[0]; ipoff+=4
    if eth != 0x0800 or len(frame)<ipoff+20: continue
    ipv4+=1
    total=struct.unpack('!H',frame[ipoff+2:ipoff+4])[0]
    frag=struct.unpack('!H',frame[ipoff+6:ipoff+8])[0]
    max_ip=max(max_ip,total)
    if frag & 0x3fff: fragments+=1
if ipv4 == 0: raise SystemExit('no IPv4 packets in pcap')
if fragments: raise SystemExit(f'IPv4 fragmentation observed: {fragments}')
if max_ip > 1500: raise SystemExit(f'physical packet exceeded veth MTU: {max_ip}')
print(f'WBD_MTU_PCAP configured={configured} inner={inner} packets={packets} ipv4={ipv4} max_ip_len={max_ip} fragments={fragments}')
PY

echo "WBD_MTU_GLOBAL_PASS configured=$MTU carrier_budget=$CARRIER dtls_plain=$DTLS link_plain=$LINK inner=$INNER physical_mtu=1500"
