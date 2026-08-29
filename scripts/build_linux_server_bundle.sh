#!/bin/sh
set -eu

ARCH=${1:-}
OUT=${2:-}
case "$ARCH" in amd64|arm64) ;; *) echo 'usage: build_linux_server_bundle.sh amd64|arm64 OUTDIR' >&2; exit 2;; esac
[ -n "$OUT" ] || { echo 'output directory required' >&2; exit 2; }
: "${GH_TOKEN:?GH_TOKEN is required to fetch the pinned wolfSSL source artifact}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"

host=$(uname -m)
case "$host" in x86_64|amd64) host_arch=amd64;; aarch64|arm64) host_arch=arm64;; *) echo "unsupported build host: $host" >&2; exit 1;; esac
[ "$host_arch" = "$ARCH" ] || { echo "native bundle build requires host=$ARCH, got $host_arch" >&2; exit 1; }

command -v python3 >/dev/null 2>&1 || { echo 'python3 required' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo 'curl required' >&2; exit 1; }
command -v unzip >/dev/null 2>&1 || { echo 'unzip required' >&2; exit 1; }

read_lock() {
    python3 - "$1" <<'PY'
import json,sys
with open('deps/security-lock.json','r',encoding='utf-8') as f:
    d=json.load(f)['dtls']['source_qualification']
print(d[sys.argv[1]])
PY
}
artifact_id=$(read_lock relay_artifact_id)
source_sha=$(read_lock git_archive_sha256)

work=$(mktemp -d)
cleanup() { rm -rf "$work"; }
trap cleanup EXIT INT TERM HUP
root=$OUT/wbd-server-$ARCH
rm -rf "$root"
mkdir -p "$root/bin" "$work/wolf-art" "$work/wolf/src" "$work/wolf/build"

curl -fL -H "Authorization: Bearer ${GH_TOKEN}" -H 'Accept: application/vnd.github+json' \
  "https://api.github.com/repos/${GITHUB_REPOSITORY}/actions/artifacts/${artifact_id}/zip" -o "$work/wolfssl-source.zip"
unzip -q "$work/wolfssl-source.zip" -d "$work/wolf-art"
source_tar=$(find "$work/wolf-art" -maxdepth 1 -type f -name 'wolfssl-*-source.tar.gz' -print -quit)
[ -n "$source_tar" ] || { echo 'pinned wolfSSL source tarball missing from artifact' >&2; exit 1; }
printf '%s  %s\n' "$source_sha" "$source_tar" | sha256sum -c -
tar -xzf "$source_tar" -C "$work/wolf/src" --strip-components=1
(cd "$work/wolf/src" && ./autogen.sh)
(cd "$work/wolf/build" && "$work/wolf/src/configure" --enable-dtls13 --disable-shared --enable-static CFLAGS='-O2 -fPIC')
make -C "$work/wolf/build" -j2 src/libwolfssl.la
gcc -O2 -Wall -Wextra -Werror -static -I"$work/wolf/build" -I"$work/wolf/src" \
  native/dtls/wbd_dtls_shim.c "$work/wolf/build/src/.libs/libwolfssl.a" -lm -lpthread \
  -o "$root/bin/wbd_dtls_shim"

export CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH"
go test ./internal/realityfront ./internal/singleflow ./internal/session ./internal/faketcp ./internal/dtlsworker ./internal/platformproxy ./cmd/wbd-faketcp-mux ./cmd/wbd-link-server-mux ./cmd/wbd-server-cert -count=1
for spec in \
  'wbd-faketcp-mux:./cmd/wbd-faketcp-mux' \
  'wbd-link-server-mux:./cmd/wbd-link-server-mux' \
  'wbd-platform-proxy-server:./cmd/wbd-platform-proxy-server' \
  'wbd-server-cert:./cmd/wbd-server-cert'; do
    name=${spec%%:*}; pkg=${spec#*:}
    go build -trimpath -ldflags='-s -w' -o "$root/bin/$name" "$pkg"
done

# V3 official composition has one public owner. The legacy wbd-reality-front
# binary is intentionally not packaged or installed as a listener.
cp scripts/linux_server_manager_v3.sh "$root/wbd-server"
cp scripts/linux_server_manager_v3.sh "$root/linux_server_manager_v3.sh"
cp scripts/linux_server_firewall.sh scripts/linux_server_guard.sh "$root/"
printf '%s\n' "$ARCH" > "$root/ARCH"
cat > "$root/README.txt" <<'EOF'
WBD V3 Linux Server bundle
==========================
1. Extract this bundle on the target Linux server.
2. sudo ./wbd-server install
3. Set the one public TCP-shaped port, e.g.: sudo /opt/wbd/bin/wbd-server set WBD_PORT 443
4. Configure server name and credentials: sudo /opt/wbd/bin/wbd-server config
5. Run: sudo /opt/wbd/bin/wbd-server doctor
6. Run: sudo /opt/wbd/bin/wbd-server start
7. Check: /opt/wbd/bin/wbd-server status

The V3 public endpoint has exactly one owner: wbd-faketcp-mux. From the first
raw SYN through steady state the same public 4-tuple and FakeTCP sequence space
are retained. During the first bounded setup phase, the raw association exposes
an ordered byte-stream presentation for real TLS 1.3 Reality-like ClientHello,
authentication, and an encrypted TLS mode-switch request/ack. There is no FIN,
RST, close_notify, or second SYN at that boundary. After the barrier the ordered
assembler is discarded and sustained VPN traffic is DTLS 1.3/FEC/datagram over
the existing first-arrival FakeTCP path, so ordinary TCP HOL is absent from the
steady-state data plane.

Front route/account secrets are loaded from the root-owned server.env file and
passed to the child environment. The official manager does not put them in the
mux command line shown by ps/systemctl status.

Runtime application binaries are statically linked. The host kernel must support
raw sockets and netfilter, and the OS must provide systemd plus nft or iptables.
EOF
chmod +x "$root/wbd-server" "$root"/*.sh "$root/bin"/*

for f in "$root/bin"/*; do
    echo "--- $f"
    file "$f"
    if ldd "$f" 2>&1 | grep -q '=>'; then
        echo "dynamic dependency found: $f" >&2
        ldd "$f" >&2 || true
        exit 1
    fi
done
sh -n "$root/wbd-server" "$root/linux_server_manager_v3.sh" "$root/linux_server_firewall.sh" "$root/linux_server_guard.sh"

# Release qualification asserts the legacy kernel-TCP Reality listener is not
# present, raw mux is the sole public owner, and official process argv carries
# no front classifier/account secrets.
[ ! -e "$root/bin/wbd-reality-front" ] || { echo 'legacy Reality listener must not ship in V3 runtime' >&2; exit 1; }
grep -q 'public_owner=raw-mux' "$root/wbd-server"
grep -q -- '--front-server-name' "$root/wbd-server"
grep -q 'WBD_FRONT_ROUTE_KEY=' "$root/wbd-server"
grep -q 'WBD_FRONT_USERNAME=' "$root/wbd-server"
grep -q 'WBD_FRONT_PASSWORD=' "$root/wbd-server"
if grep -E -- '--front-route-key|--username|--password' "$root/wbd-server" >/dev/null; then
    echo 'V3 manager must not place front secrets in process argv' >&2
    exit 1
fi

tar -C "$OUT" -czf "$OUT/wbd-linux-server-$ARCH.tar.gz" "wbd-server-$ARCH"
(
    cd "$OUT"
    sha256sum "wbd-linux-server-$ARCH.tar.gz" > "wbd-linux-server-$ARCH.tar.gz.sha256"
    sha256sum -c "wbd-linux-server-$ARCH.tar.gz.sha256"
)
echo "WBD_LINUX_SERVER_RELEASE_PASS arch=$ARCH static_runtime=1 manager=1 single_public_owner=raw-mux single_flow_v3=1 secret_argv=0 portable_checksum=1"