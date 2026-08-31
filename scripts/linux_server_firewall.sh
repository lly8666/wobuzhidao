#!/bin/sh
set -eu

ACTION=status
BACKEND=auto
PORT=40443
PORT_SET=0
STATE=/run/wbd/server-firewall.state
NFT_INPUT=
LEGACY_FRONT_PORT=
LEGACY_RAW_PORT=

usage() {
    cat >&2 <<'EOF'
usage: linux_server_firewall.sh apply|cleanup|status|render [options]
  --backend auto|nft|iptables
  --port PORT             single public WBD TCP-shaped port (default 40443)
  --state PATH            WBD-owned state file
  --nft-input FAMILY:TABLE:CHAIN
                         explicit existing nft input chain when auto-detection
                         cannot identify the host firewall

V2.3 exposes exactly one public TCP-shaped flow. The helper never flushes or
restores the host ruleset. It inserts one WBD-owned input accept for that port
and an exact kernel-RST suppression rule, and cleanup removes only WBD-owned
state. Historical front/raw rule names are recognized only for upgrade cleanup.
The platform relay is userspace egress, so this helper adds no FORWARD or
MASQUERADE rules.
EOF
}

[ $# -gt 0 ] && { ACTION=$1; shift; }
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --port) PORT=$2; PORT_SET=1; shift 2 ;;
        # Hidden upgrade compatibility. New callers must use --port. Applying
        # with legacy arguments is accepted only when both describe one port.
        --front-port) LEGACY_FRONT_PORT=$2; shift 2 ;;
        --raw-port) LEGACY_RAW_PORT=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --nft-input) NFT_INPUT=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

case "$ACTION" in apply|cleanup|status|render) ;; *) usage; exit 2 ;; esac
case "$BACKEND" in auto|nft|iptables) ;; *) echo "invalid backend: $BACKEND" >&2; exit 2 ;; esac

if [ -n "$LEGACY_FRONT_PORT" ] || [ -n "$LEGACY_RAW_PORT" ]; then
    if [ "$ACTION" != cleanup ]; then
        [ -n "$LEGACY_FRONT_PORT" ] && [ -n "$LEGACY_RAW_PORT" ] || {
            echo 'legacy --front-port/--raw-port must both be supplied; use --port' >&2; exit 2;
        }
        [ "$LEGACY_FRONT_PORT" = "$LEGACY_RAW_PORT" ] || {
            echo 'single-flow firewall requires one public port; use --port PORT' >&2; exit 2;
        }
        if [ "$PORT_SET" -eq 1 ] && [ "$PORT" != "$LEGACY_FRONT_PORT" ]; then
            echo '--port conflicts with legacy port arguments' >&2; exit 2
        fi
        PORT=$LEGACY_FRONT_PORT
    fi
fi

validate_port() {
    p=$1
    case "$p" in *[!0-9]*|'') echo "invalid port: $p" >&2; exit 2 ;; esac
    [ "$p" -gt 0 ] && [ "$p" -le 65535 ] || { echo "invalid port: $p" >&2; exit 2; }
}
validate_port "$PORT"
[ -z "$LEGACY_FRONT_PORT" ] || validate_port "$LEGACY_FRONT_PORT"
[ -z "$LEGACY_RAW_PORT" ] || validate_port "$LEGACY_RAW_PORT"

need_root() {
    [ "$(id -u)" -eq 0 ] || { echo "$ACTION requires root" >&2; exit 1; }
}

state_get() {
    key=$1
    [ -f "$STATE" ] || return 0
    sed -n "s/^${key}=//p" "$STATE" | head -n 1
}

choose_backend() {
    if [ "$BACKEND" != auto ]; then
        printf '%s\n' "$BACKEND"
        return
    fi
    if command -v nft >/dev/null 2>&1; then
        printf 'nft\n'
    elif command -v iptables >/dev/null 2>&1; then
        printf 'iptables\n'
    else
        echo 'neither nft nor iptables is installed' >&2
        exit 1
    fi
}

