#!/usr/bin/env python3
"""Read-only E0/E6 per-sample CPU/PPS/GC/batch/repair cost ledger.

Never runs traffic, changes parameters, or labels CPU improvement across VMs.
Receipts are compact numbers only; absent diag-on fields stay NOT_COLLECTED.
"""
import argparse
import json
import re
from pathlib import Path

GIB=1024**3
def quotient(num,den):
    return num/den if den else None

def model(host):
    for line in host.splitlines():
        if re.match(r"^Model name\s*:",line):
            return line.split(":",1)[1].strip()
    return None

def sampled_numeric(obj,labels,prefix="",out=None):
    if out is None:out={}
    if len(out)>200:return out
    if isinstance(obj,dict):
        for k,v in obj.items():
            name=prefix+"."+str(k) if prefix else str(k)
            if isinstance(v,(int,float)) and not isinstance(v,bool) and any(q in k.lower() for q in labels):
                out[name]=v
            elif isinstance(v,(dict,list)) and name.count(".")<5:
                sampled_numeric(v,labels,name,out)
    elif isinstance(obj,list):
        for i,v in enumerate(obj[:10]):sampled_numeric(v,labels,prefix+"."+str(i),out)
    return out

def diag(side):
    if not side or not side.get("present"):
        return {"present":False,"status":"NOT_COLLECTED_PROFILE_OFF"}
    first=side.get("first_state") or {}
    last=side.get("last_state") or {}
    f=first.get("runtime") or {}
    e=last.get("runtime") or {}
    go={}
    for key in ("total_alloc","mallocs","frees","num_gc","pause_total_ns"):
        if isinstance(f.get(key),(int,float)) and isinstance(e.get(key),(int,float)):
            go[key+"_delta"]=max(0,e[key]-f[key])
    counters=sampled_numeric(last,("repair","batch","shard","parity","ack","source","packet","message","write_call","send_call","queue","decode"))
    owner=last.get("owner") or {}
    lanes=last.get("lanes") or []
    game={key:owner.get(key) for key in (
        "DesiredLanes","ActiveLogicalLanes","PhysicalLanes",
        "GameLogicalOutbound","GameLogicalOutboundBytes",
        "GameLaneCopies","GameLaneCopyBytes","GameDelivered",
        "GameDuplicates","GameStale","GameLaneMismatches")}
    lane_summary=[]
    for row in lanes[:10]:
        stats=row.get("lane") or {}
        tx=(stats.get("TxPath") or {}).get("Encoder") or {}
        rx=(stats.get("RxPath") or {}).get("Decoder") or {}
        lane_summary.append({"ref":row.get("ref"),"inbound_records":stats.get("InboundRecords"),
                             "delivered_datagrams":stats.get("DeliveredDatagrams"),
                             "record_errors":stats.get("RecordErrors"),
                             "path_errors":stats.get("PathErrors"),
                             "source_shards":tx.get("source_shards"),
                             "parity_shards":tx.get("parity_shards"),
                             "decoder_recovered_sources":rx.get("recovered_sources")})
    return {"present":True,"status":"DIAGNOSTIC_ON_NOT_COMPARABLE_TO_OFF",
            "snapshots":side.get("samples"),"go":go,
            "game_owner":game,"lane_summary":lane_summary,
            "bounded_last_counters_not_exact_window":counters}

