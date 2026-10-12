#!/usr/bin/env python3
"""C05/C06 asymmetric IID normal-FEC independent theory (NO product imports).

Only test reference mathematics. Neither this manifest, actual netem damage
nor stage labels are accessible to production controller.
"""
from __future__ import annotations
import argparse,json
from pathlib import Path
from adaptive_n2_curve_reference import ideal,time_band

VERSION="n2-asymmetric-iid-timeband-v1"
PROFILE_SET=(0,4,8,10,12,16,20)
DIRECTIONS=("c2s","s2c")

def evaluate(manifest:dict)->dict:
    if manifest.get("model_version")!=VERSION:
        raise ValueError("wrong independent asym model version")
    common=manifest["scenario_common"]
    if (common["initial_parity"],common["min_parity"],common["max_parity"],
        common["directional_oneway_ms"],common["directional_application_mbps"],
        common["outer_mtu"],common["active_s"])!=(20,4,20,300,10,1400,240):
        raise ValueError("frozen common product configuration changed")
    m=manifest["model_settings"]
    if (m["history_windows_enabled"],m["off_proof_windows"],
        m["min_records"],m["pn_window"])!=(16,32,512,256):
        raise ValueError("expected M3 qualified source design changed")
    shapes=[{"k":20,"count":100}]
    kwargs={k:m[k] for k in
        ("report_s","maturity_s","effective_reports_per_s","min_records",
         "pn_window","fast_tau_s","slow_tau_s","stable_s",
         "off_stable_s","cooldown_s")}
    kwargs["history_windows"]=m["history_windows_enabled"]
    if len(manifest["separate_performance_scenarios"])!=2:
        raise ValueError("only C05 and C06 explicitly frozen")
    findings={}
    for scenario in manifest["separate_performance_scenarios"]:
        sid=scenario["id"]
        if sid in findings or sid not in ("C05","C06"):
            raise ValueError("unexpected or duplicate asym scenario")
        expected_p={"C05":{"c2s":1,"s2c":10},
                    "C06":{"c2s":10,"s2c":1}}[sid]
        if scenario["loss_pct_by_direction"]!=expected_p:
            raise ValueError("frozen loss direction assignment drift")
        directions={}
        for direction in DIRECTIONS:
            pct=expected_p[direction]
            prediction=ideal(shapes,pct/100,4,20)
            proposed=prediction["ideal_parity"]
            if proposed!=scenario["expected_full_k20_ideal_parity"][direction]:
                raise ValueError(f"frozen target profile mismatch {sid}/{direction}")
            band=scenario["frozen_allowed_last60"][direction]
            expected_band=(4,[4,8]) if pct==1 else (8,[8,10])
            if (proposed,band)!=expected_band:
                raise ValueError(f"frozen observed normal grade band mismatch {sid}/{direction}")
            budget=time_band(rtt_s=m["rtt_s"],from_r=20,to_r=proposed,**kwargs)
            if budget["latest_screen_s"]+60>common["active_s"]:
                raise ValueError("frozen steady stage shorter than B latest+60")
            directions[direction]={"line_loss_percent":pct,
                                   "full_k20_ideal_parity":proposed,
                                   "target_unattainable":prediction["target_unattainable"],
                                   "iid_source_residual_by_profile":prediction["residual_by_parity"],
                                   "frozen_last60_allowed_profile":band,
                                   "independent_B_timeband":budget}
        findings[sid]={"separate_one_config_one_scenario_one_action":True,
                       "directions":directions,
                       "business_active_s":common["active_s"],
                       "last60_business_active_s":[180,240]}
    return {"schema":1,"model_version":VERSION,"results":findings,
            "measurement_status":"MODEL_ONLY_NO_REAL_NETEM_ACTION",
            "product_controller_imported":False,
            "limitations":["only full-k20 IID for now; actual partial/cropped M-R and correlated loss separate",
                           "effective source report rate is a predeclared conservative design estimate",
                           "diagnostic ON CPUs and unseeded tc cannot be used for cross-run CPU proof"]}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--manifest",type=Path,required=True)
    p.add_argument("--output",type=Path,required=True)
    a=p.parse_args()
    out=evaluate(json.loads(a.manifest.read_text()))
    a.output.parent.mkdir(parents=True,exist_ok=True)
    a.output.write_text(json.dumps(out,indent=2,sort_keys=True)+"\n")
if __name__=="__main__":
    main()
