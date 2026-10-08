#!/usr/bin/env python3
"""One 300s real Linux TCP B sample: strict endpoints, stream hash, MSS/netem."""
import argparse
import json
import re
from pathlib import Path
from check_longmix_udp_compact import netem_stats
from longmix_profile import PRODUCT_SOURCE, TCP_LONG_CONNECTIONS, validate_sample

def load(path):
    return json.loads(path.read_text())

def percent(values,p):
    if not values:return None
    values=sorted(values)
    return values[min(len(values)-1,int(round((len(values)-1)*p)))]

def verify(art, requested):
    w=validate_sample(requested)
    if w.name!="B":raise ValueError("wrong workload B")
    manifest=load(art/"manifest.json")
    if manifest.get("source_sha")!=PRODUCT_SOURCE or manifest.get("workload")!="B":
        raise ValueError("product/manifest wrong")
    if manifest.get("seed")!=requested["seed"] or manifest.get("loss_percent")!=requested["loss_percent"]:
        raise ValueError("sample identity drift")
    if not re.fullmatch("[0-9a-f]{40}", manifest.get("helper_sha","")):
        raise ValueError("missing helper SHA")
    for name,mtu in (("server-tun-active.txt",9000),
       ("biz-mtu-active.txt",9000),("target-mtu-active.txt",9000),
       ("client-tproxy-ingress-mtu-active.txt",9000),
       ("router-rcli-mtu-active.txt",1400),("router-rsrv-mtu-active.txt",1400)):
        if not re.search(r"\bmtu\s+"+str(mtu)+r"\b",(art/name).read_text()):
            raise ValueError("bad actual MTU: "+name)
    if "wbd_tproxy" not in (art/"client-tproxy-active.txt").read_text():
        raise ValueError("TPROXY absent")
    stage=[json.loads(s) for s in (art/"stage-events.jsonl").read_text().splitlines()]
    start=[x for x in stage if x.get("event")=="stress_start"]
    end=[x for x in stage if x.get("event")=="stress_end"]
    if len(start)!=1 or len(end)!=1:
        raise ValueError("missing fixed netem interval")
    begin,done=start[0],end[0]
    if abs((done["monotonic_ns"]-begin["monotonic_ns"])-300_000_000_000)>2_000_000_000:
        raise ValueError("wrong impairment duration")
    drops={}
    for direction in ("c2s","s2c"):
        p0,d0=netem_stats(begin["qdisc"][direction])
        p1,d1=netem_stats(done["qdisc"][direction])
        passed,dropped=p1-p0,d1-d0
        if passed<0 or dropped<0 or passed+dropped<1000:
            raise ValueError("invalid qdisc count")
        rate=100*dropped/(passed+dropped)
        if abs(rate-requested["loss_percent"])>2.5:
            raise ValueError("actual qdisc loss not as specified")
        drops[direction]={"attempted":passed+dropped,"passed":passed,
                          "drops":dropped,"actual_percent":rate}
    biz=load(art/"tcp-biz.json")
    target=load(art/"tcp-target.json")
    for role,x in (("biz",biz),("target",target)):
        if x["role"]!=role or x["workload"]!="B" or x["duration_s"]!=300 or x["drain_s"]!=10:
            raise ValueError("TCP workload parameters drift "+role)
        if len(x["flows"])!=TCP_LONG_CONNECTIONS:
            raise ValueError("less than 3 long TCP sockets "+role)
    if biz["start_monotonic_ns"]!=target["start_monotonic_ns"]:
        raise ValueError("invalid mono time origin")
    issues=[]
    dirs={}
    peerpairs={"c2s":(biz,target),"s2c":(target,biz)}
    for direction,(send,recv) in peerpairs.items():
        send_by_id={x["index"]:x for x in send["flows"]}
        recv_by_id={x["index"]:x for x in recv["flows"]}
        if set(send_by_id)!=set(range(TCP_LONG_CONNECTIONS)) or set(recv_by_id)!=set(range(TCP_LONG_CONNECTIONS)):
            raise ValueError("wrong TCP flow indices")
        flowrows=[]
        for idx in range(TCP_LONG_CONNECTIONS):
            sender=send_by_id[idx]
            receiver=recv_by_id[idx]
            sent=sender["send_bytes"]
            got=receiver["recv_bytes"]
            same_hash=sender["send_sha256"]==receiver["recv_sha256"]
            if sent!=got or not same_hash:
                issues.append(f"{direction}: stream{idx} wrong bytes/hash sent={sent} received={got} match={same_hash}")
            if sender["socket_errors"] or receiver["socket_errors"]:
                issues.append(f"{direction}: stream{idx} socket errors")
            flowrows.append({
                "index":idx,"app_sent_bytes":sent,"first_read_bytes":got,
                "sent_sha256":sender["send_sha256"],"recv_sha256":receiver["recv_sha256"],
                "stream_integrity":sent==got and same_hash,
                "app_write_calls":sender["write_call_sizes"],
                "app_write_bytes":sender["written_bytes_by_call_size"],
                "partial_writes":sender["partial_socket_writes"],
                "backpressure_socket_timeouts":sender["socket_timeout_backpressure"],
                "max_write_block_ns":sender["write_max_block_ns"],
                "tcp_initial":sender["tcp_info_initial"],
                "tcp_final":sender["tcp_info_final"],
                "tcp_recv_initial":receiver["tcp_info_initial"],
                "tcp_recv_final":receiver["tcp_info_final"],
                "recv_max_zero_window_ms":receiver["recv_longest_zero_delivery_ms"],
                "errors":sender["socket_errors"]+receiver["socket_errors"],
            })
        short=send["short_samples"]
        short_success=sum(x.get("tx_bytes",0) for x in short)
        if direction=="s2c":
            # Short response from target to client, both sides account 96 B.
            short_success=sum(x.get("rx_bytes",0) for x in biz["short_samples"])
        sent_total=sum(x["app_sent_bytes"] for x in flowrows)+short_success
        read_total=sum(x["first_read_bytes"] for x in flowrows)+short_success
        offered=375_000_000
        if sent_total/offered<.99:
            issues.append(f"{direction}: source injection below 99% ({sent_total/offered:.4f})")
        if requested["loss_percent"]==0 and read_total/offered<.99:
            issues.append(f"{direction}: lossless goodput below 99%")
        dirs[direction]={
            "offered_target_bytes":offered,
            "send_success_bytes":sent_total,
            "recv_complete_bytes":read_total,
            "send_coverage":sent_total/offered,
            "goodput_mbps":read_total*8/300_000_000,
            "tcp_flows":flowrows,
        }
    cshort=biz["short_samples"]
    returned=[x["rtt_ns"] for x in cshort if x["classification"]=="RETURNED"]
    timed_out=[x for x in cshort if x["classification"]!="RETURNED"]
    probe={"sent":len(cshort),"returned":len(returned),
       "missing_or_timeout":len(timed_out),
       "returned_p50_ns":percent(returned,.5),
       "returned_p95_ns":percent(returned,.95),
       "returned_p99_ns":percent(returned,.99),
       "returned_max_ns":max(returned,default=None),
       "metric":"96B TCP controlled short-connection request/reply RTT; no separate UDP probe"}
    if probe["sent"]!=300:
        issues.append("short request injection not 300")
    if requested["loss_percent"]==0 and timed_out:
        issues.append("lossless TCP short request missing")
    return {
        "schema":"wbd-longmix-tcp-b-summary/v1",
        "classification":"FAIL" if issues else "PASS",
        "product_source_sha":PRODUCT_SOURCE,"helper_sha":manifest["helper_sha"],
        "source_kind":"B","seed":requested["seed"],"loss_percent":requested["loss_percent"],
        "duration_s":300,"drain_s":10,"actual_netem":drops,
        "directions":dirs,"short_tcp_probe":probe,
        "capacity_status":"NOT_ATTRIBUTED_WITHOUT_RUNNER_SATURATION_PROOF",
        "environment":{"server_tun_mtu":9000,"client_tun":"UNSUPPORTED",
                       "client_capture":"real Linux TPROXY","outer_ipv4_budget":1400},
        "assertion":"Actual Linux TCP business socket streams over formal FakeTCP products; TCP write sizes not IPv4 lengths; not Windows/ARM PHYSICAL_PASS",
        "issues":issues,
    }

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--sample",required=True)
    p.add_argument("--artifact-dir",required=True)
    p.add_argument("--output",required=True)
    args=p.parse_args()
    try:result=verify(Path(args.artifact_dir),load(Path(args.sample)))
    except (KeyError,TypeError,ValueError,OSError,IndexError) as ex:
        result={"schema":"wbd-longmix-tcp-b-summary/v1",
                "classification":"INVALID","error":str(ex)}
    Path(args.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_LONGMIX_TCP_B_RESULT",result["classification"])
    if result["classification"]!="PASS":raise SystemExit(1)
if __name__=="__main__":main()