def analyze(s,m,host):
    d=s.get("direction") or {};r=s.get("resources") or {};strict=r.get("strict_resource") or {}
    cpu=strict.get("process_cpu_seconds") or {}
    delivered=sum((d.get(side) or {}).get("total_logical_delivered_bytes_including_verified_http",(d.get(side) or {}).get("receiver_bytes",0)) for side in ("c2s","s2c"))
    submitted=sum((d.get(side) or {}).get("total_logical_sent_bytes_including_http",(d.get(side) or {}).get("sent_bytes",0)) for side in ("c2s","s2c"))
    cpu_total=sum(cpu.get(k,0) for k in ("client","server"))
    scope={}
    for side in ("c2s","s2c"):
        row=d.get(side) or {};probe=(s.get("probe") or {}).get(side) or {}
        q=(s.get("netem_realized") or {}).get(side) or {}
        udp=(row.get("udp_by_size") or {})
        scope[side]={"total_sent_bytes_including_probe":row.get("sent_bytes"),
                     "total_logical_sent_bytes_including_http":row.get("total_logical_sent_bytes_including_http"),
                     "actual_receiver_business_bytes":row.get("receiver_bytes"),
                     "sent_mbps":row.get("sent_mbps"),"goodput_mbps":row.get("goodput_mbps"),
                     "udp_app_datagrams":sum(v.get("sent_datagrams",0) for v in udp.values()),
                     "udp_missing_datagrams":sum(v.get("missing",0) for v in udp.values()),
                     "qdisc_attempted_outer_packets":q.get("attempted"),
                     "qdisc_attempted_outer_pps_not_nic_pps":quotient(q.get("attempted"),300),
                     "probe_sent":probe.get("sent"),"probe_returned":probe.get("returned"),
                     "probe_missing":probe.get("missing"),
                     "probe_returned_only_p99_ms":probe.get("p99_ms"),
                     "web_short":(s.get("web") or {}) if side=="c2s" else None}
    return {"schema":"wbd-efficiency-cost-ledger/v1",
            "source_sha":s.get("product_source_sha"),"helper_sha":s.get("helper_sha"),
            "sample":{"mode":m.get("mode"),"workload":s.get("workload"),
                      "seed":s.get("seed"),"loss_percent":s.get("loss_percent"),
                      "size_profile":s.get("size_profile"),"config":m.get("config")},
            "raw_analysis_classification":s.get("classification"),
            "raw_issues":s.get("issues"),
            "measurement_300s":True,
            "cpu":{"client_seconds":cpu.get("client"),"server_seconds":cpu.get("server"),
                   "total_product_seconds":cpu_total,
                   "total_cores_average_300s":quotient(cpu_total,300),
                   "seconds_per_delivered_gib":quotient(cpu_total,delivered/GIB),
                   "seconds_per_submitted_gib_probe_included":quotient(cpu_total,submitted/GIB)},
            "delivery":{"delivered_bytes_both_directions":delivered,
                        "submitted_bytes_both_directions_probe_included":submitted,
                        "two_directions":scope},
            "runner":{"cpu_model":model(host),"logical_cpus":int(re.search(r"nproc=(\d+)",host).group(1)) if re.search(r"nproc=(\d+)",host) else None,
                      "cgroup_quota_cores":r.get("cgroup_cpu_quota_cores"),
                      "steal_max_percent":r.get("host_steal_max_percent"),
                      "cpu_psi_some_avg10_max":r.get("cpu_psi_some_avg10_max"),
                      "host_busy_max_percent":r.get("host_busy_max_percent"),
                      "softirq_max_percent":r.get("host_softirq_max_percent")},
            "drops":{"socket_max":strict.get("socket_drop_max"),
                     "interfaces":strict.get("link_drop_delta")},
            "memory":{"peak_hwm_mib":r.get("process_hwm_max_mib"),
                      "peak_rss_mib":r.get("process_rss_max_mib")},
            "diagnostics":{side:diag((r.get("diagnostics") or {}).get(side)) for side in ("client","server")},
            "qualifiers":["read_only_artifact_no_traffic","not_cross_host_AB",
                          "cpu_profile_on_off_not_comparable",
                          "qdisc_pps_not_physical_nic_pps",
                          "returned_p99_never_hides_missing",
                          "no_optimizer_benefit_claim"]}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--artifact-dir",required=True)
    p.add_argument("--output",required=True)
    a=p.parse_args();root=Path(a.artifact_dir)
    try:
        s=json.loads((root/"summary.json").read_text())
        m=json.loads((root/"manifest.json").read_text())
        if m.get("source_sha")!=s.get("product_source_sha") or m.get("harness_sha")!=s.get("helper_sha"):
            raise ValueError("precise source/helper mismatch")
        result=analyze(s,m,(root/"runner-host.txt").read_text())
    except Exception as exc:
        result={"schema":"wbd-efficiency-cost-ledger/v1","classification":"INVALID",
                "error_type":type(exc).__name__,"error":str(exc)[:400],
                "not_performance_result":True}
        Path(a.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
        raise
    Path(a.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("E0_COST_LEDGER",result["raw_analysis_classification"],
          "cpu_s",result["cpu"]["total_product_seconds"],flush=True)

if __name__=="__main__":
    main()
