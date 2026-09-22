#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_WORKSPACE:?missing GITHUB_WORKSPACE}"
: "${GITHUB_SHA:?missing GITHUB_SHA}"
: "${WBD_REALPATH_ARTIFACT_DIR:?missing WBD_REALPATH_ARTIFACT_DIR}"

ART="$WBD_REALPATH_ARTIFACT_DIR"
CLIENT_BIN="$ART/wbd-client"
SERVER_BIN="$ART/wbd-server"
GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"
mkdir -p "$ART"

SFX="$$"
BIZ="wbiz-$SFX"
CLI="wcli-$SFX"
RTR="wrtr-$SFX"
SRV="wsrv-$SFX"
TGT="wtgt-$SFX"
NAMESPACES=("$BIZ" "$CLI" "$RTR" "$SRV" "$TGT")
CLIENT_PID=""
SERVER_PID=""
BIZ_PID=""
TGT_PID=""
CAP_PIDS=()

cleanup() {
  set +e
  for pid in "$BIZ_PID" "$TGT_PID" "$CLIENT_PID" "$SERVER_PID"; do
    if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; fi
  done
  for pid in "${CAP_PIDS[@]:-}"; do
    if [[ -n "$pid" ]]; then kill -INT "$pid" 2>/dev/null || true; fi
  done
  sleep 0.2
  for ns in "${NAMESPACES[@]}"; do ip netns del "$ns" 2>/dev/null || true; done
}
trap cleanup EXIT

for ns in "${NAMESPACES[@]}"; do
  ip netns add "$ns"
  ip -n "$ns" link set lo up
done

# Four real veth hops. The client business ingress and target are separate
# namespaces so TPROXY sees actual PREROUTING traffic instead of local output.
ip link add "wb$SFX" type veth peer name "wc0$SFX"
ip link set "wb$SFX" netns "$BIZ"
ip link set "wc0$SFX" netns "$CLI"
ip -n "$BIZ" link set "wb$SFX" name biz0
ip -n "$CLI" link set "wc0$SFX" name cbiz

ip link add "wc1$SFX" type veth peer name "wr0$SFX"
ip link set "wc1$SFX" netns "$CLI"
ip link set "wr0$SFX" netns "$RTR"
ip -n "$CLI" link set "wc1$SFX" name cwan
ip -n "$RTR" link set "wr0$SFX" name rcli

ip link add "wr1$SFX" type veth peer name "ws0$SFX"
ip link set "wr1$SFX" netns "$RTR"
ip link set "ws0$SFX" netns "$SRV"
ip -n "$RTR" link set "wr1$SFX" name rsrv
ip -n "$SRV" link set "ws0$SFX" name swan

ip link add "ws1$SFX" type veth peer name "wt0$SFX"
ip link set "ws1$SFX" netns "$SRV"
ip link set "wt0$SFX" netns "$TGT"
ip -n "$SRV" link set "ws1$SFX" name slan
ip -n "$TGT" link set "wt0$SFX" name tgt0

ip -n "$BIZ" addr add 10.40.0.2/24 dev biz0
ip -n "$CLI" addr add 10.40.0.1/24 dev cbiz
ip -n "$CLI" addr add 198.18.0.2/30 dev cwan
ip -n "$RTR" addr add 198.18.0.1/30 dev rcli
ip -n "$RTR" addr add 198.18.0.5/30 dev rsrv
ip -n "$SRV" addr add 198.18.0.6/30 dev swan
ip -n "$SRV" addr add 10.50.0.1/24 dev slan
ip -n "$TGT" addr add 10.50.0.2/24 dev tgt0

for pair in   "$BIZ biz0" "$CLI cbiz" "$CLI cwan" "$RTR rcli" "$RTR rsrv"   "$SRV swan" "$SRV slan" "$TGT tgt0"; do
  read -r ns dev <<<"$pair"
  ip -n "$ns" link set "$dev" mtu 1400 up
done

ip -n "$BIZ" route add default via 10.40.0.1
ip -n "$CLI" route add default via 198.18.0.1
ip -n "$SRV" route add default via 198.18.0.5
ip -n "$TGT" route add default via 10.50.0.1

