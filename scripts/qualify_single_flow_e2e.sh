#!/usr/bin/env bash
set -euo pipefail

FAKETCP=''
MUX=''
DTLS=''
DECOY=''
OUT='/tmp/wbd-single-flow-e2e'
while [ "$#" -gt 0 ]; do
  case "$1" in
    --faketcp) FAKETCP="$2"; shift 2 ;;
    --mux) MUX="$2"; shift 2 ;;
    --dtls) DTLS="$2"; shift 2 ;;
    --decoy) DECOY="$2"; shift 2 ;;
    --out) OUT="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
for v in FAKETCP MUX DTLS DECOY; do
  value="${!v}"
  [ -n "$value" ] && [ -x "$value" ] || { echo "missing executable --$(printf '%s' "$v" | tr '[:upper:]' '[:lower:]'): $value" >&2; exit 2; }
done
[ "$(id -u)" -eq 0 ] || { echo 'single-flow qualification requires root' >&2; exit 2; }
for cmd in ip iptables tcpdump python3 grep; do command -v "$cmd" >/dev/null || { echo "missing $cmd" >&2; exit 2; }; done

mkdir -p "$OUT" "$OUT/tickets" "$OUT/decoy-tickets"
rm -f "$OUT"/*.log "$OUT"/*.pcap "$OUT"/*.ticket "$OUT"/*tunnel.json
rm -rf "$OUT/tickets"/* "$OUT/decoy-tickets"/*

PIDS=()
NETNS=()
reset_env() {
  local p n
  for p in "${PIDS[@]:-}"; do kill -TERM "$p" 2>/dev/null || true; done
  sleep .2
  for p in "${PIDS[@]:-}"; do kill -KILL "$p" 2>/dev/null || true; done
  for n in "${NETNS[@]:-}"; do ip netns del "$n" 2>/dev/null || true; done
  PIDS=()
  NETNS=()
}
trap reset_env EXIT

wait_log() {
  local file="$1" needle="$2" loops="${3:-400}"
  local i
  for i in $(seq 1 "$loops"); do
    if [ -f "$file" ] && grep -Eq "$needle" "$file"; then return 0; fi
    sleep .05
  done
  echo "timeout waiting for $needle in $file" >&2
  tail -n 120 "$file" 2>/dev/null || true
  return 1
}

validate_tunnel_json() {
  python3 - "$1" <<'PY'
import ipaddress,json,re,sys
p=sys.argv[1]
with open(p,'r',encoding='utf-8') as f: x=json.load(f)
assert re.fullmatch(r'[0-9a-f]{32}', x['tunnel_id']), x
iface=ipaddress.ip_interface(x['address4'])
assert iface.version == 4 and iface.network.prefixlen == 32, x
assert isinstance(x['routes4'], list) and x['routes4'], x
for r in x['routes4']: assert ipaddress.ip_network(r, strict=False).version == 4
print('TUNNEL_CONFIG_PASS', x['tunnel_id'], x['address4'], ','.join(x['routes4']))
PY
}

setup_pair() {
  local c="$1" s="$2" cif="$3" sif="$4" cip="$5" sip="$6"
  ip netns add "$c"; NETNS+=("$c")
  ip netns add "$s"; NETNS+=("$s")
  ip link add "$cif" type veth peer name "$sif"
  ip link set "$cif" netns "$c"
  ip link set "$sif" netns "$s"
  ip -n "$c" addr add "$cip/24" dev "$cif"
  ip -n "$s" addr add "$sip/24" dev "$sif"
  ip -n "$c" link set lo up; ip -n "$s" link set lo up
  ip -n "$c" link set "$cif" up; ip -n "$s" link set "$sif" up
  ip netns exec "$c" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
  ip netns exec "$s" iptables -I OUTPUT -p tcp --tcp-flags RST RST -j DROP
}

run_primary() {
  local C="wbdsfc$$" S="wbdsfs$$"
  setup_pair "$C" "$S" vc vs 10.88.0.2 10.88.0.1

  cat >"$OUT/echo.py" <<'PY'
import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('127.0.0.1',47000))
while True:
    b,a=s.recvfrom(65535)
    s.sendto(b,a)
PY
  ip netns exec "$S" python3 "$OUT/echo.py" >"$OUT/echo.log" 2>&1 & PIDS+=("$!")

  ip netns exec "$C" tcpdump --immediate-mode -U -i vc -s 0 -w "$OUT/public.pcap" \
    'tcp and host 10.88.0.1 and port 443' >"$OUT/tcpdump.log" 2>&1 &
  local CAP=$!; PIDS+=("$CAP")
  wait_log "$OUT/tcpdump.log" 'listening on vc' 200

  ip netns exec "$S" "$MUX" server \
    --listen 10.88.0.1:443 --dtls-shim "$DTLS" --link-target 127.0.0.1:47000 \
    --cert "$OUT/dtls.pem" --key "$OUT/dtls.key" \
    --front-cert "$OUT/front.pem" --front-key "$OUT/front.key" \
    --server-name wbd.test --route-key 0123456789abcdef0123456789abcdef \
    --username singleflow --password singleflow-test-password \
    --ticket-dir "$OUT/tickets" --fallback-target 127.0.0.1:44444 \
    --bootstrap-timeout 12s --max-sessions 8 >"$OUT/mux.log" 2>&1 &
  local MUXPID=$!; PIDS+=("$MUXPID")
  wait_log "$OUT/mux.log" 'READY role=server-mux.*single_flow_bootstrap=true.*logical_tunnel=true' 300

  ip netns exec "$C" "$FAKETCP" client \
    --local-udp 127.0.0.1:45101 --source 10.88.0.2:41001 --remote 10.88.0.1:443 \
    --shadow-recovery legacy --reality-server-name wbd.test \
    --reality-route-key 0123456789abcdef0123456789abcdef \
    --reality-username singleflow --reality-password singleflow-test-password \
    --reality-ticket-out "$OUT/client.ticket" \
    --reality-installation-id 00112233445566778899aabbccddeeff \
    --reality-tunnel-config-out "$OUT/client-tunnel.json" \
    --reality-verify-server=false --reality-timeout 12s >"$OUT/faketcp.log" 2>&1 &
  local FC=$!; PIDS+=("$FC")
  wait_log "$OUT/faketcp.log" 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1.*logical_tunnel=1' 600
  wait_log "$OUT/faketcp.log" 'READY role=client.*single_flow_bootstrap=true' 100
  wait_log "$OUT/mux.log" 'WBD_SINGLE_FLOW_BOOTSTRAP_READY.*same_flow=1' 100
  [ "$(tr -d '\r\n' <"$OUT/client.ticket" | wc -c)" -eq 64 ]
  validate_tunnel_json "$OUT/client-tunnel.json"

  ip netns exec "$C" "$DTLS" client 46101 127.0.0.1 45101 none none >"$OUT/dtls.log" 2>&1 &
  local DPID=$!; PIDS+=("$DPID")
  wait_log "$OUT/dtls.log" 'READY role=client version=DTLSv1.3' 900
  wait_log "$OUT/mux.log" 'WBD_DTLS_SERVER_ACCEPT_PASS version=DTLSv1.3' 100
  wait_log "$OUT/mux.log" 'READY role=server version=DTLSv1.3' 100

  cat >"$OUT/probe.py" <<'PY'
import socket,time
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); s.settimeout(.25)
ok=0
for i in range(20):
    want=b'SINGLE_FLOW_ECHO_'+i.to_bytes(2,'big'); end=time.time()+3
    while time.time()<end:
        s.sendto(want,('127.0.0.1',46101))
        try: got,_=s.recvfrom(65535)
        except socket.timeout: continue
        assert got == want,(got,want); ok += 1; break
    else: raise SystemExit('echo timeout at %d'%i)
print('SINGLE_FLOW_ECHO_PASS count=%d'%ok)
PY
  ip netns exec "$C" python3 "$OUT/probe.py" >"$OUT/probe.log" 2>&1
  grep -q 'SINGLE_FLOW_ECHO_PASS count=20' "$OUT/probe.log"

  sleep .2
  kill -INT "$CAP" 2>/dev/null || true
  wait "$CAP" 2>/dev/null || true
  local SYN_LINES UNIQUE_SYN_SEQ
  SYN_LINES="$(tcpdump -nn -tt -r "$OUT/public.pcap" \
    'src host 10.88.0.2 and dst host 10.88.0.1 and src port 41001 and dst port 443 and tcp[tcpflags] & (tcp-syn|tcp-ack) = tcp-syn' 2>/dev/null || true)"
  [ -n "$SYN_LINES" ]
  UNIQUE_SYN_SEQ="$(printf '%s\n' "$SYN_LINES" | sed -n 's/.*seq \([0-9][0-9]*\).*/\1/p' | sort -u | grep -c . || true)"
  [ "$UNIQUE_SYN_SEQ" -eq 1 ]
  if tcpdump -nn -r "$OUT/public.pcap" \
      'src host 10.88.0.2 and dst host 10.88.0.1 and dst port 443 and not src port 41001' 2>/dev/null | grep -q .; then
    echo 'unexpected second client 4-tuple to public port' >&2; return 1
  fi
  [ "$(grep -c 'WBD_SINGLE_FLOW_BOOTSTRAP_READY' "$OUT/faketcp.log")" -eq 1 ]
  [ "$(grep -c '^READY role=client' "$OUT/faketcp.log")" -eq 1 ]
  kill -0 "$FC"; kill -0 "$MUXPID"
  echo 'SINGLE_FLOW_PUBLIC_INVARIANT_PASS unique_syn_seq=1 tuple=10.88.0.2:41001-10.88.0.1:443 logical_tunnel_v2=1'
}

