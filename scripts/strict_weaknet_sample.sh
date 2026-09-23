#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_WORKSPACE:?missing GITHUB_WORKSPACE}"
: "${GITHUB_SHA:?missing GITHUB_SHA}"
: "${WBD_STRICT_ARTIFACT_DIR:?missing WBD_STRICT_ARTIFACT_DIR}"
: "${WBD_STRICT_MODE:?missing WBD_STRICT_MODE}"
: "${WBD_STRICT_SCENARIO:?missing WBD_STRICT_SCENARIO}"
: "${WBD_STRICT_SEED:?missing WBD_STRICT_SEED}"
: "${WBD_STRICT_RATE_MBPS:?missing WBD_STRICT_RATE_MBPS}"
: "${WBD_STRICT_LANES:?missing WBD_STRICT_LANES}"
: "${WBD_STRICT_TC:?missing WBD_STRICT_TC}"

ART="$WBD_STRICT_ARTIFACT_DIR"
MODE="$WBD_STRICT_MODE"
SCENARIO="$WBD_STRICT_SCENARIO"
SEED="$WBD_STRICT_SEED"
RATE="$WBD_STRICT_RATE_MBPS"
LANES="$WBD_STRICT_LANES"
CLIENT_BIN="$ART/wbd-client"
SERVER_BIN="$ART/wbd-server"
GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"
STAGER="$GITHUB_WORKSPACE/tools/strict_weaknet_stage.py"
SAMPLER="$GITHUB_WORKSPACE/tools/strict_resource_sampler.py"
TC_BIN="$WBD_STRICT_TC"
DIAGNOSTIC_RATE_ONLY="${WBD_STRICT_DIAGNOSTIC_RATE_ONLY:-0}"
mkdir -p "$ART"

if [[ "$DIAGNOSTIC_RATE_ONLY" == "1" ]]; then
  case "$MODE:$LANES:$RATE:$SCENARIO" in
    normal:1:5:lossless) ;;
    game:4:1.5:lossless) ;;
    *) echo "invalid strict diagnostic tuple $MODE lanes=$LANES rate=$RATE scenario=$SCENARIO" >&2; exit 2 ;;
  esac
else
  case "$MODE:$LANES:$RATE" in
    normal:1:10) ;;
    game:4:3) ;;
    *) echo "invalid strict mode tuple $MODE lanes=$LANES rate=$RATE" >&2; exit 2 ;;
  esac
fi
case "$SCENARIO" in
  lossless) PRE_LOSS=0; STRESS_LOSS=0; POST_LOSS=0 ;;
  5205) PRE_LOSS=5; STRESS_LOSS=20; POST_LOSS=5 ;;
  5305) PRE_LOSS=5; STRESS_LOSS=30; POST_LOSS=5 ;;
  *) echo "invalid scenario $SCENARIO" >&2; exit 2 ;;
esac

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
STAGE_PID=""
SAMPLER_PID=""
KEY=""
CAP_PIDS=()

cleanup() {
  set +e
  for pid in "$BIZ_PID" "$TGT_PID" "$STAGE_PID" "$SAMPLER_PID" "$CLIENT_PID" "$SERVER_PID"; do
    if [[ -n "$pid" ]]; then kill -TERM "$pid" 2>/dev/null || true; fi
  done
  for pid in "${CAP_PIDS[@]:-}"; do
    if [[ -n "$pid" ]]; then kill -INT "$pid" 2>/dev/null || true; fi
  done
  sleep 0.3
  for ns in "${NAMESPACES[@]}"; do ip netns del "$ns" 2>/dev/null || true; done
  if [[ -n "$KEY" ]]; then rm -f "$KEY" 2>/dev/null || true; fi
  chmod -R a+rX "$ART" 2>/dev/null || true
}
trap cleanup EXIT

for ns in "${NAMESPACES[@]}"; do
  ip netns add "$ns"
  ip -n "$ns" link set lo up
done

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

for pair in "$BIZ biz0" "$CLI cbiz" "$CLI cwan" "$RTR rcli" "$RTR rsrv" "$SRV swan" "$SRV slan" "$TGT tgt0"; do
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

ip netns exec "$CLI" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP
ip netns exec "$SRV" iptables -w -I OUTPUT 1 -p tcp --tcp-flags RST RST -j DROP

# Main qualification has no bandwidth cap. Both Game and Normal share these two
# single bottleneck qdiscs; four Game lanes are not given independent links.
ip netns exec "$RTR" tc qdisc add dev rsrv root netem limit 200000 delay 300ms
ip netns exec "$RTR" tc qdisc add dev rcli root netem limit 200000 delay 300ms
ip netns exec "$RTR" tc -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-before.json"
ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-before.json"

