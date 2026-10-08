#!/usr/bin/env python3
"""Exact strict netns transform: one 300s TCP5M+UDP5M C source per direction."""
import argparse
import hashlib
import json
from pathlib import Path
from longmix_profile import WORKLOADS, validate_sample, C_UDP_SHARE
from prepare_longmix_tcp import generate as gen_b
from prepare_longmix_udp import replace

def generate(sample):
    w=validate_sample(sample)
    if w.name!="C":raise ValueError("C-only fullstack mixed 5+5Mbps")
    fake=dict(sample,workload="B",seed=WORKLOADS["B"].seed)
    script=gen_b(fake)
    script=replace(script,
      'GEN="$GITHUB_WORKSPACE/tools/longmix_tcp_business.py"',
      'GEN="$GITHUB_WORKSPACE/tools/longmix_mixed_business.py"')
    script=replace(script, '--workload B --seed "$SEED"',
                          '--workload C --seed "$SEED"',n=2)
    script=replace(script,'--output "$ART/tcp-biz.json"',
                   '--output "$ART/mixed-biz-launcher.json"')
    script=replace(script,'--output "$ART/tcp-target.json"',
                   '--output "$ART/mixed-target-launcher.json"')
    script+='''
python3 - "$ART" "$GITHUB_WORKSPACE" <<'WBDCMETA'
import hashlib,json,sys
from pathlib import Path
a=Path(sys.argv[1]);root=Path(sys.argv[2])
p=json.loads((a/"manifest.json").read_text())
p["schema"]="wbd-longmix-mixed-c/v1"
p["workload"]="C"
p["config"].update({
    "tcp_mbps_each_direction":5,"udp_mbps_each_direction":5,
    "logical_target_mbps_each_direction":10,
    "packet_sizes_target_byte_shares":[[96,10],[512,10],[1372,10],
        [8972,15],[8973,25],[65507,30]],
    "tcp_long_connections":3,"tcp_short_connections_active":1,
    "udp_probe_interval_ms":100,
    "tcp_short_interval_s":1,
    "two_protocols_one_sample":True,
    "tcp_and_udp_budget_reallocation":False
})
for name in ["tools/longmix_mixed_business.py","tools/prepare_longmix_mixed.py",
             "tools/longmix_udp_business.py","tools/longmix_tcp_business.py"]:
    p["harness_file_sha256"][name]=hashlib.sha256((root/name).read_bytes()).hexdigest()
(a/"manifest.json").write_text(json.dumps(p,indent=2,sort_keys=True)+"\\n")
WBDCMETA
'''
    return script

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--sample",required=True)
    p.add_argument("--artifact-dir",required=True)
    args=p.parse_args()
    sample=json.loads(Path(args.sample).read_text())
    script=generate(sample)
    root=Path(args.artifact_dir)
    root.mkdir(parents=True,exist_ok=True)
    out=root/"generated-longmix-mixed-c.sh"
    out.write_text(script)
    out.chmod(0o700)
    (root/"mixed-generation-receipt.json").write_text(json.dumps({
       "sample":sample,"scope":"GENERATED_ONLY_NOT_MEASUREMENT",
       "script_sha256":hashlib.sha256(script.encode()).hexdigest(),
       "template_sha256":hashlib.sha256(Path("scripts/strict_weaknet_sample.sh").read_bytes()).hexdigest()
    },indent=2)+"\n")
if __name__=="__main__":
    main()
