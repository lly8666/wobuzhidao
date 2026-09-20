#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 OPENWRT_TEST_BINARY" >&2
  exit 2
fi
if [[ $(id -u) -ne 0 ]]; then
  echo "openwrt_tproxy_netns.sh requires root" >&2
  exit 1
fi

TEST_BIN=$(readlink -f "$1")
[[ -x "$TEST_BIN" ]] || { echo "test binary not executable: $TEST_BIN" >&2; exit 2; }

suffix=$$
C="wbdoc${suffix}"
R="wbdor${suffix}"
T="wbdot${suffix}"

cleanup() {
  set +e
  for ns in "$C" "$R" "$T"; do
    ip netns pids "$ns" 2>/dev/null | xargs -r kill -TERM 2>/dev/null || true
    ip netns del "$ns" 2>/dev/null || true
  done
}
trap cleanup EXIT INT TERM

for tool in ip nft python3; do
  command -v "$tool" >/dev/null || { echo "missing required tool: $tool" >&2; exit 2; }
done

for ns in "$C" "$R" "$T"; do
  ip netns add "$ns"
  ip -n "$ns" link set lo up
done

cif="oc${suffix}"
r0="or0${suffix}"
r1="or1${suffix}"
tif="ot${suffix}"
for ifname in "$cif" "$r0" "$r1" "$tif"; do
  [[ ${#ifname} -lt 16 ]] || { echo "interface name too long: $ifname" >&2; exit 2; }
done

ip link add "$cif" type veth peer name "$r0"
ip link set "$cif" netns "$C"
ip link set "$r0" netns "$R"
ip link add "$r1" type veth peer name "$tif"
ip link set "$r1" netns "$R"
ip link set "$tif" netns "$T"

ip -n "$C" addr add 10.10.0.2/24 dev "$cif"
ip -n "$C" link set "$cif" up
ip -n "$C" route add default via 10.10.0.1

ip -n "$R" addr add 10.10.0.1/24 dev "$r0"
ip -n "$R" link set "$r0" up
ip -n "$R" addr add 10.20.0.1/24 dev "$r1"
ip -n "$R" link set "$r1" up
ip netns exec "$R" sysctl -qw net.ipv4.ip_forward=1

ip -n "$T" addr add 10.20.0.2/24 dev "$tif"
ip -n "$T" addr add 10.20.0.3/24 dev "$tif"
ip -n "$T" link set "$tif" up
ip -n "$T" route add 10.10.0.0/24 via 10.20.0.1

ip netns exec "$R" env \
  WBD_P4_OPENWRT_TPROXY_NET=1 \
  WBD_P4_OPENWRT_CLIENT_NS="$C" \
  WBD_P4_OPENWRT_TARGET_NS="$T" \
  "$TEST_BIN" -test.v -test.run '^TestPrivilegedOpenWrt(TPROXYRuntime|SocketTunnelAdapter)

