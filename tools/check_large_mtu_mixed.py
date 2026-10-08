#!/usr/bin/env python3
"""Read-only post-run large MTU qualification, per direction and size.

No acceptance by survivor-only p99. Captures are audited then deleted; errors
remain INVALID/FAIL/CAPACITY_LIMITED instead of silently passing.
"""
import argparse
import collections
import hashlib
import json
import math
import struct
from pathlib import Path
from large_mtu_resource_report import report as resource_report

def percentile(x,q):
    if not x:return None
    v=sorted(x);return v[min(len(v)-1,round((len(v)-1)*q))]

def capture(path):
    counts=collections.Counter(); fragments=collections.Counter(); ip_packets=0; malformed=0
    with path.open("rb") as f:
        hdr=f.read(24)
        if len(hdr)!=24:raise ValueError("empty pcap "+str(path))
        if hdr[:4]==b"\xd4\xc3\xb2\xa1": endian="<"
        elif hdr[:4]==b"\xa1\xb2\xc3\xd4":endian=">"
        elif hdr[:4]==b"\x4d\x3c\xb2\xa1":endian="<"
        elif hdr[:4]==b"\xa1\xb2\x3c\x4d":endian=">"
        else:raise ValueError("unknown pcap magic "+str(path))
        link=struct.unpack(endian+"I",hdr[20:24])[0]
        if link!=1:raise ValueError("pcap linktype "+str(link))
        while True:
            h=f.read(16)
            if not h:break
            if len(h)!=16:raise ValueError("partial pcap record")
            _,_,caplen,origlen=struct.unpack(endian+"IIII",h)
            if caplen>65535:raise ValueError("invalid pcap caplen")
            data=f.read(caplen)
            if len(data)!=caplen:raise ValueError("partial pcap payload")
            if len(data)<34:continue
            pos=14
            eth=struct.unpack("!H",data[12:14])[0]
            if eth==0x8100 and len(data)>38:
                eth=struct.unpack("!H",data[16:18])[0];pos=18
            if eth!=0x0800:continue
            d=data[pos:]
            if len(d)<20 or d[0]>>4!=4:malformed+=1;continue
            ihl=(d[0]&15)*4
            if ihl<20 or len(d)<ihl:malformed+=1;continue
            length=struct.unpack("!H",d[2:4])[0]
            frag=struct.unpack("!H",d[6:8])[0]
            off=(frag&8191)*8;mf=bool(frag&8192);df=bool(frag&16384)
            ip_packets+=1
            counts[str(length)]+=1
            if off or mf:
                fragments[str(length)]+=1
                if df:malformed+=1
    return {"ip_packets":ip_packets,"ip_lengths":dict(counts),
            "fragment_lengths":dict(fragments),"malformed":malformed,
            "ip_fragments_total":sum(fragments.values())}

def qdisc(row):
    d=row if isinstance(row,list) else [row]
    sent=0;drop=0
    for q in d:
        if q.get("kind")!="netem":continue
        st=q.get("stats",{})
        b=st.get("basic",st)
        sent+=int(q.get("packets",b.get("packets",0)))
        drop+=int(q.get("drops",st.get("queue",{}).get("drops",b.get("drops",0))))
    return {"passed_packets":sent,"drops":drop}

def packet_loss(stage):
    start=next(x for x in stage if x["event"]=="business_start")
    end=next(x for x in stage if x["event"]=="business_end")
    out={}
    for d in ("c2s","s2c"):
        a=qdisc(start["qdisc"][d]);b=qdisc(end["qdisc"][d])
        passed=b["passed_packets"]-a["passed_packets"]
        drops=b["drops"]-a["drops"]
        denom=passed+drops
        out[d]={"passed":passed,"dropped":drops,"attempted":denom,
                "realized_percent":100*drops/denom if denom else None}
    return out

