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

# Current product authority: single-flow is PER Transport Lane. One Logical
# Tunnel may own 1..4 complete lanes. Linux exposes one public raw mux and wires
# private LINK -> Game/race -> one shared TUN gateway -> one host NAT. The
# settings fixture records argv for every product process instead of enforcing
# the withdrawn ADR-0014 global-one-lane topology.
rm -rf "$PREFIX" "$RUN"; mkdir -p "$PREFIX/bin" "$RUN/tickets" "$ROOT/fakebin" "$ETC"
sed -i 's/^WBD_LISTEN_IP=.*/WBD_LISTEN_IP=0.0.0.0/' "$ETC/server.env"
cat >"$ROOT/fakebin/ip" <<'EOF'
#!/bin/sh
printf '%s\n' '1.1.1.1 via 10.77.0.1 dev eth0 src 10.77.0.9 uid 0'
EOF
chmod +x "$ROOT/fakebin/ip"

cat >"$PREFIX/bin/wbd-ip-gateway-shared" <<'EOF'
#!/bin/sh
: "${WBD_TEST_GATEWAY_LOG:?}"
printf '%s\n' "$@" >"$WBD_TEST_GATEWAY_LOG"
exec sleep 30
EOF
cat >"$PREFIX/bin/wbd-game-lane-server" <<'EOF'
#!/bin/sh
: "${WBD_TEST_GAME_LOG:?}"
printf '%s\n' "$@" >"$WBD_TEST_GAME_LOG"
exec sleep 30
EOF
cat >"$PREFIX/bin/wbd-link-server-mux" <<'EOF'
#!/bin/sh
: "${WBD_TEST_LINK_LOG:?}"
printf '%s\n' "$@" >"$WBD_TEST_LINK_LOG"
exec sleep 30
EOF
cat >"$PREFIX/bin/wbd-platform-proxy-server" <<'EOF'
#!/bin/sh
echo 'product run unexpectedly started legacy platform proxy' >&2
exit 98
EOF
chmod +x "$PREFIX/bin/wbd-ip-gateway-shared" "$PREFIX/bin/wbd-game-lane-server" "$PREFIX/bin/wbd-link-server-mux" "$PREFIX/bin/wbd-platform-proxy-server"

# Manager only constructs these paths for the guard-owned mux command.
: >"$ETC/front.pem"; : >"$ETC/front.key"; : >"$ETC/dtls.pem"; : >"$ETC/dtls.key"
cat >"$PREFIX/bin/linux_server_guard.sh" <<'EOF'
#!/bin/sh
: "${WBD_TEST_GUARD_LOG:?}"
printf '%s\n' "$@" >"$WBD_TEST_GUARD_LOG"
# Give the three private product stubs a deterministic scheduling window to
# record argv before manager cleanup follows this main guard exit.
sleep 1
exit 0
EOF
chmod +x "$PREFIX/bin/linux_server_guard.sh"

GUARD_LOG=$ROOT/guard.args
GATEWAY_LOG=$ROOT/gateway.args
GAME_LOG=$ROOT/game.args
LINK_LOG=$ROOT/link.args
export WBD_TEST_GUARD_LOG=$GUARD_LOG WBD_TEST_GATEWAY_LOG=$GATEWAY_LOG WBD_TEST_GAME_LOG=$GAME_LOG WBD_TEST_LINK_LOG=$LINK_LOG
OLD_PATH=$PATH; PATH=$ROOT/fakebin:$PATH; export PATH
run_manager run >/tmp/wbd-settings-wildcard.log
PATH=$OLD_PATH; export PATH
unset WBD_TEST_GUARD_LOG WBD_TEST_GATEWAY_LOG WBD_TEST_GAME_LOG WBD_TEST_LINK_LOG

