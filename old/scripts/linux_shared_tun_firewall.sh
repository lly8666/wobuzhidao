#!/bin/sh
set -eu

ACTION=status
BACKEND=auto
STATE=/run/wbd/shared-tun-firewall.state
LEASE_PREFIX=10.66.0.0/16
TUN_IF=wbdg0
NFT_FORWARD=

usage() {
    cat >&2 <<'EOF'
usage: linux_shared_tun_firewall.sh apply|cleanup|status [options]
  --backend auto|nft|iptables
  --state PATH
  --lease-prefix CIDR
  --tun-if IFNAME
  --nft-forward FAMILY:TABLE:CHAIN

Owns only WBD shared-TUN forwarding/NAT rules. It never flushes host rulesets.
The final v2.4 model is one root-namespace shared TUN plus one host NAT; there
are no per-session netns/veth/conntrack domains.
EOF
}

[ $# -gt 0 ] && { ACTION=$1; shift; }
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --lease-prefix) LEASE_PREFIX=$2; shift 2 ;;
        --tun-if) TUN_IF=$2; shift 2 ;;
        --nft-forward) NFT_FORWARD=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

case "$ACTION" in apply|cleanup|status) ;; *) usage; exit 2 ;; esac
case "$BACKEND" in auto|nft|iptables) ;; *) echo "invalid backend: $BACKEND" >&2; exit 2 ;; esac

