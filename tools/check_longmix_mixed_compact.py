#!/usr/bin/env python3
"""One mixed TCP+UDP 5+5 Mbps real-product Actions sample numeric validator.

Never treats a TCP application's 1MiB write as an IP packet. All returned
latencies are conditional on delivered samples; missing and late are explicit.
"""
import argparse
import json
import re
from pathlib import Path
from longmix_profile import PRODUCT_SOURCE, C_UDP_SHARE, validate_sample
from check_longmix_udp_compact import netem_stats
from check_longmix_tcp_compact import percent

def load(path):
    return json.loads(Path(path).read_text())

def stat_route(art):
    specs={
        "server-tun-active.txt":9000,
        "client-tproxy-ingress-mtu-active.txt":9000,
        "biz-mtu-active.txt":9000,
        "target-mtu-active.txt":9000,
        "router-rcli-mtu-active.txt":1400,
        "router-rsrv-mtu-active.txt":1400,
    }
    for name,mtu in specs.items():
        if not re.search(r"\bmtu\s+"+str(mtu)+r"\b",(art/name).read_text()):
            raise ValueError("actual MTU mismatch "+name)
    if "wbd_tproxy" not in (art/"client-tproxy-active.txt").read_text():
        raise ValueError("owned client TPROXY table absent")
    if "10.50.0.2" not in (art/"biz-target-route.txt").read_text():
        raise ValueError("biz private route not shown")
    return {"server_tun_mtu":9000,"client_tun":"UNSUPPORTED",
            "client_capture":"real Linux OpenWrt TPROXY", "client_ingress_mtu":9000,
            "underlay_mtu":1400,"outer_ipv4_budget":1400,
            "route_mode":"all"}

def stat_loss(art,loss):
    events=[json.loads(x) for x in (art/"stage-events.jsonl").read_text().splitlines() if x.strip()]
    a=[x for x in events if x.get("event")=="stress_start"]
    b=[x for x in events if x.get("event")=="stress_end"]
    if len(a)!=1 or len(b)!=1:
        raise ValueError("missing actual fixed 300s netem interval")
    first,last=a[0],b[0]
    dur=last["monotonic_ns"]-first["monotonic_ns"]
    if abs(dur-300_000_000_000)>2_000_000_000:
        raise ValueError("netem interval length mismatch")
    out={}
    for direction in ("c2s","s2c"):
        ap,ad=netem_stats(first["qdisc"][direction])
        bp,bd=netem_stats(last["qdisc"][direction])
        passed,dropped=bp-ap,bd-ad
        total=passed+dropped
        if total<1000 or passed<0 or dropped<0:raise ValueError("invalid qdisc counters")
        real=100*dropped/total
        if abs(real-loss)>2.5:raise ValueError("real loss differs from requested "+direction)
        out[direction]={"nominal_percent":loss,"actual_percent":real,
                        "passed":passed,"dropped":dropped,"attempted":total}
    return {"duration_ns":dur,"directions":out}

def flow_keyed(obj):
    fs=obj["flows"]
    if len(fs)!=3 or set(f["index"] for f in fs)!={0,1,2}:
        raise ValueError("requires three separate real bidirectional TCP connections")
    return {f["index"]:f for f in fs}

def udp_audit(src,dst,loss,issues,direction):
    sends={x["payload_bytes"]:x for x in src["data"]["per_size"]}
    receives={x["payload_bytes"]:x for x in dst["data"]["per_size"]}
    per={}
    total_offered=0
    first=0
    for size,share in C_UDP_SHARE:
        s=sends[size]
        d=receives[size]
        offered,sent,delivered=s["offered"],s["send_success"],d["valid_first_receive"]
        total_offered+=offered*size
        first+=delivered*size
        miss=max(0,sent-delivered)
        per[str(size)]={
            "target_payload_byte_percent":share,
            "offered":offered,
            "send_success":sent,
            "first_valid_received":delivered,
            "missing_after_drain":miss,
            "returned_oneway_ns":d["oneway_returned_percentiles"],
            "returned_oneway_max_ns":d["oneway_returned_max_ns"],
            "sent_ack_received":s["big_ack_received"],
            "returned_big_rtt_ns":s["big_ack_returned_percentiles"],
            "big_ack_over_1s":s["big_ack_over_1s"],
            "big_ack_over_3s":s["big_ack_over_3s"],
            "late_sender_slots":s["skip_late"],
            "send_errors":s["send_errors"],
        }
        if loss==0 and miss:
            issues.append(f"{direction}: lossless UDP {size} missing {miss} packets")
    st=src["data"]["stats"]
    received=dst["data"]["stats"]
    actual_success=st["sent_bytes"]
    coverage=actual_success / max(1,total_offered)
    goodput=first*8/300_000_000
    if coverage<.99:
        issues.append(f"{direction}: invalid UDP injection coverage {coverage:.3f}")
    if goodput < 5*(1-loss/100)*.99:
        issues.append(f"{direction}: UDP goodput under 5Mbps adjusted loss budget {goodput:.3f}")
    if received["recv_duplicates"] or received["corrupt"] or dst["data"]["recv_header_errors"]:
        issues.append(f"{direction}: UDP duplicate/corrupt")
    buckets=received["recv_wall_bytes_by_bucket"][:30000]
    longest=0
    streak=0
    for count in buckets:
        if not count:
            streak+=1
            longest=max(longest,streak)
        else:streak=0
    return {
        "target_mbps":5,"offered_bytes":total_offered,
        "successful_send_bytes":actual_success,
        "first_valid_receive_bytes":first,
        "goodput_mbps":goodput,
        "send_coverage":coverage,
        "duplicates":received["recv_duplicates"],
        "corrupt":received["corrupt"]+dst["data"]["recv_header_errors"],
        "longest_zero_first_delivery_10ms_bucket_window_ms":longest*10,
        "per_size":per
    }

