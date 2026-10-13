#!/usr/bin/env python3
"""One predeclared five-netns Normal auto 1%/300ms true mixed TCP+UDP noHOL screen.
Same protected transport/auth/FEC as C08, but genuinely independent Linux
kernel TCP flows and UDP 10ms first-arrival telemetry. Not native Win TUN.
"""
import argparse,hashlib,json,statistics
from pathlib import Path
from adaptive_n2_c08_steady1 import actual_loss,grades,digest
from adaptive_n2_offstart_recover540 import resource_evidence
from check_realpath_calibration import capture_drop
import adaptive_n2_clean300 as clean

DURATION=120
def exact(s,a,b,n=1):
    if s.count(a)!=n:raise ValueError("mixed noHOL template drift: "+a[:80]+" matched "+str(s.count(a)))
    return s.replace(a,b)
def prepare(root,out,receipt):
    clean.prepare(root,out,receipt)
    s=out.read_text()
    s=exact(s,'GEN="$GITHUB_WORKSPACE/tools/realpath_udp_duplex.py"',
            'GEN="$GITHUB_WORKSPACE/tools/adaptive_n2_mixed_nohol_business.py"')
    s=exact(s,"netem limit 200000 delay 300ms","netem limit 200000 delay 300ms loss random 1%",2)
    s=exact(s,"--duration 100 --drain 10 --rate-mbps 10 --bounded-stats",
            "--duration-s 120 --workload mixed --rate-mbps 10 --size-profile ordinary --source \"$GITHUB_SHA\" --helper n2-mixed-hol-v1",2)
    s=exact(s,'--role target --bind 10.50.0.2:18080',
            '--role target --bind 10.50.0.2:18080 --peer 10.40.0.2:28080')
    s=exact(s,'"duration_s": 100','"duration_s": 120')
    # Unlike realpath_udp_duplex, mixed kernel TCP frames check a shared
    # directional seed at BOTH endpoints. The original unequal role seeds
    # made all legitimate TCP frames fail the business CRC check.
    s=exact(s,'--seed 2026092201','--seed 2026092202')
    # The target native TCP listener must stay alive during delayed TCP
    # connection establishment and application drain. Active load remains120s.
    s=exact(s,'"drain_s": 10','"drain_s": 20')
    s=exact(s,'"loss_percent": 0','"loss_percent": 1')
    s=exact(s,'printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"',
        'printf \'%s\\n\' "$START_NS" > "$ART/start-monotonic-ns.txt"'+
        '\ndate +%s%N > "$ART/start-wall-ns.txt"')
    sampler=('\npython3 "$GITHUB_WORKSPACE/tools/strict_resource_sampler.py" \\'
       '\n  --process "client=$CLIENT_PID" --process "server=$SERVER_PID" \\'
       '\n  --namespace "router=$RTR" --interval 5'
       ' --output "$ART/resource-samples.jsonl" \\'
       '\n  > "$ART/resources.log" 2>&1 &'
       '\nRESOURCE_PID="$!"')
    s=exact(s,'date +%s%N > "$ART/start-wall-ns.txt"',
            'date +%s%N > "$ART/start-wall-ns.txt"'+sampler)
    s=exact(s,'wait "$TGT_PID"\nTGT_PID=""',
            'wait "$TGT_PID"\nTGT_PID=""'
            '\nkill -TERM "$RESOURCE_PID" 2>/dev/null || true'
            '\nwait "$RESOURCE_PID" || true\nRESOURCE_PID=""')
    # The same one fixed 120s scenario uses each direction at 10Mbps total.
    s=exact(s,'"application_mbps_each_direction": 10.0',
       '"application_mbps_each_direction": 10.0,\n        "workload": "mixed-kernel-tcp-udp-fast10ms",\n        "fast_udp_spacing_ms": 10')
    out.write_text(s)
    receipt.write_text(json.dumps({"schema":1,"source":"derived-real-five-netns",
      "template_sha256":digest(root/"scripts/realpath_calibration.sh"),
      "clean_helper_sha256":digest(root/"tools/adaptive_n2_clean300.py"),
      "mixed_helper_sha256":digest(root/"tools/adaptive_n2_mixed_nohol_business.py"),
      "realpath_helper_sha256":digest(root/"tools/adaptive_n2_mixed_nohol_realpath.py"),
      "generated_script_sha256":digest(out),
      "scenario":"one 120s continuous 10Mbps each direction mixed kernel TCP+UDP, separate 10ms 96B UDP, 300ms each-way, 1pct tc random each direction, Normal auto initial20 floor4, diagnostics ON",
      "netem_rng":"UNSEEDED", "business_seeds_both_roles":[2026092202,2026092202], "drain_seconds":20, "performance_cases":1},indent=2,sort_keys=True)+"\n")