parse_nft_spec() {
    spec=$1
    oldifs=$IFS; IFS=:
    # shellcheck disable=SC2086
    set -- $spec
    IFS=$oldifs
    [ $# -eq 3 ] || return 1
    printf '%s %s %s\n' "$1" "$2" "$3"
}

nft_chain_exists() {
    nft list chain "$1" "$2" "$3" >/dev/null 2>&1
}

find_nft_input() {
    if [ -n "$NFT_INPUT" ]; then
        parse_nft_spec "$NFT_INPUT"
        return
    fi
    for spec in 'inet:filter:input' 'inet:fw4:input' 'ip:filter:INPUT' 'ip:filter:input'; do
        values=$(parse_nft_spec "$spec") || continue
        # shellcheck disable=SC2086
        if nft_chain_exists $values; then
            printf '%s\n' "$values"
            return
        fi
    done
    rules=$(nft list ruleset 2>/dev/null || true)
    if printf '%s\n' "$rules" | grep -Eq 'hook[[:space:]]+input'; then
        echo 'unable to identify existing nft input chain; pass --nft-input FAMILY:TABLE:CHAIN' >&2
        return 2
    fi
    return 1
}

nft_delete_comment() {
    family=$1 table=$2 chain=$3 marker=$4
    nft_chain_exists "$family" "$table" "$chain" || return 0
    handles=$(nft -a list chain "$family" "$table" "$chain" 2>/dev/null | awk -v marker="$marker" '
        index($0, "comment \"" marker "\"") {
            for (i=1; i<=NF; i++) if ($i == "handle") print $(i+1)
        }')
    for handle in $handles; do
        nft delete rule "$family" "$table" "$chain" handle "$handle" 2>/dev/null || true
    done
}

nft_cleanup_chain() {
    family=$1 table=$2 chain=$3
    nft_delete_comment "$family" "$table" "$chain" wbd-server-public
    # Upgrade cleanup for pre-single-flow bundles.
    nft_delete_comment "$family" "$table" "$chain" wbd-server-front
    nft_delete_comment "$family" "$table" "$chain" wbd-server-raw
}

nft_cleanup() {
    family=$(state_get NFT_INPUT_FAMILY)
    table=$(state_get NFT_INPUT_TABLE)
    chain=$(state_get NFT_INPUT_CHAIN)
    if [ -n "$family" ] && [ -n "$table" ] && [ -n "$chain" ]; then
        nft_cleanup_chain "$family" "$table" "$chain"
    else
        for spec in 'inet filter input' 'inet fw4 input' 'ip filter INPUT' 'ip filter input'; do
            # shellcheck disable=SC2086
            nft_cleanup_chain $spec
        done
    fi
    nft delete table inet wbd_server 2>/dev/null || true
}

nft_apply() {
    command -v nft >/dev/null 2>&1 || { echo 'nft not installed' >&2; return 1; }
    nft_cleanup
    input_values=
    if input_values=$(find_nft_input); then
        # shellcheck disable=SC2086
        set -- $input_values
        in_family=$1 in_table=$2 in_chain=$3
        nft insert rule "$in_family" "$in_table" "$in_chain" tcp dport "$PORT" accept comment wbd-server-public
    else
        rc=$?
        [ "$rc" -eq 1 ] || return "$rc"
        in_family= in_table= in_chain=
    fi

    # The public WBD flow is raw TCP-shaped and has no kernel TCP socket.
    # Suppress only kernel RST packets sourced from this one public port.
    nft -f - <<EOF
add table inet wbd_server
add chain inet wbd_server output { type filter hook output priority -300; policy accept; }
add rule inet wbd_server output tcp sport $PORT tcp flags rst / rst drop comment "wbd-server-rst"
EOF
    mkdir -p "$(dirname "$STATE")"
    {
        echo BACKEND=nft
        echo PORT="$PORT"
        echo NFT_INPUT_FAMILY="$in_family"
        echo NFT_INPUT_TABLE="$in_table"
        echo NFT_INPUT_CHAIN="$in_chain"
    } >"$STATE"
    chmod 600 "$STATE" 2>/dev/null || true
}

iptables_rule_delete_all() {
    chain=$1; shift
    while iptables -C "$chain" "$@" >/dev/null 2>&1; do
        iptables -D "$chain" "$@" >/dev/null 2>&1 || break
    done
}

iptables_cleanup() {
    command -v iptables >/dev/null 2>&1 || return 0
    state_port=$(state_get PORT)
    [ -n "$state_port" ] || state_port=$PORT
    old_front=$(state_get FRONT_PORT)
    old_raw=$(state_get RAW_PORT)
    [ -n "$old_front" ] || old_front=${LEGACY_FRONT_PORT:-$state_port}
    [ -n "$old_raw" ] || old_raw=${LEGACY_RAW_PORT:-$state_port}

    iptables_rule_delete_all INPUT -p tcp --dport "$state_port" -m comment --comment wbd-server-public -j ACCEPT
    # Upgrade cleanup for historical dual-rule ownership.
    iptables_rule_delete_all INPUT -p tcp --dport "$old_front" -m comment --comment wbd-server-front -j ACCEPT
    iptables_rule_delete_all INPUT -p tcp --dport "$old_raw" -m comment --comment wbd-server-raw -j ACCEPT
    iptables_rule_delete_all OUTPUT -p tcp --sport "$state_port" --tcp-flags RST RST -m comment --comment wbd-server-rst -j DROP
    if [ "$old_raw" != "$state_port" ]; then
        iptables_rule_delete_all OUTPUT -p tcp --sport "$old_raw" --tcp-flags RST RST -m comment --comment wbd-server-rst -j DROP
    fi
}

iptables_apply() {
    command -v iptables >/dev/null 2>&1 || { echo 'iptables not installed' >&2; return 1; }
    iptables_cleanup
    iptables -I INPUT 1 -p tcp --dport "$PORT" -m comment --comment wbd-server-public -j ACCEPT
    iptables -I OUTPUT 1 -p tcp --sport "$PORT" --tcp-flags RST RST -m comment --comment wbd-server-rst -j DROP
    mkdir -p "$(dirname "$STATE")"
    {
        echo BACKEND=iptables
        echo PORT="$PORT"
    } >"$STATE"
    chmod 600 "$STATE" 2>/dev/null || true
}

cleanup_all() {
    saved=$(state_get BACKEND)
    case "$saved" in
        nft) nft_cleanup ;;
        iptables) iptables_cleanup ;;
        *)
            command -v nft >/dev/null 2>&1 && nft_cleanup || true
            command -v iptables >/dev/null 2>&1 && iptables_cleanup || true
            ;;
    esac
    rm -f "$STATE"
}

case "$ACTION" in
    render)
        selected=$(choose_backend)
        echo "WBD_LINUX_SERVER_FIREWALL_PLAN backend=$selected port=$PORT single_flow=1"
        echo 'ownership: one WBD-marked public-port accept + exact raw-RST suppression only'
        echo 'no FORWARD/MASQUERADE/global ruleset restore'
        ;;
    apply)
        need_root
        selected=$(choose_backend)
        cleanup_all
        case "$selected" in
            nft) nft_apply ;;
            iptables) iptables_apply ;;
        esac
        echo "WBD_LINUX_SERVER_FIREWALL_READY backend=$selected port=$PORT single_flow=1"
        ;;
    cleanup)
        need_root
        cleanup_all
        echo 'WBD_LINUX_SERVER_FIREWALL_CLEANUP_PASS'
        ;;
    status)
        saved=$(state_get BACKEND)
        saved_port=$(state_get PORT)
        if [ -n "$saved" ]; then
            echo "WBD_LINUX_SERVER_FIREWALL_STATUS active=1 backend=$saved port=${saved_port:-unknown} state=$STATE"
        else
            echo "WBD_LINUX_SERVER_FIREWALL_STATUS active=0 state=$STATE"
        fi
        ;;
esac
