#!/bin/sh
set -eu

ACTION=status
BACKEND=auto
FRONT_PORT=40443
RAW_PORT=40000
STATE=/run/wbd/server-firewall.state
NFT_INPUT=

usage() {
    cat >&2 <<'EOF'
usage: linux_server_firewall.sh apply|cleanup|status|render [options]
  --backend auto|nft|iptables
  --front-port PORT       Reality setup/admission TCP port (default 40443)
  --raw-port PORT         WBD raw FakeTCP destination/source port (default 40000)
  --state PATH            WBD-owned state file
  --nft-input FAMILY:TABLE:CHAIN
                         explicit existing nft input chain when auto-detection
                         cannot identify the host firewall

The helper never flushes or restores the host ruleset. It inserts only
WBD-owned input accepts and an exact FakeTCP RST suppression rule, and cleanup
removes only those rules. The platform relay is userspace egress, so this helper
does not add FORWARD or MASQUERADE rules.
EOF
}

[ $# -gt 0 ] && { ACTION=$1; shift; }
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --front-port) FRONT_PORT=$2; shift 2 ;;
        --raw-port) RAW_PORT=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --nft-input) NFT_INPUT=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

case "$ACTION" in apply|cleanup|status|render) ;; *) usage; exit 2 ;; esac
case "$BACKEND" in auto|nft|iptables) ;; *) echo "invalid backend: $BACKEND" >&2; exit 2 ;; esac
for p in "$FRONT_PORT" "$RAW_PORT"; do
    case "$p" in *[!0-9]*|'') echo "invalid port: $p" >&2; exit 2 ;; esac
    [ "$p" -gt 0 ] && [ "$p" -le 65535 ] || { echo "invalid port: $p" >&2; exit 2; }
done

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
    # If there is no existing input hook then there is no nft input firewall to
    # punch through. If an unknown input hook exists, fail rather than pretend a
    # separate accept base-chain can override an earlier drop verdict.
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

nft_cleanup() {
    family=$(state_get NFT_INPUT_FAMILY)
    table=$(state_get NFT_INPUT_TABLE)
    chain=$(state_get NFT_INPUT_CHAIN)
    if [ -n "$family" ] && [ -n "$table" ] && [ -n "$chain" ]; then
        nft_delete_comment "$family" "$table" "$chain" wbd-server-front
        nft_delete_comment "$family" "$table" "$chain" wbd-server-raw
    else
        # Crash-recovery best effort for the common chains used by auto mode.
        for spec in 'inet filter input' 'inet fw4 input' 'ip filter INPUT' 'ip filter input'; do
            # shellcheck disable=SC2086
            nft_delete_comment $spec wbd-server-front
            # shellcheck disable=SC2086
            nft_delete_comment $spec wbd-server-raw
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
        nft insert rule "$in_family" "$in_table" "$in_chain" tcp dport "$FRONT_PORT" accept comment wbd-server-front
        nft insert rule "$in_family" "$in_table" "$in_chain" tcp dport "$RAW_PORT" accept comment wbd-server-raw
    else
        rc=$?
        [ "$rc" -eq 1 ] || return "$rc"
        in_family= in_table= in_chain=
    fi

    # A raw TCP-shaped WBD association has no kernel TCP socket. Suppress only
    # kernel RST packets sourced from the WBD raw port, in a dedicated table;
    # DROP is terminal across later base chains and cleanup can delete the table.
    nft -f - <<EOF
add table inet wbd_server
add chain inet wbd_server output { type filter hook output priority -300; policy accept; }
add rule inet wbd_server output tcp sport $RAW_PORT tcp flags rst / rst drop comment "wbd-server-rst"
EOF
    mkdir -p "$(dirname "$STATE")"
    {
        echo BACKEND=nft
        echo FRONT_PORT="$FRONT_PORT"
        echo RAW_PORT="$RAW_PORT"
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
    fp=$(state_get FRONT_PORT); [ -n "$fp" ] || fp=$FRONT_PORT
    rp=$(state_get RAW_PORT); [ -n "$rp" ] || rp=$RAW_PORT
    iptables_rule_delete_all INPUT -p tcp --dport "$fp" -m comment --comment wbd-server-front -j ACCEPT
    iptables_rule_delete_all INPUT -p tcp --dport "$rp" -m comment --comment wbd-server-raw -j ACCEPT
    iptables_rule_delete_all OUTPUT -p tcp --sport "$rp" --tcp-flags RST RST -m comment --comment wbd-server-rst -j DROP
}

iptables_apply() {
    command -v iptables >/dev/null 2>&1 || { echo 'iptables not installed' >&2; return 1; }
    iptables_cleanup
    iptables -I INPUT 1 -p tcp --dport "$RAW_PORT" -m comment --comment wbd-server-raw -j ACCEPT
    iptables -I INPUT 1 -p tcp --dport "$FRONT_PORT" -m comment --comment wbd-server-front -j ACCEPT
    iptables -I OUTPUT 1 -p tcp --sport "$RAW_PORT" --tcp-flags RST RST -m comment --comment wbd-server-rst -j DROP
    mkdir -p "$(dirname "$STATE")"
    {
        echo BACKEND=iptables
        echo FRONT_PORT="$FRONT_PORT"
        echo RAW_PORT="$RAW_PORT"
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
        echo "WBD_LINUX_SERVER_FIREWALL_PLAN backend=$selected front=$FRONT_PORT raw=$RAW_PORT"
        echo 'ownership: WBD-marked input accept rules + dedicated raw-RST suppression only'
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
        echo "WBD_LINUX_SERVER_FIREWALL_READY backend=$selected front=$FRONT_PORT raw=$RAW_PORT"
        ;;
    cleanup)
        need_root
        cleanup_all
        echo 'WBD_LINUX_SERVER_FIREWALL_CLEANUP_PASS'
        ;;
    status)
        saved=$(state_get BACKEND)
        if [ -n "$saved" ]; then
            echo "WBD_LINUX_SERVER_FIREWALL_STATUS active=1 backend=$saved state=$STATE"
        else
            echo "WBD_LINUX_SERVER_FIREWALL_STATUS active=0 state=$STATE"
        fi
        ;;
esac
