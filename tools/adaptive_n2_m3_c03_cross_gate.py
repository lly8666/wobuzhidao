#!/usr/bin/env python3
"""Read-only C03 cross-initial/runtimes M3 recovery screen, frozen BEFORE load.

The 120s recover5 phase is shorter than the independent B upper+60 stable
window (140.4+60); an unmet last60 grade screen is an efficiency limitation,
not proof that an impossible complete convergence guarantee was violated.
The original C03 780s full-wave longserial gate remains mandatory separately.
"""
import argparse
import json
from pathlib import Path

VERSION="n2-m3-c03-recover5-cross-screen-v1"
MIN_LAST60_SNAPSHOTS=45
MIN_ALLOWED_FRACTION=.90

def score(summary,manifest,prep,reference,source):
    faults=[]
    limitations=[]
    if summary.get("source_sha")!=source or manifest.get("source_sha")!=source:
        faults.append("BAD_SOURCE_IDENTITY")
    if summary.get("errors") or summary.get("correctness_errors") or summary.get("capacity_flags"):
        faults.append("SUMMARY_ERRORS_CORRUPTION_OR_CAPACITY")
    c03=next((x for x in reference.get("scenarios",[]) if x.get("id")=="C03"),None)
    if not c03 or reference.get("model_version")!="n2-independent-iid-timeband-v1":
        faults.append("MISSING_OR_CHANGED_C03_INDEPENDENT_REFERENCE")
    else:
        stage=next((x for x in c03["stages"] if x["name"]=="recover5"),None)
        if not stage or (stage["loss_percent"],stage["duration_s"],
                          stage["frozen_ideal_parity"],stage["frozen_grade_screen"],
                          stage["must_settle"])!=(5,120,8,[8,10],False):
            faults.append("C03_PREDECLARED_RECOVER5_GRADE_CHANGED")
    cfg=manifest.get("config") or {}
    for k,v in {"v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
                "fec_parity":20,"duration_s":780,
                "one_way_delay_ms_each_direction":300,
                "application_mbps_each_direction":10.0,
                "loss_percent":"waveform:0-1-5-20-5-0"}.items():
        if cfg.get(k)!=v:
            faults.append("CONFIG_MISMATCH_"+k)
    if (prep.get("c03_frozen") or {}).get("model_version")!="n2-independent-iid-timeband-v1":
        faults.append("ORIGINAL_FREEZE_RECEIPT_MISSING")
    grade=(summary.get("c03_stage_last60_grade") or {}).get("recover5") or {}
    ratios={}
    for direction in ("c2s","s2c"):
        a=grade.get(direction) or {}
        val=a.get("fraction_within")
        count=a.get("snapshots") or 0
        ratios[direction]={"last60_actual_grades":a.get("actual_r"),
                           "grade_screen_allowed":[8,10],
                           "actual_fraction_in_band":val,
                           "sample_count":count,"status":a.get("status")}
        if a.get("status")!="OBSERVED" or count<MIN_LAST60_SNAPSHOTS or val is None:
            faults.append("INSUFFICIENT_REAL_TX_DIAG_"+direction)
        elif val<MIN_ALLOWED_FRACTION:
            limitations.append(direction+"_RECOVER5_GRADE_SCREEN_UNMET")
    for stage_name,check_key in (
        ("loss20","c03_loss20_grade_screen_pass"),
        ("recover0","c03_final_recovery_grade_screen_pass")):
        if not summary.get(check_key):
            faults.append(stage_name+"_REFERENCE_PROTECTION_OR_RECOVERY_UNMET")
    # C03 120s recover5 is intentionally shorter than the independent
    # B upper 140.4 +60; return observational limitation, not "unphysical".
    return {"schema":1,"version":VERSION,"source_sha":source,
            "classification":("INVALID_C03_CROSS_SOURCE_OR_MEASUREMENT" if faults else
                 "C03_RECOVER5_GRADE_SCREEN_LIMITED" if limitations else
                 "C03_RECOVER5_SCOPED_GRADE_SCREEN_PASS"),
            "hard_failures":faults,"efficiency_screen_limitations":limitations,
            "grade_window":ratios,
            "frozen_recover5_active_interval":[480,600],
            "frozen_last60_interval":[540,600],
            "independent_B_upper_s":140.4,
            "B_upper_plus_60_exceeds_stage_length":True,
            "longserial_first_delivery_gate_separately_required":True,
            "no_product_truth_from_netem":True,"not_full_n2":True,
            "physical":"NOT_RUN"}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--artifact-dir",type=Path,required=True)
    p.add_argument("--source-sha",required=True)
    p.add_argument("--reference",type=Path,required=True)
    p.add_argument("--output",type=Path,required=True)
    x=p.parse_args()
    try:
        r=lambda name:json.loads((x.artifact_dir/name).read_text())
        reference=json.loads(x.reference.read_text())
        output=score(r("summary.json"),r("manifest.json"),r("harness-prep.json"),
                     reference,x.source_sha)
    except Exception as exc:
        output={"schema":1,"version":VERSION,"classification":"INVALID_C03_CROSS_ANALYZER",
                "source_sha":x.source_sha,"hard_failures":[str(exc)],
                "efficiency_screen_limitations":[],"not_full_n2":True}
    x.output.parent.mkdir(parents=True,exist_ok=True)
    x.output.write_text(json.dumps(output,indent=2,sort_keys=True)+"\n")
    print("WBD_M3_C03_CROSS_"+output["classification"])
    if output["hard_failures"] or output["efficiency_screen_limitations"]:
        raise SystemExit(1)

if __name__=="__main__":
    main()