FILTER='tcp and host 198.18.0.2 and host 198.18.0.6'
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q in  -s 256 -B 16384 -w "$ART/c2s-pre.pcap"  "$FILTER" 2> "$ART/c2s-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q out -s 256 -B 16384 -w "$ART/c2s-post.pcap" "$FILTER" 2> "$ART/c2s-post.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q in  -s 256 -B 16384 -w "$ART/s2c-pre.pcap"  "$FILTER" 2> "$ART/s2c-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q out -s 256 -B 16384 -w "$ART/s2c-post.pcap" "$FILTER" 2> "$ART/s2c-post.tcpdump.log" &
CAP_PIDS+=("$!")
sleep 1

CERT="$ART/server-cert.pem"
KEY="/tmp/wbd-strict-server-key-$SFX.pem"
openssl req -x509 -newkey rsa:2048 -nodes -days 1 -subj '/CN=qual.test'   -keyout "$KEY" -out "$CERT" > "$ART/openssl.log" 2>&1

TUNNEL_ID="00112233445566778899aabbccddeeff"
INSTALLATION_ID="11223344556677889900aabbccddeeff"
ROUTE_KEY_HEX="00112233445566778899aabbccddeeffffeeddccbbaa00998877665544332211"

ip netns exec "$SRV" "$SERVER_BIN"   --raw-interface swan --listen-ip 198.18.0.6 --listen-port 443   --tun-name wbdg0 --lease-pool 10.66.0.0/16 --lease4 10.66.0.2/32   --tunnel-id "$TUNNEL_ID" --account qual --installation-id "$INSTALLATION_ID"   --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX"   --tls-cert "$CERT" --tls-key "$KEY" --username qual --password qualpass   --decoy 10.50.0.2:4433 --server-record-limit 1250 --mtu 1400   --fec-parity 20 --lanes "$LANES" --firewall iptables   --diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s   > "$ART/server.log" 2>&1 &
SERVER_PID="$!"

sleep 1
ip netns exec "$CLI" "$CLIENT_BIN"   --raw-interface cwan --local-ip 198.18.0.2 --source-port 40000   --server-ip 198.18.0.6 --server-port 443   --tunnel-id "$TUNNEL_ID" --lease4 10.66.0.2/32   --account qual --installation-id "$INSTALLATION_ID"   --server-name qual.test --route-key-hex "$ROUTE_KEY_HEX"   --username qual --password qualpass --client-record-limit 1300 --mtu 1400   --fec-parity 20 --lanes "$LANES" --tproxy-port 12345 --mark 66   --route-table 1066 --rule-priority 1066   --diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 1s   > "$ART/client.log" 2>&1 &
CLIENT_PID="$!"

sleep 5
kill -0 "$SERVER_PID"
kill -0 "$CLIENT_PID"

{
  uname -a
  echo "nproc=$(nproc)"
  lscpu
  echo "cgroup:"
  cat /proc/self/cgroup
  echo "cpu.max:"
  cat /sys/fs/cgroup/cpu.max 2>/dev/null || true
} > "$ART/runner-host.txt"

START_NS="$(python3 - <<'PY'
import time
print(time.monotonic_ns() + 12_000_000_000)
PY
)"
printf '%s\n' "$START_NS" > "$ART/start-monotonic-ns.txt"

"$TC_BIN" -V > "$ART/netem-tc-version.txt" 2>&1
python3 "$STAGER"   --namespace "$RTR" --tc-bin "$TC_BIN" --c2s-dev rsrv --s2c-dev rcli --start-ns "$START_NS"   --pre-loss "$PRE_LOSS" --stress-loss "$STRESS_LOSS" --post-loss "$POST_LOSS"   --seed "$SEED" --output "$ART/stage-events.jsonl" > "$ART/stage.log" 2>&1 &
STAGE_PID="$!"

sampler_args=(
  python3 "$SAMPLER" --output "$ART/resources.jsonl" --interval 1
  --process "client=$CLIENT_PID" --process "server=$SERVER_PID"
  --namespace "biz=$BIZ" --namespace "client=$CLI" --namespace "router=$RTR"
  --namespace "server=$SRV" --namespace "target=$TGT"
)
for pid in "${CAP_PIDS[@]}"; do sampler_args+=(--capture-pid "$pid"); done
"${sampler_args[@]}" > "$ART/resource-sampler.log" 2>&1 &
SAMPLER_PID="$!"

ip netns exec "$TGT" python3 "$GEN"   --role target --bind 10.50.0.2:18080   --start-ns "$START_NS" --duration 120 --drain 10 --rate-mbps "$RATE"   --seed "$((SEED*100+2))" --output "$ART/target.json" > "$ART/target.log" 2>&1 &
TGT_PID="$!"

ip netns exec "$BIZ" python3 "$GEN"   --role biz --bind 10.40.0.2:28080 --peer 10.50.0.2:18080   --start-ns "$START_NS" --duration 120 --drain 10 --rate-mbps "$RATE"   --seed "$((SEED*100+1))" --output "$ART/biz.json" > "$ART/biz.log" 2>&1 &
BIZ_PID="$!"

wait "$BIZ_PID"; BIZ_PID=""
wait "$TGT_PID"; TGT_PID=""
wait "$STAGE_PID"; STAGE_PID=""

