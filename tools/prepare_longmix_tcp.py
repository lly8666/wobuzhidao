#!/usr/bin/env python3
"""Prepare a single 300s B TCP run using the strict production netns path.

The generated business processes are genuine independent Linux TCP sockets;
the product's underlay remains FakeTCP/AF_PACKET, NOT kernel TCP.
"""
import argparse
import hashlib
import json
from pathlib import Path
from longmix_profile import WORKLOADS, validate_sample
from prepare_longmix_udp import generate as udp_generate, replace

def generate(sample):
    w=validate_sample(sample)
    if w.name!="B":
        raise ValueError("B-only sustained TCP generator")
    fake=dict(sample,workload="A",seed=WORKLOADS["A"].seed)
    s=udp_generate(fake)
    s=replace(s,
      'GEN="$GITHUB_WORKSPACE/tools/longmix_udp_business.py"',
      'GEN="$GITHUB_WORKSPACE/tools/longmix_tcp_business.py"')
    s=replace(s,'--bind 10.50.0.2:18080','--bind 10.50.0.2:18100')
    s=replace(s,'--bind 10.40.0.2:28080','--bind 10.40.0.2:28100')
    s=replace(s,'--peer 10.50.0.2:18080','--peer 10.50.0.2:18100')
    s=replace(s,
      '--rate-mbps "$RATE" --bounded-stats   --seed "$((SEED*100+2))"',
      '--workload B --seed "$SEED"')
    s=replace(s,
      '--rate-mbps "$RATE" --bounded-stats   --seed "$((SEED*100+1))"',
      '--workload B --seed "$SEED"')
    s=replace(s,'--output "$ART/target.json"','--output "$ART/tcp-target.json"')
    s=replace(s,'--output "$ART/biz.json"','--output "$ART/tcp-biz.json"')
    s=replace(s,'TGT_PID="$!"','TGT_PID="$!"\nsleep 0.8')
    # Explicit intermediate TCP connection snapshots, no plaintext or keys.
    sentinel='ip netns exec "$SRV" ip -details link show wbdg0 > "$ART/server-tun-active.txt"'
    s=replace(s,sentinel,sentinel + '''
ip netns exec "$BIZ" ss -tina -m > "$ART/tcp-client-ss.txt" 2>&1 || true
ip netns exec "$TGT" ss -tina -m > "$ART/tcp-target-ss.txt" 2>&1 || true
ip netns exec "$BIZ" cat /proc/net/snmp > "$ART/tcp-biz-snmp.txt" 2>&1 || true
ip netns exec "$TGT" cat /proc/net/snmp > "$ART/tcp-target-snmp.txt" 2>&1 || true''')
    s+='''
python3 - "$ART" "$GITHUB_WORKSPACE" <<'WBDTCPMETA'
import hashlib,json,sys
from pathlib import Path
a=Path(sys.argv[1]);root=Path(sys.argv[2])
p=json.loads((a/"manifest.json").read_text())
p["schema"]="wbd-longmix-tcp-b/v1"
p["workload"]="B"
p["config"].update({
  "logical_target_mbps_each_direction":10,
  "tcp_mbps_each_direction":10,"udp_mbps_each_direction":0,
  "tcp_long_connections":3,"tcp_short_connections_active":1,
  "tcp_short_interval_s":1,
  "tcp_application_write_sizes":[96,4096,65536,1048576],
  "no_tcp_backpressure_quota_reallocation":True,
  "tcp_socket_path":"biz real Linux TCP -> WBD TPROXY -> fake TCP underlay -> WBD shared server TUN -> target real Linux TCP"
})
p["config"].pop("packet_sizes_target_byte_shares",None)
p["harness_file_sha256"]["tools/longmix_tcp_business.py"]=hashlib.sha256(
   (root/"tools/longmix_tcp_business.py").read_bytes()).hexdigest()
p["harness_file_sha256"]["tools/prepare_longmix_tcp.py"]=hashlib.sha256(
   (root/"tools/prepare_longmix_tcp.py").read_bytes()).hexdigest()
(a/"manifest.json").write_text(json.dumps(p,indent=2,sort_keys=True)+"\\n")
WBDTCPMETA
'''
    return s

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--sample",required=True)
    p.add_argument("--artifact-dir",required=True)
    x=p.parse_args()
    sample=json.loads(Path(x.sample).read_text())
    script=generate(sample)
    art=Path(x.artifact_dir)
    art.mkdir(exist_ok=True,parents=True)
    path=art/"generated-longmix-tcp-b.sh"
    path.write_text(script)
    path.chmod(0o700)
    (art/"tcp-generation-receipt.json").write_text(json.dumps({
       "sample":sample,"schema":"wbd-longmix-tcp-b-generator/v1",
       "status":"GENERATED_NOT_PERFORMANCE",
       "source_template_sha256":hashlib.sha256(Path("scripts/strict_weaknet_sample.sh").read_bytes()).hexdigest(),
       "generated_script_sha256":hashlib.sha256(script.encode()).hexdigest()
    },indent=2)+"\n")

if __name__=="__main__":main()
