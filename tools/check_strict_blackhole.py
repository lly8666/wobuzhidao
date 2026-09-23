#!/usr/bin/env python3
import argparse, json, math
from pathlib import Path
ANALYSIS_VERSION="shared-blackhole-v1"
RECOVERY_BOUND_MS=3000
RECOVERY_WINDOW_MS=1000
RECOVERY_RATIO=0.99

def read_jsonl(path):
    out=[]
    for line in Path(path).read_text(encoding="utf-8",errors="replace").splitlines():
        try: out.append(json.loads(line))
        except Exception: pass
    return out

def qcounter(event,direction,key):
    rows=(event.get("qdisc") or {}).get(direction) or []
    return int(rows[0].get(key,0) or 0) if rows else 0

def blackout_qdisc(start,end,direction):
    return {
        "passed_packets":qcounter(end,direction,"packets")-qcounter(start,direction,"packets"),
        "drops":qcounter(end,direction,"drops")-qcounter(start,direction,"drops"),
        "overlimits":qcounter(end,direction,"overlimits")-qcounter(start,direction,"overlimits"),
    }

def recovery_from_wall(stats,start_ns,restore_mono_ns,path_delay_ms,target_mbps):
    bucket_ms=int(stats.get("recv_wall_bucket_ms",0) or 0); buckets=stats.get("recv_wall_bytes_by_bucket") or []
    if bucket_ms<=0 or RECOVERY_WINDOW_MS % bucket_ms:
        return {"valid":False,"error":f"invalid wall bucket_ms={bucket_ms}"}
    expected_ms=(restore_mono_ns-start_ns)/1e6+path_delay_ms
    start_idx=max(0,int(math.ceil(expected_ms/bucket_ms)))
    first_idx=next((i for i in range(start_idx,len(buckets)) if buckets[i]),None)
    window_bins=RECOVERY_WINDOW_MS//bucket_ms
    search_end=min(len(buckets)-window_bins+1,int(math.floor((expected_ms+RECOVERY_BOUND_MS)/bucket_ms))+1)
    threshold_bytes=target_mbps*1_000_000/8*(RECOVERY_WINDOW_MS/1000)*RECOVERY_RATIO
    win=None
    for i in range(start_idx,max(start_idx,search_end)):
        total=sum(buckets[i:i+window_bins])
        if total>=threshold_bytes:
            win={"start_bucket":i,"start_ms":i*bucket_ms,"delay_from_expected_ms":i*bucket_ms-expected_ms,
                 "bytes":total,"goodput_mbps":total*8/RECOVERY_WINDOW_MS/1000}
            break
    zero_lo=max(0,start_idx-int(1000/bucket_ms)); zero_hi=min(len(buckets),start_idx+int((RECOVERY_BOUND_MS+1000)/bucket_ms))
    longest=cur=0
    for v in buckets[zero_lo:zero_hi]:
        if v==0: cur+=1; longest=max(longest,cur)
        else: cur=0
    return {"valid":True,"bucket_ms":bucket_ms,"expected_resume_wall_ms":expected_ms,
            "first_data_delay_ms":None if first_idx is None else first_idx*bucket_ms-expected_ms,
            "qualifying_window":win,"recovery_bound_ms":RECOVERY_BOUND_MS,
            "window_ms":RECOVERY_WINDOW_MS,"window_ratio":RECOVERY_RATIO,
            "longest_zero_gap_ms_near_blackhole":longest*bucket_ms}

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--artifact-dir",required=True); ap.add_argument("--source-sha",required=True)
    ap.add_argument("--seed",type=int,required=True); ap.add_argument("--target-mbps",type=float,required=True)
    ap.add_argument("--lanes",type=int,required=True); ap.add_argument("--blackhole-ms",type=int,choices=(100,500),required=True)
    ap.add_argument("--base-summary",required=True); ap.add_argument("--output",required=True); args=ap.parse_args()
    root=Path(args.artifact_dir); base=json.loads(Path(args.base_summary).read_text(encoding="utf-8"))
    manifest=json.loads((root/"manifest.json").read_text(encoding="utf-8"))
    biz=json.loads((root/"biz.json").read_text(encoding="utf-8")); target=json.loads((root/"target.json").read_text(encoding="utf-8"))
    events=read_jsonl(root/"stage-events.jsonl"); by={x.get("event"):x for x in events}
    input_errors=[]; performance_errors=[]; expected_scenario=f"blackhole{args.blackhole_ms}"; cfg=manifest.get("config") or {}
    if manifest.get("source_sha")!=args.source_sha: input_errors.append("manifest source sha mismatch")
    if manifest.get("mode")!="game" or manifest.get("scenario")!=expected_scenario: input_errors.append("manifest mode/scenario mismatch")
    if args.lanes!=4 or int(cfg.get("lanes",0) or 0)!=4: input_errors.append("blackhole specialty requires 4 lanes")
    if abs(args.target_mbps-3.0)>1e-9 or abs(float(cfg.get("application_mbps_each_direction",0))-3.0)>1e-9: input_errors.append("blackhole specialty requires 3Mbps each direction")
    if cfg.get("fec")!="20:20" or int(cfg.get("one_way_delay_ms",0) or 0)!=300: input_errors.append("unexpected FEC/path delay")
    if int(cfg.get("blackhole_ms",0) or 0)!=args.blackhole_ms: input_errors.append("manifest blackhole duration mismatch")
    needed=("blackhole_start","blackhole_end","blackhole_restored")
    for name in needed:
        if name not in by: input_errors.append(f"missing event {name}")
    duration_ms=None; qdisc={}; recovery={}
    if all(name in by for name in needed):
        duration_ms=(by["blackhole_end"]["monotonic_ns"]-by["blackhole_start"]["monotonic_ns"])/1e6
        tol=max(20.0,args.blackhole_ms*0.10)
        if abs(duration_ms-args.blackhole_ms)>tol: input_errors.append(f"blackhole duration_ms={duration_ms:.3f} nominal={args.blackhole_ms}")
        for direction in ("c2s","s2c"):
            qdisc[direction]=blackout_qdisc(by["blackhole_start"],by["blackhole_end"],direction)
            if qdisc[direction]["drops"]<=0: input_errors.append(f"{direction} blackout qdisc drops={qdisc[direction]['drops']}")
        delay=int(cfg.get("one_way_delay_ms",0) or 0)
        recovery["c2s"]=recovery_from_wall(target["stats"],int(target["start_ns"]),by["blackhole_restored"]["monotonic_ns"],delay,args.target_mbps)
        recovery["s2c"]=recovery_from_wall(biz["stats"],int(biz["start_ns"]),by["blackhole_restored"]["monotonic_ns"],delay,args.target_mbps)
        for direction,row in recovery.items():
            if not row.get("valid"): performance_errors.append(f"{direction} recovery metric invalid")
            elif row.get("qualifying_window") is None: performance_errors.append(f"{direction} no 1s >=99% target window within 3s")
    if int((base.get("drain") or {}).get("late_after_10s_bytes",0) or 0): performance_errors.append("late delivery beyond 10s drain")
    base_cls=base.get("classifications") or {}; base_err=base.get("errors") or {}
    correctness_errors=list(base_err.get("correctness") or []); capture_errors=list(base_err.get("capture") or [])
    environment_errors=list(base_err.get("environment") or []); input_errors=list(base_err.get("input") or [])+input_errors
    cls={
      "CORRECTNESS":"PASS" if base_cls.get("CORRECTNESS")=="PASS" and not correctness_errors else "FAIL",
      "INPUT_VALIDITY":"PASS" if base_cls.get("INPUT_VALIDITY")=="PASS" and not input_errors else "FAIL",
      "CAPTURE":"PASS" if base_cls.get("CAPTURE")=="PASS" and not capture_errors else "FAIL",
      "ENVIRONMENT":"PASS" if base_cls.get("ENVIRONMENT")=="PASS" and not environment_errors else "FAIL",
    }
    if cls["CORRECTNESS"]!="PASS": perf="FAIL"
    elif any(cls[k]!="PASS" for k in ("INPUT_VALIDITY","CAPTURE","ENVIRONMENT")): perf="CAPACITY_LIMITED"
    else: perf="PASS" if not performance_errors else "FAIL"
    cls["PERFORMANCE"]=perf
    result={"schema":1,"analysis_version":ANALYSIS_VERSION,"source_sha":args.source_sha,"mode":"game","scenario":expected_scenario,
            "seed":args.seed,"lanes":args.lanes,"target_mbps_each_direction":args.target_mbps,"classifications":cls,
            "errors":{"correctness":correctness_errors,"input":input_errors,"capture":capture_errors,"environment":environment_errors,"performance":performance_errors},
            "blackhole":{"nominal_ms":args.blackhole_ms,"actual_event_ms":duration_ms,"nominal_offset_s":cfg.get("blackhole_nominal_offset_s"),"qdisc_during_event":qdisc},
            "recovery":recovery,
            "recovery_policy":{"blackout_loss_allowed":True,"bound_ms":RECOVERY_BOUND_MS,"required_window_ms":RECOVERY_WINDOW_MS,
                               "required_wall_goodput_ratio":RECOVERY_RATIO,"path_delay_alignment_ms":int(cfg.get("one_way_delay_ms",0) or 0),
                               "basis":"existing 3s repair horizon plus existing lossless 99% wall-goodput gate"},
            "base_lossless_accounting":{"classifications":base_cls,"performance_errors_ignored_for_blackout_specialty":base_err.get("performance") or [],
                                         "resource":base.get("resource"),"drain":base.get("drain"),"transport_hygiene":base.get("transport_hygiene"),"cost":base.get("cost")},
            "stage_events":events}
    Path(args.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("WBD_STRICT_BLACKHOLE_SAMPLE "+json.dumps({"scenario":expected_scenario,"seed":args.seed,"classifications":cls,"blackhole_ms":duration_ms,
          "recovery_ms":{d:((r.get("qualifying_window") or {}).get("delay_from_expected_ms")) for d,r in recovery.items()}},sort_keys=True))
    raise SystemExit(0 if all(v=="PASS" for v in cls.values()) else 1)
if __name__=="__main__": main()