if ! kill -0 "$CLIENT_PID" 2>/dev/null; then
  echo "WBD_STRICT_CLIENT_EARLY_EXIT pid=$CLIENT_PID" >&2
  exit 1
fi
if ! kill -0 "$SERVER_PID" 2>/dev/null; then
  echo "WBD_STRICT_SERVER_EARLY_EXIT pid=$SERVER_PID" >&2
  exit 1
fi

ip netns exec "$RTR" tc -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-after.json"
ip netns exec "$RTR" tc -s -j qdisc show dev rcli > "$ART/qdisc-rcli-after.json"
ip netns exec "$CLI" ip -s -j link show > "$ART/client-links-after.json"
ip netns exec "$RTR" ip -s -j link show > "$ART/router-links-after.json"
ip netns exec "$SRV" ip -s -j link show > "$ART/server-links-after.json"
ip netns exec "$CLI" ss -u -a -m -n > "$ART/client-ss-udp-after.txt" 2>&1 || true
ip netns exec "$SRV" ss -u -a -m -n > "$ART/server-ss-udp-after.txt" 2>&1 || true
ip netns exec "$CLI" ss -w -a -m -n > "$ART/client-ss-raw-after.txt" 2>&1 || true
ip netns exec "$SRV" ss -w -a -m -n > "$ART/server-ss-raw-after.txt" 2>&1 || true
ip netns exec "$CLI" nft -a list table inet wbd_tproxy > "$ART/client-tproxy-active.txt"
ip netns exec "$SRV" iptables-save > "$ART/server-iptables-active.txt"
ip netns exec "$SRV" ip -details link show wbdg0 > "$ART/server-tun-active.txt"

kill -TERM "$SAMPLER_PID" 2>/dev/null || true
wait "$SAMPLER_PID" 2>/dev/null || true
SAMPLER_PID=""

kill -TERM "$CLIENT_PID" "$SERVER_PID"
wait "$CLIENT_PID" || true
wait "$SERVER_PID" || true
CLIENT_PID=""
SERVER_PID=""
rm -f "$KEY"; KEY=""

for pid in "${CAP_PIDS[@]}"; do kill -INT "$pid" 2>/dev/null || true; done
for pid in "${CAP_PIDS[@]}"; do wait "$pid" 2>/dev/null || true; done
CAP_PIDS=()

python3 - "$ART" "$GITHUB_SHA" "$GITHUB_WORKSPACE" "$MODE" "$SCENARIO" "$SEED" "$RATE" "$LANES" "$DIAGNOSTIC_RATE_ONLY" <<'PY'
import hashlib, json, sys
from pathlib import Path
art = Path(sys.argv[1])
source, root = sys.argv[2], Path(sys.argv[3])
mode, scenario, seed, rate, lanes = sys.argv[4], sys.argv[5], int(sys.argv[6]), float(sys.argv[7]), int(sys.argv[8])
diagnostic_rate_only = sys.argv[9] == "1"
harness = [
    ".github/workflows/next-strict-weaknet.yml",
    "scripts/strict_weaknet_sample.sh",
    "tools/realpath_udp_duplex.py",
    "tools/check_strict_weaknet_loss_tolerant_v1.py",
    "tools/strict_weaknet_stage.py",
    "scripts/build_seeded_tc.sh",
    "tools/strict_resource_sampler.py",
    "tools/check_strict_weaknet.py",
    "tools/aggregate_strict_weaknet.py",
]
if diagnostic_rate_only:
    harness.extend([
        ".github/workflows/next-strict-capacity-diagnostics.yml",
        ".github/workflows/next-strict-packet-socket-diagnostic.yml",
    ])
files = {}
for rel in harness:
    data = (root / rel).read_bytes()
    files[rel] = hashlib.sha256(data).hexdigest()
manifest = {
    "schema": 1, "source_sha": source, "harness_sha": source,
    "harness_file_sha256": files, "runner": "ubuntu-24.04",
    "mode": mode, "scenario": scenario, "seed": seed,
    "config": {
        "lanes": lanes, "application_mbps_each_direction": rate,
        "fec": "20:20", "padding": "off", "mtu": 1400,
        "one_way_delay_ms": 300, "duration_s": 120, "stages_s": [30, 60, 30],
        "drain_s": 10, "qdisc_limit_packets": 200000,
        "hidden_bandwidth_limit": False,
        "packet_sizes_equal_count_cycle": [64, 256, 1200],
        "diagnostic_rate_only": diagnostic_rate_only,
    },
    "topology": "biz netns -> OpenWrt TPROXY formal client -> raw/veth -> shared router netem -> raw formal server -> shared TUN -> target netns",
}
(art / "manifest.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY

chmod -R a+rX "$ART"
printf 'WBD_STRICT_WEAKNET_CAPTURED source_sha=%s mode=%s scenario=%s seed=%s artifact=%s\n'   "$GITHUB_SHA" "$MODE" "$SCENARIO" "$SEED" "$ART"
