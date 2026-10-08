#!/usr/bin/env python3
"""Artifact-only reducer for independent 300s runs. NEVER executes any traffic."""
import argparse
import json
from collections import defaultdict
from pathlib import Path

WORKLOADS=("udp","tcp","mixed")
LOSSES=(0,5,20,30)

def analyze(root,requested):
    cases={};errors=[]; runs=[]
    product=requested["product_source_sha"];helper=requested["helper_source_sha"]
    for spec in requested["runs"]:
        workload=spec["workload"];loss=spec["loss"];seed=spec["seed"];run=spec["run_id"]
        assert workload in WORKLOADS and loss in LOSSES and isinstance(run,int)
        key=(workload,loss,seed)
        if key in cases:raise ValueError("duplicate independent sample identity "+str(key))
        path=root/str(run)/"summary.json"
        if not path.exists():
            cases[key]={"classification":"INVALID","reason":"NO_ARTIFACT_SUMMARY","run_id":run}
            errors.append("run %s has no auditable summary"%run)
            continue
        row=json.loads(path.read_text())
        if (row.get("product_source_sha")!=product or row.get("helper_sha")!=helper
                or row.get("workload")!=workload or row.get("loss_percent")!=loss
                or row.get("seed")!=seed):
            cases[key]={"classification":"INVALID","reason":"SOURCE_OR_SCENARIO_MISMATCH","run_id":run}
            errors.append("identity mismatch run %d"%run)
            continue
        item={"classification":row.get("classification","INVALID"),
              "run_id":run,"seed":seed,
              "issues":row.get("issues",[]),
              "netem":row.get("netem_realized",{})}
        for side in ("c2s","s2c"):
            d=row.get("direction",{}).get(side,{})
            item[side]={
                "sent_mbps":d.get("sent_mbps"),
                "goodput_mbps":d.get("goodput_mbps"),
                "target_achievement":d.get("target_achievement"),
                "udp_by_size":d.get("udp_by_size"),
                "large_udp_roundtrip_ms":d.get("large_udp_roundtrip_ms"),
                "tcp_integrity":d.get("tcp",{}).get("mismatch_total"),
                "tcp_backpressure":d.get("tcp",{}).get("backpressure_over_10ms"),
                "longest_active_zero_recv_ms":d.get("continuity",{}).get("longest_active_gap_ms")}
        item["probes"]=row.get("probe",{})
        resources=row.get("resources",{})
        item["capacity_evidence"]=resources.get("capacity_evidence",[])
        item["process_cpu_seconds"]=resources.get("strict_resource",{}).get("process_cpu_seconds",{})
        item["socket_drop_max"]=resources.get("strict_resource",{}).get("socket_drop_max")
        cases[key]=item
        runs.append({"workload":workload,"loss":loss,"seed":seed,"run_id":run,
                     "result":item["classification"]})
    table=[]
    for name in WORKLOADS:
        for loss in LOSSES:
            entries=[(seed,case) for (w,p,seed),case in cases.items() if w==name and p==loss]
            if not entries:
                table.append({"workload":name,"loss_percent":loss,"classification":"NOT_RUN"})
            else:
                for seed,case in sorted(entries):
                    table.append({"workload":name,"loss_percent":loss,**case})
    return {"schema":"wbd-large-mtu-aggregate/v1","data_source":"artifact_only_no_measurement",
            "product_source_sha":product,"helper_source_sha":helper,
            "runs":runs,"table":table,"errors":errors,
            "all_12_have_qualifying_summary":len(cases)==12 and not errors and all(v.get("classification")=="PASS_SCOPED_ACTIONS" for v in cases.values())}

if __name__=="__main__":
    a=argparse.ArgumentParser()
    a.add_argument("--root",required=True);a.add_argument("--manifest",required=True);a.add_argument("--output",required=True)
    x=a.parse_args()
    spec=json.loads(Path(x.manifest).read_text())
    out=analyze(Path(x.root),spec)
    Path(x.output).write_text(json.dumps(out,indent=2,sort_keys=True))
    print("WBD_LARGE_AGGREGATE_BEGIN")
    for row in out["table"]:
        print("%-5s %2s%% %-17s run=%s seed=%s"%(
            row["workload"],row["loss_percent"],row["classification"],
            row.get("run_id","-"),row.get("seed","-")))
        if "c2s" in row:
            for side in ("c2s","s2c"):
                x=row[side];p=row["probes"].get(side,{})
                print("  %s tx=%.4fMbps rx=%.4fMbps probe_p99=%s probe_missing=%s jumbo65507=%s"%(
                    side,x.get("sent_mbps") or 0,x.get("goodput_mbps") or 0,
                    p.get("p99_ms"),p.get("missing"),
                    x.get("large_udp_roundtrip_ms",{}).get("65507")))
        if row.get("issues"):print("  issues=%s"%row["issues"][:10])
    print("WBD_LARGE_AGGREGATE_END")
