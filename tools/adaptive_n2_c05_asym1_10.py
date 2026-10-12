#!/usr/bin/env python3
"""C05: one 240s stable asymmetric C2S1%/S2C10% Normal-auto real-path test, not a matrix.

This derivative uses the existing five-netns protected formal binaries and
the exact production record path. The independently predeclared k20 IID
screens are C2S[R4,R8] and S2C[R8,R10], but actual partial k/shape uncertainty must remain named.
Diagnostic ON CPU is descriptive only; never netem-controlled production.
"""
import argparse
import hashlib
import json
from pathlib import Path

import adaptive_n2_clean300 as clean
from adaptive_n2_offstart_recover540 import resource_evidence, analyze_direction
from check_realpath_calibration import capture_drop

DURATION=240
DRAIN=10
FROZEN="docs/evidence/adaptive-n2-c05-c06-asym-model-freeze-v1.json"


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def exact(text,old,new,count=1):
    n=text.count(old)
    if n!=count:
        raise ValueError(f"C05 template anchor expected {count}, saw {n}: {old!r}")
    return text.replace(old,new)


def prepare(root,script,receipt):
    # Clean300 generator is the real protected five-netns source; never edit
    # the shared calibration template or silently change cipher/admission.
    clean.prepare(root,script,receipt)
    s=script.read_text()
    # Outbound C2S goes via router->server rsrv; outbound S2C via rcli.
    # Two fixed direction-local impairments in ONE unchanged configuration.
    s=exact(s,'tc qdisc add dev rsrv root netem limit 200000 delay 300ms',
              'tc qdisc add dev rsrv root netem limit 200000 delay 300ms loss random 1%')
    s=exact(s,'tc qdisc add dev rcli root netem limit 200000 delay 300ms',
              'tc qdisc add dev rcli root netem limit 200000 delay 300ms loss random 10%')
    s=exact(s,"--duration 100 --drain 10 --rate-mbps 10 --bounded-stats",
            "--duration 240 --drain 10 --rate-mbps 10 --bounded-stats",2)
    s=exact(s,'"duration_s": 100','"duration_s": 240')
    s=exact(s,'"loss_percent": 0','"loss_percent": "asym-c2s1-s2c10"')
    marker='printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"'
    s=exact(s,marker,marker+'\ndate +%s%N > "$ART/start-wall-ns.txt"')
    # Same bounded, read-only CPU/queue sampler already used by the 540s
    # real-path fixture. Do not add per-business-packet diagnostic overhead.
    sample = (
        '\npython3 "$GITHUB_WORKSPACE/tools/strict_resource_sampler.py" \\'
        '\n  --process "client=$CLIENT_PID" --process "server=$SERVER_PID" \\'
        '\n  --namespace "router=$RTR" --interval 5'
        ' --output "$ART/resource-samples.jsonl" \\'
        '\n  > "$ART/resources.log" 2>&1 &'
        '\nRESOURCE_PID="$!"'
    )
    s=exact(s,'date +%s%N > "$ART/start-wall-ns.txt"',
            'date +%s%N > "$ART/start-wall-ns.txt"'+sample)
    s=exact(s,'wait "$TGT_PID"\nTGT_PID=""',
            'wait "$TGT_PID"\nTGT_PID=""'
            '\nkill -TERM "$RESOURCE_PID" 2>/dev/null || true'
            '\nwait "$RESOURCE_PID" || true'
            '\nRESOURCE_PID=""')
    script.write_text(s)
    frozen=json.loads((root/FROZEN).read_text())
    if frozen["model_version"]!="n2-asymmetric-iid-timeband-v1":
        raise ValueError("C05 reference version changed")
    scenario=next(x for x in frozen["separate_performance_scenarios"] if x["id"]=="C05")
    if (scenario["loss_pct_by_direction"]!={"c2s":1,"s2c":10}
        or scenario["expected_full_k20_ideal_parity"]!={"c2s":4,"s2c":8}
        or scenario["frozen_allowed_last60"]!={"c2s":[4,8],"s2c":[8,10]}):
        raise ValueError("C05 frozen directional theory contract changed")
    receipt.write_text(json.dumps({
        "schema":1,"source_template":"scripts/realpath_calibration.sh",
        "template_sha256":digest(root/"scripts/realpath_calibration.sh"),
        "clean300_helper_sha256":digest(root/"tools/adaptive_n2_clean300.py"),
        "c05_helper_sha256":digest(root/"tools/adaptive_n2_c05_asym1_10.py"),
        "frozen_manifest_sha256":digest(root/FROZEN),
        "frozen_model_version":frozen["model_version"],
        "derived_script_sha256":digest(script),
        "scenario":"C05 240s fixed IID-ish tc netem C2S1%-S2C10%, Normal auto initial20 min4, two-way10Mbps 300ms each one-way, one unchanged config",
        "netem_rng":"KERNEL_RANDOM_UNSEEDED_TC_OPTION_UNSUPPORTED",
        "netem_seed_applied":False,"business_seeds":[2026092201,2026092202],
        "diagnostics":"ON, no CPU comparison to diagnostic OFF"
    },indent=2,sort_keys=True)+"\n")