for ns in "$CLI" "$RTR" "$SRV"; do
  ip netns exec "$ns" sysctl -qw net.ipv4.conf.all.rp_filter=0
  ip netns exec "$ns" sysctl -qw net.ipv4.conf.default.rp_filter=0
done
ip netns exec "$RTR" sysctl -qw net.ipv4.ip_forward=1
ip netns exec "$CLI" sysctl -qw net.ipv4.ip_forward=1

# Kernel TCP must not answer the FakeTCP four-tuples with RST. The product raw
# endpoint remains the only consumer; this is harness firewall state.
ip netns exec "$CLI" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP
ip netns exec "$SRV" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP

# No hidden bandwidth cap. Each underlay direction gets exactly one netem delay
# qdisc; the large packet limit is reported and should not be a bottleneck.
ip netns exec "$RTR" tc qdisc add dev rsrv root netem limit 200000 delay 300ms
ip netns exec "$RTR" tc qdisc add dev rcli root netem limit 200000 delay 300ms
ip netns exec "$RTR" tc -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-before.json"
ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-before.json"

FILTER='tcp and host 198.18.0.2 and host 198.18.0.6'
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q in  -s 96 -B 8192 -w "$ART/c2s-pre.pcap"  "$FILTER" 2> "$ART/c2s-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q out -s 96 -B 8192 -w "$ART/c2s-post.pcap" "$FILTER" 2> "$ART/c2s-post.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q in  -s 96 -B 8192 -w "$ART/s2c-pre.pcap"  "$FILTER" 2> "$ART/s2c-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q out -s 96 -B 8192 -w "$ART/s2c-post.pcap" "$FILTER" 2> "$ART/s2c-post.tcpdump.log" &
CAP_PIDS+=("$!")
sleep 1

CERT="$ART/server-cert.pem"
KEY="$ART/server-key.pem"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=qual.test'   -keyout "$KEY" -out "$CERT" > "$ART/openssl.log" 2>&1

TUNNEL_ID="00112233445566778899aabbccddeeff"
INSTALLATION_ID="11223344556677889900aabbccddeeff"
ROUTE_KEY_HEX="00112233445566778899aabbccddeeffffeeddccbbaa00998877665544332211"

ip netns exec "$SRV" "$SERVER_BIN"   --raw-interface swan   --listen-ip 198.18.0.6 --listen-port 443   --tun-name wbdg0 --lease-pool 10.66.0.0/16 --lease4 10.66.0.2/32   --tunnel-id "$TUNNEL_ID" --account qual --installation-id "$INSTALLATION_ID"   --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX"   --tls-cert "$CERT" --tls-key "$KEY"   --username qual --password qualpass --decoy 10.50.0.2:4433   --server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 1   --firewall iptables   > "$ART/server.log" 2>&1 &
SERVER_PID="$!"

sleep 1
ip netns exec "$CLI" "$CLIENT_BIN"   --raw-interface cwan --local-ip 198.18.0.2 --source-port 40000   --server-ip 198.18.0.6 --server-port 443   --tunnel-id "$TUNNEL_ID" --lease4 10.66.0.2/32   --account qual --installation-id "$INSTALLATION_ID"   --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX"   --username qual --password qualpass   --client-record-limit 1300 --mtu 1400 --fec-parity 20 --lanes 1   --tproxy-port 12345 --mark 66 --route-table 1066 --rule-priority 1066   > "$ART/client.log" 2>&1 &
CLIENT_PID="$!"

sleep 3
kill -0 "$SERVER_PID"
kill -0 "$CLIENT_PID"

START_NS="$(python3 - <<'PY'
import time
print(time.monotonic_ns() + 12_000_000_000)
PY
)"
printf '%s\n' "$START_NS" > "$ART/start-monotonic-ns.txt"

ip netns exec "$TGT" python3 "$GEN"   --role target --bind 10.50.0.2:18080   --start-ns "$START_NS" --duration 8 --drain 10 --rate-mbps 0.5 --seed 2026092201   --output "$ART/target.json" > "$ART/target.log" 2>&1 &
TGT_PID="$!"

