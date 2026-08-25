#!/bin/sh
set -eu

ACTION=render
MODE=global
PORT=12345
MARK=0x66
TABLE=1066
PRIO=1066
UNDERLAY4=
UNDERLAY6=

usage() {
    cat >&2 <<'EOF'
usage: openwrt_tproxy.sh [render|apply|cleanup] [options]
  --mode global|only-cn|only-non-cn
  --port PORT
  --mark MARK
  --table TABLE
  --priority PRIO
  --underlay4 IPv4
  --underlay6 IPv6

render prints the nft/ip plan without privileges. apply installs an idempotent
TPROXY capture table plus policy routes. cleanup removes only WBD-owned state.
The user-space transparent TCP/UDP adapter is a later platform integration step.
EOF
}

if [ $# -gt 0 ]; then
    case "$1" in
        render|apply|cleanup) ACTION=$1; shift ;;
    esac
fi

while [ $# -gt 0 ]; do
    case "$1" in
        --mode) MODE=$2; shift 2 ;;
        --port) PORT=$2; shift 2 ;;
        --mark) MARK=$2; shift 2 ;;
        --table) TABLE=$2; shift 2 ;;
        --priority) PRIO=$2; shift 2 ;;
        --underlay4) UNDERLAY4=$2; shift 2 ;;
        --underlay6) UNDERLAY6=$2; shift 2 ;;
        -h|--help) usage; exit 0 ;;
        *) echo "unknown argument: $1" >&2; usage; exit 2 ;;
    esac
done

case "$MODE" in
    global|only-cn|only-non-cn) ;;
    *) echo "invalid mode: $MODE" >&2; exit 2 ;;
esac
case "$PORT" in *[!0-9]*|'') echo "invalid port" >&2; exit 2 ;; esac
case "$TABLE" in *[!0-9]*|'') echo "invalid table" >&2; exit 2 ;; esac
case "$PRIO" in *[!0-9]*|'') echo "invalid priority" >&2; exit 2 ;; esac
[ "$PORT" -gt 0 ] && [ "$PORT" -le 65535 ] || { echo "invalid port" >&2; exit 2; }

emit_cleanup() {
    cat <<EOF
nft delete table inet wbd 2>/dev/null || true
ip rule del priority $PRIO fwmark $MARK lookup $TABLE 2>/dev/null || true
ip route flush table $TABLE 2>/dev/null || true
ip -6 rule del priority $PRIO fwmark $MARK lookup $TABLE 2>/dev/null || true
ip -6 route flush table $TABLE 2>/dev/null || true
EOF
}

rule4=
rule6=
case "$MODE" in
    global)
        rule4="meta l4proto { tcp, udp } tproxy to :$PORT meta mark set $MARK accept"
        rule6="$rule4"
        ;;
    only-cn)
        rule4="meta l4proto { tcp, udp } ip daddr @cn4 tproxy to :$PORT meta mark set $MARK accept"
        rule6="meta l4proto { tcp, udp } ip6 daddr @cn6 tproxy to :$PORT meta mark set $MARK accept"
        ;;
    only-non-cn)
        rule4="meta l4proto { tcp, udp } ip daddr != @cn4 tproxy to :$PORT meta mark set $MARK accept"
        rule6="meta l4proto { tcp, udp } ip6 daddr != @cn6 tproxy to :$PORT meta mark set $MARK accept"
        ;;
esac

bypass4=
bypass6=
[ -z "$UNDERLAY4" ] || bypass4="ip daddr $UNDERLAY4 return"
[ -z "$UNDERLAY6" ] || bypass6="ip6 daddr $UNDERLAY6 return"

emit_apply() {
    emit_cleanup
    cat <<EOF
nft -f - <<'WBD_NFT'
table inet wbd {
    set cn4 { type ipv4_addr; flags interval; }
    set cn6 { type ipv6_addr; flags interval; }
    chain prerouting {
        type filter hook prerouting priority mangle; policy accept;
        meta mark $MARK return
        fib daddr type local return
        $bypass4
        $bypass6
        $rule4
        $rule6
    }
}
WBD_NFT
ip rule add priority $PRIO fwmark $MARK lookup $TABLE
ip route add local 0.0.0.0/0 dev lo table $TABLE
ip -6 rule add priority $PRIO fwmark $MARK lookup $TABLE
ip -6 route add local ::/0 dev lo table $TABLE
EOF
}

case "$ACTION" in
    render)
        echo "# WBD_OPENWRT_TPROXY_PLAN mode=$MODE port=$PORT mark=$MARK table=$TABLE priority=$PRIO"
        echo "# underlay escape rules are emitted before any TPROXY rule"
        emit_apply
        ;;
    cleanup)
        if [ "$(id -u)" -ne 0 ]; then
            echo "cleanup requires root" >&2
            exit 1
        fi
        # shellcheck disable=SC2046
        sh -c "$(emit_cleanup)"
        ;;
    apply)
        if [ "$(id -u)" -ne 0 ]; then
            echo "apply requires root" >&2
            exit 1
        fi
        tmp="/tmp/wbd-tproxy.$$.sh"
        trap 'rm -f "$tmp"' EXIT INT TERM
        emit_apply >"$tmp"
        sh "$tmp"
        echo "WBD_OPENWRT_TPROXY_READY mode=$MODE port=$PORT mark=$MARK table=$TABLE"
        ;;
esac