def tcp_audit(src,dst,loss,issues,direction,short_success):
    sender,receiver=flow_keyed(src),flow_keyed(dst)
    rows=[]
    nsend=nrecv=0
    for idx in range(3):
        s,r=sender[idx],receiver[idx]
        nsend+=s["send_bytes"]
        nrecv+=r["recv_bytes"]
        good=(s["send_bytes"]==r["recv_bytes"]
              and s["send_sha256"]==r["recv_sha256"])
        if not good:
            issues.append(f"{direction}: TCP connection{idx} hash or byte mismatch")
        if s["socket_errors"] or r["socket_errors"]:
            issues.append(f"{direction}: TCP connection{idx} socket error")
        rows.append({
          "index":idx,"app_sent_bytes":s["send_bytes"],
          "recv_bytes":r["recv_bytes"],
          "full_stream_sha256_sent":s["send_sha256"],
          "full_stream_sha256_received":r["recv_sha256"],
          "stream_integrity":good,
          "write_sizes_as_application_blocks":s["app_write_sizes"],
          "write_calls_by_size":s["write_call_sizes"],
          "partial_socket_writes":s["partial_socket_writes"],
          "backpressure_socket_timeouts":s["socket_timeout_backpressure"],
          "write_block_max_ns":s["write_max_block_ns"],
          "tcp_info_initial":s["tcp_info_initial"],
          "tcp_info_final":s["tcp_info_final"],
          "longest_stream_zero_delivery_ms":r["recv_longest_zero_delivery_ms"]
        })
    total_sent=nsend+short_success
    total_received=nrecv+short_success
    target=5*1_000_000/8*300
    if total_sent<.99*target:
        issues.append(f"{direction}: TCP source failed 5Mbps target: {total_sent/target:.3f}")
    if loss==0 and total_received<.99*target:
        issues.append(f"{direction}: TCP lossless goodput under 99% target")
    return {
        "target_mbps":5,"offered_target_bytes":target,
        "successful_send_bytes":total_sent,
        "received_complete_bytes":total_received,
        "actual_source_coverage":total_sent/target,
        "goodput_mbps":total_received*8/300_000_000,
        "flows":rows,
    }

