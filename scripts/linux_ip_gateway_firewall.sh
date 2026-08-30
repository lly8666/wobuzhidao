#!/bin/sh
set -eu

ACTION=status
BACKEND=auto
STATE=/run/wbd/ip-gateway-firewall.state
TRANSIT_PREFIX=198.18.240.0/24
INNER_PREFIX=10.66.0.0/30
NFT_FORWARD=
SLOT=
NETNS=
TUN_IF=
NS_IF=
TRANSIT_IP=

usage() {
    cat >&2 <<'EOF'
usage: linux_ip_gateway_firewall.sh apply|cleanup|status|session-add|session-del [options]
  --backend auto|nft|iptables
  --state PATH
  --transit-prefix CIDR
  --inner-prefix CIDR
  --nft-forward FAMILY:TABLE:CHAIN
  session-add/session-del also require:
  --slot N --netns NAME --tun-if IFNAME --ns-if IFNAME --transit-ip IPv4

The helper owns only WBD-marked host rules and one WBD-owned NAT/filter set
inside each WBD session network namespace. It never flushes the host ruleset.
Host NAT maps the unique transit prefix to physical egress. Inner NAT lives in
the per-session namespace, so identical Windows inner tuples remain isolated by
that namespace's independent conntrack table.
EOF
}

[ $# -gt 0 ] && { ACTION=$1; shift; }
while [ $# -gt 0 ]; do
    case "$1" in
        --backend) BACKEND=$2; shift 2 ;;
        --state) STATE=$2; shift 2 ;;
        --transit-prefix) TRANSIT_PREFIX=$2; shift 2 ;;
        --inner-prefix) INNER_PREFIX=$2; shift 2 ;;
        --nft-forward) NFT_FORWARD=$2; shift 2 ;;
        --slot) SLOT=$2; shift 2 ;;
        --netns) NETNS=$2; shift 2 ;;
        --tun-if) TUN_IF=$2; shift 2 ;;
        --ns-if) NS_IF=$2; shift 2 ;;
        --transit-ip) TRANSIT_IP=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

case "$ACTION" in apply|cleanup|status|session-add|session-del) ;; *) usage; exit 2 ;; esac
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
    saved_family=$(state_get NFT_FORWARD_FAMILY)
    saved_table=$(state_get NFT_FORWARD_TABLE)
    saved_chain=$(state_get NFT_FORWARD_CHAIN)
    if [ -n "$saved_family" ] && [ -n "$saved_table" ] && [ -n "$saved_chain" ]; then
        printf '%s %s %s\n' "$saved_family" "$saved_table" "$saved_chain"; return
    fi
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
ns_iptables_delete_all() {
    table=$1 chain=$2; shift 2
    while ip netns exec "$NETNS" iptables -t "$table" -C "$chain" "$@" >/dev/null 2>&1; do
        ip netns exec "$NETNS" iptables -t "$table" -D "$chain" "$@" >/dev/null 2>&1 || break
    done
}

restore_forwarding() {
    old=$(state_get IP_FORWARD_OLD)
    if [ "$old" = 0 ]; then printf '0\n' >/proc/sys/net/ipv4/ip_forward 2>/dev/null || true; fi
}