def size_direction(src,dst):
    tx=src.get("udp_tx",{})
    rx=dst.get("udp_rx",{})
    sizes={}
    for k in sorted(set(tx)|set(rx),key=int):
        sent=tx.get(k,{}).get("packets",0)
        accepted=rx.get(k,{}).get("packets",0)
        sizes[k]={"sent_datagrams":sent,"delivered":accepted,
                  "missing":max(0,sent-accepted),
                  "bytes_sent":tx.get(k,{}).get("bytes",0),
                  "bytes_received":rx.get(k,{}).get("bytes",0)}
    rtt={}
    for k,values in src.get("udp_big_rtt_ns",{}).items():
        measured=[i/1e6 for i in values if i>=0]
        rtt[k]={"returned":len(measured),
                "missing_or_timeout":max(0,tx.get(k,{}).get("packets",0)-len(measured)),
                "p50_ms":percentile(measured,.5),"p95_ms":percentile(measured,.95),
                "p99_ms":percentile(measured,.99),"max_ms":max(measured) if measured else None,
                "over_1s":sum(v>1000 for v in measured),"over_3s":sum(v>3000 for v in measured)}
    for k in ("8972","8973","65507"):
        if k not in rtt:
            rtt[k]={"returned":0,"missing_or_timeout":tx.get(k,{}).get("packets",0),
                    "p50_ms":None,"p95_ms":None,"p99_ms":None,"max_ms":None,
                    "over_1s":0,"over_3s":0}
    return sizes,rtt

def tcp_direction(src,dst):
    s=src.get("tcp_tx",{});r=dst.get("tcp_rx",{})
    mismatch=[]
    for flow,one in s.items():
        two=r.get(flow)
        if two is None or one["bytes"]!=two["bytes"] or one["sha256"]!=two["sha256"] or two.get("bad"):
            mismatch.append({"flow":flow,"sent_bytes":one["bytes"],
                             "received_bytes":two["bytes"] if two else None,
                             "checksum_equal":two is not None and one["sha256"]==two["sha256"]})
    return {"flows_sent":len(s),"flows_received":len(r),
            "sent_bytes":sum(x["bytes"] for x in s.values()),
            "received_bytes":sum(x["bytes"] for x in r.values()),
            "hash_or_length_mismatch":mismatch[:30],
            "mismatch_total":len(mismatch),
            "mss_seen":src.get("tcp_mss",[]),
            "backpressure_over_10ms":src.get("tcp_backpressure_events",0),
            "write_max_ms":src.get("tcp_write_max_ns",0)/1e6,
            "write_total_s":src.get("tcp_write_block_ns",0)/1e9,
            "connect_attempts":src.get("tcp_connect_attempts",0),
            "connect_failed":src.get("tcp_connections_failed",0)}

def bucket_gap(src,dst):
    sent={int(k):v for k,v in src.get("send_bucket_10ms",{}).items()}
    recv={int(k):v for k,v in dst.get("receive_bucket_10ms",{}).items()}
    # Active-gap only when sender was injecting near the matching 600ms+ delay.
    runs=[];active=0;longest=0;start=None
    for i in range(0,30000):
        if recv.get(i,0)==0 and any(sent.get(j,0)>0 for j in range(max(0,i-100),max(0,i-30))):
            if start is None:start=i
            active+=1
        else:
            if start is not None:
                runs.append([start,i,(i-start)*10])
                longest=max(longest,i-start);start=None
    if start is not None:
        runs.append([start,30000,(30000-start)*10]);longest=max(longest,30000-start)
    return {"active_sender_zero_recv_buckets":active,"longest_active_gap_ms":longest*10,
            "largest_gap_windows":sorted(runs,key=lambda x:-x[2])[:10],
            "method":"10ms receive empty while same-direction sender active in prior 300..1000ms; diagnostic, not definitive HOL"}