need_root() { [ "$(id -u)" -eq 0 ] || { echo "$ACTION requires root" >&2; exit 1; }; }
state_get() {
    key=$1
    [ -f "$STATE" ] || return 0
    sed -n "s/^${key}=//p" "$STATE" | head -n 1
}
choose_backend() {
    if [ "$BACKEND" != auto ]; then printf '%s\n' "$BACKEND"; return; fi
    if command -v nft >/dev/null 2>&1; then printf 'nft\n'
    elif command -v iptables >/dev/null 2>&1; then printf 'iptables\n'
    else echo 'neither nft nor iptables is installed' >&2; exit 1
    fi
}
selected_backend() {
    saved=$(state_get BACKEND)
    if [ -n "$saved" ]; then printf '%s\n' "$saved"; else choose_backend; fi
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
nft_chain_exists() { nft list chain "$1" "$2" "$3" >/dev/null 2>&1; }
find_nft_forward() {
    if [ -n "$NFT_FORWARD" ]; then parse_nft_spec "$NFT_FORWARD"; return; fi
    sf=$(state_get NFT_FORWARD_FAMILY); st=$(state_get NFT_FORWARD_TABLE); sc=$(state_get NFT_FORWARD_CHAIN)
    if [ -n "$sf" ] && [ -n "$st" ] && [ -n "$sc" ]; then printf '%s %s %s\n' "$sf" "$st" "$sc"; return; fi
    for spec in 'inet:filter:forward' 'inet:fw4:forward' 'ip:filter:FORWARD' 'ip:filter:forward'; do
        values=$(parse_nft_spec "$spec") || continue
        # shellcheck disable=SC2086
        if nft_chain_exists $values; then printf '%s\n' "$values"; return; fi
    done
    rules=$(nft list ruleset 2>/dev/null || true)
    if printf '%s\n' "$rules" | grep -Eq 'hook[[:space:]]+forward'; then
        echo 'unable to identify existing nft forward chain; pass --nft-forward FAMILY:TABLE:CHAIN' >&2
        return 2
    fi
    return 1
}
nft_delete_comment() {
    family=$1 table=$2 chain=$3 marker=$4
    nft_chain_exists "$family" "$table" "$chain" || return 0
    handles=$(nft -a list chain "$family" "$table" "$chain" 2>/dev/null | awk -v marker="$marker" '
        index($0, "comment \"" marker "\"") { for (i=1;i<=NF;i++) if ($i=="handle") print $(i+1) }')
    for handle in $handles; do nft delete rule "$family" "$table" "$chain" handle "$handle" 2>/dev/null || true; done
}

iptables_delete_all() {
    table=$1 chain=$2; shift 2
    while iptables -t "$table" -C "$chain" "$@" >/dev/null 2>&1; do
        iptables -t "$table" -D "$chain" "$@" >/dev/null 2>&1 || break
    done
}
iptables_cleanup() {
    command -v iptables >/dev/null 2>&1 || return 0
    lp=$(state_get LEASE_PREFIX); [ -n "$lp" ] || lp=$LEASE_PREFIX
    ti=$(state_get TUN_IF); [ -n "$ti" ] || ti=$TUN_IF
    iptables_delete_all filter FORWARD -i "$ti" -s "$lp" -m comment --comment wbd-shared-tun-out -j ACCEPT
    iptables_delete_all filter FORWARD -o "$ti" -d "$lp" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment wbd-shared-tun-in -j ACCEPT
    iptables_delete_all nat POSTROUTING -s "$lp" ! -o "$ti" -m comment --comment wbd-shared-tun-nat -j MASQUERADE
}
iptables_apply() {
    command -v iptables >/dev/null 2>&1 || { echo 'iptables not installed' >&2; return 1; }
    iptables_cleanup
    iptables -I FORWARD 1 -i "$TUN_IF" -s "$LEASE_PREFIX" -m comment --comment wbd-shared-tun-out -j ACCEPT
    iptables -I FORWARD 1 -o "$TUN_IF" -d "$LEASE_PREFIX" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment wbd-shared-tun-in -j ACCEPT
    iptables -t nat -I POSTROUTING 1 -s "$LEASE_PREFIX" ! -o "$TUN_IF" -m comment --comment wbd-shared-tun-nat -j MASQUERADE
}

nft_cleanup() {
    if values=$(find_nft_forward 2>/dev/null); then
        # shellcheck disable=SC2086
        set -- $values
        nft_delete_comment "$1" "$2" "$3" wbd-shared-tun-out
        nft_delete_comment "$1" "$2" "$3" wbd-shared-tun-in
    fi
    nft delete table ip wbd_shared_tun 2>/dev/null || true
}
nft_apply() {
    command -v nft >/dev/null 2>&1 || { echo 'nft not installed' >&2; return 1; }
    nft_cleanup
    forward_values=
    if forward_values=$(find_nft_forward); then
        # shellcheck disable=SC2086
        set -- $forward_values
        f_family=$1 f_table=$2 f_chain=$3
        nft insert rule "$f_family" "$f_table" "$f_chain" iifname "$TUN_IF" ip saddr "$LEASE_PREFIX" accept comment wbd-shared-tun-out
        nft insert rule "$f_family" "$f_table" "$f_chain" oifname "$TUN_IF" ip daddr "$LEASE_PREFIX" ct state established,related accept comment wbd-shared-tun-in
    else
        rc=$?
        [ "$rc" -eq 1 ] || return "$rc"
        f_family= f_table= f_chain=
    fi
    nft -f - <<EOF
add table ip wbd_shared_tun
add chain ip wbd_shared_tun postrouting { type nat hook postrouting priority srcnat; policy accept; }
add rule ip wbd_shared_tun postrouting ip saddr $LEASE_PREFIX oifname != "$TUN_IF" masquerade comment "wbd-shared-tun-nat"
EOF
    NFT_APPLY_FORWARD_FAMILY=$f_family
    NFT_APPLY_FORWARD_TABLE=$f_table
    NFT_APPLY_FORWARD_CHAIN=$f_chain
}

restore_forwarding() {
    old=$(state_get IP_FORWARD_OLD)
    if [ "$old" = 0 ]; then printf '0\n' >/proc/sys/net/ipv4/ip_forward 2>/dev/null || true; fi
}

case "$ACTION" in
    apply)
        need_root
        mkdir -p "$(dirname "$STATE")"
        selected=$(choose_backend)
        old_forward=$(cat /proc/sys/net/ipv4/ip_forward)
        printf '1\n' >/proc/sys/net/ipv4/ip_forward
        case "$selected" in
            iptables) iptables_apply; f_family=; f_table=; f_chain= ;;
            nft)
                nft_apply
                f_family=${NFT_APPLY_FORWARD_FAMILY:-}; f_table=${NFT_APPLY_FORWARD_TABLE:-}; f_chain=${NFT_APPLY_FORWARD_CHAIN:-}
                ;;
        esac
        {
            echo BACKEND="$selected"
            echo LEASE_PREFIX="$LEASE_PREFIX"
            echo TUN_IF="$TUN_IF"
            echo IP_FORWARD_OLD="$old_forward"
            echo NFT_FORWARD_FAMILY="$f_family"
            echo NFT_FORWARD_TABLE="$f_table"
            echo NFT_FORWARD_CHAIN="$f_chain"
        } >"$STATE"
        chmod 600 "$STATE" 2>/dev/null || true
        echo "WBD_SHARED_TUN_FIREWALL_READY backend=$selected tun=$TUN_IF lease_prefix=$LEASE_PREFIX nat=host"
        ;;
    cleanup)
        need_root
        saved=$(state_get BACKEND)
        case "$saved" in
            iptables) iptables_cleanup ;;
            nft) nft_cleanup ;;
            *)
                command -v iptables >/dev/null 2>&1 && iptables_cleanup || true
                command -v nft >/dev/null 2>&1 && nft_cleanup || true
                ;;
        esac
        restore_forwarding
        rm -f "$STATE"
        echo 'WBD_SHARED_TUN_FIREWALL_CLEANUP_PASS isolation=shared_tun'
        ;;
    status)
        saved=$(state_get BACKEND)
        if [ -n "$saved" ]; then
            echo "WBD_SHARED_TUN_FIREWALL_STATUS active=1 backend=$saved state=$STATE tun=$(state_get TUN_IF) lease_prefix=$(state_get LEASE_PREFIX)"
        else
            echo "WBD_SHARED_TUN_FIREWALL_STATUS active=0 state=$STATE"
        fi
        ;;
esac
