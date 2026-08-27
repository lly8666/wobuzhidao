#!/bin/sh
set -eu

MANAGER=${WBD_SERVER_MANAGER:-./scripts/linux_server_manager.sh}
ROOT=${WBD_SETTINGS_TEST_ROOT:-/tmp/wbd-server-settings.$$}
ETC=$ROOT/etc
PREFIX=$ROOT/opt
RUN=$ROOT/run

cleanup() { rm -rf "$ROOT"; }
trap cleanup EXIT INT TERM HUP
mkdir -p "$ETC" "$PREFIX" "$RUN"

run_manager() {
    env WBD_ETC="$ETC" WBD_PREFIX="$PREFIX" WBD_RUN="$RUN" sh "$MANAGER" "$@"
}

[ "$(id -u)" -eq 0 ] || { echo 'linux_server_settings_test.sh requires root' >&2; exit 1; }

# A fresh config exposes one public port and no historical split-port knobs.
run_manager set WBD_PORT 443 >/tmp/wbd-settings-set.log
CONFIG=$ETC/server.env
grep -q "^WBD_PORT='443'$" "$CONFIG"
if grep -Eq '^WBD_(FRONT|RAW)_PORT=' "$CONFIG"; then
    echo 'fresh config still exposes split public ports' >&2
    cat "$CONFIG" >&2
    exit 1
fi

# Supported settings survive validation and sensitive values never print back.
run_manager set WBD_SERVER_NAME edge.example >/tmp/wbd-settings-name.log
grep -q 'regen-certs' /tmp/wbd-settings-name.log
run_manager set WBD_USERNAME shared-user >/dev/null
run_manager set WBD_PASSWORD very-secret-password >/dev/null
run_manager set WBD_ROUTE_KEY 0123456789abcdef0123456789abcdef >/dev/null
out=$(run_manager show-config)
printf '%s\n' "$out" | grep -q '^WBD_PASSWORD=<redacted>$'
printf '%s\n' "$out" | grep -q '^WBD_ROUTE_KEY=<redacted>$'
if printf '%s\n' "$out" | grep -q 'very-secret-password'; then
    echo 'show-config leaked password' >&2; exit 1
fi

# Invalid or obsolete public-port settings fail instead of diverging front/raw.
if run_manager set WBD_PORT 0 >/tmp/wbd-settings-bad.log 2>&1; then
    echo 'WBD_PORT=0 unexpectedly accepted' >&2; exit 1
fi
if run_manager set WBD_PORT 65536 >/tmp/wbd-settings-bad.log 2>&1; then
    echo 'WBD_PORT=65536 unexpectedly accepted' >&2; exit 1
fi
if run_manager set WBD_FRONT_PORT 443 >/tmp/wbd-settings-bad.log 2>&1; then
    echo 'obsolete WBD_FRONT_PORT unexpectedly accepted' >&2; exit 1
fi

# Historical equal-port config remains readable for a zero-surprise migration.
rm -rf "$ETC"; mkdir -p "$ETC"
cat >"$ETC/server.env" <<'EOF'
WBD_LISTEN_IP=0.0.0.0
WBD_FRONT_PORT=4443
WBD_RAW_PORT=4443
WBD_SERVER_NAME=edge.example
WBD_DECOY_TARGET=www.cloudflare.com:443
WBD_ROUTE_KEY=0123456789abcdef
WBD_USERNAME=wbd
WBD_PASSWORD=secret
EOF
run_manager show-config >/tmp/wbd-settings-legacy-equal.log

# Historical different-port config cannot be guessed. The explicit WBD_PORT set
# both resolves the migration and removes obsolete keys atomically.
sed -i 's/WBD_RAW_PORT=4443/WBD_RAW_PORT=5555/' "$ETC/server.env"
if run_manager show-config >/tmp/wbd-settings-legacy-diff.log 2>&1; then
    echo 'different legacy public ports unexpectedly accepted' >&2; exit 1
fi
grep -q 'legacy WBD_FRONT_PORT/WBD_RAW_PORT differ' /tmp/wbd-settings-legacy-diff.log
run_manager set WBD_PORT 8443 >/dev/null
grep -q "^WBD_PORT='8443'$" "$ETC/server.env"
if grep -Eq '^WBD_(FRONT|RAW)_PORT=' "$ETC/server.env"; then
    echo 'explicit WBD_PORT migration left legacy keys behind' >&2; exit 1
fi

echo 'WBD_LINUX_SERVER_SETTINGS_PASS shared_port=1 migration=fail_closed secrets=redacted'