def counter(path):
    arr=json.loads(path.read_text())
    val=next((a for a in arr if a.get("kind")=="netem"),None)
    if val is None:raise ValueError(f"missing netem qdisc in {path}")
    return {key:int(val.get(key,0)) for key in ("packets","drops","bytes","overlimits","requeues","backlog","qlen")}


def actual_loss(art,dev):
    a=counter(art/f"qdisc-{dev}-before.json")
    b=counter(art/f"qdisc-{dev}-after.json")
    packet_delta=b["packets"]-a["packets"]
    drop_delta=b["drops"]-a["drops"]
    if packet_delta<=0 or drop_delta<0:
        raise ValueError("invalid qdisc packet/drop difference "+dev)
    loss_pct=drop_delta/(packet_delta+drop_delta)*100
    return {"tx_packets":packet_delta,"netem_drops":drop_delta,
            "observed_drop_over_packets_plus_drops_pct":loss_pct,
            "wire_bytes":b["bytes"]-a["bytes"],
            "overlimits_delta":b["overlimits"]-a["overlimits"],
            "requeues_delta":b["requeues"]-a["requeues"],
            "post_backlog":b["backlog"],"post_queue_length":b["qlen"],
            "semantics":"qdisc netem actual injection (not source first-loss proof)"}


def grades(art,name,server,start_ns):
    rows=[]
    last_encoder={}
    for line in (art/name).read_text().splitlines():
        if not line.strip():continue
        ev=json.loads(line)
        prod=ev.get("product") or {}
        if server:
            prod=prod.get("tunnel") or ((prod.get("tunnels") or [{}])[0])
        for lane in ((prod or {}).get("lanes") or []):
            c=lane.get("control") or {}
            if not c.get("n2_controller_enabled"):continue
            last_encoder=(((lane.get("lane") or {}).get("TxPath") or {}).get("Encoder") or {})
            r=lane.get("actual_tx_parity")
            if r not in (0,4,8,10,12,16,20):
                raise ValueError("invalid actual TX profile "+str(r))
            rows.append({"second":round((ev["unix_ns"]-start_ns)/1e9,3),
                         "parity":r,"source_seq":c.get("n2_source_report_seq"),
                         "qualified_seq":c.get("n2_lower_assessed_source_seq"),
                         "lower_gate":c.get("n2_lower_gate"),
                         "reason":c.get("n2_decision_reason"),
                         "confidence_upper":c.get("n2_confidence_upper"),
                         "committed":c.get("n2_committed_switches",0),
                         "evidence":c.get("n2_source_evidence") or {}})
    active=[r for r in rows if 0<=r["second"]<DURATION]
    stable=[r for r in rows if DURATION-60<=r["second"]<DURATION]
    allowed=(8,10) if server else (4,8)
    protected=[r for r in stable if allowed[0]<=r["parity"]<=allowed[1]]
    # No drain minutes or idle traffic can improve the final60s denominator.
    eligible=len(stable)>=45
    within=len(protected)/len(stable) if stable else None
    return {"encoder_cumulative":last_encoder,
            "snapshots_total":len(rows),"active":len(active),
            "stable_active":len(stable),"final60_in_frozen_grade_range_fraction":within,
            "final60_grade_screen":list(allowed),"stable_window_adequate":eligible,
            "stable_meets_90pct_screen":bool(eligible and within>=.9),
            "active_profiles":sorted(set(r["parity"] for r in active)),
            "stable_profiles":sorted(set(r["parity"] for r in stable)),
            "grade_seconds_over_allowed_max":sum(max(0,(0,4,8,10,12,16,20).index(r["parity"])-(3 if server else 2)) for r in stable),
            "last_active":active[-1] if active else None,
            "first_active":active[0] if active else None,
            "last_qualified":next((r for r in reversed(active) if r["qualified_seq"]),None),
            "limitations":["one-second snapshots are a proxy for actual business-active seconds",
                           "M+R uncertain same-Seq repair, not real first-attempt PLR",
                           "actual k of cropped groups cannot be inferred as full20"],
    }


