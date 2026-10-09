#!/usr/bin/env python3
"""Generate ONE 15ms fullstack sparse Normal/FEC20:20 protector from strict topology.

The existing audited five-netns topology and product parameters are reused
verbatim; only delay, sparse business pacing, claims and trace metadata change.
No product source, socket queues, CPU profiling or loss gates altered.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

SOURCE = "scripts/strict_weaknet_sample.sh"
PRODUCT = "a2db258b436a41fdee98c6c53abec9bab6ce600f"

def derive(original):
    s=original
    def swap(a,b,n=1):
        nonlocal s
        found=s.count(a)
        if found!=n:
            raise ValueError("strict upstream drift: %r has %d != %d uses"%(a[:75],found,n))
        s=s.replace(a,b)
    swap('STAGER="$GITHUB_WORKSPACE/tools/strict_weaknet_stage.py"',
         'STAGER="$GITHUB_WORKSPACE/tools/e1_lowrtt_sparse_stage.py"')
    swap('    normal:1:10) ;;','    normal:1:0.328) ;;')
    swap('delay 300ms','delay 15ms',2)
    swap('--rate-mbps "$RATE" --bounded-stats',
         '--rate-mbps "$RATE" --sparse-interval-ms 45 --probe-interval 0',2)
    # 120s + 10s drain, 45ms each 96B/1372B/4068B; fixed 0% loss.
    swap('workflow_rel = ".github/workflows/next-shared-blackhole.yml" if blackhole_ms else ".github/workflows/next-strict-weaknet.yml"',
         'workflow_rel = ".github/workflows/next-e1-lowrtt-fullstack.yml"')
    swap('    "tools/strict_weaknet_stage.py",',
         '    "tools/e1_lowrtt_sparse_stage.py",\n    "tools/prepare_e1_lowrtt_fullstack.py",\n    "tools/check_e1_lowrtt_fullstack.py",')
    swap('"one_way_delay_ms": 300','"one_way_delay_ms": 15')
    swap('"packet_sizes_equal_count_cycle": [64, 256, 1200]',
         '"packet_sizes_equal_count_cycle": [96, 1372, 4068]')
    swap('"generator_stats": "bounded-v1-exact-dedupe"',
         '"generator_stats": "sparse-45ms-exact-per-size-timing"')
    swap('"schema": 1, "source_sha": source,',
         '"schema": "wbd-e1-15ms-sparse-fullstack/v1", "source_sha": source,')
    swap('"mode": mode, "scenario": scenario, "seed": seed,',
         '"mode": mode, "scenario": scenario, "seed": seed, "fec_partial_window_ms": 32, "sparse_period_ms": 45,')
    # Disable per-record diagnostic ObserveTiming; this is a business test,
    # not a CPU profile. Do not alter Normal client routing or Game config.
    swap('--diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s','')
    swap('--diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 1s','')
    return s

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--output",required=True,type=Path)
    a=p.parse_args()
    original=Path(SOURCE).read_text()
    result=derive(original)
    a.output.parent.mkdir(parents=True,exist_ok=True)
    a.output.write_text(result)
    a.output.chmod(0o755)
    a.output.with_name(a.output.name+".receipt.json").write_text(json.dumps({
        "schema":"wbd-e1-lowrtt-harness-derivation/v1","product_source_sha":PRODUCT,
        "strict_template_sha256":hashlib.sha256(original.encode()).hexdigest(),
        "derived_script_sha256":hashlib.sha256(result.encode()).hexdigest(),
        "netem_one_way_ms":15,"loss_percent":0,"period_ms":45,
        "sizes":[96,1372,4068],"duration_s":120,"one_workload":True,
        "fullstack_expected":True,"cpu_gain":"UNPROVEN",
    },indent=2,sort_keys=True)+"\n")
    print("WBD_E1_LOWRTT_SINGLE_TOPOLOGY_DERIVED")
if __name__=="__main__":main()
