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

run_manager() { env WBD_ETC="$ETC" WBD_PREFIX="$PREFIX" WBD_RUN="$RUN" sh "$MANAGER" "$@"; }

[ "$(id -u)" -eq 0 ] || { echo 'linux_server_settings_test.sh requires root' >&2; exit 1; }

# A fresh config exposes one public port and no historical split-port knobs.
run_manager set WBD_PORT 443 >/tmp/wbd-settings-set.log
CONFIG=$ETC/server.env
grep -q "^WBD_PORT='443'$" "$CONFIG"
if grep -Eq '^WBD_(FRONT|RAW)_PORT=' "$CONFIG"; then
    echo 'fresh config still exposes split public ports' >&2; cat "$CONFIG" >&2; exit 1
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
if printf '%s\n' "$out" | grep -q 'very-secret-password'; then echo 'show-config leaked password' >&2; exit 1; fi

# Invalid or obsolete public-port settings fail instead of diverging front/raw.
if run_manager set WBD_PORT 0 >/tmp/wbd-settings-bad.log 2>&1; then echo 'WBD_PORT=0 unexpectedly accepted' >&2; exit 1; fi
if run_manager set WBD_PORT 65536 >/tmp/wbd-settings-bad.log 2>&1; then echo 'WBD_PORT=65536 unexpectedly accepted' >&2; exit 1; fi
if run_manager set WBD_FRONT_PORT 443 >/tmp/wbd-settings-bad.log 2>&1; then echo 'obsolete WBD_FRONT_PORT unexpectedly accepted' >&2; exit 1; fi

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
if run_manager show-config >/tmp/wbd-settings-legacy-diff.log 2>&1; then echo 'different legacy public ports unexpectedly accepted' >&2; exit 1; fi
grep -q 'legacy WBD_FRONT_PORT/WBD_RAW_PORT differ' /tmp/wbd-settings-legacy-diff.log
run_manager set WBD_PORT 8443 >/dev/null
grep -q "^WBD_PORT='8443'$" "$ETC/server.env"
if grep -Eq '^WBD_(FRONT|RAW)_PORT=' "$ETC/server.env"; then echo 'explicit WBD_PORT migration left legacy keys behind' >&2; exit 1; fi

# ADR-0012 product mode resolves wildcard config to one concrete shared raw
# FakeTCP mux ingress. The mux may host 1..4 independent same-flow Transport
# Lane associations for one Logical Tunnel; no parallel kernel TCP front runs.
rm -rf "$PREFIX" "$RUN"; mkdir -p "$PREFIX/bin" "$RUN/tickets" "$ROOT/fakebin" "$ETC"
sed -i 's/^WBD_LISTEN_IP=.*/WBD_LISTEN_IP=0.0.0.0/' "$ETC/server.env"
cat >"$ROOT/fakebin/ip" <<'EOF'
#!/bin/sh
printf '%s\n' '1.1.1.1 via 10.77.0.1 dev eth0 src 10.77.0.9 uid 0'
EOF
chmod +x "$ROOT/fakebin/ip"
for f in wbd-platform-proxy-server wbd-game-lane-server wbd-link-server-mux; do
    cat >"$PREFIX/bin/$f" <<'EOF'
#!/bin/sh
exec sleep 30
EOF
    chmod +x "$PREFIX/bin/$f"
done
# Manager only constructs these paths for the guard-owned mux command.
: >"$ETC/front.pem"; : >"$ETC/front.key"; : >"$ETC/dtls.pem"; : >"$ETC/dtls.key"
cat >"$PREFIX/bin/linux_server_guard.sh" <<'EOF'
#!/bin/sh
: "${WBD_TEST_GUARD_LOG:?}"
printf '%s\n' "$@" >"$WBD_TEST_GUARD_LOG"
exit 0
EOF
chmod +x "$PREFIX/bin/linux_server_guard.sh"
GUARD_LOG=$ROOT/guard.args
export WBD_TEST_GUARD_LOG=$GUARD_LOG
OLD_PATH=$PATH; PATH=$ROOT/fakebin:$PATH; export PATH
run_manager run >/tmp/wbd-settings-wildcard.log
PATH=$OLD_PATH; export PATH; unset WBD_TEST_GUARD_LOG
grep -q '^10.77.0.9:8443$' "$GUARD_LOG"
if grep -q '^0.0.0.0:8443$' "$GUARD_LOG"; then echo 'wildcard WBD_LISTEN_IP leaked into raw FakeTCP --listen' >&2; cat "$GUARD_LOG" >&2; exit 1; fi
grep -q 'public_raw=10.77.0.9:8443' /tmp/wbd-settings-wildcard.log
grep -q 'max_tunnel_lanes=4' /tmp/wbd-settings-wildcard.log
grep -q '^--front-cert$' "$GUARD_LOG"
grep -q '^--server-name$' "$GUARD_LOG"
grep -q '^--ticket-dir$' "$GUARD_LOG"
if grep -Eq 'wbd-reality-front"[[:space:]]+server' "$MANAGER"; then echo 'product manager still starts a parallel Reality TCP listener' >&2; exit 1; fi

# The systemd unit must rely on normal control-group termination and cap storms.
if grep -q 'ExecStop=/bin/kill' "$MANAGER"; then echo 'manager still emits fragile ExecStop=/bin/kill $MAINPID' >&2; exit 1; fi
grep -q 'StartLimitBurst=5' "$MANAGER"
grep -q 'KillMode=control-group' "$MANAGER"

echo 'WBD_LINUX_SERVER_SETTINGS_PASS shared_public_raw_mux=1 max_tunnel_lanes=4 per_lane_same_flow=1 wildcard_raw_ipv4=resolved restart_storm=capped migration=fail_closed secrets=redacted'
