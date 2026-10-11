#!/usr/bin/env python3
"""One N2 protected Normal auto 360s, 300ms one-way 0/1/5/20/5/0 wave.

One source, one fixed policy, one generator seed pair, one real netns scenario
with six predeclared >=60s damage phases. This is not an A/B or an automatic
PASS: invalid injection, capacity, business correctness, actual local TX grade
and post-recovery first-unique are separate classifications. No pcap upload.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
from check_realpath_calibration import capture_drop

PHASES=(("clean0",0,0),("loss1",60,1),("loss5",120,5),
        ("loss20",180,20),("recover5",240,5),("recover0",300,0))
DURATION=360
DRAIN=10
SAMPLE_RATE=10.0

def digest(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()

def exact(s,a,b):
    if s.count(a)!=1:
        raise ValueError(f"wave template anchor {a[:70]!r} appeared {s.count(a)} times; fail closed")
    return s.replace(a,b)

def prepare(root,script,receipt):
    template=root/"scripts/realpath_calibration.sh"
    s=template.read_text(encoding="utf-8")
    s=exact(s,"--client-record-limit 1300 --mtu 1400 --fec-parity 20 --lanes 1",
        '--client-record-limit 0 --mtu 1400 --fec-mode auto --fec-min-parity 4 --lanes 1 --diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 1s')
    s=exact(s,"--server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 1",
        '--server-record-limit 0 --mtu 1400 --fec-parity 20 --lanes 1 --diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s')
    if s.count("--duration 8 --drain 10 --rate-mbps 0.5")!=2:
        raise ValueError("wave requires exactly two real simultaneous UDP directions")
    s=s.replace("--duration 8 --drain 10 --rate-mbps 0.5",
                "--duration 360 --drain 10 --rate-mbps 10 --bounded-stats")
    s=exact(s,'"fec_parity": 20,',
        '"fec_parity": 20,\n        "server_config_fec_parity": 20,\n'
        '        "client_record_cap_config": 0,\n'
        '        "server_record_cap_config": 0,\n'
        '        "v3_client_fec_mode": "auto",\n'
        '        "v3_client_fec_min_parity": 4,\n'
        '        "v3_client_record_cipher": "chacha20-poly1305",')
    s=exact(s,'"application_mbps_each_direction": 0.5',
            '"application_mbps_each_direction": 10.0')
    s=exact(s,'"duration_s": 8','"duration_s": 360')
    s=exact(s,'"loss_percent": 0','"loss_percent": "waveform:0-1-5-20-5-0"')
    s=exact(s,'CAP_PIDS=()\n\ncleanup() {',
      'CAP_PIDS=()\nSTAGE_PID=""\nRESOURCE_PID=""\n\ncleanup() {')
    s=exact(s,'for pid in "$BIZ_PID" "$TGT_PID" "$CLIENT_PID" "$SERVER_PID"; do',
       'for pid in "$BIZ_PID" "$TGT_PID" "$CLIENT_PID" "$SERVER_PID" "$STAGE_PID" "$RESOURCE_PID"; do')
    stage='''
python3 "$GITHUB_WORKSPACE/tools/adaptive_n2_loss_wave_stage.py" \\
  --namespace "$RTR" --c2s-dev rsrv --s2c-dev rcli \\
  --seed 601 --start-ns "$START_NS" --output "$ART/stage-events.jsonl" \\
  > "$ART/stage.log" 2>&1 &
STAGE_PID="$!"
python3 "$GITHUB_WORKSPACE/tools/strict_resource_sampler.py" \\
  --process "client=$CLIENT_PID" --process "server=$SERVER_PID" \\
  --namespace "router=$RTR" --interval 5 --output "$ART/resource-samples.jsonl" \\
  > "$ART/resources.log" 2>&1 &
RESOURCE_PID="$!"
'''
    s=exact(s,'printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"',
              'printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"\n'+stage)
    s=exact(s,'wait "$TGT_PID"\nTGT_PID=""',
           'wait "$TGT_PID"\nTGT_PID=""\nwait "$STAGE_PID"\nSTAGE_PID=""\nkill -TERM "$RESOURCE_PID" 2>/dev/null || true\nwait "$RESOURCE_PID" || true\nRESOURCE_PID=""')
    script.parent.mkdir(parents=True,exist_ok=True)
    script.write_text(s,encoding="utf-8")
    receipt.write_text(json.dumps({
        "schema":1,"source_template":"scripts/realpath_calibration.sh",
        "template_sha256":digest(template),"derived_script_sha256":digest(script),
        "wave_stage_file_sha256":digest(root/"tools/adaptive_n2_loss_wave_stage.py"),
        "analyzer_file_sha256":digest(root/"tools/adaptive_n2_wave300.py"),
        "waveform":[{"name":a,"start_s":b,"end_s":b+60,"loss_percent":c}
                    for a,b,c in PHASES],
        "harness_loss_seed":601,"business_seeds":[2026092201,2026092202],
        "configuration":"Normal auto min4 initial/max20, each-way10Mbps, 300ms one-way, default auto record cap0, outer MTU1400, diagnostics ON",
        "classification":"single predeclared 360s impairment scenario, not multiple configurations",
    },sort_keys=True,indent=2)+"\n",encoding="utf-8")

def netem(snapshot):
    for x in snapshot:
        if x.get("kind")=="netem":
            return x
    return None

def phase_injection(path,errors):
    if not path.is_file():
        errors.append("wave stage events missing")
        return [],{}
    try:
        events=[json.loads(x) for x in path.read_text().splitlines() if x.strip()]
    except (ValueError,OSError) as e:
        errors.append("invalid wave stage event file: "+str(e))
        return [],{}
    if any(x.get("event")=="harness_error" for x in events):
        errors.append("tc waveform stage raised an error")
    phases=[]
    for name,offset,want in PHASES:
        begin=next((x for x in events if x.get("event")=="phase_start" and x.get("name")==name),None)
        end=next((x for x in events if x.get("event")=="phase_end" and x.get("name")==name),None)
        if begin is None or end is None:
            errors.append("stage start/end missing: "+name)
            continue
        if begin.get("offset_seconds")!=offset or end.get("offset_seconds")!=offset+60 or begin.get("loss_percent")!=want or end.get("loss_percent")!=want:
            errors.append("stage time/loss contradiction: "+name)
        if abs(begin.get("clock_delta_from_target_ns",10**18))>2_000_000_000 or abs(end.get("clock_delta_from_target_ns",10**18))>2_000_000_000:
            errors.append("stage scheduler delayed >2s: "+name)
        stage={"name":name,"start_s":offset,"end_s":offset+60,"expected_loss_percent":want,
               "observed_start_wall_ns":begin.get("time_wall_ns"),
               "observed_end_wall_ns":end.get("time_wall_ns")}
        for direction in ("c2s","s2c"):
            x=netem(begin[direction]);y=netem(end[direction])
            if x is None or y is None:
                errors.append("missing netem qdisc counter "+direction+" "+name)
                continue
            packets=int(y.get("packets",0))-int(x.get("packets",0))
            drops=int(y.get("drops",0))-int(x.get("drops",0))
            if packets<0 or drops<0 or packets+drops<=0:
                errors.append("invalid qdisc delta "+direction+" "+name)
                continue
            approx=100*drops/(packets+drops)
            stage[direction]={"packets_counter_delta":packets,"drop_counter_delta":drops,
                              "observed_drop_over_packets_plus_drops_pct":approx,
                              "qdisc_counter_semantics":"netem packets and drops; actual injection witness, not inner PLR"}
            tolerance={0:0.75,1:1.5,5:2.5,20:5.0}[want]
            if abs(approx-want)>tolerance:
                errors.append(f"{direction}/{name} injection outside {want}% ±{tolerance}pp: {approx:.3f}%")
        phases.append(stage)
    if len(phases)!=len(PHASES):
        errors.append("not all six stages observed")
    return phases,{"events":len(events),"stage_names":[x["name"] for x in phases]}

def analyze_direction(src,dst):
    sent=src["sent_packets"]
    received=dst["recv_unique_packets"]
    rows=[]
    ss=src.get("sent_packets_by_second",[])
    rr=dst.get("recv_packets_by_second",[])
    for name,offset,want in PHASES:
        emitted=sum(ss[offset:offset+60])
        unique=sum(rr[offset:offset+60])
        rows.append({"stage":name,"expected_outer_loss_percent":want,
                     "sent":emitted,"first_unique":unique,
                     "missing":max(0,emitted-unique),
                     "first_unique_ratio":(unique/emitted if emitted else None)})
    wall=dst.get("recv_wall_packets_by_second",[])
    zeros=0;longest=0
    for count in wall[:DURATION]:
        if count==0:zeros+=1;longest=max(longest,zeros)
        else:zeros=0
    return {"sent":sent,"received_first_unique":received,"sent_bytes":src["sent_bytes"],
            "received_first_unique_bytes":dst["recv_unique_bytes"],"missing":max(0,sent-received),
            "source_send_failures":src.get("send_failures"),"receiver_corrupt":dst.get("corrupt"),
            "receiver_duplicates":dst.get("recv_duplicates"),"source_send_lag_p99_ns":src.get("send_lag_p99_ns"),
            "receive_p99_ns_returned_only":dst.get("oneway_p99_ns"),
            "maximum_wall_seconds_without_delivery":longest,"stages":rows}

def parity_trace(path,server,stages,errors):
    if not path.is_file():
        errors.append("missing local-TX grade diagnostic "+path.name);return {}
    try:
        events=[json.loads(s) for s in path.read_text().splitlines() if s.strip()]
    except (ValueError,OSError) as e:
        errors.append("bad grade diagnostic "+path.name+" "+str(e));return {}
    rows=[]
    for event in events:
        product=event.get("product") or {}
        if server:
            product=product.get("tunnel") if product.get("present") else ((product.get("tunnels") or [{}])[0])
            product=product or {}
        for lane in product.get("lanes",[]):
            ctl=lane.get("control") or {}
            if ctl.get("n2_controller_enabled"):
                rows.append({"t":event.get("unix_ns"),"r":lane.get("actual_tx_parity"),
                             "commits":ctl.get("n2_committed_switches",0),
                             "ordinary":ctl.get("n2_ordinary"),
                             "advised":ctl.get("n2_advised_parity"),
                             "reason":ctl.get("n2_decision_reason",""),
                             "version":ctl.get("n2_decision_version",0),
                             "source_seq":ctl.get("n2_source_report_seq",0),
                             "generation":ctl.get("n2_source_generation",0),
                             "bound_width":ctl.get("n2_first_loss_bound_width",None)})
    if not rows or not any(x["version"]>0 for x in rows):
        errors.append("no real authenticated N2 source controller snapshots: "+path.name)
    bins=[]
    for stage in stages:
        a=stage.get("observed_start_wall_ns")
        b=stage.get("observed_end_wall_ns")
        batch=[x for x in rows if isinstance(x["t"],int) and a is not None and b is not None and a<=x["t"]<b]
        bins.append({"stage":stage["name"],"samples":len(batch),
                     "actual_tx_profiles":sorted(set(x["r"] for x in batch if x["r"] is not None)),
                     "reasons":{reason:sum(x["reason"]==reason for x in batch)
                                for reason in sorted(set(x["reason"] for x in batch))},
                     "first_commit":batch[0]["commits"] if batch else None,
                     "last_commit":batch[-1]["commits"] if batch else None})
    return {"samples":len(rows),"initial_actual":rows[0]["r"] if rows else None,
            "final_actual":rows[-1]["r"] if rows else None,
            "observed_profiles":sorted(set(x["r"] for x in rows if x["r"] is not None)),
            "max_committed_switches":max((x["commits"] for x in rows),default=0),
            "last_source_seq":max((x["source_seq"] for x in rows),default=0),
            "stages":bins,"first_and_last_samples":rows[:4]+rows[-6:]}

def resource_evidence(path,delivered,errors):
    if not path.is_file():
        errors.append("resource sampling not available");return {}
    try:
        rows=[json.loads(s) for s in path.read_text().splitlines() if s.strip()]
    except (ValueError,OSError) as e:
        errors.append("resource sample JSONL unreadable: "+str(e));return {}
    if len(rows)<2:
        errors.append("fewer than two resource samples")
        return {"samples":len(rows)}
    hertz=os.sysconf("SC_CLK_TCK")
    usage={}
    for kind in ("client","server"):
        observed=[]
        for row in rows:
            stat=((row.get("processes") or {}).get(kind) or {}).get("stat")
            if stat and "utime_ticks" in stat and "stime_ticks" in stat:
                observed.append(stat["utime_ticks"]+stat["stime_ticks"])
        if len(observed)<2 or observed[-1]<observed[0]:
            usage[kind]={"status":"UNAVAILABLE"}
        else:
            seconds=(observed[-1]-observed[0])/hertz
            usage[kind]={"status":"OBSERVED_DIAGNOSTIC_ON","cpu_s":seconds,
                         "cpu_s_per_first_unique_gib_all_directions":(seconds/(delivered/(1024**3)) if delivered else None)}
    return {"samples":len(rows),"clock_ticks_per_second":hertz,"products":usage,
            "notice":"diagnostic ON; do not compare CPU seconds across different CPUs or OFF runs"}

def check(art,source,output):
    errors=[]
    correctness=[]
    capacity=[]
    payload={}
    try:
        manifest=json.loads((art/"manifest.json").read_text())
        prep=json.loads((art/"harness-prep.json").read_text())
        cfg=manifest.get("config") or {}
        for key,expect in {
            "lanes":1,"fec_parity":20,"server_config_fec_parity":20,
            "client_record_cap_config":0,"server_record_cap_config":0,
            "v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
            "mtu":1400,"one_way_delay_ms_each_direction":300,
            "duration_s":360,"application_mbps_each_direction":10.0,
            "loss_percent":"waveform:0-1-5-20-5-0"
        }.items():
            if cfg.get(key)!=expect:errors.append(f"{key}: expected {expect!r}; measured {cfg.get(key)!r}")
        if manifest.get("source_sha")!=source or manifest.get("harness_sha")!=source:
            errors.append("wrong SOURCE identity")
        if not prep.get("template_sha256") or not prep.get("derived_script_sha256"):
            errors.append("missing helper/template provenance")
        stage,stage_receipt=phase_injection(art/"stage-events.jsonl",errors)
        biz=json.loads((art/"biz.json").read_text())["stats"]
        tgt=json.loads((art/"target.json").read_text())["stats"]
        payload={"c2s":analyze_direction(biz,tgt),"s2c":analyze_direction(tgt,biz)}
        for direction,details in payload.items():
            if details["source_send_failures"] or details["receiver_corrupt"] or details["receiver_duplicates"]:
                correctness.append(f"{direction}: corrupted/duplicate/local source send failure")
            if details["sent"]==0 or details["received_first_unique"]==0:
                correctness.append(f"{direction}: no independent first-unique delivery")
            if details["sent"]<0.98*360*10_000_000/8/520:
                capacity.append(f"{direction}: below source injection expectation")
            if details["maximum_wall_seconds_without_delivery"]>=5:
                capacity.append(f"{direction}: >=5s continuous no-first-delivery interval")
        probes={"sent":biz.get("probe_sent",0),"returned":biz.get("probe_recv",0),
                "missing":biz.get("probe_timeouts"),"returned_only_rtt_p99_ns":biz.get("probe_rtt_p99_ns"),
                "rtt_by_second":biz.get("probe_rtt_ns_by_second",[])[:DURATION]}
        if not probes["sent"]:errors.append("missing probe instrument")
        if "client_alive=1 " not in (art/"process-state-after-drain.txt").read_text() or "server_alive=1 " not in (art/"process-state-after-drain.txt").read_text():
            correctness.append("protected product exited before drain")
        cap={}
        for name in ("c2s-pre","c2s-post","s2c-pre","s2c-post"):
            drop=capture_drop(art/(name+".tcpdump.log"))
            cap[name]={"capture_kernel_drops":drop}
            if drop is None or drop>0:
                capacity.append(f"capture {name}: kernel dropped or unaccounted")
        ctrl={"c2s":parity_trace(art/"client-diag.jsonl",False,stage,errors),
              "s2c":parity_trace(art/"server-diag.jsonl",True,stage,errors)}
        delivered=sum(int(x["received_first_unique_bytes"]) for x in payload.values())
        resources=resource_evidence(art/"resource-samples.jsonl",delivered,errors)
        # Distinct staged evidence: rising while damage worsens and falling
        # only after recovery, not a token observed at some unrelated time.
        damaged={"loss1","loss5","loss20"}
        recovering={"recover5","recover0"}
        observed_rise=all(any("RAISE" in row.get("reasons",{}) for row in ctrl[d].get("stages",[])
                              if row.get("stage") in damaged) for d in ("c2s","s2c"))
        observed_down=all(any("LOWER_ONE" in row.get("reasons",{}) for row in ctrl[d].get("stages",[])
                              if row.get("stage") in recovering) for d in ("c2s","s2c"))
        both_applied=all(ctrl[d].get("max_committed_switches",0)>=2 and
                         len(ctrl[d].get("observed_profiles",[]))>=2 for d in ("c2s","s2c"))
        # Never classify unverified/noisy source evidence as a production PASS.
        if errors:verdict="INVALID_OR_INSUFFICIENT_MEASUREMENT"
        elif correctness:verdict="FAIL_CORRECTNESS"
        elif capacity:verdict="CAPACITY_LIMITED_REVIEW"
        elif observed_rise and observed_down and both_applied:verdict="SCOPED_WAVEFORM_CONTROL_OBSERVED_REVIEW_PERFORMANCE"
        else:verdict="MEASURED_NOT_QUALIFIED_CONTROLLER_WAVEFORM"
        result={"schema":1,"source_sha":source,"classification":verdict,
                "not_full_n2_pass":True,"errors":errors,"correctness_errors":correctness,
                "capacity_flags":capacity,"waveform":stage,"stage_receipt":stage_receipt,
                "input_and_unique_delivery":payload,"probe":probes,"capture":cap,
                "actual_directional_control":ctrl,"resource":resources,
                "harness":prep,"configuration":cfg,
                "limitations":["one 360s Normal auto waveform and runner; requires independent aggressive/lowRTT/asymmetric/sparse/mixed/Game4 gates",
                               "high loss may legitimately leave first-unique gaps; no automatic all-probe-zero-loss assumption",
                               "diagnostic ON resource samples not comparable with OFF or other CPU model",
                               "qdisc observed drop fraction is network injected stress, not exact physical first-attempt PLR"]}
    except Exception as exc:
        result={"schema":1,"source_sha":source,"classification":"INVALID_OR_INSUFFICIENT_MEASUREMENT",
                "errors":errors+["analyzer exception: "+str(exc)],"correctness_errors":correctness,
                "capacity_flags":capacity,"not_full_n2_pass":True}
    # The four pcap files remain ephemeral. Preserve SHA256 and size, then
    # delete owned raw capture even if a prior analyzer gate failed.
    digests={}
    for p in art.glob("*.pcap"):
        digests[p.name]={"size_bytes":p.stat().st_size,"sha256":digest(p)}
        p.unlink()
    result["deleted_owned_pcaps"]=digests
    output.parent.mkdir(parents=True,exist_ok=True)
    output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_ADAPTIVE_N2_WAVE300_"+result["classification"])
    if result["classification"] in ("INVALID_OR_INSUFFICIENT_MEASUREMENT","FAIL_CORRECTNESS"):
        raise SystemExit(1)

def main():
    ap=argparse.ArgumentParser()
    cmd=ap.add_subparsers(dest="command",required=True)
    x=cmd.add_parser("prepare")
    x.add_argument("--root",type=Path,default=Path("."))
    x.add_argument("--script",type=Path,required=True)
    x.add_argument("--receipt",type=Path,required=True)
    y=cmd.add_parser("check")
    y.add_argument("--artifact-dir",type=Path,required=True)
    y.add_argument("--source-sha",required=True)
    y.add_argument("--output",type=Path,required=True)
    args=ap.parse_args()
    if args.command=="prepare":prepare(args.root,args.script,args.receipt)
    else:check(args.artifact_dir,args.source_sha,args.output)

if __name__=="__main__":
    main()