iptables_global_cleanup() {
    command -v iptables >/dev/null 2>&1 || return 0
    tp=$(state_get TRANSIT_PREFIX); [ -n "$tp" ] || tp=$TRANSIT_PREFIX
    iptables_delete_all filter FORWARD -s "$tp" -m comment --comment wbd-ipg-transit-out -j ACCEPT
    iptables_delete_all filter FORWARD -d "$tp" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment wbd-ipg-transit-in -j ACCEPT
    iptables_delete_all nat POSTROUTING -s "$tp" -m comment --comment wbd-ipg-transit-nat -j MASQUERADE
}
iptables_global_apply() {
    command -v iptables >/dev/null 2>&1 || { echo 'iptables not installed' >&2; return 1; }
    iptables_global_cleanup
    iptables -I FORWARD 1 -s "$TRANSIT_PREFIX" -m comment --comment wbd-ipg-transit-out -j ACCEPT
    iptables -I FORWARD 1 -d "$TRANSIT_PREFIX" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment wbd-ipg-transit-in -j ACCEPT
    iptables -t nat -I POSTROUTING 1 -s "$TRANSIT_PREFIX" -m comment --comment wbd-ipg-transit-nat -j MASQUERADE
}
iptables_session_del() {
    ip netns list | awk '{print $1}' | grep -Fx "$NETNS" >/dev/null 2>&1 || return 0
    marker="wbd-ipg-$SLOT"
    ns_iptables_delete_all nat POSTROUTING -s "$INNER_PREFIX" -o "$NS_IF" -m comment --comment "$marker-snat" -j SNAT --to-source "$TRANSIT_IP"
    ns_iptables_delete_all filter FORWARD -i "$TUN_IF" -o "$NS_IF" -m comment --comment "$marker-out" -j ACCEPT
    ns_iptables_delete_all filter FORWARD -i "$NS_IF" -o "$TUN_IF" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "$marker-in" -j ACCEPT
}
iptables_session_add() {
    iptables_session_del
    marker="wbd-ipg-$SLOT"
    ip netns exec "$NETNS" iptables -t nat -I POSTROUTING 1 -s "$INNER_PREFIX" -o "$NS_IF" -m comment --comment "$marker-snat" -j SNAT --to-source "$TRANSIT_IP"
    ip netns exec "$NETNS" iptables -I FORWARD 1 -i "$TUN_IF" -o "$NS_IF" -m comment --comment "$marker-out" -j ACCEPT
    ip netns exec "$NETNS" iptables -I FORWARD 1 -i "$NS_IF" -o "$TUN_IF" -m conntrack --ctstate ESTABLISHED,RELATED -m comment --comment "$marker-in" -j ACCEPT
}

nft_global_cleanup() {
    if values=$(find_nft_forward 2>/dev/null); then
        # shellcheck disable=SC2086
        set -- $values
        nft_delete_comment "$1" "$2" "$3" wbd-ipg-transit-out
        nft_delete_comment "$1" "$2" "$3" wbd-ipg-transit-in
    fi
    nft delete table ip wbd_ipg_host 2>/dev/null || true
}
nft_global_apply() {
    command -v nft >/dev/null 2>&1 || { echo 'nft not installed' >&2; return 1; }
    nft_global_cleanup
    forward_values=
    if forward_values=$(find_nft_forward); then
        # shellcheck disable=SC2086
        set -- $forward_values
        f_family=$1 f_table=$2 f_chain=$3
        nft insert rule "$f_family" "$f_table" "$f_chain" ip saddr "$TRANSIT_PREFIX" accept comment wbd-ipg-transit-out
        nft insert rule "$f_family" "$f_table" "$f_chain" ip daddr "$TRANSIT_PREFIX" ct state established,related accept comment wbd-ipg-transit-in
    else
        rc=$?
        [ "$rc" -eq 1 ] || return "$rc"
        f_family= f_table= f_chain=
    fi
    nft -f - <<EOF
add table ip wbd_ipg_host
add chain ip wbd_ipg_host postrouting { type nat hook postrouting priority srcnat; policy accept; }
add rule ip wbd_ipg_host postrouting ip saddr $TRANSIT_PREFIX masquerade comment "wbd-ipg-transit-nat"
EOF
    NFT_APPLY_FORWARD_FAMILY=$f_family
    NFT_APPLY_FORWARD_TABLE=$f_table
    NFT_APPLY_FORWARD_CHAIN=$f_chain
}
nft_session_del() {
    ip netns list | awk '{print $1}' | grep -Fx "$NETNS" >/dev/null 2>&1 || return 0
    ip netns exec "$NETNS" nft delete table ip wbd_ipg_session 2>/dev/null || true
}
nft_session_add() {
    nft_session_del
    ip netns exec "$NETNS" nft -f - <<EOF
add table ip wbd_ipg_session
add chain ip wbd_ipg_session forward { type filter hook forward priority filter; policy accept; }
add chain ip wbd_ipg_session postrouting { type nat hook postrouting priority srcnat; policy accept; }
add rule ip wbd_ipg_session forward iifname "$TUN_IF" oifname "$NS_IF" accept comment "wbd-ipg-$SLOT-out"
add rule ip wbd_ipg_session forward iifname "$NS_IF" oifname "$TUN_IF" ct state established,related accept comment "wbd-ipg-$SLOT-in"
add rule ip wbd_ipg_session postrouting ip saddr $INNER_PREFIX oifname "$NS_IF" snat to $TRANSIT_IP comment "wbd-ipg-$SLOT-snat"
EOF
}

