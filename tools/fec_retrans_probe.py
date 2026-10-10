#!/usr/bin/env python3
"""Exactly one authorized 300ms/5% FEC-off real UDP retransmission audit."""
import argparse, json, os, subprocess
from pathlib import Path
from fec_policy_batch import BRANCH, SOURCE, cases, filehash, one

CONFIG=Path(".github/fec-retrans-probe.json")
def exact():
    config=json.loads(CONFIG.read_text())
    wanted={"schema":"wbd-fec-retrans-probe/v1","source_sha":SOURCE,
        "phase":"single_300ms_5pct_fec_off_udp","nonce":2}
    if config!=wanted:raise ValueError("not an authorized exact one-case retransmission audit")
    c=dict(cases("B")[3],loss=5)
    if (c["id"],c["workload"],c["loss"],c["fec"],c["parity"],
        c["duration_s"],c["drain_s"],c["delay_ms"],c["rate_mbps"],c["seed"],c["lanes"])!=(
        "s04","udp",5,"off",0,120,3,300,10,1910,1):
        raise ValueError("wrong exact cohort case")
    return c

def run(root):
    c=exact()
    if (os.environ.get("GITHUB_ACTIONS")!="true" or
        os.environ.get("GITHUB_REF_NAME")!=BRANCH or
        os.environ.get("GITHUB_EVENT_NAME")!="push"):
        raise ValueError("only explicit experiment config push in Actions")
    helper=subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip()
    for role in ("client","server"):
        if not (root/"binaries"/("wbd-"+role)).is_file():raise ValueError("frozen product missing")
    try:
        result=one(c,root,helper,wire_audit=True)
        case_dir=root/c["id"]
        def j(name):return json.loads((case_dir/name).read_text())
        wire=j("retrans-wire.json")
        summary=j("summary.json")
        ledger=j("efficiency-ledger.json")
        observed={}
        p_loss=.05
        ideal_factor=p_loss/(1-p_loss)
        if wire.get("theory_p")!=p_loss or abs(wire.get("theory_repeats_over_fresh_percent",0)-100*ideal_factor)>1e-9:
            raise ValueError("observer theory does not match actual 5 percent loss")
        for direction,info in wire["direction"].items():
            attempt=summary["netem_realized"][direction]["attempted"]
            packet_count=info["tcp_outer_packets"]
            frac=packet_count/attempt if attempt else 0
            observed[direction]={
                "outer_ingress_tcp_packets":packet_count,
                "qdisc_attempted_outer_packets":attempt,
                "packet_capture_coverage_vs_qdisc":frac,
                "fresh_data_packets":info["fresh_data_packets"],
                "retrans_data_packets":info["repeat_data_packets"],
                "fresh_data_payload_bytes":info["fresh_data_payload_bytes"],
                "retrans_payload_bytes":info["repeat_data_payload_bytes"],
                "retrans_data_ip_bytes":info["repeat_data_ip_bytes"],
                "all_outer_ip_bytes":info["tcp_outer_ip_bytes"],
                "retrans_over_fresh_payload_percent":info["repeat_over_fresh_payload_percent"],
                "retrans_percent_of_all_data_payload":info["repeat_fraction_data_payload_percent"],
                "retrans_percent_of_data_ip_bytes":info["repeat_fraction_data_ip_bytes_percent"],
                "kernel_observer_drops":info["kernel_packet_socket_drops"],
                "theory_needed_retrans_payload_bytes_geometric":info["fresh_data_payload_bytes"]*ideal_factor,
                "actual_as_fraction_of_geometric_theory":info["repeat_data_payload_bytes"]/(info["fresh_data_payload_bytes"]*ideal_factor) if info["fresh_data_payload_bytes"] else None}
        ok=bool(result["classification"]=="VALID_OBSERVATION" and result["sample_exit"]==0
             and result["owned_cleanup"]["clean"] and wire["capture_complete"]
             and all(.95 <= v["packet_capture_coverage_vs_qdisc"] <= 1.06 for v in observed.values())
             and all(v["kernel_observer_drops"]==0 for v in observed.values()))
        report={"schema":"wbd-fec-retrans-probe-result/v1",
           "scope":"single_real_business_300ms_one_way_udp_5pct_both_directions_fec_off",
           "source_sha":SOURCE,"helper_sha":helper,"case":c,
           "classification":"VALID_WIRE_OBSERVATION_NOT_PRODUCT_QUALIFICATION" if ok else "INVALID_OR_BUSINESS_FAIL",
           "sample_classification":result["classification"],
           "sample_issues":result["issues"],
           "sample_exit":result["sample_exit"],
           "owned_cleanup":result["owned_cleanup"],
           "wire_observer_sha256":filehash(case_dir/"retrans-wire.json"),
           "fixed_loss_actual":summary["netem_realized"],
           "all_sent_probe_deadline":{k:v.get("deadline") for k,v in summary.get("probe",{}).items()},
           "business_udp_by_size":{k:v["udp_by_size"] for k,v in summary["direction"].items()},
           "cpu_comparison":"NOT_COMPARABLE_SINGLE_FORENSIC_SAMPLE",
           "actual_runtime_shadow_peak":"NOT_COLLECTED_PROFILE_OFF",
           "theory_assumption":"Independent per-transmission loss p=.05; ideal sender detects all erasures, retries indefinitely. Repeat over fresh = p/(1-p) = 5.263158%; repeat fraction of all data transmissions = 5%. This ideal is NOT mandated by bounded product repair and does not include ACK/control overhead.",
           "observation":observed,
           "limitations":["outer ingress AF_PACKET classification by same sequence+payload length+ciphertext", 
             "post-netem loss and control packets excluded from retrans share denominator", 
             "wall-clock packet selection near business edges may differ from tc stage by a few milliseconds",
             "observer must have zero capture drops; qdisc coverage must be >=95%",
             "no claim 4096 capacity cause without runtime stats",
             "no FEC default, repair budget or product changes"]}
        (root/"retrans-report.json").write_text(json.dumps(report,sort_keys=True,indent=2)+"\n")
        print("FEC_RETRANS_PROBE_RESULT",json.dumps({k:{m:v[m] for m in
            ("retrans_over_fresh_payload_percent","retrans_data_packets","fresh_data_packets","packet_capture_coverage_vs_qdisc")} for k,v in observed.items()}),"classification",report["classification"],flush=True)
        if not ok:raise SystemExit("FEC_RETRANS_INVALID_EVIDENCE_NOT_A_PASS")
    except Exception:
        (root/"retrans-run-failed.json").write_text(json.dumps({"classification":"FAIL","case":c,
              "source_sha":SOURCE,"helper_sha":helper,"note":"see sanitized individual receipts; never infer missing numbers"},indent=2))
        raise

def main():
    a=argparse.ArgumentParser()
    a.add_argument("--mode",choices=("verify","run"),required=True)
    a.add_argument("--root",required=True)
    x=a.parse_args()
    case=exact()
    print("FEC_RETRANS_EXACT_PLAN",case,flush=True)
    if x.mode=="run":
        root=Path(x.root).resolve()
        root.mkdir(parents=True,exist_ok=True)
        run(root)
if __name__=="__main__":main()