def check(art,source,output):
    errors=[]
    invalid=[]
    capacity=[]
    manifest=json.loads((art/"manifest.json").read_text())
    prep=json.loads((art/"harness-prep.json").read_text())
    cfg=manifest.get("config",{})
    if manifest.get("source_sha")!=source or manifest.get("harness_sha")!=source:
        invalid.append("build/harness SOURCE is not the run exact HEAD")
    for key,want in {
        "lanes":1,"fec_parity":20,"server_config_fec_parity":20,
        "client_record_cap_config":0,"server_record_cap_config":0,
        "v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
        "mtu":1400,"one_way_delay_ms_each_direction":300,
        "loss_percent":"asym-c2s1-s2c10","duration_s":240,
        "application_mbps_each_direction":10.0
    }.items():
        if cfg.get(key)!=want:invalid.append(f"wrong exact config {key}={cfg.get(key)!r}; want{want!r}")
    if digest(art/"harness-prep.json")=="":invalid.append("unreachable receipt")
    qdisc={}
    for dev,want in (("rsrv",1),("rcli",10)):
        try:
            qdisc[dev]=actual_loss(art,dev)
            actual=qdisc[dev]["observed_drop_over_packets_plus_drops_pct"]
            if abs(actual-want)>max(0.4,want*.10):
                invalid.append("tc actual directional loss not observed on "+dev)
            if qdisc[dev]["overlimits_delta"] or qdisc[dev]["requeues_delta"]:
                capacity.append("qdisc pressure on "+dev)
        except Exception as exc:
            invalid.append(str(exc))
    biz=json.loads((art/"biz.json").read_text())["stats"]
    tgt=json.loads((art/"target.json").read_text())["stats"]
    payload={}
    for direction,src,dst in (("c2s",biz,tgt),("s2c",tgt,biz)):
        sent=src["sent_packets"];unique=dst["recv_unique_packets"]
        received=dst["recv_unique_bytes"]
        payload[direction]={
            "sent":sent,"received_first_unique":unique,
            "missing":max(0,sent-unique),"sent_bytes":src["sent_bytes"],
            "first_unique_bytes":received,
            "goodput_mbps":received*8/(DURATION*1_000_000),
            "onway_p50_ns":dst.get("oneway_p50_ns"),
            "onway_p95_ns":dst.get("oneway_p95_ns"),
            "onway_p99_ns":dst.get("oneway_p99_ns"),
            "source_send_failures":src.get("send_failures"),
            "source_send_lag_p99_ns":src.get("send_lag_p99_ns"),
            "receiver_corrupt":dst.get("corrupt"),
            "receiver_duplicates":dst.get("recv_duplicates"),
            "duration_s":DURATION
        }
        if sent<=0 or unique<=0 or src.get("send_failures") or dst.get("corrupt") or dst.get("recv_duplicates"):
            errors.append(direction+" source/liveness/integrity violated")
        if src.get("send_lag_p99_ns") is None:
            invalid.append(direction+" source pacing health unknown")
        if sent<.96*DURATION*10_000_000/8/520:
            capacity.append(direction+" underinjected vs nominal")
        wall=dst.get("recv_wall_packets_by_second",[])[:DURATION]
        if wall and max((len(list(g)) for k,g in __import__('itertools').groupby(wall,lambda x:x==0) if k),default=0)>=5:
            capacity.append(direction+" >=5s continuous first-unique delivery gap")
    probes={"sent":biz.get("probe_sent",0),"returned":biz.get("probe_recv",0),
            "timeouts":biz.get("probe_timeouts"),"returned_only_rtt_p99_ns":biz.get("probe_rtt_p99_ns")}
    if not probes["sent"]:invalid.append("probe instrumentation missing")
    statuses=(art/"process-state-after-drain.txt").read_text()
    if "client_alive=1 " not in statuses or "server_alive=1 " not in statuses:
        errors.append("product process died before drain")
    captures={}
    pcap_hashes={}
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post"):
        captures[stem]={"kernel_capture_drops":capture_drop(art/f"{stem}.tcpdump.log")}
        if captures[stem]["kernel_capture_drops"] is None or captures[stem]["kernel_capture_drops"]>0:
            capacity.append(stem+" capture drops/UNKNOWN")
        path=art/f"{stem}.pcap"
        if path.is_file():
            pcap_hashes[stem]={"sha256":digest(path),"bytes":path.stat().st_size}
            path.unlink()
        else:
            invalid.append("missing owned raw capture "+stem)
    start_wall=int((art/"start-wall-ns.txt").read_text().strip())
    ctrls={}
    for direction,name,server in (("c2s","client-diag.jsonl",False),("s2c","server-diag.jsonl",True)):
        ctrls[direction]=grades(art,name,server,start_wall)
        if not ctrls[direction]["stable_window_adequate"]:
            invalid.append(direction+" incomplete active final60 diagnostic")
    delivered=sum(x["first_unique_bytes"] for x in payload.values())
    if not (art/"resource-samples.jsonl").is_file():
        invalid.append("resource sampler data missing; no CPU utilization qualification")
    cpu=resource_evidence(art/"resource-samples.jsonl",delivered,errors)
    protected=all(c["stable_meets_90pct_screen"] for c in ctrls.values())
    if not protected and not invalid:
        errors.append("C05 frozen last60 directional grade bands [4,8]/[8,10] failed >=90pct screen")
    classification=("INVALID_MEASUREMENT" if invalid else
       "CAPACITY_LIMITED" if capacity else
       "C05_SCOPED_GRADE_SCREEN_PASS_NOT_FULL_N2" if protected and not errors else
       "FAIL_N2_C05_ADAPTIVE_EFFICIENCY")
    total_qdisc_bytes=sum(x["wire_bytes"] for x in qdisc.values())
    total_business_sent_bytes=sum(x["sent_bytes"] for x in payload.values())
    gross_qdisc_amplification=(total_qdisc_bytes/total_business_sent_bytes
                              if total_business_sent_bytes else None)
    result={"schema":1,"source_sha":source,"scenario":"C05_300ms_each_way_10mbps_each_asym_1pct_10pct_240s",
            "model_version":prep["frozen_model_version"],"model_freeze_sha256":prep["frozen_manifest_sha256"],
            "classification":classification,"errors":errors,"measurement_invalid":invalid,
            "capacity_flags":capacity,"scoped_grade_screen_pass":protected and not errors and not invalid and not capacity,
            "not_full_n2_pass":True,"config":cfg,"helper":prep,
            "qdisc_actual_injection":qdisc,"business_first_unique":payload,
            "gross_qdisc_byte_amplification_over_generated_business":
                gross_qdisc_amplification,
            "probe":probes,"directional_controller":ctrls,
            "cpu_diagnostic_on":cpu,"captured_raw_pcap_deleted_after_hash":pcap_hashes,
            "capture_kernel_drop":captures,"limitations":[
                "stable grade model assumes full k20 IID only, actual cropped and partial k not exact",
                "no diagnostic OFF optimization proof; cross-run CPU comparisons invalid",
                "no per-record parity bytes yet; qdisc total wire bytes includes all outer traffic",
                "10ms noHOL and mixed TCP/UDP/MTU not qualified by single UDP generator",
                "initial weaknet missing and returned-only RTT p99 recorded, never fabricated zero",
                "N3-N6/physical and 2.73s/80s legacy remain OPEN"] }
    output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_N2_C05_"+classification)
    if invalid or errors or capacity:
        raise SystemExit(1)


def main():
    p=argparse.ArgumentParser()
    sub=p.add_subparsers(dest="cmd",required=True)
    x=sub.add_parser("prepare")
    x.add_argument("--root",type=Path,default=Path("."))
    x.add_argument("--script",type=Path,required=True)
    x.add_argument("--receipt",type=Path,required=True)
    x=sub.add_parser("check")
    x.add_argument("--artifact-dir",type=Path,required=True)
    x.add_argument("--source-sha",required=True)
    x.add_argument("--output",type=Path,required=True)
    a=p.parse_args()
    if a.cmd=="prepare":prepare(a.root,a.script,a.receipt)
    else:check(a.artifact_dir,a.source_sha,a.output)


if __name__=="__main__":
    main()
