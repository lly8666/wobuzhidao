#!/usr/bin/env python3
"""Derive exactly one large-MTU sample from the audited strict fullstack topology.

Fail closed on upstream template changes.  No second endpoint pair, no per-run
performance matrix, no product source modifications.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--output",required=True)
    p.add_argument("--workload",choices=["udp","tcp","mixed"],required=True)
    p.add_argument("--loss",type=int,choices=[0,5,20,30],required=True)
    x=p.parse_args()
    original=Path("scripts/strict_weaknet_sample.sh").read_text()
    s=original
    def swap(old,new,n=1):
        nonlocal s
        actual=s.count(old)
        if actual!=n:raise RuntimeError("upstream topology mismatch count=%d expected=%d: %s"%(actual,n,old[:90]))
        s=s.replace(old,new)
    swap('GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"',
         'GEN="$GITHUB_WORKSPACE/tools/large_mtu_mixed_business.py"')
    swap('STAGER="$GITHUB_WORKSPACE/tools/strict_weaknet_stage.py"',
         'STAGER="$GITHUB_WORKSPACE/tools/large_mtu_loss_stage.py"')
    swap('done\n\nip -n "$BIZ" route add default via 10.40.0.1',
         '''done
# Keep only the two underlay WAN veth pairs at 1400. Inner packet-bearing
# edges are 9000 so 8973 => 8996+25 and 65507 => 8 OS fragments.
for spec in "$BIZ biz0" "$CLI cbiz" "$SRV slan" "$TGT tgt0"; do
  read -r ns dev <<<"$spec"
  ip -n "$ns" link set "$dev" mtu 9000
done

ip -n "$BIZ" route add default via 10.40.0.1''')
    swap('--username qual --password qualpass --client-record-limit 1300 --mtu 1400',
         '--username qual --password qualpass --client-record-limit 1300 --mtu 1400 --route-mode all')
    swap('printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"',
         '''printf '%s\\n' "$START_NS" > "$ART/start-monotonic-ns.txt"
ip -n "$BIZ" -d -j link show biz0 > "$ART/inner-biz-link.json"
ip -n "$CLI" -d -j link show cbiz > "$ART/inner-client-link.json"
ip -n "$SRV" -d -j link show wbdg0 > "$ART/inner-server-tun.json"
ip -n "$TGT" -d -j link show tgt0 > "$ART/inner-target-link.json"
ip netns exec "$BIZ" ip route get 8.8.8.8 > "$ART/inner-biz-route.txt"
ip netns exec "$TGT" ip route get 10.66.0.2 > "$ART/inner-target-route.txt"
# Read-only bounded snaplen; analyzer consumes metadata then deletes all pcaps.
ip netns exec "$BIZ" tcpdump -n -U -i biz0 -s 96 -B 8192 -w "$ART/inner-biz.pcap" "ip" 2>"$ART/inner-biz.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$TGT" tcpdump -n -U -i tgt0 -s 96 -B 8192 -w "$ART/inner-target.pcap" "ip" 2>"$ART/inner-target.tcpdump.log" &
CAP_PIDS+=("$!")''')
    swap('--post-loss "$POST_LOSS"     --seed "$SEED" --output "$ART/stage-events.jsonl" > "$ART/stage.log"',
         '--post-loss "$POST_LOSS"     --seed "$SEED" --fixed-loss "$WBD_LARGE_LOSS" --output "$ART/stage-events.jsonl" > "$ART/stage.log"')
    def replace_role(role,bind,peer,out):
        nonlocal s
        patt=r'^ip netns exec "\$'+('TGT' if role=='target' else 'BIZ')+r'" python3 "\$GEN".*$'
        line=('ip netns exec "$'+('TGT' if role=='target' else 'BIZ')+
              '" python3 "$GEN" --role '+role+
              ' --workload "$WBD_LARGE_WORKLOAD" --bind '+bind+
              ' --peer '+peer+
              ' --start-ns "$START_NS" --seed "$SEED" --source "$GITHUB_SHA"'+
              ' --helper "$WBD_HARNESS_SHA" --output "$ART/'+out+'.json"'+
              ' > "$ART/'+out+'.log" 2>&1 &')
        s,n=re.subn(patt,lambda _:line,s,flags=re.M)
        if n!=1:raise RuntimeError("missing exact role line "+role)
    replace_role("target","8.8.8.8:18080","10.40.0.2:28080","target")
    replace_role("biz","10.40.0.2:28080","8.8.8.8:18080","biz")
    swap('workflow_rel = ".github/workflows/next-shared-blackhole.yml" if blackhole_ms else ".github/workflows/next-strict-weaknet.yml"',
         'workflow_rel = ".github/workflows/next-large-mtu-mixed.yml"')
    begin=s.index('harness = [\n')
    end=s.index('\nif fec_screen:',begin)
    s=s[:begin]+'''harness = [
    workflow_rel,
    "scripts/strict_weaknet_sample.sh",
    "scripts/build_seeded_tc.sh",
    "tools/strict_resource_sampler.py",
    "tools/large_mtu_mixed_business.py",
    "tools/large_mtu_loss_stage.py",
    "tools/prepare_large_mtu_harness.py",
    "tools/check_large_mtu_mixed.py",
]
'''.rstrip()+s[end:]
    swap('"schema": 1, "source_sha": source,',
         '"schema": "wbd-large-mtu-mixed/v1", "source_sha": source,')
    swap('"mode": mode, "scenario": scenario, "seed": seed,',
         '"mode": mode, "scenario": os.environ["WBD_LARGE_WORKLOAD"] + "-p" + os.environ["WBD_LARGE_LOSS"], "seed": seed,')
    swap('"one_way_delay_ms": 300, "duration_s": 120, "stages_s": [30, 60, 30],',
         '"one_way_delay_ms": 300, "duration_s": 300, "stages_s": [[0,300,int(os.environ["WBD_LARGE_LOSS"])]],')
    swap('"drain_s": 10,','"drain_s": 15,')
    swap('"packet_sizes_equal_count_cycle": [64, 256, 1200],',
         '''"packet_sizes_count_cycle": {"udp":{"96":30,"256":20,"512":14,"1000":10,"1372":9,"4068":6,"8972":5,"8973":4,"65507":2},
                                      "mixed":{"96":50,"256":20,"512":10,"1372":8,"8972":5,"8973":5,"65507":2}},
        "inner_edge_mtu": 9000, "server_shared_tun_mtu": 9000,
        "client_ingress": "TPROXY; no client TUN in this Linux strict topology",
        "outer_connection_mtu": 1400, "route_mode": "all",
        "netem_loss_both_directions_percent": int(os.environ["WBD_LARGE_LOSS"]),''')
    Path(x.output).write_text(s)
    Path(x.output+".receipt.json").write_text(json.dumps({
        "schema":"wbd-large-mtu-harness-template/v1",
        "original_sha256":hashlib.sha256(original.encode()).hexdigest(),
        "generated_sha256":hashlib.sha256(s.encode()).hexdigest(),
        "workload":x.workload,"loss":x.loss,"one_sample":True},indent=2))

if __name__=="__main__":main()
