"""Explicit separate long-test harness, reusing only the formal strict topology."""
import argparse
import hashlib
import json
from pathlib import Path
from soak_weaknet_stage import plan


def main():
    a=argparse.ArgumentParser();a.add_argument('--artifact-dir',required=True);a.add_argument('--duration',type=int,choices=[180,1800],required=True)
    x=a.parse_args();root=Path(x.artifact_dir);root.mkdir(parents=True,exist_ok=True);(root/'capture').mkdir(exist_ok=True)
    script=Path('scripts/strict_weaknet_sample.sh').read_text()
    def replace(old,new,count=1):
        nonlocal script
        if script.count(old)!=count:raise ValueError('strict topology template changed: '+old[:80])
        script=script.replace(old,new)
    rotate=600 if x.duration==1800 else 50
    replace('STAGER="$GITHUB_WORKSPACE/tools/strict_weaknet_stage.py"','STAGER="$GITHUB_WORKSPACE/tools/soak_weaknet_stage.py"')
    replace('--post-loss "$POST_LOSS"     --seed',f'--post-loss "$POST_LOSS" --duration {x.duration} --seed')
    replace('--duration 120 --drain 10',f'--duration {x.duration} --drain 60 --bounded-stats',2)
    replace('--tproxy-port 12345',f'--rotate-min {rotate}s --rotate-max {rotate}s --tproxy-port 12345')
    for point in ['c2s-pre','c2s-post','s2c-pre','s2c-post']:
        replace(f'-s 256 -B 16384 -w "$ART/{point}.pcap"',f'-s 128 -B 16384 -Z root -G 60 -w "$ART/capture/{point}-%s.pcap"')
    replace('printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"',
            'printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"\npython3 -c \'import time,sys;print(time.time_ns()+int(sys.argv[1])-time.monotonic_ns())\' "$START_NS" > "$ART/start-unix-ns.txt"')
    replace('CAP_PIDS=()\n\npython3 -',f'CAP_PIDS=()\n\npython3 tools/soak_capture.py --artifact-dir "$ART" --duration {x.duration}\n\npython3 -')
    replace('"one_way_delay_ms": 300, "duration_s": 120, "stages_s": [30, 60, 30],',f'"one_way_delay_ms": 300, "duration_s": {x.duration}, "stages_s": None,')
    replace('"drain_s": 10,','"drain_s": 60,')
    replace('"schema": 1, "source_sha": source, "harness_sha": source,','"schema": "wbd-target-soak/v1", "source_sha": source, "harness_sha": source,')
    replace('"harness_file_sha256": files,',f'"harness_file_sha256": files, "qualification": "{ "FORMAL" if x.duration==1800 else "DIAGNOSTIC_ONLY" }", "stage_plan": {repr(plan(x.duration))}, "rotate_seconds": {rotate}, "capture_snaplen": 128,')
    replace('    workflow_rel,','    ".github/workflows/next-target-soak.yml",\n    "tools/prepare_soak_harness.py",\n    "tools/soak_stats.py",\n    "tools/soak_capture.py",\n    "tools/soak_weaknet_stage.py",\n    "tools/check_target_soak.py",')
    (root/'generated-soak.sh').write_text(script)
    (root/'generated-harness-receipt.json').write_text(json.dumps(dict(duration=x.duration,rotate=rotate,sha256=hashlib.sha256(script.encode()).hexdigest(),source_template='scripts/strict_weaknet_sample.sh'),indent=2)+'\n')


if __name__=='__main__':main()
