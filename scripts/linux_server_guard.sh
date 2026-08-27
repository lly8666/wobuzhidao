#!/bin/sh
set -eu

BACKEND=auto
FRONT_PORT=40443
RAW_PORT=40000
STATE=/run/wbd/server-firewall.state
NFT_INPUT=
FIREWALL_SCRIPT=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/linux_server_firewall.sh

usage() {
    cat >&2 <<'EOF'
usage: linux_server_guard.sh [firewall options] -- COMMAND [ARG...]
  --backend auto|nft|iptables
  --front-port PORT
  --raw-port PORT
  --state PATH
  --nft-input FAMILY:TABLE:CHAIN

Applies the WBD-owned Linux server firewall rules, runs COMMAND as the server
supervisor/process, forwards TERM/INT/HUP, then removes only WBD-owned firewall
state on every normal/signal exit. A subsequent start also pre-cleans stale WBD
rules left by a prior crash.
EOF
}

fw_args=
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
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

build_fw_args() {
    set -- --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE"
    if [ -n "$NFT_INPUT" ]; then
        set -- "$@" --nft-input "$NFT_INPUT"
    fi
    # Print one shell-quoted argument per line is unnecessary here; callers use
    # the same simple validated values. The guard invokes the helper explicitly
    # below so no config text is ever eval'd or sourced as root.
    printf '%s\n' "$@"
}

cleanup_done=0
child=
cleanup() {
    [ "$cleanup_done" -eq 0 ] || return 0
    cleanup_done=1
    set +e
    if [ -n "$child" ] && kill -0 "$child" 2>/dev/null; then
        kill -TERM "$child" 2>/dev/null || true
        wait "$child" 2>/dev/null || true
    fi
    if [ -n "$NFT_INPUT" ]; then
        "$FIREWALL_SCRIPT" cleanup --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE" --nft-input "$NFT_INPUT"
    else
        "$FIREWALL_SCRIPT" cleanup --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE"
    fi
    rc=$?
    if [ "$rc" -ne 0 ]; then
        sleep 1
        if [ -n "$NFT_INPUT" ]; then
            "$FIREWALL_SCRIPT" cleanup --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE" --nft-input "$NFT_INPUT" || true
        else
            "$FIREWALL_SCRIPT" cleanup --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE" || true
        fi
    fi
}
trap cleanup EXIT INT TERM HUP

if [ -n "$NFT_INPUT" ]; then
    "$FIREWALL_SCRIPT" apply --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE" --nft-input "$NFT_INPUT"
else
    "$FIREWALL_SCRIPT" apply --backend "$BACKEND" --front-port "$FRONT_PORT" --raw-port "$RAW_PORT" --state "$STATE"
fi

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