def main():
    a=argparse.ArgumentParser()
    a.add_argument("--artifact-dir",required=True);a.add_argument("--source",required=True)
    a.add_argument("--helper",required=True);a.add_argument("--workload",choices=["udp","tcp","mixed"],required=True)
    a.add_argument("--loss",type=int,choices=[0,5,20,30],required=True)
    a.add_argument("--seed",type=int,required=True);a.add_argument("--output",required=True)
    a.add_argument("--target-mbps",type=float,choices=[3.0,10.0],required=True)
    a.add_argument("--size-profile",choices=["ordinary","jumbo","boundary"],required=True)
    a.add_argument("--mode",choices=["normal","game"],required=True)
    a.add_argument("--lanes",type=int,choices=[1,4],required=True)
    a.add_argument("--diagnostic-mode",choices=["0","1"],required=True)
    x=a.parse_args();root=Path(x.artifact_dir)
    issues=[];capture_receipts=[]
    try:
        manifest=json.loads((root/"manifest.json").read_text())
        if manifest["source_sha"]!=x.source or manifest["harness_sha"]!=x.helper:issues.append("SOURCE_HELPER_MISMATCH")
        if (manifest["config"]["duration_s"]!=300 or manifest["config"]["outer_connection_mtu"]!=1400 or manifest["config"].get("drain_s")!=3 or manifest["config"].get("formal_default_tick_ms")!=100 or manifest["config"].get("record_limit_configured")!=0):issues.append("WRONG_MANIFEST")
        if manifest["scenario"]!=x.workload+"-p"+str(x.loss):issues.append("WRONG_WORKLOAD_LOSS")
        if (manifest.get("mode")!=x.mode or manifest["config"].get("lanes")!=x.lanes or
            manifest["config"].get("application_mbps_each_direction")!=x.target_mbps):
            issues.append("WRONG_MODE_LANES_RATE")
        biz=json.loads((root/"biz.json").read_text());target=json.loads((root/"target.json").read_text())
        if any(d["source_sha"]!=x.source or d["helper_sha"]!=x.helper or d["seed"]!=x.seed or d.get("size_profile")!=x.size_profile or d.get("configured_per_direction_mbps")!=x.target_mbps for d in (biz,target)):issues.append("PROCESS_IDENTITY")
        b=biz["counters"];t=target["counters"]
        stages=[json.loads(l) for l in (root/"stage-events.jsonl").read_text().splitlines()]
        netem=packet_loss(stages)
        # Enforce actual on-wire impairment stage timing. A manifest claiming
        # drain_s=3 cannot silently keep real endpoints/tc running for 15s.
        stage_start=next(row["monotonic_ns"] for row in stages if row["event"]=="business_start")
        stage_end=next(row["monotonic_ns"] for row in stages if row["event"]=="business_end")
        drain_end=next(row["monotonic_ns"] for row in stages if row["event"]=="drain_end")
        if not (299_500_000_000<=stage_end-stage_start<=300_500_000_000):
            issues.append("BUSINESS_STAGE_DURATION_MISMATCH")
        if not (2_500_000_000<=drain_end-stage_end<=3_500_000_000):
            issues.append("DRAIN_STAGE_DURATION_MISMATCH")
        if any(z["attempted"]<100 for z in netem.values()):issues.append("NETEM_NO_EFFECTIVE_TRAFFIC")
        for side in ("c2s","s2c"):
            z=netem[side]
            if z["realized_percent"] is None or abs(z["realized_percent"]-x.loss)>max(2,0.20*x.loss):
                issues.append("NETEM_LOSS_MISMATCH_"+side)
        links={}
        for file,key in (("inner-biz-link.json","biz"),("inner-client-link.json","client_input"),
                         ("inner-server-tun.json","server_tun"),("inner-target-link.json","target")):
            entries=json.loads((root/file).read_text());mtu=entries[0]["mtu"];links[key]=mtu
            if key=="server_tun":
                if not 576<=mtu<9000:issues.append("INVALID_DERIVED_TUN_MTU_"+key)
            elif mtu!=9000:issues.append("INNER_VETH_MTU_MISMATCH_"+key)
        sizes_c2s,rtt_c2s=size_direction(b,t)
        sizes_s2c,rtt_s2c=size_direction(t,b)
        tcp_c2s=tcp_direction(b,t);tcp_s2c=tcp_direction(t,b)
        direction={}
        for key,src,dst,sz,rtt,tcp in (("c2s",b,t,sizes_c2s,rtt_c2s,tcp_c2s),
                                        ("s2c",t,b,sizes_s2c,rtt_s2c,tcp_s2c)):
            offered=src["offered_bytes"];sent=src["sent_bytes"]
            received=dst["received_bytes"];requested=x.target_mbps*1e6/8*300
            achieved=sent/requested
            direction[key]={"offered_bytes":offered,"sent_bytes":sent,"receiver_bytes":received,
                            "sent_mbps":sent*8/300/1e6,"goodput_mbps":received*8/300/1e6,
                            "target_mbps":x.target_mbps,"target_achievement":achieved,
                            "injection_skipped_bytes":src.get("skipped_bytes",0),
                            "generator_send_errors":src["send_errors"],
                            "source_corrupt_or_malformed_count":src.get("corrupt",0),
                            "receiver_corrupt_or_malformed_count":dst.get("corrupt",0),
                            "source_probe_send_errors":src.get("probe_send_errors",0),
                            "udp_by_size":sz,"large_udp_roundtrip_ms":rtt,
                            "tcp":tcp,"continuity":bucket_gap(src,dst)}
            if achieved<.99:issues.append("INSUFFICIENT_INJECTION_"+key)
            if src["send_errors"]:issues.append("GENERATOR_SEND_ERROR_"+key)
            if src.get("corrupt",0) or dst.get("corrupt",0):issues.append("PAYLOAD_CORRUPT_OR_MALFORMED_"+key)
            if src.get("probe_send_errors",0):issues.append("PROBE_SEND_ERROR_"+key)
            if tcp["mismatch_total"]:issues.append("TCP_HASH_MISMATCH_"+key)
            if x.workload in ("tcp","mixed") and any(str(i) not in src.get("tcp_tx",{}) for i in range(4)):
                issues.append("TCP_FOUR_SUSTAINED_FLOWS_MISSING_"+key)
            if x.loss==0 and any(v["missing"] for v in sz.values()):issues.append("LOSSLESS_UDP_MISSING_"+key)
        probe_summary={}
        for name,side in (("c2s",b),("s2c",t)):
            probe=side.get("probe_rtt_ns",[])
            probe_summary[name]={"sent":side["probe_sent"],"returned":side["probe_received"],
                                 "missing":side["probe_sent"]-side["probe_received"],
                                 "send_errors":side["probe_send_errors"],
                                 "p50_ms":percentile([v/1e6 for v in probe],.50),
                                 "p95_ms":percentile([v/1e6 for v in probe],.95),
                                 "p99_ms":percentile([v/1e6 for v in probe],.99),
                                 "max_ms":max(probe)/1e6 if probe else None}
            if side["probe_sent"]<1450:issues.append("PROBE_COVERAGE_"+name)
            if x.loss==0 and probe_summary[name]["missing"]:issues.append("LOSSLESS_PROBE_MISSING_"+name)
        web={}
        if x.workload in ("tcp","mixed"):
            web_b=json.loads((root/"biz-http.json").read_text())
            web_t=json.loads((root/"target-http.json").read_text())
            for receipt,role in ((web_b,"biz"),(web_t,"target")):
                if (receipt.get("product_source_sha")!=x.source or
                    receipt.get("helper_sha")!=x.helper or
                    receipt.get("seed")!=x.seed or receipt.get("role")!=role):
                    issues.append("SOURCE_HELPER_MISMATCH_HTTP_"+role)
            returned=web_b.get("returned",0)
            delays=[v["duration_ns"]/1e6 for v in web_b.get("completed",[])]
            web={"http_https_requests":web_b.get("requests"),
                 "returned":returned,"missing":max(0,20-returned),
                 "https_certificate_verified":web_b.get("tls_verified",0),
                 "returned_p99_ms":percentile(delays,.99),
                 "return_failures":web_b.get("failed",[]),
                 "http_server_served":web_t.get("http_served"),
                 "https_server_served":web_t.get("https_served"),
                 "server_errors":web_t.get("server_errors")}
            if (web["http_https_requests"]!=20 or returned!=20 or
                web["https_certificate_verified"]!=10 or
                web["http_server_served"]!=10 or web["https_server_served"]!=10 or
                web["server_errors"] or web["return_failures"]):
                issues.append("HTTP_HTTPS_SHORT_E2E_NOT_QUALIFIED")
        # Formal budget counts all business bytes, including separately
        # validated HTTP request/response streams. Never treat overshoot as
        # better throughput. A single-packet 8KiB measurement allowance is
        # not permission to pace above the configured per-direction budget.
        web_sent={"c2s":0,"s2c":0}
        if x.workload in ("tcp","mixed"):
            completed=web_b.get("completed",[])
            web_sent["c2s"]=sum(int(row["request_bytes"]) for row in completed)
            web_sent["s2c"]=sum(int(row["wire_response_bytes"]) for row in completed)
        requested_bytes=int(x.target_mbps*125000*300)
        for side in ("c2s","s2c"):
            item=direction[side]
            item["http_https_sent_bytes_separately"]=web_sent[side]
            item["total_logical_sent_bytes_including_http"]=item["sent_bytes"]+web_sent[side]
            item["total_logical_delivered_bytes_including_verified_http"]=item["receiver_bytes"]+web_sent[side]
            item["total_logical_sent_mbps_including_http"]=item["total_logical_sent_bytes_including_http"]*8/300/1e6
            if item["total_logical_sent_bytes_including_http"]>requested_bytes+8192:
                issues.append("LOGICAL_BUSINESS_RATE_OVER_BUDGET_"+side)
        resources=resource_report(root,int(biz["start_monotonic_ns"]),int(biz["start_monotonic_ns"])+300_000_000_000)
        runtime_game_evidence={}
        diag_on=x.diagnostic_mode=="1"
        if bool(manifest["config"].get("cpu_contention_profile"))!=diag_on:
            issues.append("WRONG_DIAGNOSTIC_PROFILE_MANIFEST")
        for diag_side in ("client","server"):
            d=resources["diagnostics"][diag_side]
            if not diag_on:
                if d["present"]:issues.append("PROFILE_OFF_DIAGNOSTIC_ENABLED_"+diag_side)
                continue
            if not d["present"] or d.get("parse_errors",0):
                issues.append("PROFILE_ON_DIAGNOSTIC_MISSING_OR_INVALID_"+diag_side)
                continue
            state=d.get("last_state") or {}
            owner=state.get("owner") or {}
            lanes=state.get("lanes") or []
            runtime_game_evidence[diag_side]={
                "snapshot_count":d.get("samples"),
                "desired_lanes":owner.get("DesiredLanes"),
                "active_logical_lanes":owner.get("ActiveLogicalLanes"),
                "physical_lanes":owner.get("PhysicalLanes"),
                "game_logical_outbound":owner.get("GameLogicalOutbound"),
                "game_lane_copies":owner.get("GameLaneCopies"),
                "game_delivered":owner.get("GameDelivered"),
                "game_duplicates":owner.get("GameDuplicates"),
                "lane_refs":[lane.get("ref") for lane in lanes]
            }
            if x.mode=="game" and (
                owner.get("DesiredLanes")!=4 or owner.get("ActiveLogicalLanes")!=4
                or len(lanes)!=4 or (owner.get("GameLogicalOutbound") or 0)<=0
                or (owner.get("GameLaneCopies") or 0)<=0):
                issues.append("GAME4_ACTUAL_RACE_NOT_QUALIFIED_"+diag_side)
        if resources["strict_resource"].get("errors"):
            issues.append("RESOURCE_AUDIT_WARNINGS")
        if resources["strict_resource"].get("socket_drop_max",0):
            issues.append("LOCAL_SOCKET_DROP")
        if any(v.get("rx",0)>0 or v.get("tx",0)>0 for v in resources["strict_resource"].get("link_drop_delta",{}).values()):
            issues.append("LOCAL_INTERFACE_DROP")
        result={"schema":"wbd-large-mtu-analysis/v1","product_source_sha":x.source,
                "helper_sha":x.helper,"workload":x.workload,"loss_percent":x.loss,"seed":x.seed,"size_profile":x.size_profile,"target_mbps":x.target_mbps,
                "netem_realized":netem,"mtu":links,"route_mode":"all",
                "path":"biz -> client TPROXY -> raw TCP-shaped -> router netem -> shared server TUN -> target",
                "direction":direction,"probe":probe_summary,"web":web,"resources":resources,
                "diagnostic_mode":"on" if diag_on else "off",
                "runtime_game_evidence":runtime_game_evidence}
    except Exception as ex:
        issues.append("ANALYZER_EXCEPTION:"+repr(ex));result={"error":repr(ex)}
    finally:
        # Capture artifact policy applies to both success and failures.
        for path in root.glob("*.pcap*"):
            digest=hashlib.sha256()
            with path.open("rb") as f:
                while True:
                    data=f.read(1024*1024)
                    if not data:break
                    digest.update(data)
            row={"filename":path.name,"bytes":path.stat().st_size,"sha256":digest.hexdigest()}
            try:
                row["packet_distribution"]=capture(path)
            except Exception as ex:
                row["parse_error"]=str(ex);issues.append("CAPTURE_PARSE_"+path.name)
            capture_receipts.append(row)
            path.unlink()
    result.update({"capture_cleanup":{"deleted_raw_pcap":True,"capture_receipts":capture_receipts},
                   "issues":issues,
                   "classification":"INVALID" if any(y.startswith(("ANALYZER_EXCEPTION","PROCESS_IDENTITY","SOURCE_HELPER","WRONG_MANIFEST","WRONG_WORKLOAD","CAPTURE_PARSE")) for y in issues) else
                   "CAPACITY_LIMITED" if any(z.startswith("INSUFFICIENT_INJECTION") for z in issues) and result.get("resources",{}).get("capacity_limited_evidenced") else
                   "FAIL" if issues else "PASS_SCOPED_ACTIONS",
                   "never_physical_pass":True})
    Path(x.output).write_text(json.dumps(result,indent=2,sort_keys=True))
    print("WBD_LARGE_MTU_ANALYSIS",result["classification"],"issues",issues,flush=True)
    if result["classification"]!="PASS_SCOPED_ACTIONS":raise SystemExit(1)

if __name__=="__main__":main()
