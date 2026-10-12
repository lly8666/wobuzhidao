#!/usr/bin/env python3
"""C10 one real 240s frozen continuous-time GE source/repair impairment.

Derive the formal protected five-netns, never simulate record/FEC product.
Common GE state is chosen by kernel CLOCK_MONOTONIC from frozen 900 events,
not by packet-driven netem gemodel. Each BAD packet drops independently 50%.
"""
import argparse,hashlib,json,itertools
from pathlib import Path
import adaptive_n2_clean300 as clean
from adaptive_n2_c08_steady1 import counter,grades as normal_grades
from adaptive_n2_offstart_recover540 import resource_evidence
from check_realpath_calibration import capture_drop
from adaptive_n2_ge_eventmap import make_trace

DURATION=240
FROZEN="docs/evidence/adaptive-n2-c10-ctmc-event-trace-freeze-v1.json"

def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def exact(s,old,new,n=1):
    seen=s.count(old)
    if seen!=n:raise ValueError(f"C10 protected script expected {n} anchor, got {seen}: {old}")
    return s.replace(old,new)

def prepare(root,script,receipt):
    frozen=json.loads((root/FROZEN).read_text())
    trace=make_trace(frozen)
    clean.prepare(root,script,receipt)
    s=script.read_text()
    s=exact(s,"--duration 100 --drain 10 --rate-mbps 10 --bounded-stats",
            "--duration 240 --drain 10 --rate-mbps 10 --bounded-stats",2)
    s=exact(s,'"duration_s": 100','"duration_s": 240')
    s=exact(s,'"loss_percent": 0','"loss_percent": "ge-clock450-50-bad50-240s"')
    marker='printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"'
    hookup="""
date +%s%N > "$ART/start-wall-ns.txt"
# Both fixed router egress paths use the SAME monotonic GE event list/epoch.
ip netns exec "$RTR" tc qdisc add dev rsrv clsact
ip netns exec "$RTR" tc qdisc add dev rcli clsact
ip netns exec "$RTR" tc filter add dev rsrv egress pref 1 bpf da obj "$ART/ge-eventmap.o" sec classifier
ip netns exec "$RTR" tc filter add dev rcli egress pref 1 bpf da obj "$ART/ge-eventmap.o" sec classifier
python3 "$GITHUB_WORKSPACE/tools/adaptive_n2_ge_eventmap.py" load \\
  --trace "$ART/ge-frozen-trace.json" --start-ns "$START_NS" \\
  --output "$ART/ge-maps-loaded.json"
python3 "$GITHUB_WORKSPACE/tools/strict_resource_sampler.py" \\
  --process "client=$CLIENT_PID" --process "server=$SERVER_PID" \\
  --namespace "router=$RTR" --interval 5 \\
  --output "$ART/resource-samples.jsonl" > "$ART/resources.log" 2>&1 &
RESOURCE_PID="$!"
"""
    s=exact(s,marker,marker+"\n"+hookup)
    after='wait "$TGT_PID"\nTGT_PID=""'
    audit="""
python3 "$GITHUB_WORKSPACE/tools/adaptive_n2_ge_eventmap.py" audit \\
  --trace "$ART/ge-frozen-trace.json" --start-ns "$START_NS" \\
  --output "$ART/ge-maps-audited.json"
kill -TERM "$RESOURCE_PID" 2>/dev/null || true
wait "$RESOURCE_PID" || true
RESOURCE_PID=""
"""
    s=exact(s,after,after+"\n"+audit)
    script.write_text(s)
    receipt.write_text(json.dumps({
        "schema":1,"scenario":"C10 one GE 450ms/50ms, BAD conditional50pct",
        "source_template":"scripts/realpath_calibration.sh",
        "template_sha256":digest(root/"scripts/realpath_calibration.sh"),
        "clean_helper_sha256":digest(root/"tools/adaptive_n2_clean300.py"),
        "c10_helper_sha256":digest(root/"tools/adaptive_n2_c10_ge_realpath.py"),
        "eventmap_driver_sha256":digest(root/"tools/adaptive_n2_ge_eventmap.py"),
        "eventmap_bpf_sha256":digest(root/"tools/adaptive_n2_ge_eventmap.c"),
        "frozen_manifest_sha256":digest(root/FROZEN),
        "frozen_trace_sha256":frozen["canonical_trace_sha256"],
        "frozen_trace_transition_count":len(trace["boundaries_ms"]),
        "frozen_model_version":frozen["model_version"],
        "derived_script_sha256":digest(script),
        "business_active_seconds":DURATION,"drain_s":10,
        "frozen_BAD_drop_percent":50,
        "directional_router_interfaces":["rsrv","rcli"],
        "diagnostics":"ON; no CPU optimization attribution"
    },indent=2,sort_keys=True)+"\n")

def longest_undelivered(seq):
    return max((sum(1 for _ in g) for v,g in itertools.groupby(seq,lambda x:x==0) if v),default=0)

