#!/usr/bin/env python3
"""Prepare one audited A sample from the mature strict fullstack topology.

Generation-only: never runs a performance sample itself. The generated script
must be launched ONCE by a separate Actions run after helper preflight.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
from longmix_profile import (
    PRODUCT_SOURCE, A_UDP_SHARE, INNER_TUN_MTU,
    OUTER_IPV4_PACKET_BUDGET, validate_sample
)

STRICT = "scripts/strict_weaknet_sample.sh"
GEN = "tools/longmix_udp_business.py"
STAGER = "tools/longmix_constant_stage.py"

def replace(s, old, new, n=1):
    actual = s.count(old)
    if actual != n:
        raise ValueError(f"strict template drift; {old[:70]!r} count={actual} wanted={n}")
    return s.replace(old, new)

def generate(sample):
    w = validate_sample(sample)
    if w.name != "A":
        raise ValueError("UDP-only A driver cannot be used for TCP workloads")
    raw = Path(STRICT).read_text()
    raw = replace(raw,
       'GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"',
       'GEN="$GITHUB_WORKSPACE/tools/longmix_udp_business.py"')
    raw = replace(raw,
       'STAGER="$GITHUB_WORKSPACE/tools/strict_weaknet_stage.py"',
       'STAGER="$GITHUB_WORKSPACE/tools/longmix_constant_stage.py"')
    raw = replace(raw,
       '--pre-loss "$PRE_LOSS" --stress-loss "$STRESS_LOSS" --post-loss "$POST_LOSS"',
       '--loss-percent "$WBD_LONGMIX_LOSS"')
    raw = replace(raw, '--duration 120 --drain 10',
                       '--duration 300 --drain 10', n=2)
    raw = replace(raw, 'ip -n "$SRV" addr add 8.8.8.1/24 dev slan',
                       'ip -n "$SRV" addr add 10.50.0.1/24 dev slan')
    raw = replace(raw, 'ip -n "$TGT" addr add 8.8.8.8/24 dev tgt0',
                       'ip -n "$TGT" addr add 10.50.0.2/24 dev tgt0')
    raw = replace(raw, 'ip -n "$TGT" route add default via 8.8.8.1',
                       'ip -n "$TGT" route add default via 10.50.0.1')
    raw = replace(raw, '--decoy 8.8.8.8:4433', '--decoy 10.50.0.2:4433')
    raw = replace(raw, '--bind 8.8.8.8:18080', '--bind 10.50.0.2:18080')
    raw = replace(raw, '--peer 8.8.8.8:18080', '--peer 10.50.0.2:18080')
    raw = replace(raw,
       '"$CLIENT_BIN"   --raw-interface cwan',
       '"$CLIENT_BIN"   --dns-hijack=false --route-mode all --raw-interface cwan')
    raw = replace(raw,
       'ip -n "$ns" link set "$dev" mtu 1400 up',
       '''mtu=1400
  case "$dev" in biz0|cbiz|slan|tgt0) mtu=9000 ;; esac
  ip -n "$ns" link set "$dev" mtu "$mtu" up''')
    # The capture is bounded even if a failed run never makes it to cleanup.
    # Only outer TCP headers are retained, never business payload plaintext.
    raw = replace(raw, 'tcpdump -n -U -i ',
                       'tcpdump -c 40000 -n -U -i ', n=4)
    # Legacy generator's formal scenario is forced lossless; the new seeded
    # stage uses the independently specified fixed impairment throughout.
    raw = replace(raw, 'python3 "$STAGER" --namespace "$RTR"',
                       'python3 "$STAGER" --namespace "$RTR"', n=2)
    # Store auditable route/MTU state while both endpoints remain active.
    sentinel = 'ip netns exec "$SRV" ip -details link show wbdg0 > "$ART/server-tun-active.txt"'
    raw = replace(raw, sentinel, sentinel + '''
ip netns exec "$BIZ" ip -4 route get 10.50.0.2 > "$ART/biz-target-route.txt"
ip netns exec "$BIZ" ip -details link show biz0 > "$ART/biz-mtu-active.txt"
ip netns exec "$CLI" ip -details link show cbiz > "$ART/client-tproxy-ingress-mtu-active.txt"
ip netns exec "$RTR" ip -details link show rcli > "$ART/router-rcli-mtu-active.txt"
ip netns exec "$RTR" ip -details link show rsrv > "$ART/router-rsrv-mtu-active.txt"
ip netns exec "$TGT" ip -details link show tgt0 > "$ART/target-mtu-active.txt"
ip netns exec "$CLI" ip -4 route get 198.18.0.6 > "$ART/client-underlay-route.txt"
ip netns exec "$TGT" ip -4 route get 10.40.0.2 > "$ART/target-return-route.txt"''')
    # Original manifest is only a scaffold; overwrite all claims that
    # differ, and hash the actual candidate helper files.
    raw += '''
python3 - "$ART" "$GITHUB_WORKSPACE" <<'WBDLONGMETA'
import hashlib, json, os, sys
from pathlib import Path
a = Path(sys.argv[1]); root = Path(sys.argv[2])
p = json.loads((a / "manifest.json").read_text())
p["schema"] = "wbd-longmix-udp-a/v1"
p["scenario"] = "fixed-p" + os.environ["WBD_LONGMIX_LOSS"]
p["workload"] = "A"
p["loss_percent"] = int(os.environ["WBD_LONGMIX_LOSS"])
p["helper_sha"] = os.environ["WBD_HARNESS_SHA"]
p["config"].update({
  "duration_s":300,"stages_s":[300],"drain_s":10,
  "packet_sizes_equal_count_cycle": None,
  "packet_sizes_target_byte_shares": [[96,5],[256,5],[512,5],
    [1000,5],[1372,5],[4068,10],[8972,15],[8973,20],[65507,30]],
  "inner_server_tun_mtu":9000,"client_capture_type":"Linux OpenWrt TPROXY (no client TUN)",
  "client_tproxy_ingress_mtu":9000,"target_interface_mtu":9000,
  "outer_packet_budget":1400,"underlay_interface_mtu":1400,
  "app_probes_hz":10,"probe_payload_bytes":96,
  "large_ack_reservation_bytes_per_second":8192,
  "no_unused_budget_reallocation":True,
  "loss_percent_each_direction":int(os.environ["WBD_LONGMIX_LOSS"]),
  "netem_phase":"one 300s fixed impairment, no 30/60/30 stage"
})
p["topology"] = ("biz netns actual IPv4 UDP socket -> Linux formal client"
  " route-mode=all TPROXY -> AF_PACKET raw TCP-like underlay -> fixed"
  " two-direction netem -> formal Linux server -> actual shared TUN"
  " MTU9000 -> target netns actual UDP socket; reverse active")
extra=["tools/longmix_profile.py", "tools/longmix_udp_business.py",
       "tools/longmix_constant_stage.py", "tools/prepare_longmix_udp.py"]
for name in extra:
    p["harness_file_sha256"][name] = hashlib.sha256((root/name).read_bytes()).hexdigest()
(a/"manifest.json").write_text(json.dumps(p,indent=2,sort_keys=True)+"\\n")
WBDLONGMETA
'''
    return raw

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--sample", required=True)
    ap.add_argument("--artifact-dir", required=True)
    args = ap.parse_args()
    sample = json.loads(Path(args.sample).read_text())
    script = generate(sample)
    art = Path(args.artifact_dir)
    art.mkdir(parents=True, exist_ok=True)
    path = art/"generated-longmix-udp-a.sh"
    path.write_text(script)
    path.chmod(0o700)
    (art/"longmix-generation-receipt.json").write_text(json.dumps({
        "scope":"GENERATED_ONLY_NOT_MEASUREMENT",
        "source_sha":PRODUCT_SOURCE,
        "sample":sample,
        "generated_sha256":hashlib.sha256(script.encode()).hexdigest(),
        "template_sha256":hashlib.sha256(Path(STRICT).read_bytes()).hexdigest()
    }, indent=2)+"\n")

if __name__ == "__main__":
    main()
