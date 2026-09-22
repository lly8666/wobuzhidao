#!/usr/bin/env bash
set -euo pipefail

DEST="${1:?usage: build_seeded_tc.sh DEST}"
VERSION="7.2.0"
ARCHIVE="iproute2-${VERSION}.tar.xz"
SHA256="4c2fa124c2cf0afd7ca34d1eeacba6ba048a56f6374e2aab93dafbdbd4eea9c0"
URL="https://www.kernel.org/pub/linux/utils/net/iproute2/${ARCHIVE}"
ROOT="${RUNNER_TEMP:-/tmp}/wbd-iproute2-${VERSION}"

mkdir -p "$ROOT" "$(dirname "$DEST")"
cd "$ROOT"
if [[ ! -f "$ARCHIVE" ]]; then
  curl -fL --retry 3 --retry-delay 2 -o "$ARCHIVE" "$URL"
fi
printf '%s  %s\n' "$SHA256" "$ARCHIVE" | sha256sum -c -

rm -rf "iproute2-${VERSION}"
tar -xJf "$ARCHIVE"
cd "iproute2-${VERSION}"
./configure > "$ROOT/configure.log"
make -j2 tc/tc > "$ROOT/make-tc.log"
install -m 0755 tc/tc "$DEST"
"$DEST" -V
