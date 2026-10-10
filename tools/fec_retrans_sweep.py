#!/usr/bin/env python3
"""One GitHub Actions job, 36 strictly serial fresh 120s FEC-off UDP realpath cases.

Numbers from failed/incomplete cases remain evidence, never success. No product change.
"""
import argparse, datetime, json, os, subprocess, time
from pathlib import Path
from fec_policy_batch import BRANCH, SOURCE, filehash, one

CONFIG=Path(".github/fec-retrans-sweep.json")
DELAYS=(50,100,150)
LOSSES=(1,5,10)
RATES=(10,20,30,50)
N=36

def cases():
    out=[]
    for di,delay in enumerate(DELAYS):
        for li,loss in enumerate(LOSSES):
            seed=31001+di*100+li*10
            for rate in RATES:
                out.append(dict(id="s%02d"%(len(out)+1),workload="udp",loss=loss,
                    fec="off",parity=0,seed=seed,rate_mbps=rate,lanes=1,
                    mode="normal",duration_s=120,drain_s=3,delay_ms=delay))
    assert len(out)==N and len({x["id"] for x in out})==N
    return out

def exact():
    d=json.loads(CONFIG.read_text())
    wanted={"schema":"wbd-fec-retrans-sweep/v1","phase":"36_serial_120s_udp_off",
            "source_sha":SOURCE,"nonce":1}
    if d!=wanted:raise ValueError("unauthorized 36-case sweep config")
    out=cases()
    if set((x["delay_ms"],x["loss"],x["rate_mbps"]) for x in out)!={
            (t,l,r) for t in DELAYS for l in LOSSES for r in RATES}:
        raise ValueError("missing/duplicate matrix tuple")
    if any(x["duration_s"]!=120 or x["drain_s"]!=3 or x["fec"]!="off" or
           x["parity"]!=0 or x["lanes"]!=1 or x["workload"]!="udp" for x in out):
        raise ValueError("unsafe case plan")
    return out

def read(d,name):
    try:return json.loads((d/name).read_text())
    except (OSError,ValueError):return {}

def maybe_int(v):
    return int(v) if isinstance(v,(int,float)) and not isinstance(v,bool) else None

