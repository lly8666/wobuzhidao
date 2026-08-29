#!/bin/sh
set -eu

PREFIX=${WBD_PREFIX:-/opt/wbd}
ETC=${WBD_ETC:-/etc/wbd}
RUN=${WBD_RUN:-/run/wbd}
UNIT=/etc/systemd/system/wbd-server.service
CONFIG=$ETC/server.env
SELF_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)

need_root() { [ "$(id -u)" -eq 0 ] || { echo 'root required' >&2; exit 1; }; }
q() { printf "'%s'" "$(printf %s "$1" | sed "s/'/'\\''/g")"; }

write_default_config() {
    [ -e "$CONFIG" ] && return 0
    umask 077
    mkdir -p "$ETC"
    route_key=$(cat /proc/sys/kernel/random/uuid | tr -d '-')
    password=$(cat /proc/sys/kernel/random/uuid | tr -d '-')
    cat >"$CONFIG" <<EOF
# WBD V3 Linux server. The public port has exactly one owner: raw FakeTCP mux.
# Reality-like TLS 1.3 setup/auth runs inside that same raw association.
WBD_LISTEN_IP=0.0.0.0
WBD_PORT=40443
WBD_SERVER_NAME=www.cloudflare.com
WBD_ROUTE_KEY=$route_key
WBD_USERNAME=wbd
WBD_PASSWORD=$password
WBD_MAX_SESSIONS=64
WBD_TICKET_TTL=60s
WBD_PLATFORM_LISTEN=127.0.0.1:49000
WBD_LINK_LISTEN=127.0.0.1:47000
WBD_UDP_IDLE=30s
WBD_TCP_IDLE=30s
WBD_FIREWALL_BACKEND=auto
WBD_NFT_INPUT=
EOF
    chmod 600 "$CONFIG"
}

load_config() {
    [ -r "$CONFIG" ] || { echo "missing $CONFIG" >&2; exit 1; }
    # shellcheck disable=SC1090
    . "$CONFIG"
    : "${WBD_LISTEN_IP:=0.0.0.0}"
    : "${WBD_PORT:=40443}"
    : "${WBD_SERVER_NAME:=www.cloudflare.com}"
    : "${WBD_MAX_SESSIONS:=64}" "${WBD_TICKET_TTL:=60s}"
    : "${WBD_PLATFORM_LISTEN:=127.0.0.1:49000}" "${WBD_LINK_LISTEN:=127.0.0.1:47000}"
    : "${WBD_UDP_IDLE:=30s}" "${WBD_TCP_IDLE:=30s}" "${WBD_FIREWALL_BACKEND:=auto}" "${WBD_NFT_INPUT:=}"
    case "$WBD_PORT" in *[!0-9]*|'') echo 'WBD_PORT must be numeric' >&2; exit 1;; esac
    [ "$WBD_PORT" -ge 1 ] && [ "$WBD_PORT" -le 65535 ] || { echo 'WBD_PORT must be 1..65535' >&2; exit 1; }
    [ -n "${WBD_ROUTE_KEY:-}" ] && [ ${#WBD_ROUTE_KEY} -ge 16 ] || { echo 'WBD_ROUTE_KEY must be >=16 chars' >&2; exit 1; }
    [ -n "${WBD_USERNAME:-}" ] && [ -n "${WBD_PASSWORD:-}" ] || { echo 'WBD_USERNAME/WBD_PASSWORD required' >&2; exit 1; }
    case "$WBD_FIREWALL_BACKEND" in auto|nft|iptables) ;; *) echo 'WBD_FIREWALL_BACKEND must be auto, nft, or iptables' >&2; exit 1;; esac
}

