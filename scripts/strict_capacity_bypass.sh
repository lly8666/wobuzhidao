#!/usr/bin/env bash
set -euo pipefail

: "${GITHUB_WORKSPACE:?missing GITHUB_WORKSPACE}"
: "${GITHUB_SHA:?missing GITHUB_SHA}"
: "${WBD_CAPACITY_ARTIFACT_DIR:?missing WBD_CAPACITY_ARTIFACT_DIR}"
: "${WBD_CAPACITY_RATE_MBPS:?missing WBD_CAPACITY_RATE_MBPS}"
: "${WBD_CAPACITY_SEED:?missing WBD_CAPACITY_SEED}"
: "${WBD_STRICT_TC:?missing WBD_STRICT_TC}"

ART="$WBD_CAPACITY_ARTIFACT_DIR"
RATE="$WBD_CAPACITY_RATE_MBPS"
SEED="$WBD_CAPACITY_SEED"
TC_BIN="$WBD_STRICT_TC"
GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"
SAMPLER="$GITHUB_WORKSPACE/tools/strict_resource_sampler.py"
DURATION=60
DRAIN=10
mkdir -p "$ART"

SFX="$$"
BIZ="cbiz-$SFX"
CLI="ccli-$SFX"
RTR="crtr-$SFX"
SRV="csrv-$SFX"
TGT="ctgt-$SFX"
NAMESPACES=("$BIZ" "$CLI" "$RTR" "$SRV" "$TGT")
BIZ_PID=""
TGT_PID=""
SAMPLER_PID=""
CAP_PIDS=()

cleanup() {
  set +e
  for pid in "$BIZ_PID" "$TGT_PID" "$SAMPLER_PID"; do
    [[ -n "$pid" ]] && kill -TERM "$pid" 2>/dev/null || true
  done
  for pid in "${CAP_PIDS[@]:-}"; do
    [[ -n "$pid" ]] && kill -INT "$pid" 2>/dev/null || true
  done
  sleep 0.3
  for ns in "${NAMESPACES[@]}"; do ip netns del "$ns" 2>/dev/null || true; done
  chmod -R a+rX "$ART" 2>/dev/null || true
}
trap cleanup EXIT

for ns in "${NAMESPACES[@]}"; do
  ip netns add "$ns"
  ip -n "$ns" link set lo up
done

ip link add "cb$SFX" type veth peer name "cc0$SFX"
ip link set "cb$SFX" netns "$BIZ"
ip link set "cc0$SFX" netns "$CLI"
ip -n "$BIZ" link set "cb$SFX" name biz0
ip -n "$CLI" link set "cc0$SFX" name cbiz

ip link add "cc1$SFX" type veth peer name "cr0$SFX"
ip link set "cc1$SFX" netns "$CLI"
ip link set "cr0$SFX" netns "$RTR"
ip -n "$CLI" link set "cc1$SFX" name cwan
ip -n "$RTR" link set "cr0$SFX" name rcli

ip link add "cr1$SFX" type veth peer name "cs0$SFX"
ip link set "cr1$SFX" netns "$RTR"
ip link set "cs0$SFX" netns "$SRV"
ip -n "$RTR" link set "cr1$SFX" name rsrv
ip -n "$SRV" link set "cs0$SFX" name swan

ip link add "cs1$SFX" type veth peer name "ct0$SFX"
ip link set "cs1$SFX" netns "$SRV"
ip link set "ct0$SFX" netns "$TGT"
ip -n "$SRV" link set "cs1$SFX" name slan
ip -n "$TGT" link set "ct0$SFX" name tgt0

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
ip -n "$RTR" route add 10.40.0.0/24 via 198.18.0.2 dev rcli
ip -n "$RTR" route add 10.50.0.0/24 via 198.18.0.6 dev rsrv
ip -n "$SRV" route add default via 198.18.0.5
ip -n "$TGT" route add default via 10.50.0.1

for ns in "$CLI" "$RTR" "$SRV"; do
  ip netns exec "$ns" sysctl -qw net.ipv4.conf.all.rp_filter=0
  ip netns exec "$ns" sysctl -qw net.ipv4.conf.default.rp_filter=0
  ip netns exec "$ns" sysctl -qw net.ipv4.ip_forward=1
done

"$TC_BIN" qdisc add dev lo root pfifo_fast 2>/dev/null || true
ip netns exec "$RTR" "$TC_BIN" qdisc add dev rsrv root netem limit 200000 delay 300ms
ip netns exec "$RTR" "$TC_BIN" qdisc add dev rcli root netem limit 200000 delay 300ms

for item in "biz:$BIZ" "client:$CLI" "router:$RTR" "server:$SRV" "target:$TGT"; do
  label="${item%%:*}"; ns="${item#*:}"
  ip netns exec "$ns" ip -s -j link show > "$ART/$label-links-before.json"
done
ip netns exec "$RTR" "$TC_BIN" -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-before.json"
ip netns exec "$RTR" "$TC_BIN" -s -j qdisc show dev rcli > "$ART/qdisc-rcli-before.json"