def analyze(art,sample):
    if validate_sample(sample).name!="C":
        raise ValueError("not C mixed")
    m=load(art/"manifest.json")
    if m.get("source_sha")!=PRODUCT_SOURCE or m.get("workload")!="C":
        raise ValueError("mixed sample wrong product/workload")
    if (m.get("seed"),m.get("loss_percent"))!=(sample["seed"],sample["loss_percent"]):
        raise ValueError("seed/loss mismatch")
    if not re.fullmatch("[0-9a-f]{40}",m.get("helper_sha","")):
        raise ValueError("missing exact helper sha")
    cfg=m["config"]
    if (cfg["tcp_mbps_each_direction"],cfg["udp_mbps_each_direction"])!=(5,5):
        raise ValueError("mixed split not fixed 5/5")
    if cfg["duration_s"]!=300 or cfg["drain_s"]!=10:
        raise ValueError("wrong sample/drain")
    env=stat_route(art)
    netem=stat_loss(art,sample["loss_percent"])
    data={}
    for role in ("biz","target"):
        data[role]={
            "udp":load(art/("udp-"+role+".json")),
            "tcp":load(art/("tcp-"+role+".json")),
            "launcher":load(art/("mixed-"+role+"-launcher.json")),
        }
        receipt=data[role]["launcher"]
        if receipt["exit_codes"]!={"tcp":0,"udp":0}:
            raise ValueError("mixed child exited nonzero "+role)
        if receipt["tcp_mbps_each_direction"]!=5 or receipt["udp_mbps_each_direction"]!=5:
            raise ValueError("mixed endpoint quota changed")
        if data[role]["tcp"]["seed"]!=sample["seed"] or data[role]["tcp"]["role"]!=role:
            raise ValueError("wrong TCP endpoint "+role)
        if data[role]["udp"]["role"]!=role or data[role]["udp"]["workload"]!="C":
            raise ValueError("wrong UDP endpoint "+role)
    if data["biz"]["tcp"]["start_monotonic_ns"]!=data["target"]["tcp"]["start_monotonic_ns"]:
        raise ValueError("TCP streams did not share monotonic start")
    if data["biz"]["udp"]["start_monotonic_ns"]!=data["target"]["udp"]["start_monotonic_ns"]:
        raise ValueError("UDP streams did not share monotonic start")
    issues=[]
    p=sample["loss_percent"]
    flow={}
    shorts=data["biz"]["tcp"]["short_samples"]
    shorts_ok=[x["rtt_ns"] for x in shorts if x["classification"]=="RETURNED"]
    short_bytes=96*len(shorts_ok)
    if len(shorts)!=300:
        issues.append("TCP short request source not 300")
    if p==0 and len(shorts_ok)!=300:
        issues.append("lossless TCP short transactions missing")
    for direction,src,dst in (("c2s","biz","target"),("s2c","target","biz")):
        du=udp_audit(data[src]["udp"],data[dst]["udp"],p,issues,direction)
        dt=tcp_audit(data[src]["tcp"],data[dst]["tcp"],p,issues,direction,short_bytes)
        flow[direction]={"udp":du,"tcp":dt,
             "combined_goodput_mbps":du["goodput_mbps"]+dt["goodput_mbps"],
             "combined_target_mbps":10,
             "combined_offered_bytes":du["offered_bytes"]+dt["offered_target_bytes"],
             "combined_first_valid_bytes":du["first_valid_receive_bytes"]+dt["received_complete_bytes"]}
    probes=data["biz"]["udp"]["probes"]
    pr={
        "sent":probes["probe_sent"],"returned":probes["probe_recv"],
        "timeouts":probes["probe_timeouts"],
        "returned_p50_ns":probes.get("probe_rtt_p50_ns"),
        "returned_p95_ns":probes.get("probe_rtt_p95_ns"),
        "returned_p99_ns":probes.get("probe_rtt_p99_ns"),
        "note":"UDP 96B independent, missing not included in conditional p99",
    }
    if pr["sent"]!=3000:issues.append("independent 96B UDP probe not fully offered")
    if p==0 and pr["returned"]!=pr["sent"]:
        issues.append("lossless UDP probe incomplete")
    return {
      "schema":"wbd-longmix-mixed-c-summary/v1",
      "classification":"FAIL" if issues else "PASS",
      "product_source_sha":PRODUCT_SOURCE,"helper_sha":m["helper_sha"],
      "workload":"C","loss_percent":p,"seed":sample["seed"],
      "duration_s":300,"drain_s":10,
      "quota":"TCP5Mbps+UDP5Mbps per direction; no automatic transfer",
      "environment":env,"actual_netem":netem,
      "directions":flow,
      "independent_udp_96_probe":pr,
      "short_tcp_transactions":{
          "offered":len(shorts),
          "returned":len(shorts_ok),
          "missing_or_error":len(shorts)-len(shorts_ok),
          "returned_rtt_p50_ns":percent(shorts_ok,.5),
          "returned_rtt_p95_ns":percent(shorts_ok,.95),
          "returned_rtt_p99_ns":percent(shorts_ok,.99),
          "returned_max_ns":max(shorts_ok,default=None)
      },
      "warnings":["Linux client is TPROXY not Wintun/client TUN",
        "Missing UDP large packet never hidden by returned small probe p99",
        "TCP in-stream ordered waits must not be called outer cross-business HOL",
        "Capacity attribution requires runner PSI/softirq/queue evidence"],
      "issues":issues
    }

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--sample",required=True)
    p.add_argument("--artifact-dir",required=True)
    p.add_argument("--output",required=True)
    args=p.parse_args()
    try:result=analyze(Path(args.artifact_dir),load(Path(args.sample)))
    except (KeyError,TypeError,ValueError,OSError,IndexError,StopIteration) as ex:
        result={"schema":"wbd-longmix-mixed-c-summary/v1",
                "classification":"INVALID","error":str(ex),
                "qualifier":"No A/B/C performance PASS from missing evidence"}
    Path(args.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_LONGMIX_MIXED_C_RESULT",result["classification"])
    if result["classification"]!="PASS":
        raise SystemExit(1)
if __name__=="__main__":main()