resolve_raw_listen_ip() {
    case "$WBD_LISTEN_IP" in
        0.0.0.0)
            command -v ip >/dev/null 2>&1 || { echo 'WBD_LISTEN_IP=0.0.0.0 requires iproute2' >&2; return 1; }
            raw_ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for (i=1;i<=NF;i++) if ($i=="src" && i<NF) {print $(i+1); exit}}')
            [ -n "$raw_ip" ] && [ "$raw_ip" != 0.0.0.0 ] || { echo 'cannot resolve concrete public IPv4; set WBD_LISTEN_IP explicitly' >&2; return 1; }
            printf '%s\n' "$raw_ip"
            ;;
        *) printf '%s\n' "$WBD_LISTEN_IP" ;;
    esac
}

set_config() {
    need_root; write_default_config
    key=${1:-}; value=${2-}
    case "$key" in
      WBD_LISTEN_IP|WBD_PORT|WBD_SERVER_NAME|WBD_ROUTE_KEY|WBD_USERNAME|WBD_PASSWORD|WBD_MAX_SESSIONS|WBD_TICKET_TTL|WBD_PLATFORM_LISTEN|WBD_LINK_LISTEN|WBD_UDP_IDLE|WBD_TCP_IDLE|WBD_FIREWALL_BACKEND|WBD_NFT_INPUT) ;;
      *) echo "unsupported V3 setting: $key" >&2; exit 2;;
    esac
    quoted=$(q "$value"); tmp="$CONFIG.tmp.$$"
    awk -v k="$key" -v line="$key=$quoted" 'BEGIN{done=0} index($0,k"=")==1 {print line; done=1; next} {print} END{if(!done) print line}' "$CONFIG" >"$tmp"
    chmod 600 "$tmp"; mv "$tmp" "$CONFIG"; load_config
    if [ "$key" = WBD_SERVER_NAME ]; then
        echo "$key updated; run: wbd-server regen-certs && wbd-server restart"
    else
        echo "$key updated; run: wbd-server restart"
    fi
}