require_session_args() {
    for v in SLOT NETNS TUN_IF NS_IF TRANSIT_IP; do
        eval "value=\${$v:-}"
        [ -n "$value" ] || { echo "$v is required for $ACTION" >&2; exit 2; }
    done
}

case "$ACTION" in
    apply)
        need_root
        mkdir -p "$(dirname "$STATE")"
        selected=$(choose_backend)
        old_forward=$(cat /proc/sys/net/ipv4/ip_forward)
        printf '1\n' >/proc/sys/net/ipv4/ip_forward
        case "$selected" in
            iptables) iptables_global_apply; f_family=; f_table=; f_chain= ;;
            nft)
                nft_global_apply
                f_family=${NFT_APPLY_FORWARD_FAMILY:-}; f_table=${NFT_APPLY_FORWARD_TABLE:-}; f_chain=${NFT_APPLY_FORWARD_CHAIN:-}
                ;;
        esac
        {
            echo BACKEND="$selected"
            echo TRANSIT_PREFIX="$TRANSIT_PREFIX"
            echo INNER_PREFIX="$INNER_PREFIX"
            echo IP_FORWARD_OLD="$old_forward"
            echo NFT_FORWARD_FAMILY="$f_family"
            echo NFT_FORWARD_TABLE="$f_table"
            echo NFT_FORWARD_CHAIN="$f_chain"
        } >"$STATE"
        chmod 600 "$STATE" 2>/dev/null || true
        echo "WBD_IP_GATEWAY_FIREWALL_READY backend=$selected transit=$TRANSIT_PREFIX isolation=netns"
        ;;
    cleanup)
        need_root
        saved=$(state_get BACKEND)
        case "$saved" in
            iptables) iptables_global_cleanup ;;
            nft) nft_global_cleanup ;;
            *)
                command -v iptables >/dev/null 2>&1 && iptables_global_cleanup || true
                command -v nft >/dev/null 2>&1 && nft_global_cleanup || true
                ;;
        esac
        restore_forwarding
        rm -f "$STATE"
        echo 'WBD_IP_GATEWAY_FIREWALL_CLEANUP_PASS isolation=netns'
        ;;
    session-add|session-del)
        need_root; require_session_args
        selected=$(selected_backend)
        case "$selected:$ACTION" in
            iptables:session-add) iptables_session_add ;;
            iptables:session-del) iptables_session_del ;;
            nft:session-add) nft_session_add ;;
            nft:session-del) nft_session_del ;;
        esac
        echo "WBD_IP_GATEWAY_FIREWALL_SESSION action=$ACTION backend=$selected slot=$SLOT netns=$NETNS"
        ;;
    status)
        saved=$(state_get BACKEND)
        if [ -n "$saved" ]; then echo "WBD_IP_GATEWAY_FIREWALL_STATUS active=1 backend=$saved state=$STATE isolation=netns"; else echo "WBD_IP_GATEWAY_FIREWALL_STATUS active=0 state=$STATE"; fi
        ;;
esac