FILTER='udp and host 10.40.0.2 and host 10.50.0.2'
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q in  -s 128 -B 16384 -w "$ART/c2s-pre.pcap"  "$FILTER" 2> "$ART/c2s-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q out -s 128 -B 16384 -w "$ART/c2s-post.pcap" "$FILTER" 2> "$ART/c2s-post.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rsrv -Q in  -s 128 -B 16384 -w "$ART/s2c-pre.pcap"  "$FILTER" 2> "$ART/s2c-pre.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$RTR" tcpdump -n -U -i rcli -Q out -s 128 -B 16384 -w "$ART/s2c-post.pcap" "$FILTER" 2> "$ART/s2c-post.tcpdump.log" &
CAP_PIDS+=("$!")
sleep 1

{
  uname -a
  echo "nproc=$(nproc)"
  lscpu
  cat /proc/self/cgroup
  cat /sys/fs/cgroup/cpu.max 2>/dev/null || true
} > "$ART/runner-host.txt"
"$TC_BIN" -V > "$ART/netem-tc-version.txt" 2>&1

START_NS="$(python3 - <<'PY'
import time
print(time.monotonic_ns() + 5_000_000_000)
PY
)"
printf '%s\n' "$START_NS" > "$ART/start-monotonic-ns.txt"

sampler_args=(
  python3 "$SAMPLER" --output "$ART/resources.jsonl" --interval 1
  --namespace "biz=$BIZ" --namespace "client=$CLI" --namespace "router=$RTR"
  --namespace "server=$SRV" --namespace "target=$TGT"
)
for pid in "${CAP_PIDS[@]}"; do sampler_args+=(--capture-pid "$pid"); done
"${sampler_args[@]}" > "$ART/resource-sampler.log" 2>&1 &
SAMPLER_PID="$!"

ip netns exec "$TGT" python3 "$GEN" --role target --bind 10.50.0.2:18080 \
  --start-ns "$START_NS" --duration "$DURATION" --drain "$DRAIN" --rate-mbps "$RATE" \
  --seed "$((SEED*100+2))" --output "$ART/target.json" > "$ART/target.log" 2>&1 &
TGT_PID="$!"

ip netns exec "$BIZ" python3 "$GEN" --role biz --bind 10.40.0.2:28080 --peer 10.50.0.2:18080 \
  --start-ns "$START_NS" --duration "$DURATION" --drain "$DRAIN" --rate-mbps "$RATE" \
  --seed "$((SEED*100+1))" --output "$ART/biz.json" > "$ART/biz.log" 2>&1 &
BIZ_PID="$!"

wait "$BIZ_PID"; BIZ_PID=""
wait "$TGT_PID"; TGT_PID=""

ip netns exec "$RTR" "$TC_BIN" -s -j qdisc show dev rsrv > "$ART/qdisc-rsrv-after.json"
ip netns exec "$RTR" "$TC_BIN" -s -j qdisc show dev rcli > "$ART/qdisc-rcli-after.json"
for item in "biz:$BIZ" "client:$CLI" "router:$RTR" "server:$SRV" "target:$TGT"; do
  label="${item%%:*}"; ns="${item#*:}"
  ip netns exec "$ns" ip -s -j link show > "$ART/$label-links-after.json"
  ip netns exec "$ns" ss -u -a -m -n > "$ART/$label-ss-udp-after.txt" 2>&1 || true
done

kill -TERM "$SAMPLER_PID" 2>/dev/null || true
wait "$SAMPLER_PID" 2>/dev/null || true
SAMPLER_PID=""

for pid in "${CAP_PIDS[@]}"; do kill -INT "$pid" 2>/dev/null || true; done
for pid in "${CAP_PIDS[@]}"; do wait "$pid" 2>/dev/null || true; done
CAP_PIDS=()

python3 - "$ART" "$GITHUB_SHA" "$GITHUB_WORKSPACE" "$RATE" "$SEED" "$DURATION" "$DRAIN" <<'PY'
import hashlib, json, sys
from pathlib import Path
art=Path(sys.argv[1]); source=sys.argv[2]; root=Path(sys.argv[3])
rate=float(sys.argv[4]); seed=int(sys.argv[5]); duration=int(sys.argv[6]); drain=int(sys.argv[7])
rels=[
    ".github/workflows/next-strict-capacity-diagnostics.yml",
    "scripts/strict_capacity_bypass.sh",
    "tools/realpath_udp_duplex.py",
    "tools/strict_resource_sampler.py",
    "tools/check_strict_capacity_bypass.py",
    "scripts/build_seeded_tc.sh",
]
files={r: hashlib.sha256((root/r).read_bytes()).hexdigest() for r in rels}
manifest={
  "schema":1,"source_sha":source,"harness_sha":source,"harness_file_sha256":files,
  "runner":"ubuntu-24.04","kind":"same-topology-product-bypass",
  "config":{"application_mbps_each_direction":rate,"mtu":1400,"one_way_delay_ms":300,
            "duration_s":duration,"drain_s":drain,"qdisc_limit_packets":200000,
            "loss_percent":0,"packet_sizes_equal_count_cycle":[64,256,1200]},
  "seed":seed,
  "topology":"biz -> forwarding client netns -> router netem -> forwarding server netns -> target; no WBD client/server process",
}
(art/"manifest.json").write_text(json.dumps(manifest,indent=2,sort_keys=True)+"\n",encoding="utf-8")
PY
chmod -R a+rX "$ART"
printf 'WBD_STRICT_CAPACITY_BYPASS_CAPTURED source_sha=%s rate=%s seed=%s artifact=%s\n' "$GITHUB_SHA" "$RATE" "$SEED" "$ART"