install_unit() {
    cat >"$UNIT" <<EOF
[Unit]
Description=WBD V3 Single-Flow Linux Server
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=30
StartLimitBurst=5

[Service]
Type=simple
ExecStart=$PREFIX/bin/wbd-server run
Restart=on-failure
RestartSec=2
KillMode=control-group
TimeoutStopSec=10s
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

regen_certs() {
    need_root; load_config
    "$PREFIX/bin/wbd-server-cert" -name "$WBD_SERVER_NAME" -cert "$ETC/front.pem" -key "$ETC/front.key"
    "$PREFIX/bin/wbd-server-cert" -name wbd-dtls.local -cert "$ETC/dtls.pem" -key "$ETC/dtls.key"
    chmod 600 "$ETC"/*.key
    echo 'WBD V3 certificates regenerated; restart WBD to use them'
}

install_files() {
    need_root
    arch=$(uname -m)
    case "$arch" in x86_64|amd64) want=amd64;; aarch64|arm64) want=arm64;; *) echo "unsupported arch: $arch" >&2; exit 1;; esac
    [ -x "$SELF_DIR/bin/wbd-faketcp-mux" ] || { echo 'run install from extracted WBD V3 server bundle' >&2; exit 1; }
    bundle_arch=$(cat "$SELF_DIR/ARCH" 2>/dev/null || true)
    [ "$bundle_arch" = "$want" ] || { echo "bundle arch=$bundle_arch host=$want" >&2; exit 1; }
    command -v systemctl >/dev/null || { echo 'systemd/systemctl is required' >&2; exit 1; }
    if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then echo 'host requires nft or iptables' >&2; exit 1; fi
    mkdir -p "$PREFIX/bin" "$ETC" "$RUN/tickets"; chmod 700 "$RUN/tickets"
    for f in wbd-faketcp-mux wbd-link-server-mux wbd-platform-proxy-server wbd_dtls_shim wbd-server-cert; do install -m 0755 "$SELF_DIR/bin/$f" "$PREFIX/bin/$f"; done
    # wbd-reality-front may be present as a diagnostic/legacy tool, but V3 does
    # not install or execute it as a public listener.
    install -m 0755 "$SELF_DIR/linux_server_firewall.sh" "$PREFIX/bin/linux_server_firewall.sh"
    install -m 0755 "$SELF_DIR/linux_server_guard.sh" "$PREFIX/bin/linux_server_guard.sh"
    install -m 0755 "$SELF_DIR/linux_server_manager_v3.sh" "$PREFIX/bin/wbd-server"
    write_default_config; load_config
    [ -s "$ETC/front.pem" ] && [ -s "$ETC/front.key" ] || "$PREFIX/bin/wbd-server-cert" -name "$WBD_SERVER_NAME" -cert "$ETC/front.pem" -key "$ETC/front.key"
    [ -s "$ETC/dtls.pem" ] && [ -s "$ETC/dtls.key" ] || "$PREFIX/bin/wbd-server-cert" -name wbd-dtls.local -cert "$ETC/dtls.pem" -key "$ETC/dtls.key"
    chmod 600 "$ETC"/*.key "$CONFIG"; install_unit; systemctl enable wbd-server.service >/dev/null
    echo "WBD V3 installed but not started. Configure $CONFIG, run 'wbd-server doctor', then 'wbd-server start'."
}

run_server() {
    need_root; load_config; raw_ip=$(resolve_raw_listen_ip)
    mkdir -p "$RUN/tickets"; chmod 700 "$RUN/tickets"; rm -f "$RUN/tickets"/* 2>/dev/null || true
    pids=""
    cleanup() { set +e; for p in $pids; do kill -TERM "$p" 2>/dev/null || true; done; wait 2>/dev/null || true; }
    trap cleanup EXIT; trap 'exit 0' INT TERM HUP
    echo "WBD_LINUX_SERVER_BIND_V3 public_owner=raw-mux public=$raw_ip:$WBD_PORT reality_like=in-flow link=$WBD_LINK_LISTEN platform=$WBD_PLATFORM_LISTEN"
    "$PREFIX/bin/wbd-platform-proxy-server" -listen "$WBD_PLATFORM_LISTEN" -udp-idle "$WBD_UDP_IDLE" -tcp-idle "$WBD_TCP_IDLE" & pids="$pids $!"
    "$PREFIX/bin/wbd-link-server-mux" -listen "$WBD_LINK_LISTEN" -service "$WBD_PLATFORM_LISTEN" -ticket-dir "$RUN/tickets" -ticket-ttl "$WBD_TICKET_TTL" -max-sessions "$WBD_MAX_SESSIONS" & pids="$pids $!"
    guard="$PREFIX/bin/linux_server_guard.sh"
    set -- "$guard" --backend "$WBD_FIREWALL_BACKEND" --front-port "$WBD_PORT" --raw-port "$WBD_PORT" --state "$RUN/server-firewall.state"
    [ -z "$WBD_NFT_INPUT" ] || set -- "$@" --nft-input "$WBD_NFT_INPUT"
    set -- "$@" -- "$PREFIX/bin/wbd-faketcp-mux" server --listen "$raw_ip:$WBD_PORT" --dtls-shim "$PREFIX/bin/wbd_dtls_shim" --link-target "$WBD_LINK_LISTEN" --cert "$ETC/dtls.pem" --key "$ETC/dtls.key" --max-sessions "$WBD_MAX_SESSIONS" --front-server-name "$WBD_SERVER_NAME" --front-cert "$ETC/front.pem" --front-key "$ETC/front.key" --front-route-key "$WBD_ROUTE_KEY" --username "$WBD_USERNAME" --password "$WBD_PASSWORD" --ticket-dir "$RUN/tickets"
    "$@" & main=$!; pids="$pids $main"
    if wait "$main"; then rc=0; else rc=$?; fi
    exit "$rc"
}

uninstall_files() {
    need_root
    port=40443; backend=auto; nft_input=
    if [ -r "$CONFIG" ]; then . "$CONFIG"; port=${WBD_PORT:-40443}; backend=${WBD_FIREWALL_BACKEND:-auto}; nft_input=${WBD_NFT_INPUT:-}; fi
    systemctl disable --now wbd-server.service 2>/dev/null || true
    if [ -x "$PREFIX/bin/linux_server_firewall.sh" ]; then
        set -- "$PREFIX/bin/linux_server_firewall.sh" cleanup --backend "$backend" --front-port "$port" --raw-port "$port" --state "$RUN/server-firewall.state"
        [ -z "$nft_input" ] || set -- "$@" --nft-input "$nft_input"; "$@" 2>/dev/null || true
    fi
    rm -f "$UNIT"; systemctl daemon-reload; rm -rf "$PREFIX" "$ETC" "$RUN"; echo 'WBD V3 uninstalled'
}

doctor() {
    load_config; fail=0; printf 'config: OK (%s)\n' "$CONFIG"
    for f in wbd-faketcp-mux wbd-link-server-mux wbd-platform-proxy-server wbd_dtls_shim wbd-server-cert linux_server_firewall.sh linux_server_guard.sh; do
        if [ -x "$PREFIX/bin/$f" ]; then echo "binary: OK $f"; else echo "binary: MISSING $f"; fail=1; fi
    done
    if command -v nft >/dev/null 2>&1; then echo 'firewall: OK nft'; elif command -v iptables >/dev/null 2>&1; then echo 'firewall: OK iptables'; else echo 'firewall: MISSING nft/iptables'; fail=1; fi
    [ -r "$ETC/front.pem" ] && [ -r "$ETC/front.key" ] && echo 'Reality-like TLS certificate: OK' || { echo 'Reality-like TLS certificate: MISSING'; fail=1; }
    [ -r "$ETC/dtls.pem" ] && [ -r "$ETC/dtls.key" ] && echo 'DTLS certificate: OK' || { echo 'DTLS certificate: MISSING'; fail=1; }
    if raw_ip=$(resolve_raw_listen_ip); then echo "public: owner=raw-mux endpoint=$raw_ip:$WBD_PORT reality_like=in-flow second_listener=0"; else fail=1; fi
    echo "single-flow: TLS1.3 bootstrap -> encrypted switch -> DTLS1.3 datagrams; no second SYN/FIN/close_notify at boundary"
    if [ "$fail" -ne 0 ]; then echo 'WBD_SERVER_DOCTOR_FAIL'; return 1; fi
    echo 'WBD_SERVER_DOCTOR_PASS'
}

show_config() { load_config; sed -E 's/^(WBD_(PASSWORD|ROUTE_KEY))=.*/\1=<redacted>/' "$CONFIG"; }

case "${1:-help}" in
 install) install_files;; uninstall) uninstall_files;; run) run_server;;
 start|stop|restart) need_root; systemctl "$1" wbd-server.service;;
 pause) need_root; systemctl stop wbd-server.service;; resume) need_root; systemctl start wbd-server.service;;
 status) systemctl --no-pager --full status wbd-server.service || true;; logs) exec journalctl -u wbd-server.service -f -n 100;;
 config) need_root; write_default_config; ${EDITOR:-vi} "$CONFIG";;
 set) [ $# -eq 3 ] || { echo 'usage: wbd-server set KEY VALUE' >&2; exit 2; }; set_config "$2" "$3";;
 regen-certs) regen_certs;; doctor) doctor;; show-config) show_config;;
 help|-h|--help) cat <<EOF
usage: wbd-server COMMAND
  install|uninstall|start|stop|pause|resume|restart|status|logs
  config|set|regen-certs|doctor|show-config

V3 exposes exactly one public TCP-shaped endpoint. The raw FakeTCP mux owns the
public 4-tuple from the first SYN. Real TLS 1.3 Reality-like setup/auth runs as a
bounded ordered phase inside that same association, followed by an encrypted
mode-switch barrier and DTLS 1.3/FEC datagrams on the same sequence space.
There is no second public Reality TCP listener and no ordinary-TCP steady-state
payload/HOL.
EOF
 ;;
 *) echo "unknown command: $1" >&2; exit 2;;
esac
