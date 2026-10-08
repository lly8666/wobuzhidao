#!/usr/bin/env python3
"""One-sample A real-socket Actions validation (not physical or TCP)."""
import argparse
import json
import re
from pathlib import Path
from longmix_profile import PRODUCT_SOURCE, A_UDP_SHARE, validate_sample

def read(path):
    return json.loads(path.read_text())

def netem_stats(rows):
    matches=[x for x in rows if x.get("kind")=="netem"]
    if len(matches)!=1:
        raise ValueError("must have exactly one netem qdisc")
    q=matches[0]
    return int(q["packets"]),int(q["drops"])

def validate(art, sample):
    validate_sample(sample)
    if sample["workload"]!="A":raise ValueError("not A")
    manifest=read(art/"manifest.json")
    if manifest["source_sha"]!=PRODUCT_SOURCE:raise ValueError("source mismatch")
    if not re.fullmatch(r"[0-9a-f]{40}",manifest.get("helper_sha","")):
        raise ValueError("helper mismatch")
    if manifest["workload"]!="A" or manifest["loss_percent"]!=sample["loss_percent"]:
        raise ValueError("scenario mismatch")
    if manifest["seed"]!=sample["seed"] or manifest["config"]["duration_s"]!=300:
        raise ValueError("seed/time mismatch")
    for name,mtu in (("server-tun-active.txt",9000),
                     ("biz-mtu-active.txt",9000),
                     ("client-tproxy-ingress-mtu-active.txt",9000),
                     ("target-mtu-active.txt",9000),
                     ("router-rcli-mtu-active.txt",1400),
                     ("router-rsrv-mtu-active.txt",1400)):
        t=(art/name).read_text()
        if re.search(r"\bmtu\s+"+str(mtu)+r"\b",t) is None:
            raise ValueError("not actual MTU "+name)
    if "10.50.0.2" not in (art/"biz-target-route.txt").read_text():
        raise ValueError("wrong private target")
    if "wbd_tproxy" not in (art/"client-tproxy-active.txt").read_text():
        raise ValueError("TPROXY bypass/missing")
    stages=[json.loads(s) for s in (art/"stage-events.jsonl").read_text().splitlines()]
    a=[s for s in stages if s.get("event")=="stress_start"]
    z=[s for s in stages if s.get("event")=="stress_end"]
    if len(a)!=1 or len(z)!=1:raise ValueError("no exact netem stage")
    a,z=a[0],z[0]
    if abs((z["monotonic_ns"]-a["monotonic_ns"])-300_000_000_000)>2_000_000_000:
        raise ValueError("not 300 second netem")
    actual={}
    for direction in ("c2s","s2c"):
        p0,d0=netem_stats(a["qdisc"][direction])
        p1,d1=netem_stats(z["qdisc"][direction])
        passed,dropped=p1-p0,d1-d0
        attempts=passed+dropped
        if attempts<1000 or passed<0 or dropped<0:
            raise ValueError("bad qdisc accounting "+direction)
        ratio=100*dropped/attempts
        if abs(ratio-sample["loss_percent"])>2.5:
            raise ValueError("netem nominal/real discrepancy "+direction)
        actual[direction]={"attempted":attempts,"passed":passed,"dropped":dropped,
                "measured_drop_percent":ratio,"denominator":"passed_plus_dropped"}
    biz,target=read(art/"biz.json"),read(art/"target.json")
    if (biz["role"],target["role"])!=("biz","target"):raise ValueError("wrong endpoint")
    if biz["start_monotonic_ns"]!=target["start_monotonic_ns"]:
        raise ValueError("not same host monotonic origin")
    errors=[]
    directions={}
    for direction,src,dst in (("c2s",biz,target),("s2c",target,biz)):
        srcstats=src["data"]["stats"]
        dststats=dst["data"]["stats"]
        per={}
        offered=0
        for size,pct in A_UDP_SHARE:
            send=next(x for x in src["data"]["per_size"] if x["payload_bytes"]==size)
            recv=next(x for x in dst["data"]["per_size"] if x["payload_bytes"]==size)
            offered+=send["offered"]*size
            miss=max(0,send["send_success"]-recv["valid_first_receive"])
            per[str(size)]={
               "target_byte_percent":pct,"offered":send["offered"],
               "sent":send["send_success"],"valid_first_received":recv["valid_first_receive"],
               "missing_after_drain":miss,"send_error":send["send_errors"],
               "late_slot":send["skip_late"],
               "oneway_returned_p50_p95_p99_ns":recv["oneway_returned_percentiles"],
               "big_ack_returned":send["big_ack_received"],
               "big_ack_rtt_returned_p50_p95_p99_ns":send["big_ack_returned_percentiles"],
               "big_ack_over_1s":send["big_ack_over_1s"],
               "big_ack_over_3s":send["big_ack_over_3s"]
            }
        buckets=dststats["recv_wall_bytes_by_bucket"][:30000]
        longest=0
        cur=0
        for value in buckets:
            if value==0:
                cur+=1
                longest=max(longest,cur)
            else:cur=0
        goodput=dststats["recv_unique_bytes"]*8/300_000_000
        directions[direction]={
          "offered_bytes":offered,"successfully_sent_bytes":srcstats["sent_bytes"],
          "first_valid_received_bytes":dststats["recv_unique_bytes"],
          "goodput_mbps":goodput,
          "send_coverage":srcstats["sent_packets"]/max(1,sum(x["offered"] for x in src["data"]["per_size"])),
          "recv_duplicates":dststats["recv_duplicates"],
          "recv_corrupt":dststats["corrupt"]+dst["data"]["recv_header_errors"],
          "send_errors":sum(x["send_errors"] for x in src["data"]["per_size"]),
          "scheduled_skip":srcstats["skipped_slots"],
          "longest_10ms_zero_delivery_window_ms":longest*10,
          "per_size":per
        }
        if directions[direction]["recv_duplicates"] or directions[direction]["recv_corrupt"]:
            errors.append(direction+": integrity/duplicate")
        if directions[direction]["send_coverage"]<.99:
            errors.append(direction+": source insufficient")
        if sample["loss_percent"]==0 and any(x["missing_after_drain"] for x in per.values()):
            errors.append(direction+": lossless missing")
        if goodput<10*(1-sample["loss_percent"]/100)*.99:
            errors.append(direction+": goodput under loss-aware target")
    probe=biz["probes"]
    probe_rtt={k:probe.get(k) for k in
               ("probe_rtt_p50_ns","probe_rtt_p95_ns","probe_rtt_p99_ns")}
    if probe["probe_sent"]<2970:errors.append("probe source incomplete")
    if sample["loss_percent"]==0 and probe["probe_sent"]!=probe["probe_recv"]:
        errors.append("lossless probe missing")
    return {
      "schema":"wbd-longmix-udp-summary/v1",
      "product_source_sha":PRODUCT_SOURCE,"helper_sha":manifest["helper_sha"],
      "workload":"A","seed":sample["seed"],"loss_percent":sample["loss_percent"],
      "duration_s":300,"drain_s":10,"actual_netem":actual,"directions":directions,
      "independent_96_probe":{"sent":probe["probe_sent"],
         "returned":probe["probe_recv"],"timeouts":probe["probe_timeouts"],
         "returned_rtt_percentiles_ns":probe_rtt,
         "note":"timeouts excluded from returned conditional percentiles, never from sent denominator"},
      "environment":{"server_tun_mtu":9000,"client_tun":"UNSUPPORTED",
           "client_linux_capture":"genuine OpenWrt TPROXY","client_ingress_mtu":9000,
           "real_underlay_mtu":1400,"outer_budget":1400,"route_mode":"all"},
      "classification":"FAIL" if errors else "PASS",
      "qualification_scope":"one scoped Linux client TPROXY -> formal server shared TUN A workload, not Windows/ARM/P7 nor paired RTT",
      "errors":errors
    }

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--artifact-dir",required=True)
    p.add_argument("--sample",required=True)
    p.add_argument("--output",required=True)
    a=p.parse_args()
    try:res=validate(Path(a.artifact_dir),read(Path(a.sample)))
    except (KeyError,OSError,ValueError,TypeError,IndexError,StopIteration) as ex:
        res={"schema":"wbd-longmix-udp-summary/v1","classification":"INVALID",
             "error":str(ex),"scope":"no sample PASS"}
    Path(a.output).write_text(json.dumps(res,indent=2,sort_keys=True)+"\n")
    print("WBD_LONGMIX_RESULT",res["classification"])
    if res["classification"]!="PASS":raise SystemExit(1)

if __name__=="__main__":main()