ip netns exec "$BIZ" python3 "$GEN"   --role biz --bind 10.40.0.2:28080 --peer 10.50.0.2:18080   --start-ns "$START_NS" --duration 8 --drain 10 --rate-mbps 0.5 --seed 2026092202   --output "$ART/biz.json" > "$ART/biz.log" 2>&1 &
BIZ_PID="$!"

wait "$BIZ_PID"
BIZ_PID=""
wait "$TGT_PID"
TGT_PID=""

# The formal processes must still be alive after the complete business drain.
kill -0 "$CLIENT_PID"
kill -0 "$SERVER_PID"

ip netns exec "$RTR" tc -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-after.json"
ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-after.json"
ip netns exec "$CLI" nft -a list table inet wbd_tproxy > "$ART/client-tproxy-active.txt"
ip netns exec "$CLI" ip -4 rule show > "$ART/client-rules-active.txt"
ip netns exec "$CLI" ip -4 route show table 1066 > "$ART/client-table-active.txt"
ip netns exec "$SRV" ip -details link show wbdg0 > "$ART/server-tun-active.txt"
ip netns exec "$SRV" ip -4 route show > "$ART/server-routes-active.txt"
ip netns exec "$SRV" iptables-save > "$ART/server-iptables-active.txt"
ip netns exec "$RTR" ip -4 route show > "$ART/router-routes.txt"
ip netns exec "$CLI" ip -s -j link show > "$ART/client-links.json"
ip netns exec "$RTR" ip -s -j link show > "$ART/router-links.json"
ip netns exec "$SRV" ip -s -j link show > "$ART/server-links.json"

kill -TERM "$CLIENT_PID" "$SERVER_PID"
wait "$CLIENT_PID" || true
wait "$SERVER_PID" || true
CLIENT_PID=""
SERVER_PID=""
sleep 1

# Prove WBD-owned client state is gone after graceful shutdown.
ip netns exec "$CLI" nft list tables > "$ART/client-nft-after.txt" 2>&1 || true
ip netns exec "$CLI" ip -4 rule show > "$ART/client-rules-after.txt"
ip netns exec "$SRV" ip -4 route show > "$ART/server-routes-after.txt"
ip netns exec "$SRV" iptables-save > "$ART/server-iptables-after.txt"

for pid in "${CAP_PIDS[@]}"; do kill -INT "$pid" 2>/dev/null || true; done
for pid in "${CAP_PIDS[@]}"; do wait "$pid" 2>/dev/null || true; done
CAP_PIDS=()

python3 - "$ART" "$GITHUB_SHA" "$GITHUB_WORKSPACE" <<'PY'
import hashlib, json, sys
from pathlib import Path
art = Path(sys.argv[1])
source = sys.argv[2]
root = Path(sys.argv[3])
harness = [
    ".github/workflows/next-realpath-calibration.yml",
    "scripts/realpath_calibration.sh",
    "tools/realpath_udp_duplex.py",
    "tools/check_realpath_calibration.py",
]
files = {}
for rel in harness:
    data = (root / rel).read_bytes()
    files[rel] = hashlib.sha256(data).hexdigest()
manifest = {
    "schema": 1,
    "source_sha": source,
    "harness_sha": source,
    "harness_file_sha256": files,
    "runner": "ubuntu-24.04",
    "topology": {
        "business": "10.40.0.2/24 -> client TPROXY",
        "client_underlay": "198.18.0.2/30",
        "router_client": "198.18.0.1/30",
        "router_server": "198.18.0.5/30",
        "server_underlay": "198.18.0.6/30",
        "server_shared_tun": "wbdg0 10.66.0.0/16 lease 10.66.0.2/32",
        "target": "10.50.0.2/24",
    },
    "config": {
        "lanes": 1,
        "fec_parity": 20,
        "padding": False,
        "mtu": 1400,
        "one_way_delay_ms_each_direction": 300,
        "loss_percent": 0,
        "qdisc_limit_packets": 200000,
        "rate_limit": None,
        "application_mbps_each_direction": 0.5,
        "duration_s": 8,
        "drain_s": 10,
    },
}
(art / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY

printf 'WBD_REALPATH_CALIBRATION_CAPTURED source_sha=%s artifact=%s\n' "$GITHUB_SHA" "$ART"