def check(art,source,output):
    bad=[];invalid=[];capacity=[]
    required=["manifest.json","harness-prep.json","ge-maps-loaded.json","ge-maps-audited.json",
              "biz.json","target.json","start-wall-ns.txt","resource-samples.jsonl"]
    absent=[x for x in required if not (art/x).is_file()]
    if absent:
        result={"schema":1,"source_sha":source,"classification":"INVALID_C10_MISSING_EVIDENCE",
                "errors":[],"measurement_invalid":absent,"capacity_flags":[],
                "not_full_N2":True}
        output.write_text(json.dumps(result,indent=2)+"\n")
        raise SystemExit(1)
    manifest=json.loads((art/"manifest.json").read_text())
    prep=json.loads((art/"harness-prep.json").read_text())
    cfg=manifest.get("config") or {}
    freeze=json.loads(Path(FROZEN).read_text())
    trace=make_trace(freeze)
    if manifest.get("source_sha")!=source or manifest.get("harness_sha")!=source:
        invalid.append("exact SOURCE identity mismatch")
    for key,want in {"lanes":1,"fec_parity":20,"server_config_fec_parity":20,
        "client_record_cap_config":0,"server_record_cap_config":0,
        "v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
        "mtu":1400,"one_way_delay_ms_each_direction":300,
        "loss_percent":"ge-clock450-50-bad50-240s","duration_s":240,
        "application_mbps_each_direction":10.0}.items():
        if cfg.get(key)!=want:invalid.append(f"exact config {key}: {cfg.get(key)} != {want}")
    if prep.get("frozen_trace_sha256")!=freeze["canonical_trace_sha256"]:
        invalid.append("model trace receipt mismatch")
    a=json.loads((art/"ge-maps-loaded.json").read_text())
    b=json.loads((art/"ge-maps-audited.json").read_text())
    if (a.get("model_version")!=freeze["model_version"] or
        b.get("model_version")!=freeze["model_version"] or
        a.get("trace_sha256")!=freeze["canonical_trace_sha256"] or
        b.get("trace_sha256")!=freeze["canonical_trace_sha256"] or
        len(a.get("maps") or [])!=2 or len(b.get("maps") or [])!=2):
        invalid.append("missing exact two-direction GE freeze/epoch map receipt")
    ge={}
    for loaded,audited in zip(a.get("maps") or [],b.get("maps") or []):
        if loaded["map_id"]!=audited["map_id"] or loaded["start_ns"]!=audited["start_ns"]:
            invalid.append("BPF map epoch/identity was changed between before/after")
        if loaded.get("boundaries_ms")!=trace["boundaries_ms"] or audited.get("boundaries_ms")!=trace["boundaries_ms"]:
            invalid.append("BPF map runtime event table drift")
        if audited["good_packets"]<100 or audited["bad_packets"]<100:
            invalid.append("insufficient GE GOOD/BAD kernel packet witnesses")
        ratio=audited["bad_dropped"]/max(1,audited["bad_packets"])
        if not .4<=ratio<=.6:
            invalid.append("kernel conditional BAD loss not 40-60pct")
        ge[str(audited["map_id"])]={"good_packets":audited["good_packets"],
           "bad_packets":audited["bad_packets"],"bad_dropped":audited["bad_dropped"],
           "bad_erase_fraction":ratio,"same_epoch_and_eventlist":True}
    qdisc={}
    for dev in ("rsrv","rcli"):
        try:
            q0=counter(art/f"qdisc-{dev}-before.json")
            q1=counter(art/f"qdisc-{dev}-after.json")
            dp=q1["packets"]-q0["packets"]
            db=q1["bytes"]-q0["bytes"]
            if dp<=0 or db<=0:invalid.append("missing genuine GE line qdisc counters "+dev)
            qdisc[dev]={"forwarded_packets":dp,"forwarded_outer_bytes":db,
                "post_backlog":q1["backlog"],"post_queue_length":q1["qlen"],
                "overlimits_delta":q1["overlimits"]-q0["overlimits"],
                "requeues_delta":q1["requeues"]-q0["requeues"],
                "netem_drops_note":"Kernel BPF filter drops upstream of delay netem qdisc and must be taken from GE maps, not netem drops"}
            if qdisc[dev]["overlimits_delta"] or qdisc[dev]["requeues_delta"]:
                capacity.append("router qdisc capacity pressure "+dev)
        except Exception as ex: invalid.append("real qdisc "+dev+": "+str(ex))
    biz=json.loads((art/"biz.json").read_text())["stats"]
    tgt=json.loads((art/"target.json").read_text())["stats"]
    payload={}
    for direction,src,dst in (("c2s",biz,tgt),("s2c",tgt,biz)):
        sent=int(src["sent_packets"])
        received=int(dst["recv_unique_packets"])
        wall=(dst.get("recv_wall_packets_by_second") or [])[:DURATION]
        gap=longest_undelivered(wall) if wall else None
        ratio=received/max(1,sent)
        payload[direction]={"sent":sent,"first_unique":received,"missing":max(0,sent-received),
            "first_unique_ratio":ratio,"first_unique_bytes":dst.get("recv_unique_bytes"),
            "return_only_oneway_p99_ns":dst.get("oneway_p99_ns"),
            "send_failures":src.get("send_failures"),"corrupt":dst.get("corrupt"),
            "duplicates":dst.get("recv_duplicates"),"max_zero_first_delivery_s":gap}
        if src.get("send_failures") or dst.get("corrupt") or dst.get("recv_duplicates"):
            bad.append(direction+" real integrity or source emission failure")
        if sent<.96*DURATION*10_000_000/8/520:
            capacity.append(direction+" underinjected source")
        if ratio<.90 or gap is None or gap>4:
            bad.append(direction+" GE first-delivery <90% or >=5s blackout")
    statuses=(art/"process-state-after-drain.txt").read_text() if (art/"process-state-after-drain.txt").is_file() else ""
    if "client_alive=1 " not in statuses or "server_alive=1 " not in statuses:
        bad.append("product process did not survive full protected GE business")
    captures={}
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post"):
        drops=capture_drop(art/f"{stem}.tcpdump.log")
        captures[stem]=drops
        if drops is None or drops>0:capacity.append(stem+" capture drops/unknown")
        path=art/f"{stem}.pcap"
        if path.is_file():path.unlink()
        else:invalid.append("missing owned capture "+stem)
    sw=int((art/"start-wall-ns.txt").read_text().strip())
    ctrl={}
    for direction,name,server in (("c2s","client-diag.jsonl",False),("s2c","server-diag.jsonl",True)):
        x=normal_grades(art,name,server,sw)
        # No IID [R4,R8] grade-screen when correlated GE R20 itself can
        # be target unattainable. Profiles/qualified M/R are DIAGNOSTIC.
        ctrl[direction]={k:v for k,v in x.items()
                         if k not in ("final60_grade_screen","stable_meets_90pct_screen",
                                      "final60_in_frozen_grade_range_fraction","grade_seconds_over_allowed8")}
        if x["stable_active"]<45:invalid.append(direction+" GE product TX source diagnostics unavailable")
    delivered=sum((x["first_unique_bytes"] or 0) for x in payload.values())
    cpu=resource_evidence(art/"resource-samples.jsonl",delivered,bad)
    wire=sum(v["forwarded_outer_bytes"] for v in qdisc.values())
    generated=biz.get("sent_bytes",0)+tgt.get("sent_bytes",0)
    result={"schema":1,"source_sha":source,"scenario":"C10_ONE_CONTINUOUS_TIME_GE_900_EVENTS_240S",
        "classification":"INVALID_C10_MEASUREMENT" if invalid else
           "CAPACITY_LIMITED" if capacity else
           "C10_SCOPED_TIME_CORRELATED_GE_BUSINESS_PASS_NOT_FULL_N2" if not bad else
           "FAIL_C10_FIRST_UNIQUE_OR_PROTECTION",
        "errors":bad,"measurement_invalid":invalid,"capacity_flags":capacity,
        "model_version":freeze["model_version"],
        "true_CTMC_event_trace_sha256":freeze["canonical_trace_sha256"],
        "fixed_BAD_dwell_ms":freeze["exact_bad_state_ms"],
        "ge_BPF_kernel_good_bad_drop_counters":ge,"router_qdisc":qdisc,
        "business_first_unique":payload,
        "gross_forwarded_outer_wire_over_source_business":wire/generated if generated else None,
        "controller_DIAGNOSTIC_no_IID_grade_claim":ctrl,"cpu_diagnostic_on":cpu,
        "capture_kernel_drops":captures,
        "probe":{"sent":biz.get("probe_sent"),"returned":biz.get("probe_recv"),
                 "returned_only_rtt_p99_ns":biz.get("probe_rtt_p99_ns")},
        "not_full_N2":True,"physical":"NOT_RUN",
        "limitations":["GE BAD may erase systematic and parity simultaneously; fullk20 R20 model target may be unattainable",
          "Kernel GOOD/BAD erasures differ from IID average5pct and netem qdisc packet drop counters",
          "Source-first original damaged group k/R confidence and MDS residual need separate independent audit",
          "CPU diagnostic ON without same VM OFF comparison is not performance optimization proof",
          "mixed TCP/UDP 10ms noHOL, Game/MTU and legacy2.73s/80s remain OPEN"]}
    output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_N2_C10_"+result["classification"])
    if invalid or capacity or bad:raise SystemExit(1)

def main():
    p=argparse.ArgumentParser()
    sp=p.add_subparsers(dest="cmd",required=True)
    x=sp.add_parser("prepare")
    x.add_argument("--root",type=Path,default=Path("."))
    x.add_argument("--script",type=Path,required=True)
    x.add_argument("--receipt",type=Path,required=True)
    x=sp.add_parser("check")
    x.add_argument("--artifact-dir",type=Path,required=True)
    x.add_argument("--source-sha",required=True)
    x.add_argument("--output",type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.root,a.script,a.receipt)
    else:check(a.artifact_dir,a.source_sha,a.output)
if __name__=="__main__":main()
