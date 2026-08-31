#!/bin/sh
set -eu

BACKEND=auto
FRONT_PORT=40443
RAW_PORT=40000
PUBLIC_PORT=
STATE=/run/wbd/server-firewall.state
NFT_INPUT=
FIREWALL_SCRIPT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/linux_server_firewall.sh

usage() {
    cat >&2 <<'EOF'
usage: linux_server_guard.sh [firewall options] -- COMMAND [ARG...]
  --backend auto|nft|iptables
  --port PORT             V3 single public WBD/FakeTCP port
  --front-port PORT       legacy V2 compatibility
  --raw-port PORT         legacy V2 compatibility
  --state PATH
  --nft-input FAMILY:TABLE:CHAIN

Applies WBD-owned Linux server firewall state, runs COMMAND, forwards
TERM/INT/HUP, then removes only WBD-owned state on normal/signal exit. V3 uses
--port so there is exactly one public WBD port. A subsequent start pre-cleans
stale WBD rules left by a prior crash.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --port) PUBLIC_PORT=$2; FRONT_PORT=$2; RAW_PORT=$2; shift 2 ;;
        --front-port) FRONT_PORT=$2; shift 2 ;;
        --raw-port) RAW_PORT=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --nft-input) NFT_INPUT=$2; shift 2 ;;
        --) shift; break ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown guard argument: $1" >&2; usage; exit 2 ;;
    esac
done
[ $# -gt 0 ] || { echo 'server COMMAND is required after --' >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo 'linux_server_guard.sh requires root' >&2; exit 1; }
[ "$FRONT_PORT" = "$RAW_PORT" ] && PUBLIC_PORT=$RAW_PORT

cleanup_done=0
child=
run_firewall() {
    action=$1
    set -- sh "$FIREWALL_SCRIPT" "$action" --backend "$BACKEND"
    if [ -n "$PUBLIC_PORT" ]; then
        set -- "$@" --port "$PUBLIC_PORT"
    else
        set -- "$@" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT"
    fi
    set -- "$@" --state "$STATE"
    [ -z "$NFT_INPUT" ] || set -- "$@" --nft-input "$NFT_INPUT"
    "$@"
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