def summarize(case,receipt,folder):
    summary=read(folder,"summary.json")
    ledger=read(folder,"efficiency-ledger.json")
    wire=read(folder,"retrans-wire.json")
    manifest=read(folder,"manifest.json")
    flags=read(folder,"runtime-flags.json")
    row={"case":case,"status":"INVALID_OR_INCOMPLETE","classification":receipt.get("classification"),
         "source_sha":receipt.get("source_sha"),"helper_sha":receipt.get("helper_sha"),
         "sample_exit":receipt.get("sample_exit"),"sample_exception":receipt.get("sample_exception"),
         "analyzer_exit":receipt.get("analyzer_exit"),"ledger_exit":receipt.get("ledger_exit"),
         "analyzer_issues":summary.get("issues",[]),"cleanup":receipt.get("owned_cleanup"),
         "binary_sha256":receipt.get("binary_sha256"),
         "config_actual":manifest.get("config"),"runtime_flags":flags,
         "capture_complete":wire.get("capture_complete",False),
         "wire_observer_sha256":receipt.get("sha256",{}).get("retrans-wire.json"),
         "runner":ledger.get("runner"),"memory_mib":ledger.get("memory"),
         "cpu":ledger.get("cpu"),"business_delivery":{},"wire":{}}
    # Never claim a nominal rate was delivered: include actual observed injection.
    sides=summary.get("direction") or {}
    probes=summary.get("probe") or {}
    qdisc=summary.get("netem_realized") or {}
    obs=wire.get("direction") or {}
    for side in ("c2s","s2c"):
        biz=sides.get(side) or {};p=probes.get(side) or {};q=qdisc.get(side) or {};w=obs.get(side) or {}
        packets=biz.get("udp_by_size") or {}
        row["business_delivery"][side]={
            "sent_mbps":biz.get("sent_mbps"),"goodput_mbps":biz.get("goodput_mbps"),
            "target_mbps":biz.get("target_mbps"),"target_achievement":biz.get("target_achievement"),
            "offered_bytes":biz.get("offered_bytes"),"submitted_bytes":biz.get("sent_bytes"),
            "receiver_bytes":biz.get("receiver_bytes"),
            "udp_sent":sum(k.get("sent_datagrams",0) for k in packets.values()),
            "udp_missing":sum(k.get("missing",0) for k in packets.values()),
            "udp_by_size":packets,
            "probe_sent":p.get("sent"),"probe_returned":p.get("returned"),
            "probe_missing":p.get("missing"),"probe_deadline_all_sent":p.get("deadline"),
            "probe_returned_only_p99_ms":p.get("p99_ms")}
        fresh=w.get("fresh_data_payload_bytes") or 0
        repeat=w.get("repeat_data_payload_bytes") or 0
        attempted=q.get("attempted") or 0
        observed=w.get("tcp_outer_packets") or 0
        row["wire"][side]={
            "fresh_data_packets":w.get("fresh_data_packets"),"repeat_data_packets":w.get("repeat_data_packets"),
            "fresh_payload_bytes":fresh,"repeat_payload_bytes":repeat,
            "retrans_over_fresh_payload_pct":(100*repeat/fresh if fresh else None),
            "retrans_fraction_all_data_payload_pct":(100*repeat/(fresh+repeat) if fresh+repeat else None),
            "fresh_data_ip_bytes":w.get("fresh_data_ip_bytes"),"retrans_data_ip_bytes":w.get("repeat_data_ip_bytes"),
            "all_outer_ip_bytes_including_ack":w.get("tcp_outer_ip_bytes"),
            "retrans_over_all_outer_ip_bytes_pct":(100*w.get("repeat_data_ip_bytes",0)/w["tcp_outer_ip_bytes"] if w.get("tcp_outer_ip_bytes") else None),
            "qdisc_attempted":attempted,"qdisc_dropped":q.get("dropped"),
            "realized_netem_loss_pct":q.get("realized_percent"),
            "observer_tcp_outer_packets":observed,
            "observer_qdisc_coverage":observed/attempted if attempted else None,
            "observer_kernel_drops":w.get("kernel_packet_socket_drops"),
            "ciphertext_conflicts":w.get("same_seq_different_ciphertext"),
            "capture_ok":w.get("capture_ok")}
    model=case["loss"]/(100-case["loss"])
    row["ideal_geometric_retrans_over_fresh_pct"]=100*model
    if not (wire.get("theory_p")==case["loss"]/100 and
            abs(wire.get("theory_repeats_over_fresh_percent",0)-100*model)<1e-8 and
            wire.get("theory_repeat_fraction_all_data_percent")==float(case["loss"])):
        row["analyzer_issues"].append("WRONG_IDEAL_THEORY_METADATA")
    b0=case["rate_mbps"]
    exact_scope=bool(
        summary.get("product_source_sha")==SOURCE
        and summary.get("loss_percent")==case["loss"]
        and summary.get("delay_ms")==case["delay_ms"]
        and summary.get("duration_s")==120
        and summary.get("target_mbps")==b0
        and summary.get("fec_parity")==0
        and manifest.get("source_sha")==SOURCE
        and manifest.get("config",{}).get("one_way_delay_ms")==case["delay_ms"]
        and manifest.get("config",{}).get("application_mbps_each_direction")==b0
        and all(flags.get(role,{}).get("--fec-parity")=="0" and
                flags.get(role,{}).get("--mtu")=="1400" for role in ("client","server"))
        and all(row["wire"][side]["capture_ok"] and
                row["wire"][side]["observer_kernel_drops"]==0 and
                row["wire"][side]["observer_qdisc_coverage"] is not None and
                0.95<=row["wire"][side]["observer_qdisc_coverage"]<=1.06
                for side in ("c2s","s2c")))
    row["capture_scope_ok"]=exact_scope
    good=bool(exact_scope and receipt.get("sample_exit")==0 and
              receipt.get("owned_cleanup",{}).get("clean")
              and summary.get("classification")=="VALID_OBSERVATION"
              and receipt.get("classification")=="VALID_OBSERVATION"
              and receipt.get("analyzer_exit")==0 and receipt.get("ledger_exit")==0
              and not row["analyzer_issues"]
              and ledger.get("cpu",{}).get("client_seconds") is not None
              and ledger.get("cpu",{}).get("server_seconds") is not None
              and all(ledger.get("memory",{}).get("peak_hwm_mib",{}).get(role) is not None
                  and ledger.get("memory",{}).get("peak_rss_mib",{}).get(role) is not None for role in ("client","server")))
    row["status"]="VALID_SCOPED_REAL_OBSERVATION" if good else (
        "INSUFFICIENT_INJECTION" if any("INSUFFICIENT_INJECTION" in x for x in row["analyzer_issues"])
        else "UNQUALIFIED_OR_MISSING_EVIDENCE")
    return row

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--mode",choices=("verify","run"),required=True)
    ap.add_argument("--root",required=True)
    a=ap.parse_args();plan=exact()
    print("FEC_SWEEP_VERIFIED",N,SOURCE,"ONE_JOB_STRICT_SERIAL",flush=True)
    if a.mode=="verify":return
    if (os.environ.get("GITHUB_ACTIONS")!="true" or
        os.environ.get("GITHUB_REF_NAME")!=BRANCH or
        os.environ.get("GITHUB_EVENT_NAME")!="push"):
        raise ValueError("only exact experiment branch config push allowed")
    helper=subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip()
    root=Path(a.root).resolve()
    root.mkdir(parents=True,exist_ok=True)
    binaries={role:filehash(root/"binaries"/("wbd-"+role)) for role in ("client","server")}
    receipt={"schema":"wbd-fec-retrans-sweep-result/v1",
        "state":"RUNNING_NO_SCOPE_PASS_YET","source_sha":SOURCE,"helper_sha":helper,
        "started_utc":datetime.datetime.now(datetime.timezone.utc).isoformat(),
        "planned_cases":36,"completed_cases":0,"one_job_sequential":True,
        "product_binary_sha256":binaries,"cases":[{"case":c,"status":"NOT_RUN"} for c in plan],
        "cpu_comparability":"SAME_VM_ONLY_HOST_PSI_AND_QUOTA_MUST_BE_CHECKED",
        "profile":"OFF","product_defaults_modified":False,
        "observer_overhead":"TWO_ROUTER_AF_PACKET_COUNTERS_NOT_INCLUDED_IN_PRODUCT_CPU;HOST_RESOURCE_PRESSURE_STILL_CONFOUNDS",
        "physical_nic":"NOT_COLLECTED","product_qualification":False}
    def write():
        (root/"sweep-manifest.json").write_text(json.dumps(receipt,sort_keys=True,indent=2)+"\n")
    write()
    fatal=False
    for idx,case in enumerate(plan):
        t=time.monotonic()
        print("FEC_SWEEP_CASE_START",idx+1,N,case,flush=True)
        folder=root/case["id"]
        try:
            raw=one(case,root,helper,wire_audit=True,sweep=True)
            row=summarize(case,raw,folder)
            row["elapsed_wall_s"]=round(time.monotonic()-t,2)
            (folder/"sweep-row.json").write_text(json.dumps(row,sort_keys=True,indent=2)+"\n")
            receipt["cases"][idx]=row
            receipt["completed_cases"]=idx+1
            if not raw.get("owned_cleanup",{}).get("clean") or raw.get("classification")=="INTEGRITY_STOP":
                fatal=True;receipt["stop_reason"]="INTEGRITY_OR_OWNED_RESOURCE_LEAK"
            if raw.get("sample_exception") or raw.get("sample_exit") not in (0,):
                fatal=True;receipt["stop_reason"]="SHELL_OR_GENERATOR_FAILURE"
            if raw.get("binary_sha256")!=binaries:
                fatal=True;receipt["stop_reason"]="FROZEN_BINARY_CHANGED"
            print("FEC_SWEEP_CASE_END",case["id"],row["status"],
                  "sent",[(d,row["business_delivery"][d]["sent_mbps"]) for d in ("c2s","s2c")],
                  "repeat_pct",[(d,row["wire"][d]["retrans_over_fresh_payload_pct"]) for d in ("c2s","s2c")],
                  flush=True)
        except Exception as exc:
            fatal=True
            receipt["stop_reason"]="EXCEPTION_"+type(exc).__name__
            receipt["cases"][idx]={"case":case,"status":"INFRA_INVALID_EXCEPTION","reason":type(exc).__name__}
            receipt["completed_cases"]=idx+1
            print("FEC_SWEEP_CASE_EXCEPTION",case["id"],type(exc).__name__,flush=True)
        finally:
            write()
        if fatal:break
        # Completion, fresh resource/namespace teardown and independent cooldown
        # verified per case before the next tuple.
        time.sleep(3)
    statuses=[r["status"] for r in receipt["cases"]]
    receipt["valid_cases"]=statuses.count("VALID_SCOPED_REAL_OBSERVATION")
    receipt["not_run_cases"]=statuses.count("NOT_RUN")
    receipt["ended_utc"]=datetime.datetime.now(datetime.timezone.utc).isoformat()
    receipt["state"]="ALL_36_VALID_OBSERVATIONS_NOT_PRODUCT_PASS" if receipt["valid_cases"]==36 else "PARTIAL_OR_INVALID_NOT_PRODUCT_PASS"
    write()
    print("FEC_SWEEP_FINAL",receipt["state"],"valid",receipt["valid_cases"],
          "completed",receipt["completed_cases"],"not_run",receipt["not_run_cases"],flush=True)
    if receipt["valid_cases"]!=36:raise SystemExit("FEC_SWEEP_PARTIAL_OR_FAILED_KEEP_NUMERIC_ARTIFACT")
if __name__=="__main__":main()
