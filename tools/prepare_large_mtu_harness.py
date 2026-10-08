#!/usr/bin/env python3
"""Derive exactly one large-MTU sample from the audited strict fullstack topology.

Fail closed on upstream template changes.  No second endpoint pair, no per-run
performance matrix, no product source modifications.
"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--output",required=True)
    p.add_argument("--workload",choices=["udp","tcp","mixed"],required=True)
    p.add_argument("--loss",type=int,choices=[0,5,20,30,5205],required=True)
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
    # Formal client and server already support Game 2/3/4 lanes. Only the
    # generated efficiency harness widens its exact sample tuple; the
    # upstream strict_weaknet_sample.sh default acceptance stays untouched.
    swap('    game:4:3) ;;',
         '    game:2:3|game:3:3|game:4:3) ;;')
    swap('done\n\nip -n "$BIZ" route add default via 10.40.0.1',
         '''done
# Keep only the two underlay WAN veth pairs at 1400. Inner packet-bearing
# edges are 9000 so 8973 => 8996+25 and 65507 => 8 OS fragments.
for spec in "$BIZ biz0" "$CLI cbiz" "$SRV slan" "$TGT tgt0"; do
  read -r ns dev <<<"$spec"
  ip -n "$ns" link set "$dev" mtu 9000
done

ip -n "$BIZ" route add default via 10.40.0.1''')
    # Limit on-disk raw captures to <=8 x 16MiB outer per tap and 4 x
    # 16MiB inner per tap; no unbounded multi-minute pcap accumulation.
    for tap in ("c2s-pre", "c2s-post", "s2c-pre", "s2c-post"):
        swap('-s 256 -B 16384 -w "$ART/'+tap+'.pcap"',
             '-s 96 -B 16384 -C 16 -W 8 -w "$ART/'+tap+'.pcap"')
    swap('--username qual --password qualpass --client-record-limit 1300 --mtu 1400',
         '--username qual --password qualpass --client-record-limit 0 --mtu 1400 --route-mode all')
    swap('printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"',
         '''printf '%s\\n' "$START_NS" > "$ART/start-monotonic-ns.txt"
ip -n "$BIZ" -d -j link show biz0 > "$ART/inner-biz-link.json"
ip -n "$CLI" -d -j link show cbiz > "$ART/inner-client-link.json"
ip -n "$SRV" -d -j link show wbdg0 > "$ART/inner-server-tun.json"
ip -n "$TGT" -d -j link show tgt0 > "$ART/inner-target-link.json"
ip netns exec "$BIZ" ip route get 8.8.8.8 > "$ART/inner-biz-route.txt"
ip netns exec "$TGT" ip route get 10.66.0.2 > "$ART/inner-target-route.txt"
# Read-only bounded snaplen; analyzer consumes metadata then deletes all pcaps.
ip netns exec "$BIZ" tcpdump -n -U -i biz0 -s 96 -B 8192 -C 16 -W 4 -w "$ART/inner-biz.pcap" "ip" 2>"$ART/inner-biz.tcpdump.log" &
CAP_PIDS+=("$!")
ip netns exec "$TGT" tcpdump -n -U -i tgt0 -s 96 -B 8192 -C 16 -W 4 -w "$ART/inner-target.pcap" "ip" 2>"$ART/inner-target.tcpdump.log" &
CAP_PIDS+=("$!")''')
    swap('--post-loss "$POST_LOSS"     --seed "$SEED" --output "$ART/stage-events.jsonl" > "$ART/stage.log"',
         '--post-loss "$POST_LOSS"     --seed "$SEED" --fixed-loss "$WBD_LARGE_LOSS" --output "$ART/stage-events.jsonl" > "$ART/stage.log"')
    swap("--server-record-limit 1250 --mtu 1400", "--server-record-limit 0 --mtu 1400")
    swap("-subj '/CN=qual.test'   -keyout", "-subj '/CN=qual.test' -addext 'subjectAltName=DNS:qual.test'   -keyout")
    # HTTP and authenticated HTTPS share this one traffic sample. Owned
    # sidecars run only for TCP/mixed, and are included in PID cleanup.
    swap('BIZ_PID=""\nTGT_PID=""\nSTAGE_PID=""',
         'BIZ_PID=""\nTGT_PID=""\nWEB_BIZ_PID=""\nWEB_TGT_PID=""\nSTAGE_PID=""')
    swap('for pid in "$BIZ_PID" "$TGT_PID" "$STAGE_PID" "$SAMPLER_PID" "$CLIENT_PID" "$SERVER_PID"; do',
         'for pid in "$BIZ_PID" "$TGT_PID" "$WEB_BIZ_PID" "$WEB_TGT_PID" "$STAGE_PID" "$SAMPLER_PID" "$CLIENT_PID" "$SERVER_PID"; do')

    # A normal profile-off performance run must not enable per-record timings.
    # Server ObserveTiming is coupled to diagnostic-jsonl, not CPU pprof.
    if os.environ.get("WBD_EFF_DIAGNOSTIC", "0") == "0":
        swap('--diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s', '')
        swap('--diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 1s', '')
    def replace_role(role,bind,peer,out):
        nonlocal s
        patt=r'^ip netns exec "\$'+('TGT' if role=='target' else 'BIZ')+r'" python3 "\$GEN".*$'
        line=('ip netns exec "$'+('TGT' if role=='target' else 'BIZ')+
              '" python3 "$GEN" --role '+role+
              ' --workload "$WBD_LARGE_WORKLOAD" --bind '+bind+
              ' --peer '+peer+
              ' --size-profile "$WBD_EFF_SIZE_PROFILE" --rate-mbps "$WBD_STRICT_RATE_MBPS" --start-ns "$START_NS" --seed "$SEED" --source "$GITHUB_SHA"'+
              ' --helper "$WBD_HARNESS_SHA" --output "$ART/'+out+'.json"'+
              ' > "$ART/'+out+'.log" 2>&1 &')
        s,n=re.subn(patt,lambda _:line,s,flags=re.M)
        if n!=1:raise RuntimeError("missing exact role line "+role)
    replace_role("target","8.8.8.8:18080","10.40.0.2:28080","target")
    replace_role("biz","10.40.0.2:28080","8.8.8.8:18080","biz")
    swap('wait "$BIZ_PID"; BIZ_PID=""\nwait "$TGT_PID"; TGT_PID=""',
         '''if [[ "$WBD_LARGE_WORKLOAD" != udp ]]; then
  ip netns exec "$TGT" python3 "$GITHUB_WORKSPACE/tools/efficiency_http_https.py" --role target --bind 8.8.8.8 --peer 10.40.0.2 --start-ns "$START_NS" --seed "$SEED" --tls-cert "$CERT" --tls-key "$KEY" --source "$GITHUB_SHA" --helper "$WBD_HARNESS_SHA" --output "$ART/target-http.json" > "$ART/target-http.log" 2>&1 &
  WEB_TGT_PID="$!"
  ip netns exec "$BIZ" python3 "$GITHUB_WORKSPACE/tools/efficiency_http_https.py" --role biz --bind 10.40.0.2 --peer 8.8.8.8 --start-ns "$START_NS" --seed "$SEED" --tls-cert "$CERT" --tls-key "$KEY" --source "$GITHUB_SHA" --helper "$WBD_HARNESS_SHA" --output "$ART/biz-http.json" > "$ART/biz-http.log" 2>&1 &
  WEB_BIZ_PID="$!"
fi
wait "$BIZ_PID"; BIZ_PID=""
wait "$TGT_PID"; TGT_PID=""
if [[ -n "$WEB_BIZ_PID" ]]; then wait "$WEB_BIZ_PID"; WEB_BIZ_PID=""; fi
if [[ -n "$WEB_TGT_PID" ]]; then wait "$WEB_TGT_PID"; WEB_TGT_PID=""; fi''')
    swap('workflow_rel = ".github/workflows/next-shared-blackhole.yml" if blackhole_ms else ".github/workflows/next-strict-weaknet.yml"',
         'workflow_rel = ".github/workflows/next-efficiency-e0-single.yml"')
    begin=s.index('harness = [\n')
    end=s.index('\nif fec_screen:',begin)
    s=s[:begin]+'''harness = [
    workflow_rel,
    "scripts/strict_weaknet_sample.sh",
    "scripts/build_seeded_tc.sh",
    "tools/strict_resource_sampler.py",
    "tools/large_mtu_mixed_business.py",
    "tools/efficiency_http_https.py",
    "tools/large_mtu_loss_stage.py",
    "tools/prepare_large_mtu_harness.py",
    "tools/check_large_mtu_mixed.py",
    "tools/efficiency_cost_ledger.py",
    "tools/large_mtu_resource_report.py",
    "tools/check_strict_weaknet.py",
]
'''.rstrip()+s[end:]
    swap('"schema": 1, "source_sha": source,',
         '"schema": "wbd-large-mtu-mixed/v1", "source_sha": source,')
    swap('"mode": mode, "scenario": scenario, "seed": seed,',
         '"mode": mode, "scenario": os.environ["WBD_LARGE_WORKLOAD"] + "-p" + os.environ["WBD_LARGE_LOSS"], "seed": seed,')
    swap('"one_way_delay_ms": 300, "duration_s": 120, "stages_s": [30, 60, 30],',
         '"one_way_delay_ms": 300, "duration_s": 300, "stages_s": ([[0,75,5],[75,225,20],[225,300,5]] if os.environ["WBD_LARGE_LOSS"]=="5205" else [[0,300,int(os.environ["WBD_LARGE_LOSS"])] ]),')
    swap('"drain_s": 10,','"drain_s": 3,')
    swap('"packet_sizes_equal_count_cycle": [64, 256, 1200],',
         '''"packet_sizes_count_cycle": (
          {"udp":{"96":50,"256":10,"512":10,"1372":10,"4068":10,"8936":5,"8937":4,"65507":1},
           "mixed":{"96":50,"256":10,"512":10,"1372":10,"4068":10,"8936":5,"8937":4,"65507":1}}
          if os.environ["WBD_EFF_SIZE_PROFILE"]=="boundary" else
          ({"udp":{"96":30,"256":20,"512":14,"1000":10,"1372":9,"4068":6,"8972":5,"8973":4,"65507":2},
            "mixed":{"96":50,"256":20,"512":10,"1372":8,"8972":5,"8973":5,"65507":2}}
           if os.environ["WBD_EFF_SIZE_PROFILE"]=="jumbo" else
           {"udp":{"96":30,"256":20,"512":15,"1000":15,"1372":10,"4068":10},
            "mixed":{"96":50,"256":20,"512":10,"1000":5,"1372":10,"4068":5}})),
        "inner_edge_mtu": 9000, "server_shared_tun_mtu": "actual ip-link receipt",
        "record_limit_configured": 0, "formal_default_tick_ms": 100,
        "http_https_short_requests": 0 if os.environ["WBD_LARGE_WORKLOAD"]=="udp" else 20,
        "tcp_reserved_https_mbps": 0 if os.environ["WBD_LARGE_WORKLOAD"]=="udp" else 0.02,
        "client_ingress": "TPROXY; no client TUN in this Linux strict topology",
        "outer_connection_mtu": 1400, "route_mode": "all",
        "netem_loss_both_directions_percent": ("waveform:5-20-5" if os.environ["WBD_LARGE_LOSS"]=="5205" else int(os.environ["WBD_LARGE_LOSS"])),''')
    Path(x.output).write_text(s)
    Path(x.output+".receipt.json").write_text(json.dumps({
        "schema":"wbd-large-mtu-harness-template/v1",
        "original_sha256":hashlib.sha256(original.encode()).hexdigest(),
        "generated_sha256":hashlib.sha256(s.encode()).hexdigest(),
        "workload":x.workload,"loss":x.loss,"one_sample":True},indent=2))

if __name__=="__main__":main()
