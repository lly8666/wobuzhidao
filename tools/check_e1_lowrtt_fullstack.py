#!/usr/bin/env python3
"""One source-frozen actual FakeTCP/TPROXY/TUN 15ms sparse UDP protector.

Must never infer FEC recovery from lossless business alone. This checks
zero-loss delivery, three payload size cohorts, p99, and native qdisc.
"""
import argparse
import json
from pathlib import Path

SOURCE="a2db258b436a41fdee98c6c53abec9bab6ce600f"
SIZES=(96,1372,4068)
SMALL_P99_NS=180_000_000
OTHER_P99_NS=350_000_000

def review(root,source):
    out={"schema":"wbd-e1-15ms-sparse-fullstack-review/v1",
         "status":"FAIL","source_sha":source,
         "scope":"REAL_CLIENT_SERVER_TPROXY_TUN_SPARSE_NORMAL1_15MS",
         "cpu_gain":"UNPROVEN","fec_repair_under_erasure":"NOT_TESTED",
         "host_matched_cpu":"NOT_TESTED","errors":[],"directions":{}}
    errors=out["errors"]
    try:
        manifest=json.loads((root/"manifest.json").read_text())
        config=manifest["config"]
        for key,actual,expected in (
            ("source",manifest.get("source_sha"),source),
            ("kind",manifest.get("schema"),"wbd-e1-15ms-sparse-fullstack/v1"),
            ("mode",manifest.get("mode"),"normal"),
            ("scenario",manifest.get("scenario"),"lossless"),
            ("lanes",config.get("lanes"),1),
            ("delay_ms",config.get("one_way_delay_ms"),15),
            ("duration",config.get("duration_s"),120),
            ("period",manifest.get("sparse_period_ms"),45),
            ("fec_window",manifest.get("fec_partial_window_ms"),32),
            ("fec",config.get("fec"),"20:20")):
            if actual!=expected:errors.append("MANIFEST_%s_MISMATCH"%key)
        if config.get("packet_sizes_equal_count_cycle")!=list(SIZES):
            errors.append("SIZES_PROFILE_INVALID")
        biz=json.loads((root/"biz.json").read_text())
        target=json.loads((root/"target.json").read_text())
        for role,row in (("biz",biz),("target",target)):
            if row.get("role")!=role or row.get("sparse_interval_ms")!=45 or row.get("sparse_sizes")!=list(SIZES):
                errors.append(role.upper()+"_WRONG_GENERATOR_OR_SCOPE")
        for name,recv,send in (("c2s",target,biz),("s2c",biz,target)):
            rx=recv.get("stats",{})
            tx=send.get("stats",{})
            sent=tx.get("sent_packets",0)
            got=rx.get("recv_unique_packets",0)
            item={"sent":sent,"received":got,"cohorts":{}}
            out["directions"][name]=item
            if not isinstance(sent,int) or sent<2500 or got!=sent:
                errors.append(name+"_LOSSLESS_COUNTS_MISMATCH")
            for field in ("recv_duplicates","corrupt","unexpected"):
                if rx.get(field)!=0:
                    errors.append(name+"_RX_"+field.upper())
            for field in ("send_failures","skipped_slots"):
                if tx.get(field)!=0:
                    errors.append(name+"_TX_"+field.upper())
            by_size=rx.get("oneway_by_size",{})
            for size in SIZES:
                stat=by_size.get(str(size),{})
                item["cohorts"][str(size)]={"count":stat.get("count",0),"p99_ns":stat.get("p99_ns")}
                if type(stat.get("count")) is not int or stat["count"]<800:
                    errors.append(name+"_MISSING_SIZE_"+str(size))
                    continue
                p99=stat.get("p99_ns")
                threshold=SMALL_P99_NS if size==96 else OTHER_P99_NS
                if type(p99) is not int or not 5_000_000<=p99<=threshold:
                    errors.append(name+"_P99_BUDGET_"+str(size))
        stage=[json.loads(row) for row in (root/"stage-events.jsonl").read_text().splitlines() if row.strip()]
        expected={"pre_start","stress_start","post_start","post_end","drain_start"}
        events={row.get("event") for row in stage}
        if not expected.issubset(events):errors.append("STAGE_EVENTS_INCOMPLETE")
        for row in stage:
            if row.get("event")=="harness_error":
                errors.append("STAGE_HARNESS_ERROR")
            loss=row.get("loss_percent")
            if loss is not None and (loss.get("c2s")!=0 or loss.get("s2c")!=0):
                errors.append("NOT_ZERO_LOSS_STAGE")
            for qdisc in (row.get("qdisc") or {}).values():
                if not any(v.get("kind")=="netem" for v in qdisc if isinstance(v,dict)):
                    errors.append("MISSING_REAL_NETEM")
    except (OSError,ValueError,KeyError,TypeError,json.JSONDecodeError) as exc:
        errors.append("MISSING_OR_MALFORMED_EVIDENCE_"+type(exc).__name__)
    if not errors:out["status"]="PASS_SCOPED_FULLSTACK_15MS_SPARSE_LOSSLESS"
    return out

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--artifact-dir",required=True,type=Path)
    p.add_argument("--source-sha",required=True)
    p.add_argument("--output",required=True,type=Path)
    a=p.parse_args()
    if a.source_sha!=SOURCE:
        p.error("product source SHA must equal the qualified 32ms candidate")
    report=review(a.artifact_dir,a.source_sha)
    a.output.parent.mkdir(parents=True,exist_ok=True)
    a.output.write_text(json.dumps(report,indent=2,sort_keys=True)+"\n")
    print("WBD_E1_FULLSTACK_SPARSE_15MS",report["status"],report["errors"])
    if report["status"]!="PASS_SCOPED_FULLSTACK_15MS_SPARSE_LOSSLESS":
        raise SystemExit(1)

if __name__=="__main__":main()
