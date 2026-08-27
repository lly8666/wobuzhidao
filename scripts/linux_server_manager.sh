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
# One public TCP port is shared by Reality-like admission and raw FakeTCP.
WBD_LISTEN_IP=0.0.0.0
WBD_PORT=40443
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
    : "${WBD_LISTEN_IP:=0.0.0.0}"
    : "${WBD_SERVER_NAME:=www.cloudflare.com}" "${WBD_DECOY_TARGET:=www.cloudflare.com:443}"
    : "${WBD_MAX_FRONT_CONNS:=64}" "${WBD_MAX_SESSIONS:=64}" "${WBD_TICKET_TTL:=60s}"
    : "${WBD_PLATFORM_LISTEN:=127.0.0.1:49000}" "${WBD_LINK_LISTEN:=127.0.0.1:47000}"
    : "${WBD_UDP_IDLE:=30s}" "${WBD_TCP_IDLE:=30s}" "${WBD_FIREWALL_BACKEND:=auto}" "${WBD_NFT_INPUT:=}"

    # New releases expose exactly one public port. For an old config, accept it
    # only when both historical ports were already equal. Never silently choose
    # one of two different public ports during upgrade: the operator must state
    # the intended shared port with `wbd-server set WBD_PORT PORT`.
    if [ -z "${WBD_PORT:-}" ]; then
        legacy_front=${WBD_FRONT_PORT:-}
        legacy_raw=${WBD_RAW_PORT:-}
        if [ -n "$legacy_front" ] || [ -n "$legacy_raw" ]; then
            if [ -z "$legacy_front" ] || [ -z "$legacy_raw" ] || [ "$legacy_front" != "$legacy_raw" ]; then
                echo 'legacy WBD_FRONT_PORT/WBD_RAW_PORT differ; run: wbd-server set WBD_PORT PORT' >&2
                exit 1
            fi
            WBD_PORT=$legacy_front
        else
            WBD_PORT=40443
        fi
    fi
    case "$WBD_PORT" in *[!0-9]*|'') echo 'WBD_PORT must be numeric' >&2; exit 1;; esac
    [ "$WBD_PORT" -ge 1 ] && [ "$WBD_PORT" -le 65535 ] || { echo 'WBD_PORT must be 1..65535' >&2; exit 1; }
    # Keep the firewall helper's internal front/raw arguments compatible while
    # enforcing a single product-level public port.
    WBD_FRONT_PORT=$WBD_PORT
    WBD_RAW_PORT=$WBD_PORT

    [ -n "${WBD_ROUTE_KEY:-}" ] && [ ${#WBD_ROUTE_KEY} -ge 16 ] || { echo 'WBD_ROUTE_KEY must be >=16 chars' >&2; exit 1; }
    [ -n "${WBD_USERNAME:-}" ] && [ -n "${WBD_PASSWORD:-}" ] || { echo 'WBD_USERNAME/WBD_PASSWORD required' >&2; exit 1; }
    case "$WBD_FIREWALL_BACKEND" in auto|nft|iptables) ;; *) echo 'WBD_FIREWALL_BACKEND must be auto, nft, or iptables' >&2; exit 1;; esac
    case "$WBD_DECOY_TARGET" in *:*) ;; *) echo 'WBD_DECOY_TARGET must be host:port' >&2; exit 1;; esac
}

set_config() {
    need_root; write_default_config
    key=${1:-}; value=${2-}
    case "$key" in
      WBD_LISTEN_IP|WBD_PORT|WBD_SERVER_NAME|WBD_DECOY_TARGET|WBD_ROUTE_KEY|WBD_USERNAME|WBD_PASSWORD|WBD_MAX_FRONT_CONNS|WBD_MAX_SESSIONS|WBD_TICKET_TTL|WBD_PLATFORM_LISTEN|WBD_LINK_LISTEN|WBD_UDP_IDLE|WBD_TCP_IDLE|WBD_FIREWALL_BACKEND|WBD_NFT_INPUT) ;;
      *) echo "unsupported setting: $key" >&2; exit 2;;
    esac
    quoted=$(q "$value")
    tmp="$CONFIG.tmp.$$"
    if [ "$key" = WBD_PORT ]; then
        # Setting the new shared port is also the explicit migration action for
        # old two-port configs, so remove obsolete keys atomically.
        awk -v k="$key" -v line="$key=$quoted" 'BEGIN{done=0} index($0,"WBD_FRONT_PORT=")==1 || index($0,"WBD_RAW_PORT=")==1 {next} index($0,k"=")==1 {print line; done=1; next} {print} END{if(!done) print line}' "$CONFIG" >"$tmp"
    else
        awk -v k="$key" -v line="$key=$quoted" 'BEGIN{done=0} index($0,k"=")==1 {print line; done=1; next} {print} END{if(!done) print line}' "$CONFIG" >"$tmp"
    fi
    chmod 600 "$tmp"; mv "$tmp" "$CONFIG"
    load_config
    if [ "$key" = WBD_SERVER_NAME ]; then
        echo "$key updated; run: wbd-server regen-certs && wbd-server restart"
    else
        echo "$key updated; run: wbd-server restart"
    fi
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
    echo 'WBD certificates regenerated; restart WBD to use them'
}

