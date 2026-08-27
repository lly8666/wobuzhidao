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
# WBD Linux server configuration. Edit then run: wbd-server restart
WBD_LISTEN_IP=0.0.0.0
WBD_FRONT_PORT=40443
WBD_RAW_PORT=40000
WBD_SERVER_NAME=www.cloudflare.com
WBD_DECOY_TARGET=www.cloudflare.com:443
WBD_ROUTE_KEY=$route_key
WBD_USERNAME=wbd
WBD_PASSWORD=$password
WBD_MAX_FRONT_CONNS=64
WBD_MAX_SESSIONS=64
WBD_TICKET_TTL=60s
WBD_PLATFORM_LISTEN=127.0.0.1:49000
WBD_LINK_LISTEN=127.0.0.1:47000
WBD_UDP_IDLE=30s
WBD_TCP_IDLE=30s
WBD_FIREWALL_BACKEND=auto
# Optional nft input chain, e.g. inet:filter:input. Empty lets helper manage its own input hook.
WBD_NFT_INPUT=
EOF
    chmod 600 "$CONFIG"
}

load_config() {
    [ -r "$CONFIG" ] || { echo "missing $CONFIG" >&2; exit 1; }
    # shellcheck disable=SC1090
    . "$CONFIG"
    : "${WBD_LISTEN_IP:=0.0.0.0}" "${WBD_FRONT_PORT:=40443}" "${WBD_RAW_PORT:=40000}"
    : "${WBD_SERVER_NAME:=www.cloudflare.com}" "${WBD_DECOY_TARGET:=www.cloudflare.com:443}"
    : "${WBD_MAX_FRONT_CONNS:=64}" "${WBD_MAX_SESSIONS:=64}" "${WBD_TICKET_TTL:=60s}"
    : "${WBD_PLATFORM_LISTEN:=127.0.0.1:49000}" "${WBD_LINK_LISTEN:=127.0.0.1:47000}"
    : "${WBD_UDP_IDLE:=30s}" "${WBD_TCP_IDLE:=30s}" "${WBD_FIREWALL_BACKEND:=auto}" "${WBD_NFT_INPUT:=}"
    [ -n "${WBD_ROUTE_KEY:-}" ] && [ ${#WBD_ROUTE_KEY} -ge 16 ] || { echo 'WBD_ROUTE_KEY must be >=16 chars' >&2; exit 1; }
    [ -n "${WBD_USERNAME:-}" ] && [ -n "${WBD_PASSWORD:-}" ] || { echo 'WBD_USERNAME/WBD_PASSWORD required' >&2; exit 1; }
}

install_unit() {
    cat >"$UNIT" <<EOF
[Unit]
Description=WBD Linux Server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=$PREFIX/bin/wbd-server run
ExecStop=/bin/kill -TERM \$MAINPID
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
AmbientCapabilities=CAP_NET_ADMIN CAP_NET_RAW
CapabilityBoundingSet=CAP_NET_ADMIN CAP_NET_RAW CAP_NET_BIND_SERVICE
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

install_files() {
    need_root
    arch=$(uname -m)
    case "$arch" in x86_64|amd64) want=amd64;; aarch64|arm64) want=arm64;; *) echo "unsupported arch: $arch" >&2; exit 1;; esac
    [ -x "$SELF_DIR/bin/wbd-reality-front" ] || { echo 'run install from extracted WBD server bundle' >&2; exit 1; }
    bundle_arch=$(cat "$SELF_DIR/ARCH" 2>/dev/null || true)
    [ "$bundle_arch" = "$want" ] || { echo "bundle arch=$bundle_arch host=$want" >&2; exit 1; }
    mkdir -p "$PREFIX/bin" "$ETC" "$RUN/tickets"
    chmod 700 "$RUN/tickets"
    for f in wbd-reality-front wbd-faketcp-mux wbd-link-server-mux wbd-platform-proxy-server wbd_dtls_shim; do
        install -m 0755 "$SELF_DIR/bin/$f" "$PREFIX/bin/$f"
    done
    install -m 0755 "$SELF_DIR/linux_server_firewall.sh" "$PREFIX/bin/linux_server_firewall.sh"
    install -m 0755 "$SELF_DIR/linux_server_guard.sh" "$PREFIX/bin/linux_server_guard.sh"
    install -m 0755 "$SELF_DIR/linux_server_manager.sh" "$PREFIX/bin/wbd-server"
    install -m 0755 "$SELF_DIR/wbd-server-cert" "$PREFIX/bin/wbd-server-cert"
    write_default_config
    if [ ! -s "$ETC/front.pem" ] || [ ! -s "$ETC/front.key" ]; then "$PREFIX/bin/wbd-server-cert" -name "$(. "$CONFIG"; printf %s "$WBD_SERVER_NAME")" -cert "$ETC/front.pem" -key "$ETC/front.key"; fi
    if [ ! -s "$ETC/dtls.pem" ] || [ ! -s "$ETC/dtls.key" ]; then "$PREFIX/bin/wbd-server-cert" -name wbd-dtls.local -cert "$ETC/dtls.pem" -key "$ETC/dtls.key"; fi
    chmod 600 "$ETC"/*.key "$CONFIG"
    install_unit
    systemctl enable wbd-server.service >/dev/null
    echo "WBD installed. Edit $CONFIG then run: wbd-server restart"
}

run_server() {
    need_root; load_config
    mkdir -p "$RUN/tickets"; chmod 700 "$RUN/tickets"
    rm -f "$RUN/tickets"/* 2>/dev/null || true
    pids=""
    cleanup() { set +e; for p in $pids; do kill -TERM "$p" 2>/dev/null || true; done; wait 2>/dev/null || true; }
    trap cleanup EXIT INT TERM HUP
    "$PREFIX/bin/wbd-platform-proxy-server" -listen "$WBD_PLATFORM_LISTEN" -udp-idle "$WBD_UDP_IDLE" -tcp-idle "$WBD_TCP_IDLE" & pids="$pids $!"
    "$PREFIX/bin/wbd-link-server-mux" -listen "$WBD_LINK_LISTEN" -service "$WBD_PLATFORM_LISTEN" -ticket-dir "$RUN/tickets" -ticket-ttl "$WBD_TICKET_TTL" -max-sessions "$WBD_MAX_SESSIONS" & pids="$pids $!"
    "$PREFIX/bin/wbd-reality-front" server -listen "$WBD_LISTEN_IP:$WBD_FRONT_PORT" -target "$WBD_DECOY_TARGET" -server-name "$WBD_SERVER_NAME" -cert "$ETC/front.pem" -key "$ETC/front.key" -route-key "$WBD_ROUTE_KEY" -username "$WBD_USERNAME" -password "$WBD_PASSWORD" -ticket-dir "$RUN/tickets" -max-conns "$WBD_MAX_FRONT_CONNS" & pids="$pids $!"
    guard="$PREFIX/bin/linux_server_guard.sh"
    set -- "$guard" --backend "$WBD_FIREWALL_BACKEND" --front-port "$WBD_FRONT_PORT" --raw-port "$WBD_RAW_PORT" --state "$RUN/server-firewall.state"
    [ -z "$WBD_NFT_INPUT" ] || set -- "$@" --nft-input "$WBD_NFT_INPUT"
    set -- "$@" -- "$PREFIX/bin/wbd-faketcp-mux" server --listen "$WBD_LISTEN_IP:$WBD_RAW_PORT" --dtls-shim "$PREFIX/bin/wbd_dtls_shim" --link-target "$WBD_LINK_LISTEN" --cert "$ETC/dtls.pem" --key "$ETC/dtls.key" --max-sessions "$WBD_MAX_SESSIONS"
    "$@" & main=$!; pids="$pids $main"
    wait "$main"; rc=$?; exit "$rc"
}

show_config() { load_config; sed -E 's/^(WBD_(PASSWORD|ROUTE_KEY))=.*/\1=<redacted>/' "$CONFIG"; }

case "${1:-help}" in
 install) install_files ;;
 uninstall) need_root; systemctl disable --now wbd-server.service 2>/dev/null || true; rm -f "$UNIT"; systemctl daemon-reload; "$PREFIX/bin/linux_server_firewall.sh" cleanup --backend auto --front-port 40443 --raw-port 40000 --state "$RUN/server-firewall.state" 2>/dev/null || true; rm -rf "$PREFIX" "$ETC" "$RUN"; echo 'WBD uninstalled' ;;
 run) run_server ;;
 start|stop|restart) need_root; systemctl "$1" wbd-server.service ;;
 pause) need_root; systemctl stop wbd-server.service ;;
 resume) need_root; systemctl start wbd-server.service ;;
 status) systemctl --no-pager --full status wbd-server.service || true ;;
 logs) exec journalctl -u wbd-server.service -f -n 100 ;;
 config) need_root; write_default_config; ${EDITOR:-vi} "$CONFIG" ;;
 show-config) show_config ;;
 help|-h|--help) cat <<EOF
usage: wbd-server COMMAND
  install       install this amd64/arm64 bundle and create defaults
  uninstall     stop, remove firewall state, binaries, config and service
  start|resume  start server
  stop|pause    stop server and clean WBD firewall rules
  restart       restart after config changes
  status        systemd status
  logs          follow journal logs
  config        edit $CONFIG
  show-config   print settings with secrets redacted

Main settings: WBD_FRONT_PORT, WBD_RAW_PORT, WBD_LISTEN_IP, WBD_SERVER_NAME,
WBD_DECOY_TARGET, WBD_ROUTE_KEY, WBD_USERNAME, WBD_PASSWORD, session limits,
timeouts and firewall backend. The decoy domain/target are used only by the
Reality-like setup front; sustained VPN payload remains FakeTCP -> DTLS 1.3 -> LINK.
EOF
 ;;
 *) echo "unknown command: $1" >&2; exit 2;;
esac
