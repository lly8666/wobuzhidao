#!/bin/sh
set -eu

ARCH=${1:-}
OUT=${2:-}
case "$ARCH" in amd64|arm64) ;; *) echo 'usage: build_linux_server_bundle.sh amd64|arm64 OUTDIR' >&2; exit 2;; esac
[ -n "$OUT" ] || { echo 'output directory required' >&2; exit 2; }
: "${GH_TOKEN:?GH_TOKEN is required to fetch the pinned wolfSSL source artifact}"
: "${GITHUB_REPOSITORY:?GITHUB_REPOSITORY is required}"
BUILD_SOURCE_SHA=${WBD_SOURCE_SHA:-${GITHUB_SHA:-unknown}}

host=$(uname -m)
case "$host" in x86_64|amd64) host_arch=amd64;; aarch64|arm64) host_arch=arm64;; *) echo "unsupported build host: $host" >&2; exit 1;; esac
[ "$host_arch" = "$ARCH" ] || { echo "native bundle build requires host=$ARCH, got $host_arch" >&2; exit 1; }

command -v python3 >/dev/null 2>&1 || { echo 'python3 required' >&2; exit 1; }
command -v curl >/dev/null 2>&1 || { echo 'curl required' >&2; exit 1; }
command -v unzip >/dev/null 2>&1 || { echo 'unzip required' >&2; exit 1; }

LOCK=deps/security-lock.json
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
go test ./internal/realityfront ./internal/session ./internal/faketcp ./internal/dtlsworker ./internal/platformproxy ./internal/gamelane ./cmd/wbd-game-lane-server ./cmd/wbd-link-server-mux ./cmd/wbd-server-cert -count=1
for spec in \
  'wbd-reality-front:./cmd/wbd-reality-front' \
  'wbd-faketcp-mux:./cmd/wbd-faketcp-mux' \
  'wbd-link-server-mux:./cmd/wbd-link-server-mux' \
  'wbd-game-lane-server:./cmd/wbd-game-lane-server' \
  'wbd-platform-proxy-server:./cmd/wbd-platform-proxy-server' \
  'wbd-server-cert:./cmd/wbd-server-cert'; do
    name=${spec%%:*}; pkg=${spec#*:}
    go build -trimpath -ldflags='-s -w' -o "$root/bin/$name" "$pkg"
done

cp scripts/linux_server_manager.sh "$root/wbd-server"
cp scripts/linux_server_manager.sh "$root/linux_server_manager.sh"
cp scripts/linux_server_firewall.sh scripts/linux_server_guard.sh "$root/"
printf '%s\n' "$ARCH" > "$root/ARCH"
printf '%s\n' "$BUILD_SOURCE_SHA" > "$root/SOURCE_SHA"
cat > "$root/README.txt" <<'EOF'
WBD Linux Server bundle
=======================
1. Extract this bundle on the target Linux server.
2. sudo ./wbd-server install
3. Set one public port, for example: sudo /opt/wbd/bin/wbd-server set WBD_PORT 443
4. Configure domain and credentials: sudo /opt/wbd/bin/wbd-server config
5. Run: sudo /opt/wbd/bin/wbd-server doctor
6. Run: sudo /opt/wbd/bin/wbd-server start
7. Check: /opt/wbd/bin/wbd-server status

Commands: install, uninstall, start, stop, pause, resume, restart, status, logs,
config, set, regen-certs, doctor, show-config. Configuration is
/etc/wbd/server.env.

ADR-0014 product mode exposes one public raw wbd-faketcp-mux listener on
WBD_PORT. Each connected Logical Tunnel may bind exactly one active public WBD
transport. That transport owns one FakeTCP SYN/4-tuple/sequence lineage: its
Reality-like real TLS 1.3 setup is the first payload phase of that exact
association, then the same association crosses the bootstrap barrier with no
FIN/RST/new WBD SYN into DTLS 1.3 -> LINK -> FEC without ordinary kernel-TCP
stream HOL.

The product LINK mux connects directly to the private platform service. The
bundled Game Lane server remains a research/reference binary and is not started
by the product wbd-server run path. No extra public listener is introduced.

The bundled wbd-reality-front binary is diagnostic/reference only. The product
wbd-server run path never starts it as a public listener.

SOURCE_SHA records the exact substantive repository source head used to build
this bundle. Pair Windows and Linux physical-test artifacts only when their
SOURCE_SHA values are identical.

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
sh -n "$root/wbd-server" "$root/linux_server_manager.sh" "$root/linux_server_firewall.sh" "$root/linux_server_guard.sh"

tar -C "$OUT" -czf "$OUT/wbd-linux-server-$ARCH.tar.gz" "wbd-server-$ARCH"
(
    cd "$OUT"
    sha256sum "wbd-linux-server-$ARCH.tar.gz" > "wbd-linux-server-$ARCH.tar.gz.sha256"
    sha256sum -c "wbd-linux-server-$ARCH.tar.gz.sha256"
)
echo "WBD_LINUX_SERVER_RELEASE_PASS arch=$ARCH static_runtime=1 manager=1 public_listener=1 max_tunnel_lanes=1 game_product=0 source_sha=$BUILD_SOURCE_SHA portable_checksum=1"