install_files() {
    need_root
    arch=$(uname -m)
    case "$arch" in x86_64|amd64) want=amd64;; aarch64|arm64) want=arm64;; *) echo "unsupported arch: $arch" >&2; exit 1;; esac
    [ -x "$SELF_DIR/bin/wbd-reality-front" ] || { echo 'run install from extracted WBD server bundle' >&2; exit 1; }
    bundle_arch=$(cat "$SELF_DIR/ARCH" 2>/dev/null || true)
    [ "$bundle_arch" = "$want" ] || { echo "bundle arch=$bundle_arch host=$want" >&2; exit 1; }
    command -v systemctl >/dev/null || { echo 'systemd/systemctl is required' >&2; exit 1; }
    if ! command -v nft >/dev/null 2>&1 && ! command -v iptables >/dev/null 2>&1; then echo 'host requires nft or iptables' >&2; exit 1; fi
    mkdir -p "$PREFIX/bin" "$ETC" "$RUN/tickets"
    chmod 700 "$RUN/tickets"
    for f in wbd-reality-front wbd-faketcp-mux wbd-link-server-mux wbd-platform-proxy-server wbd_dtls_shim wbd-server-cert; do
        install -m 0755 "$SELF_DIR/bin/$f" "$PREFIX/bin/$f"
    done
    install -m 0755 "$SELF_DIR/linux_server_firewall.sh" "$PREFIX/bin/linux_server_firewall.sh"
    install -m 0755 "$SELF_DIR/linux_server_guard.sh" "$PREFIX/bin/linux_server_guard.sh"
    install -m 0755 "$SELF_DIR/linux_server_manager.sh" "$PREFIX/bin/wbd-server"
    write_default_config
    load_config
    if [ ! -s "$ETC/front.pem" ] || [ ! -s "$ETC/front.key" ]; then "$PREFIX/bin/wbd-server-cert" -name "$WBD_SERVER_NAME" -cert "$ETC/front.pem" -key "$ETC/front.key"; fi
    if [ ! -s "$ETC/dtls.pem" ] || [ ! -s "$ETC/dtls.key" ]; then "$PREFIX/bin/wbd-server-cert" -name wbd-dtls.local -cert "$ETC/dtls.pem" -key "$ETC/dtls.key"; fi
    chmod 600 "$ETC"/*.key "$CONFIG"
    install_unit
    systemctl enable wbd-server.service >/dev/null
    echo "WBD installed but not started. Configure $CONFIG (especially domain/port/credentials), run 'wbd-server doctor', then 'wbd-server start'."
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
    "$PREFIX/bin/wbd-reality-front" server -listen "$WBD_LISTEN_IP:$WBD_PORT" -target "$WBD_DECOY_TARGET" -server-name "$WBD_SERVER_NAME" -cert "$ETC/front.pem" -key "$ETC/front.key" -route-key "$WBD_ROUTE_KEY" -username "$WBD_USERNAME" -password "$WBD_PASSWORD" -ticket-dir "$RUN/tickets" -max-conns "$WBD_MAX_FRONT_CONNS" & pids="$pids $!"
    guard="$PREFIX/bin/linux_server_guard.sh"
    set -- "$guard" --backend "$WBD_FIREWALL_BACKEND" --front-port "$WBD_PORT" --raw-port "$WBD_PORT" --state "$RUN/server-firewall.state"
    [ -z "$WBD_NFT_INPUT" ] || set -- "$@" --nft-input "$WBD_NFT_INPUT"
    set -- "$@" -- "$PREFIX/bin/wbd-faketcp-mux" server --listen "$WBD_LISTEN_IP:$WBD_PORT" --dtls-shim "$PREFIX/bin/wbd_dtls_shim" --link-target "$WBD_LINK_LISTEN" --cert "$ETC/dtls.pem" --key "$ETC/dtls.key" --max-sessions "$WBD_MAX_SESSIONS"
    "$@" & main=$!; pids="$pids $main"
    if wait "$main"; then rc=0; else rc=$?; fi
    exit "$rc"
}

uninstall_files() {
    need_root
    fp=40443; rp=40443; backend=auto; nft_input=
    if [ -r "$CONFIG" ]; then
        # Capture the active ownership parameters before stopping/deleting the service.
        # Old two-port configs are still cleaned with their exact historic values.
        # shellcheck disable=SC1090
        . "$CONFIG"
        if [ -n "${WBD_PORT:-}" ]; then
            fp=$WBD_PORT; rp=$WBD_PORT
        else
            fp=${WBD_FRONT_PORT:-40443}; rp=${WBD_RAW_PORT:-40000}
        fi
        backend=${WBD_FIREWALL_BACKEND:-$backend}; nft_input=${WBD_NFT_INPUT:-}
    fi
    systemctl disable --now wbd-server.service 2>/dev/null || true
    if [ -x "$PREFIX/bin/linux_server_firewall.sh" ]; then
        set -- "$PREFIX/bin/linux_server_firewall.sh" cleanup --backend "$backend" --front-port "$fp" --raw-port "$rp" --state "$RUN/server-firewall.state"
        [ -z "$nft_input" ] || set -- "$@" --nft-input "$nft_input"
        "$@" 2>/dev/null || true
    fi
    rm -f "$UNIT"
    systemctl daemon-reload
    rm -rf "$PREFIX" "$ETC" "$RUN"
    echo 'WBD uninstalled'
}

doctor() {
    load_config
    fail=0
    printf 'config: OK (%s)\n' "$CONFIG"
    for f in wbd-reality-front wbd-faketcp-mux wbd-link-server-mux wbd-platform-proxy-server wbd_dtls_shim wbd-server-cert linux_server_firewall.sh linux_server_guard.sh; do
        if [ -x "$PREFIX/bin/$f" ]; then echo "binary: OK $f"; else echo "binary: MISSING $f"; fail=1; fi
    done
    if command -v systemctl >/dev/null 2>&1; then echo 'systemd: OK'; else echo 'systemd: MISSING'; fail=1; fi
    if command -v nft >/dev/null 2>&1; then echo 'firewall: OK nft'; elif command -v iptables >/dev/null 2>&1; then echo 'firewall: OK iptables'; else echo 'firewall: MISSING nft/iptables'; fail=1; fi
    [ -r "$ETC/front.pem" ] && [ -r "$ETC/front.key" ] && echo 'front certificate: OK' || { echo 'front certificate: MISSING'; fail=1; }
    [ -r "$ETC/dtls.pem" ] && [ -r "$ETC/dtls.key" ] && echo 'DTLS certificate: OK' || { echo 'DTLS certificate: MISSING'; fail=1; }
    echo "public: $WBD_LISTEN_IP:$WBD_PORT reality_admission=1 faketcp=1"
    echo "front:  server_name=$WBD_SERVER_NAME decoy=$WBD_DECOY_TARGET"
    echo "limits: front=$WBD_MAX_FRONT_CONNS sessions=$WBD_MAX_SESSIONS ticket_ttl=$WBD_TICKET_TTL"
    if [ "$fail" -ne 0 ]; then echo 'WBD_SERVER_DOCTOR_FAIL'; return 1; fi
    echo 'WBD_SERVER_DOCTOR_PASS'
}

show_config() { load_config; sed -E 's/^(WBD_(PASSWORD|ROUTE_KEY))=.*/\1=<redacted>/' "$CONFIG"; }