run_fallback() {
  reset_env
  local C="wbdsffbc$$" S="wbdsffbs$$"
  setup_pair "$C" "$S" fbc fbs 10.89.0.2 10.89.0.1
  rm -f "$OUT/fallback.ticket" "$OUT/fallback-tunnel.json"
  rm -rf "$OUT/decoy-tickets"/*

  ip netns exec "$S" "$DECOY" --listen 127.0.0.1:44444 \
    --cert "$OUT/front.pem" --key "$OUT/front.key" \
    --username probe --password probe-password --ticket-dir "$OUT/decoy-tickets" \
    >"$OUT/fallback-decoy.log" 2>&1 &
  local DECOYPID=$!; PIDS+=("$DECOYPID")
  wait_log "$OUT/fallback-decoy.log" 'WBD_SINGLE_FLOW_DECOY_READY.*logical_tunnel_v2=1' 300

  ip netns exec "$S" "$MUX" server \
    --listen 10.89.0.1:443 --dtls-shim "$DTLS" --link-target 127.0.0.1:47000 \
    --cert "$OUT/dtls.pem" --key "$OUT/dtls.key" \
    --front-cert "$OUT/front.pem" --front-key "$OUT/front.key" \
    --server-name wbd.test --route-key 0123456789abcdef0123456789abcdef \
    --username singleflow --password singleflow-test-password \
    --ticket-dir "$OUT/tickets" --fallback-target 127.0.0.1:44444 \
    --bootstrap-timeout 12s --max-sessions 8 >"$OUT/fallback-mux.log" 2>&1 &
  local MUXPID=$!; PIDS+=("$MUXPID")
  wait_log "$OUT/fallback-mux.log" 'READY role=server-mux.*fallback=true.*logical_tunnel=true' 300
  [ "$(grep -c '^BOUND role=server' "$OUT/fallback-mux.log" || true)" -eq 0 ]

  ip netns exec "$C" "$FAKETCP" client \
    --local-udp 127.0.0.1:45102 --source 10.89.0.2:41002 --remote 10.89.0.1:443 \
    --shadow-recovery legacy --reality-server-name wbd.test \
    --reality-route-key fedcba9876543210fedcba9876543210 \
    --reality-username probe --reality-password probe-password \
    --reality-ticket-out "$OUT/fallback.ticket" \
    --reality-installation-id fedcba98765432100123456789abcdef \
    --reality-tunnel-config-out "$OUT/fallback-tunnel.json" \
    --reality-verify-server=false --reality-timeout 12s >"$OUT/fallback-client.log" 2>&1 &
  local FC=$!; PIDS+=("$FC")

  wait_log "$OUT/fallback-mux.log" 'WBD_SINGLE_FLOW_FALLBACK remote=10.89.0.2:41002 sni=wbd.test' 600
  wait_log "$OUT/fallback-decoy.log" 'WBD_SINGLE_FLOW_DECOY_AUTH_PASS version=304.*ticket_nonzero=true' 200
  wait_log "$OUT/fallback-client.log" 'WBD_SINGLE_FLOW_BOOTSTRAP_READY tls=304 server_name=wbd.test same_flow=1 logical_tunnel=1' 200
  [ "$(tr -d '\r\n' <"$OUT/fallback.ticket" | wc -c)" -eq 64 ]
  validate_tunnel_json "$OUT/fallback-tunnel.json"
  [ "$(grep -c '^BOUND role=server' "$OUT/fallback-mux.log" || true)" -eq 0 ]
  if grep -q 'WBD_SINGLE_FLOW_BOOTSTRAP_READY remote=10.89.0.2:41002' "$OUT/fallback-mux.log"; then
    echo 'wrong-marker probe was incorrectly admitted as WBD' >&2; return 1
  fi
  kill -0 "$FC"; kill -0 "$MUXPID"
  echo 'SINGLE_FLOW_FALLBACK_PASS same_public_flow=1 tls13_decoy=1 logical_tunnel_v2=1 wbd_dtls_worker=0'
}

run_primary
run_fallback

echo 'SINGLE_FLOW_E2E_QUALIFY_PASS primary=1 fallback=1 one_public_flow=1 logical_tunnel_v2=1'
