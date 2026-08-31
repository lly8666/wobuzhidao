#!/bin/sh
set -eu

BACKEND=auto
PORT=40443
PORT_SET=0
STATE=/run/wbd/server-firewall.state
NFT_INPUT=
LEGACY_FRONT_PORT=
LEGACY_RAW_PORT=
FIREWALL_SCRIPT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/linux_server_firewall.sh

usage() {
    cat >&2 <<'EOF'
usage: linux_server_guard.sh [firewall options] -- COMMAND [ARG...]
  --backend auto|nft|iptables
  --port PORT
  --state PATH
  --nft-input FAMILY:TABLE:CHAIN

Applies the WBD-owned single-public-port firewall rules, runs COMMAND as the
server supervisor/process, forwards TERM/INT/HUP, then removes only WBD-owned
firewall state on every normal/signal exit. A subsequent start also pre-cleans
stale WBD rules left by a prior crash.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --port) PORT=$2; PORT_SET=1; shift 2 ;;
        # Hidden compatibility for a bundle upgraded in place. Both old names
        # must already describe the same single-flow port.
        --front-port) LEGACY_FRONT_PORT=$2; shift 2 ;;
        --raw-port) LEGACY_RAW_PORT=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --nft-input) NFT_INPUT=$2; shift 2 ;;
        --) shift; break ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown guard argument: $1" >&2; usage; exit 2 ;;
    esac
done
[ $# -gt 0 ] || { echo 'server COMMAND is required after --' >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo 'linux_server_guard.sh requires root' >&2; exit 1; }

if [ -n "$LEGACY_FRONT_PORT" ] || [ -n "$LEGACY_RAW_PORT" ]; then
    [ -n "$LEGACY_FRONT_PORT" ] && [ -n "$LEGACY_RAW_PORT" ] || {
        echo 'legacy --front-port/--raw-port must both be supplied; use --port' >&2; exit 2;
    }
    [ "$LEGACY_FRONT_PORT" = "$LEGACY_RAW_PORT" ] || {
        echo 'single-flow guard requires one public port; use --port PORT' >&2; exit 2;
    }
    if [ "$PORT_SET" -eq 1 ] && [ "$PORT" != "$LEGACY_FRONT_PORT" ]; then
        echo '--port conflicts with legacy port arguments' >&2; exit 2
    fi
    PORT=$LEGACY_FRONT_PORT
fi
case "$PORT" in *[!0-9]*|'') echo "invalid port: $PORT" >&2; exit 2;; esac
[ "$PORT" -gt 0 ] && [ "$PORT" -le 65535 ] || { echo "invalid port: $PORT" >&2; exit 2; }

cleanup_done=0
child=
run_firewall() {
    action=$1
    if [ -n "$NFT_INPUT" ]; then
        sh "$FIREWALL_SCRIPT" "$action" --backend "$BACKEND" --port "$PORT" --state "$STATE" --nft-input "$NFT_INPUT"
    else
        sh "$FIREWALL_SCRIPT" "$action" --backend "$BACKEND" --port "$PORT" --state "$STATE"
    fi
}

cleanup() {
    [ "$cleanup_done" -eq 0 ] || return 0
    cleanup_done=1
    set +e
    if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
        kill -TERM "$child" 2>/dev/null || true
        wait "$child" 2>/dev/null || true
    fi
    run_firewall cleanup
    rc=$?
    if [ "$rc" -ne 0 ]; then
        sleep 1
        run_firewall cleanup || true
    fi
}
trap cleanup EXIT INT TERM HUP

run_firewall apply
"$@" &
child=$!
set +e
wait "$child"
rc=$?
set -e
child=
cleanup
trap - EXIT INT TERM HUP
exit "$rc"