case "${1:-help}" in
 install) install_files ;;
 uninstall) uninstall_files ;;
 run) run_server ;;
 start|stop|restart) need_root; systemctl "$1" wbd-server.service ;;
 pause) need_root; systemctl stop wbd-server.service ;;
 resume) need_root; systemctl start wbd-server.service ;;
 status) systemctl --no-pager --full status wbd-server.service || true ;;
 logs) exec journalctl -u wbd-server.service -f -n 100 ;;
 config) need_root; write_default_config; ${EDITOR:-vi} "$CONFIG" ;;
 set) [ $# -eq 3 ] || { echo 'usage: wbd-server set KEY VALUE' >&2; exit 2; }; set_config "$2" "$3" ;;
 regen-certs) regen_certs ;;
 doctor) doctor ;;
 show-config) show_config ;;
 help|-h|--help) cat <<EOF
usage: wbd-server COMMAND
  install       install this amd64/arm64 bundle, enable service, do not start it
  uninstall     stop, remove firewall state, binaries, config and service
  start|resume  start server
  stop|pause    stop server and clean WBD firewall rules
  restart       restart after config changes
  status        systemd status
  logs          follow journal logs
  config        edit $CONFIG
  set KEY VALUE set one supported option without an editor
  regen-certs   regenerate local TLS/DTLS certificates (run after server-name change)
  doctor        validate config, runtime files and host facilities
  show-config   print settings with secrets redacted

Main settings: WBD_PORT, WBD_LISTEN_IP, WBD_SERVER_NAME, WBD_DECOY_TARGET,
WBD_ROUTE_KEY, WBD_USERNAME, WBD_PASSWORD, session limits, timeouts and firewall
backend. WBD_PORT is the one public TCP port shared by Reality-like admission and
raw FakeTCP. The decoy domain/target are used only by the setup front; sustained
VPN payload remains FakeTCP -> DTLS 1.3 -> LINK.
EOF
 ;;
 *) echo "unknown command: $1" >&2; exit 2;;
esac
