#!/usr/bin/env python3
"""Read-only hosted-runner CPU cgroup *visibility* diagnosis.

A missing /sys/fs/cgroup/cpu.max may reflect cgroup-v2 namespace root.
It must NEVER be substituted by "max" or interpreted as absence of a
physical/hypervisor quota. No mount, sysctl or product-network changes.
Report only coarse normalized scope, not raw cgroup paths or host IDs.
"""
import argparse
import json
from pathlib import Path

def parse_mountinfo(text):
    for line in text.splitlines():
        try:
            front,back=line.split(" - ",1)
            pre=front.split()
            post=back.split()
            if (len(pre)>4 and len(post)>0 and
                    pre[4]=="/sys/fs/cgroup" and post[0]=="cgroup2"):
                return {"type":"cgroup2","mount_root":pre[3]=="/"}
        except (IndexError,ValueError):
            continue
    return {"type":"UNKNOWN","mount_root":False}

def classify(cgroups,mountinfo,cpu_max,cpu_stat,controllers):
    mount=parse_mountinfo(mountinfo)
    current_root="0::/" in cgroups.splitlines()
    result={
        "schema":"wbd-e4-runner-cpu-scope/v1",
        "read_only":True,"mount_type":mount["type"],
        "cgroup_namespace_root":bool(current_root and mount["mount_root"]),
        "cpu_stat_visible":bool(cpu_stat),
        "controllers_visible":bool(controllers),
        "cpu_max_visible":cpu_max is not None,
        "quota_visibility":"UNKNOWN",
        "effective_host_or_hypervisor_quota":"UNKNOWN",
        "suitable_as_same_vm_identifier":False,
        "use_as_performance_normalization":False,
    }
    if mount["type"]=="cgroup2" and cpu_max is not None:
        fields=cpu_max.split()
        if len(fields)==2 and fields[1].isdigit() and int(fields[1])>0 and (
                fields[0]=="max" or (fields[0].isdigit() and int(fields[0])>0)):
            result["quota_visibility"]="CGROUP_V2_CPU_MAX_EXPLICITLY_VISIBLE"
            result["visible_cpu_max"]=cpu_max
        else:
            result["quota_visibility"]="INVALID_CGROUP_V2_CPU_MAX"
    elif (mount["type"]=="cgroup2" and cpu_max is None and
          current_root and mount["mount_root"] and cpu_stat and controllers):
        result["quota_visibility"]="CGROUP_V2_NAMESPACE_ROOT_NO_CPU_MAX"
        # cgroup-v2 root has no cpu.max. The namespace may hide ancestors,
        # therefore NEVER convert this to a quota=unlimited assertion.
    elif mount["type"]=="cgroup2":
        result["quota_visibility"]="UNKNOWN_CGROUP_V2_CPU_MAX_NOT_VISIBLE"
    return result

def snapshot():
    def read(path):
        try:return Path(path).read_text(encoding="utf-8").strip()
        except (OSError,UnicodeError):return None
    return classify(
        read("/proc/self/cgroup") or "",
        read("/proc/self/mountinfo") or "",
        read("/sys/fs/cgroup/cpu.max"),
        read("/sys/fs/cgroup/cpu.stat"),
        read("/sys/fs/cgroup/cgroup.controllers"))

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--output",required=True,type=Path)
    args=p.parse_args()
    receipt=snapshot()
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(receipt,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("WBD_E4_CGROUP_QUOTA_VISIBILITY",receipt["quota_visibility"])
    # Unknown is an honest diagnostic result, not a product failure.

if __name__=="__main__":main()