# Public raw mux uses a concrete source IPv4 and remains the only product public
# listener. Reality-like setup is carried inside each FakeTCP association.
grep -q '^10.77.0.9:8443$' "$GUARD_LOG"
if grep -q '^0.0.0.0:8443$' "$GUARD_LOG"; then echo 'wildcard WBD_LISTEN_IP leaked into raw FakeTCP --listen' >&2; cat "$GUARD_LOG" >&2; exit 1; fi
grep -q 'public_raw=10.77.0.9:8443' /tmp/wbd-settings-wildcard.log
grep -q 'max_tunnel_lanes=4' /tmp/wbd-settings-wildcard.log
grep -q 'shared_tun=127.0.0.1:49100' /tmp/wbd-settings-wildcard.log
grep -q 'game=127.0.0.1:48500' /tmp/wbd-settings-wildcard.log
grep -q '^--front-cert$' "$GUARD_LOG"
grep -q '^--server-name$' "$GUARD_LOG"
grep -q '^--ticket-dir$' "$GUARD_LOG"
grep -q '^--tunnel-pool$' "$GUARD_LOG"
if grep -Eq 'wbd-reality-front"[[:space:]]+server' "$MANAGER"; then echo 'product manager still starts a parallel Reality TCP listener' >&2; exit 1; fi

# Shared gateway owns the one Linux TUN/NAT product boundary.
grep -q '^-listen$' "$GATEWAY_LOG"
grep -q '^127.0.0.1:49100$' "$GATEWAY_LOG"
grep -q '^-lease-prefix$' "$GATEWAY_LOG"
grep -q '^10.66.0.0/16$' "$GATEWAY_LOG"
grep -q '^-tun-if$' "$GATEWAY_LOG"
grep -q '^wbdg0$' "$GATEWAY_LOG"
grep -q '^-firewall-helper$' "$GATEWAY_LOG"
grep -q 'linux_shared_tun_firewall.sh$' "$GATEWAY_LOG"

# Game/race is a product hop with a four-lane ceiling, fed by LINK and forwarding
# to the shared TUN gateway.
grep -q '^-listen$' "$GAME_LOG"
grep -q '^127.0.0.1:48500$' "$GAME_LOG"
grep -q '^-service$' "$GAME_LOG"
grep -q '^127.0.0.1:49100$' "$GAME_LOG"
grep -q '^-max-lanes$' "$GAME_LOG"
grep -q '^4$' "$GAME_LOG"

# LINK feeds Game for raced packet service and also knows the authenticated raw
# IP shared gateway boundary.
grep -q '^-listen$' "$LINK_LOG"
grep -q '^127.0.0.1:47000$' "$LINK_LOG"
grep -q '^-service$' "$LINK_LOG"
grep -q '^127.0.0.1:48500$' "$LINK_LOG"
grep -q '^-raw-ip-service$' "$LINK_LOG"
grep -q '^127.0.0.1:49100$' "$LINK_LOG"

run_start=$(awk '/^run_server\(\) \{/{on=1} /^uninstall_files\(\) \{/{on=0} on{print}' "$MANAGER")
printf '%s\n' "$run_start" | grep -Fq 'wbd-ip-gateway-shared" -listen "$WBD_SHARED_TUN_LISTEN"'
printf '%s\n' "$run_start" | grep -Fq 'wbd-game-lane-server" -listen "$WBD_GAME_LISTEN" -service "$WBD_SHARED_TUN_LISTEN"'
printf '%s\n' "$run_start" | grep -Fq 'wbd-link-server-mux" -listen "$WBD_LINK_LISTEN" -service "$WBD_GAME_LISTEN" -raw-ip-service "$WBD_SHARED_TUN_LISTEN"'
if printf '%s\n' "$run_start" | grep -q 'wbd-platform-proxy-server'; then echo 'product manager still starts legacy platform proxy' >&2; exit 1; fi

# The systemd unit must rely on normal control-group termination and cap storms.
if grep -q 'ExecStop=/bin/kill' "$MANAGER"; then echo 'manager still emits fragile ExecStop=/bin/kill $MAINPID' >&2; exit 1; fi
grep -q 'StartLimitBurst=5' "$MANAGER"
grep -q 'KillMode=control-group' "$MANAGER"

echo 'WBD_LINUX_SERVER_SETTINGS_PASS shared_public_raw_mux=1 per_lane_single_flow=1 max_tunnel_lanes=4 game_product=1 shared_tun=1 host_nat=1 wildcard_raw_ipv4=resolved restart_storm=capped migration=fail_closed secrets=redacted'