def percentile(values,p):
    if not values:return None
    vals=sorted(values)
    return vals[min(len(vals)-1,int((len(vals)-1)*p))]
def longest_zero(buckets,start,end):
    longest=0;current=0
    for b in range(start,end):
        if buckets.get(str(b),0)==0:
            current+=1;longest=max(longest,current)
        else:current=0
    return longest
def check(art,source,out):
    invalid=[];errors=[];capacity=[]
    manifest=json.loads((art/"manifest.json").read_text())
    cfg=manifest.get("config") or {}
    if manifest.get("source_sha")!=source or manifest.get("harness_sha")!=source:invalid.append("SOURCE mismatch")
    for key,want in {"duration_s":120,"workload":"mixed-kernel-tcp-udp-fast10ms",
      "loss_percent":1,"application_mbps_each_direction":10.0,
      "one_way_delay_ms_each_direction":300,"v3_client_fec_mode":"auto",
      "v3_client_fec_min_parity":4,"lanes":1,"mtu":1400}.items():
        if cfg.get(key)!=want:invalid.append(f"frozen config {key}={cfg.get(key)!r} expected {want!r}")
    q={}
    for name in ("rsrv","rcli"):
        try:
            row=actual_loss(art,name);q[name]=row
            if not .4<=row["observed_drop_over_packets_plus_drops_pct"]<=1.6:
                invalid.append(name+" netem injection outside nominal1pct")
            if row["overlimits_delta"] or row["requeues_delta"]:capacity.append(name+" link queue cap")
        except Exception as ex:invalid.append(str(ex))
    stats={role:json.loads((art/(role+".json")).read_text()) for role in ("biz","target")}
    for role,data in stats.items():
        if data.get("schema")!="wbd-large-mixed/v1" or data.get("source_sha")!=source or data.get("workload")!="mixed" or data.get("duration_seconds")!=120:
            invalid.append(role+" business source/mixed time mismatch")
    lanes={}
    for direction,src_role,dst_role in (("c2s","biz","target"),("s2c","target","biz")):
        a=stats[src_role]["counters"];b=stats[dst_role]["counters"]
        sent=a.get("fast_udp_sent",0);rec=b.get("fast_udp_unique_received",0)
        delays=b.get("fast_udp_oneway_ns",[])
        tcp_src=a.get("tcp_tx") or {};tcp_dst=b.get("tcp_rx") or {}
        short_tx=a.get("short_tcp_first_tx_ns") or {}
        short_rx=b.get("short_tcp_first_rx_ns") or {}
        short_delays=[short_rx[k]-ns for k,ns in short_tx.items()
                     if k in short_rx and short_rx[k]>=ns]
        row={"fast_sent":sent,"fast_first_unique_received":rec,"fast_missing":max(0,sent-rec),
             "fast_return_fraction":rec/max(1,sent),
             "fast_p50_oneway_ms":None if not delays else percentile(delays,.5)/1e6,
             "fast_p99_oneway_ms":None if not delays else percentile(delays,.99)/1e6,
             "fast_max_oneway_ms":None if not delays else max(delays)/1e6,
             "fast_max_consecutive_empty_10ms_bins_steady":longest_zero(
                  b.get("fast_udp_recv_10ms") or {},150,11800),
             "fast_source_schedule_miss_gt10ms":a.get("fast_udp_late_send_gt10ms",0),
             "fast_send_errors":a.get("fast_udp_send_errors",0),
             "tcp_connections_failed":a.get("tcp_connections_failed",0),
             "tcp_connect_attempts":a.get("tcp_connect_attempts",0),
             "long_tcp_frames_source":{k:v.get("frames") for k,v in tcp_src.items() if int(k)<1000},
             "long_tcp_frames_received":{k:v.get("frames") for k,v in tcp_dst.items() if int(k)<1000},
             "short_tcp_sent_first":len(short_tx),
             "short_tcp_received_first":len(short_delays),
             "short_tcp_first_payload_p99_ms":None if not short_delays else percentile(short_delays,.99)/1e6,
             "tcp_rx_corrupt":b.get("tcp_rx_corrupt"),"tcp_rx_seq_errors":b.get("tcp_rx_seq_errors"),
             "short_tcp_recorded_first_both_ways":True,
             "other_bulk_udp_unique":b.get("unique_udp_received",0)}
        lanes[direction]=row
        if sent<10000 or rec<=0 or len(delays)<rec:
            invalid.append(direction+" independent 10ms source/first arrivals missing")
        if rec/sent<.96 if sent>0 else True:
            errors.append(direction+" independent fast UDP first-unique below96pct")
        if row["fast_p99_oneway_ms"] is None or row["fast_p99_oneway_ms"]>900:
            errors.append(direction+" fast-UDP one-way p99 over900ms")
        if row["fast_max_consecutive_empty_10ms_bins_steady"]>30:
            errors.append(direction+" >300ms fast UDP aggregate 10ms gap")
        if row["short_tcp_sent_first"]<30 or row["short_tcp_received_first"]<25:
            errors.append(direction+" genuine short TCP parallel streams insufficient")
        if row["tcp_rx_corrupt"] or row["tcp_rx_seq_errors"]:
            errors.append(direction+" TCP corrupt/sequence failure")
        if row["fast_send_errors"] or row["fast_source_schedule_miss_gt10ms"]>sent*.03:
            capacity.append(direction+" fast source injector late/failing")
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post"):
        try:
            n=capture_drop(art/(stem+".tcpdump.log"))
            if n is None or n>0:capacity.append(stem+" kernel capture dropped "+str(n))
            p=art/(stem+".pcap")
            if not p.is_file():invalid.append("raw capture absent "+stem)
            else:p.unlink()
        except Exception as ex:invalid.append(str(ex))
    process=(art/"process-state-after-drain.txt").read_text()
    if "client_alive=1 " not in process or "server_alive=1 " not in process:
        errors.append("protected actual client/server died")
    resources={}
    if not (art/"resource-samples.jsonl").is_file():
        invalid.append("read-only resource samples absent")
    else:
        total_delivered=sum(x["counters"].get("received_bytes",0) for x in stats.values())
        resources=resource_evidence(art/"resource-samples.jsonl",total_delivered,errors)
    status=("INVALID_MEASUREMENT" if invalid else "CAPACITY_LIMITED" if capacity else
            "FAIL_INDEPENDENT_10MS_TCP_UDP_NOHOL_SCREEN" if errors else
            "SCOPED_MIXED_TCP_UDP_10MS_NOHOL_SCREEN_PASS_NOT_FULL_N2")
    obj={"schema":1,"classification":status,"source_sha":source,
       "frozen_case":"one exact 120s 300ms/way 1pct both netem, Normal auto, mixed true kernel TCP+UDP flows, fast UDP every 10ms",
       "directional":lanes,"actual_netem":q,"resources":resources,
       "invalid":invalid,"errors":errors,"capacity":capacity,
       "not_proven":["native Windows Wintun/TCP direct","all-flow per-packet latency","C10 GE p99","N2 C04/C05 grade FAIL","CPU OFF benchmark","physical"],"not_full_N2":True}
    out.write_text(json.dumps(obj,indent=2,sort_keys=True)+"\n")
    print("WBD_N2_MIXED_NOHOL_"+status)
    if status!="SCOPED_MIXED_TCP_UDP_10MS_NOHOL_SCREEN_PASS_NOT_FULL_N2":raise SystemExit(1)
    return obj

if __name__=="__main__":
    p=argparse.ArgumentParser()
    p.add_argument("mode",choices=["prepare","verify"])
    p.add_argument("--root",type=Path,default=Path("."))
    p.add_argument("--script",type=Path)
    p.add_argument("--receipt",type=Path)
    p.add_argument("--artifact-dir",type=Path)
    p.add_argument("--source-sha")
    p.add_argument("--output",type=Path)
    a=p.parse_args()
    if a.mode=="prepare":prepare(a.root,a.script,a.receipt)
    else:check(a.artifact_dir,a.source_sha,a.output)
