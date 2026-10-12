#!/usr/bin/env python3
"""Strict, independent one-M3-R1 20pct->5pct active realpath acceptance.

This is not the automatic FEC controller, and the product NEVER receives tc
damage/stage truth. All limits were frozen BEFORE the first real sample.
"""
from __future__ import annotations
import argparse
import hashlib
import json
from pathlib import Path

GATE_VERSION = "n2-m3-recovery-active-grade-and-first-delivery-v1"
STAGES=(("loss20",0,120,20,(16,20)),("recover5",120,330,5,(8,10)))
RECOVERY_RECEIVE_RATIO_MIN=.90
MAX_RECOVERY_NO_DELIVERY_SECONDS=4
MIN_FINAL60_GRADE_FRACTION=.90

def filehash(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def longest_gap(counts,start=0):
    best=cur=0
    for x in counts:
        if x < 0:
            raise ValueError("negative wall first-delivery sample")
        if x == 0:
            cur+=1
            best=max(best,cur)
        else:
            cur=0
    return best

def score(summary,manifest,biz,target,freeze,sha):
    faults=[]
    if summary.get("source_sha")!=sha or manifest.get("source_sha")!=sha or manifest.get("harness_sha")!=sha:
        faults.append("SOURCE_SHA_IDENTITY_MISMATCH")
    if (freeze.get("model_version")!="n2-m3-iid-recover-high20-to5-v1" or
        freeze.get("duration_active_s")!=330 or
        freeze.get("scenario")!="M3-R1"):
        faults.append("FROZEN_MODEL_NOT_M3_R1")
    cfg=manifest.get("config") or {}
    for key,want in {"v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
                     "server_config_fec_parity":20,"duration_s":330,
                     "mtu":1400,"one_way_delay_ms_each_direction":300,
                     "application_mbps_each_direction":10.0,
                     "loss_percent":"waveform:20-5"}.items():
        if cfg.get(key)!=want: faults.append("MISMATCH_CONFIG_"+key)
    if summary.get("errors") or summary.get("correctness_errors"):
        faults.append("SUMMARY_ERRORS_OR_CORRUPTION")
    if summary.get("capacity_flags"):
        faults.append("CAPACITY_FLAGS_SEPARATE_NOT_SCOPED_PASS")
    phases=summary.get("waveform") or []
    if len(phases)!=len(STAGES):
        faults.append("BAD_PHASE_COUNT")
    found={}
    for name,start,end,p,bounds in STAGES:
        ph=next((x for x in phases if x.get("name")==name),None)
        if not ph or ph.get("start_s")!=start or ph.get("end_s")!=end:
            faults.append("PHASE_TIMING_MISSING_"+name)
            continue
        per_direction={}
        for d in ("c2s","s2c"):
            x=ph.get(d) or {}
            received=int(x.get("drop_counter_delta") or 0)
            emitted=int(x.get("packets_counter_delta") or 0)
            if emitted<=0 or received<0: faults.append("TC_COUNTER_INVALID_"+name+"_"+d)
            ratio=100*received/(emitted+received) if emitted+received>0 else None
            if ratio is None or abs(ratio-p)>max(.45,p*.11):
                faults.append("ACTUAL_QDISC_LOSS_OUTSIDE_PREDECLARED_"+name+"_"+d)
            per_direction[d]={"netem_loss_measured_pct":ratio,
                              "netem_packets_counter_delta":emitted,
                              "netem_drops_counter_delta":received,
                              "wire_bytes_delta":x.get("wire_bytes_counter_delta")}
        found[name]=per_direction
    grades={}
    snapshots=summary.get("m3_stage_last60_grade") or {}
    for name,start,end,p,bounds in STAGES:
        grades[name]={}
        for d in ("c2s","s2c"):
            obs=(snapshots.get(name) or {}).get(d) or {}
            status=obs.get("status")
            rate=obs.get("fraction_within")
            count=obs.get("snapshots") or 0
            grades[name][d]={"status":status,"sample_count":count,
                "profile_set":obs.get("actual_r"),
                "frozen_allowed":list(bounds),"allowed_fraction":rate}
            if status!="OBSERVED" or count<45 or rate is None or rate<MIN_FINAL60_GRADE_FRACTION:
                faults.append("GRADE_FROZEN_LAST60_"+name+"_"+d)
    if not summary.get("m3_loss20_grade_screen_pass"):
        faults.append("HIGH20_GRADE_SCREEN_MISSING")
    if not summary.get("m3_final_recovery_grade_screen_pass"):
        faults.append("RECOVERY5_GRADE_SCREEN_MISSING")
    payload={}
    for d,src,dst in (("c2s",biz,target),("s2c",target,biz)):
        a,b=(src.get("stats") or {}),(dst.get("stats") or {})
        sent=a.get("sent_packets_by_second") or []
        unique=b.get("recv_wall_packets_by_second") or []
        if len(sent)<330 or len(unique)<330:
            faults.append("MISSING_PER_SECOND_SOURCE_FIRST_UNIQUE_"+d)
            continue
        n_sent=sum(sent[120:330])
        n_unique=sum(unique[120:330])
        n_ratio=n_unique/n_sent if n_sent else 0
        gap=longest_gap(unique[120:330],120)
        payload[d]={"recovery_active_s":[120,330],
            "source_sent":n_sent,"receiver_wall_first_unique":n_unique,
            "source_to_wall_unique_ratio":n_ratio,"longest_zero_first_unique_s":gap,
            "entire_run_sent":a.get("sent_packets"),
            "entire_run_first_unique":b.get("recv_unique_packets"),
            "entire_run_missing":max(0,int(a.get("sent_packets") or 0)-
                                       int(b.get("recv_unique_packets") or 0)),
            "receiver_oneway_p99_ns":b.get("oneway_p99_ns"),
            "receiver_corrupt":b.get("corrupt"),"recv_duplicates":b.get("recv_duplicates"),
            "source_send_failures":a.get("send_failures")}
        if n_sent==0 or n_ratio<RECOVERY_RECEIVE_RATIO_MIN:
            faults.append("RECOVERY_BUSINESS_FIRST_UNIQUE_RATIO_"+d)
        if gap>MAX_RECOVERY_NO_DELIVERY_SECONDS:
            faults.append("RECOVERY_FIRST_UNIQUE_BLACKOUT_"+d)
        if a.get("send_failures") or b.get("corrupt") or b.get("recv_duplicates"):
            faults.append("INTEGRITY_OR_SEND_FAILURE_"+d)
    return {"schema":1,"gate_version":GATE_VERSION,"source_sha":sha,
        "scope":"M3-R1_ONE_HIGH20_TO5_REALPATH_NOT_FULL_N2_OR_PHYSICAL",
        "classification":("SCOPED_M3_FROZEN_PROTECTION_RECOVERY_FIRST_UNIQUE_PASS"
                          if not faults else "M3_NOT_QUALIFIED_OR_FAILED"),
        "errors":faults,
        "netem_directions":found,"actual_fec_grade_last60":grades,
        "first_unique_recovery":payload,
        "frozen_5pct_stage_clock_seconds":[120,330],
        "independent_B_timeband_upper_from_recover5_start_s":freeze.get("ordinary_recovery_independent_B_upper_s"),
        "max_recovery_undelivered_seconds":MAX_RECOVERY_NO_DELIVERY_SECONDS,
        "min_recovery_first_unique_ratio":RECOVERY_RECEIVE_RATIO_MIN,
        "not_full_n2":True,"physical":"NOT_RUN",
        "limitations":["A grade win does not establish diagnostic-OFF CPU improvement",
                       "5pct stage observed tc loss does not identify exact physical source first-loss",
                       "M3-R1 is not the old C03/C04 waveform and doesn't retest 2^31 cumulative wrap",
                       "No GE correlated burst, reorder, feedback loss, sparse, mix TCP/UDP, Game qualification",
                       "Whole-run FEC and total wire amplification need separate evidence"]}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--artifact-dir",type=Path,required=True)
    p.add_argument("--source-sha",required=True)
    p.add_argument("--frozen-model",type=Path,required=True)
    p.add_argument("--output",type=Path,required=True)
    args=p.parse_args()
    try:
        def r(name):
            return json.loads((args.artifact_dir/name).read_text(encoding="utf8"))
        freeze=json.loads(args.frozen_model.read_text(encoding="utf8"))
        result=score(r("summary.json"),r("manifest.json"),r("biz.json"),r("target.json"),
                     freeze,args.source_sha)
    except Exception as exc:
        result={"schema":1,"gate_version":GATE_VERSION,"source_sha":args.source_sha,
                "classification":"M3_INVALID_ANALYZER","errors":[str(exc)],
                "not_full_n2":True}
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_M3_RECOVERY_"+result["classification"])
    if result["errors"]:
        raise SystemExit(1)

if __name__=="__main__":
    main()
