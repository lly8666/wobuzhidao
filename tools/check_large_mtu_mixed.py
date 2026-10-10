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
    return {"ip_packets":ip_packets,"ip_bytes":sum(int(k)*v for k,v in counts.items()),"ip_lengths":dict(counts),
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

def _qdisc_interval(start, end, side):
    a=qdisc(start["qdisc"][side]);b=qdisc(end["qdisc"][side])
    passed=b["passed_packets"]-a["passed_packets"]
    drops=b["drops"]-a["drops"]
    if passed < 0 or drops < 0:
        raise ValueError("netem qdisc reset inside phase "+side)
    attempted=passed+drops
    return {"passed":passed,"dropped":drops,"attempted":attempted,
            "realized_percent":100*drops/attempted if attempted else None}

def packet_loss(stage):
    # A staged 5205 waveform is ONE 300-second sample. Never compute
    # cumulative qdisc counters across a qdisc change, since tc may reset
    # them; use each phase's own before/after snap instead.
    if any(x.get("event")=="pre_start" for x in stage):
        duration=next(x for x in stage if x["event"]=="business_start")["configured_duration_s"]
        if duration not in (120,300):
            raise ValueError("unsupported waveform duration")
        periods=[("pre",0,duration//4,5),("stress",duration//4,3*duration//4,20),
                 ("post",3*duration//4,duration,5)]
        starts={x["event"]:x for x in stage}
        base=starts["business_start"]["monotonic_ns"]
        phases={}
        for name,begin,end,want in periods:
            begin_row=starts[name+"_start"]
            end_row=starts[name+"_end"]
            begin_ns=begin_row["monotonic_ns"]
            end_ns=end_row["monotonic_ns"]
            if (abs((begin_ns-base)/1e9-begin)>2.0 or
                abs((end_ns-base)/1e9-end)>2.0 or end_ns <= begin_ns):
                raise ValueError("5205 stage timing mismatch: "+name)
            if any(begin_row["loss_percent"].get(side)!=want or
                   end_row["loss_percent"].get(side)!=want for side in ("c2s","s2c")):
                raise ValueError("5205 loss stage label mismatch: "+name)
            phases[name]={"expected_percent":want,"start_offset_s":(begin_ns-base)/1e9,
                          "end_offset_s":(end_ns-base)/1e9,
                          "c2s":_qdisc_interval(begin_row,end_row,"c2s"),
                          "s2c":_qdisc_interval(begin_row,end_row,"s2c")}
        out={}
        for side in ("c2s","s2c"):
            passed=sum(phase[side]["passed"] for phase in phases.values())
            drops=sum(phase[side]["dropped"] for phase in phases.values())
            denom=passed+drops
            out[side]={"passed":passed,"dropped":drops,"attempted":denom,
                       "realized_percent":100*drops/denom if denom else None}
        return out,phases
    start=next(x for x in stage if x["event"]=="business_start")
    end=next(x for x in stage if x["event"]=="business_end")
    return {side:_qdisc_interval(start,end,side) for side in ("c2s","s2c")},{}

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

def phase_delivery(src,dst,kind):
    """Actual send-time-attributed first-delivery ledger, not qdisc inference.

    'pre/stress/post' belong to the sender's monotonic timestamp embedded
    in the real business UDP wire; a delayed packet stays in its original
    damage phase, rather than being reassigned to the arrival phase.
    """
    tags=("pre","stress","post")
    tx=src.get(kind+"_stage_tx",{})
    rx=dst.get(kind+"_stage_rx",{})
    if not isinstance(tx,dict) or not isinstance(rx,dict):
        raise ValueError("malformed "+kind+" stage telemetry")
    if any(name not in tx for name in tags) or any(name not in rx for name in tags):
        raise ValueError("missing "+kind+" stage telemetry in real endpoint receipts")
    out={}
    for name in tags:
        send=tx[name];received=rx[name]
        n=send.get("count",0);ok=received.get("count",0)
        if not isinstance(n,int) or not isinstance(ok,int) or n<0 or ok<0 or ok>n:
            raise ValueError("invalid "+kind+" phase counts "+name)
        sent_sizes=send.get("by_size",{});recv_sizes=received.get("by_size",{})
        sizes={}
        for k in set(sent_sizes)|set(recv_sizes):
            a=sent_sizes.get(k,0);b=recv_sizes.get(k,0)
            if b>a or a<0 or b<0:
                raise ValueError("invalid "+kind+" phase size "+name+"/"+k)
            sizes[k]={"sent":a,"delivered":b,"missing":a-b}
        if sum(v["sent"] for v in sizes.values())!=n or sum(v["delivered"] for v in sizes.values())!=ok:
            raise ValueError("inconsistent "+kind+" phase-by-size count "+name)
        over1=received.get("over_1s",0);over3=received.get("over_3s",0)
        if not (0<=over3<=over1<=ok):
            raise ValueError("invalid "+kind+" deadline counters "+name)
        out[name]={"sent":n,"delivered":ok,"missing":n-ok,
                   "over_1s":over1,"over_3s":over3,
                   "max_delivered_age_ms":received.get("max_delay_ns",0)/1e6,
                   "by_size":sizes}
    if (tx.get("outside",{}).get("count",0)!=0 or
        rx.get("outside",{}).get("count",0)!=0):
        raise ValueError(kind+" sent timestamp outside 300s business")
    return out

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

def bucket_gap(src,dst,delay_ms=300,duration_s=300):
    sent={int(k):v for k,v in src.get("send_bucket_10ms",{}).items()}
    recv={int(k):v for k,v in dst.get("receive_bucket_10ms",{}).items()}
    # Active-gap only when sender was injecting near the matching 600ms+ delay.
    runs=[];active=0;longest=0;start=None
    for i in range(0,duration_s*100):
        if recv.get(i,0)==0 and any(sent.get(j,0)>0 for j in range(max(0,i-max(10,int((2*delay_ms+100)/10))),max(0,i-max(1,int(delay_ms/10))))):
            if start is None:start=i
            active+=1
        else:
            if start is not None:
                runs.append([start,i,(i-start)*10])
                longest=max(longest,i-start);start=None
    if start is not None:
        runs.append([start,duration_s*100,(duration_s*100-start)*10]);longest=max(longest,duration_s*100-start)
    return {"active_sender_zero_recv_buckets":active,"longest_active_gap_ms":longest*10,
            "largest_gap_windows":sorted(runs,key=lambda x:-x[2])[:10],
            "method":f"10ms empty receiver with prior sender activity, delay={delay_ms}ms; diagnostic, NOT definitive HOL"}

def main():
    a=argparse.ArgumentParser()
    a.add_argument("--artifact-dir",required=True);a.add_argument("--source",required=True)
    a.add_argument("--helper",required=True);a.add_argument("--workload",choices=["udp","tcp","mixed"],required=True)
    a.add_argument("--loss",type=int,choices=[0,1,5,10,20,30,5205],required=True)
    a.add_argument("--duration-s",type=int,default=300,choices=[15,120,300])
    a.add_argument("--delay-ms",type=int,default=300,choices=[15,50,100,150,300])
    a.add_argument("--fec-parity",type=int,default=20,choices=[0,4,8,10,12,16,20])
    a.add_argument("--fec-experiment",action="store_true")
    a.add_argument("--seed",type=int,required=True);a.add_argument("--output",required=True)
    a.add_argument("--target-mbps",type=float,choices=[3.0,10.0,20.0,30.0,50.0],required=True)
    a.add_argument("--size-profile",choices=["ordinary","jumbo","boundary"],required=True)
    a.add_argument("--mode",choices=["normal","game"],required=True)
    a.add_argument("--lanes",type=int,choices=[1,2,3,4],required=True)
    a.add_argument("--diagnostic-mode",choices=["0","1"],required=True)
    a.add_argument("--sweep-experiment",action="store_true")
    a.add_argument("--simd-ab",action="store_true")
    x=a.parse_args();root=Path(x.artifact_dir)
    issues=[];capture_receipts=[]
    try:
        manifest=json.loads((root/"manifest.json").read_text())
        if manifest["source_sha"]!=x.source or manifest["harness_sha"]!=x.helper:issues.append("SOURCE_HELPER_MISMATCH")
        if (manifest["config"]["duration_s"]!=x.duration_s or manifest["config"]["outer_connection_mtu"]!=1400 or manifest["config"].get("drain_s")!=3 or manifest["config"].get("formal_default_tick_ms")!=100 or manifest["config"].get("record_limit_configured")!=0):issues.append("WRONG_MANIFEST")
        if (manifest["config"].get("one_way_delay_ms")!=x.delay_ms or manifest["config"].get("fec_parity")!=x.fec_parity):issues.append("WRONG_DELAY_OR_FEC_MANIFEST")
        if manifest["scenario"]!=x.workload+"-p"+str(x.loss):issues.append("WRONG_WORKLOAD_LOSS")
        if (manifest.get("mode")!=x.mode or manifest["config"].get("lanes")!=x.lanes or
            manifest["config"].get("application_mbps_each_direction")!=x.target_mbps):
            issues.append("WRONG_MODE_LANES_RATE")
        biz=json.loads((root/"biz.json").read_text());target=json.loads((root/"target.json").read_text())
        if any(d["source_sha"]!=x.source or d["helper_sha"]!=x.helper or d["seed"]!=x.seed or d.get("size_profile")!=x.size_profile or d.get("configured_per_direction_mbps")!=x.target_mbps or d.get("duration_seconds")!=x.duration_s for d in (biz,target)):issues.append("PROCESS_IDENTITY")
        b=biz["counters"];t=target["counters"]
        if x.fec_experiment:
            if not ((x.simd_ab and x.workload=="mixed" and x.duration_s in (15,120,300)
                      and x.loss in (0,1,5205) and x.delay_ms in (15,300)
                      and (x.mode,x.lanes,x.target_mbps) in (("normal",1,10),("game",2,3))
                      and (x.loss!=5205 or (x.duration_s in (120,300) and x.delay_ms==300)))
                  or (x.sweep_experiment and x.workload=="udp" and x.duration_s==120
                      and x.loss in (1,5,10) and x.delay_ms in (50,100,150)
                      and x.target_mbps in (10,20,30,50) and x.mode=="normal" and x.lanes==1 and x.fec_parity==0)
                 or (not x.simd_ab and not x.sweep_experiment and x.duration_s in (15,120)
                     and x.loss in (0,1,5) and x.delay_ms in (15,300)
                     and x.mode=="normal" and x.lanes==1)):
                issues.append("INVALID_EXPERIMENT_TUPLE")
            flags=json.loads((root/"runtime-flags.json").read_text())
            for role in ("client","server"):
                if flags.get(role,{}).get("--fec-parity")!=str(x.fec_parity) or flags.get(role,{}).get("--mtu")!="1400":
                    issues.append("ACTUAL_PRODUCT_CLI_FEC_OR_MTU_MISMATCH_"+role)
            if any(row.get("configured_delay_ms")!=x.delay_ms or row.get("configured_duration_s")!=x.duration_s for row in [json.loads(l) for l in (root/"stage-events.jsonl").read_text().splitlines() if l.strip()]):
                issues.append("ACTUAL_STAGE_DELAY_DURATION_MISMATCH")
        stages=[json.loads(l) for l in (root/"stage-events.jsonl").read_text().splitlines()]
        netem,loss_stages=packet_loss(stages)
        # Enforce actual on-wire impairment stage timing. A manifest claiming
        # drain_s=3 cannot silently keep real endpoints/tc running for 15s.
        stage_start=next(row["monotonic_ns"] for row in stages if row["event"]=="business_start")
        stage_end=next(row["monotonic_ns"] for row in stages if row["event"]=="business_end")
        drain_end=next(row["monotonic_ns"] for row in stages if row["event"]=="drain_end")
        if not ((x.duration_s-0.5)*1e9<=stage_end-stage_start<=(x.duration_s+0.5)*1e9):
            issues.append("BUSINESS_STAGE_DURATION_MISMATCH")
        if not (2_500_000_000<=drain_end-stage_end<=3_500_000_000):
            issues.append("DRAIN_STAGE_DURATION_MISMATCH")
        if any(z["attempted"]<100 for z in netem.values()):issues.append("NETEM_NO_EFFECTIVE_TRAFFIC")
        if x.loss==5205:
            expected=[("pre",5),("stress",20),("post",5)]
            duration=x.duration_s
            expected_wave=[[0,duration//4,5],[duration//4,3*duration//4,20],
                           [3*duration//4,duration,5]]
            if manifest["config"].get("stages_s")!=expected_wave:
                issues.append("5205_MANIFEST_WAVEFORM_MISMATCH")
            if [tuple((name,loss_stages.get(name,{}).get("expected_percent"))) for name,_ in expected]!=expected:
                issues.append("5205_STAGE_MISSING_OR_WRONG")
            for name,want in expected:
                for side in ("c2s","s2c"):
                    q=loss_stages[name][side]
                    if q["attempted"]<100 or q["realized_percent"] is None or abs(q["realized_percent"]-want)>2.0:
                        issues.append("5205_REALIZED_LOSS_MISMATCH_"+side+"_"+name)
        else:
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
            received=dst["received_bytes"];requested=x.target_mbps*1e6/8*x.duration_s
            achieved=sent/requested
            direction[key]={"offered_bytes":offered,"sent_bytes":sent,"receiver_bytes":received,
                            "sent_mbps":sent*8/x.duration_s/1e6,"goodput_mbps":received*8/x.duration_s/1e6,
                            "target_mbps":x.target_mbps,"duration_s":x.duration_s,"delay_ms":x.delay_ms,"fec_parity":x.fec_parity,"target_achievement":achieved,
                            "injection_skipped_bytes":src.get("skipped_bytes",0),
                            "generator_send_errors":src["send_errors"],
                            "source_corrupt_or_malformed_count":src.get("corrupt",0),
                            "receiver_corrupt_or_malformed_count":dst.get("corrupt",0),
                            "source_probe_send_errors":src.get("probe_send_errors",0),
                            "udp_by_size":sz,"large_udp_roundtrip_ms":rtt,
                            "tcp":tcp,"continuity":bucket_gap(src,dst,x.delay_ms,x.duration_s)}
            if x.loss==5205 and x.workload in ("udp","mixed"):
                # Require actual per-send-phase business delivery and deadline
                # measurements, not just the pre/stress/post netem qdisc trace.
                try:
                    udp_phases=phase_delivery(src,dst,"udp")
                    # A probe is an echo: the SOURCE counts its own reply.
                    # dst receives kind=3 and sends kind=4 but does not
                    # record this probe as dst.probe_received.
                    probe_phases=phase_delivery(src,src,"probe")
                    tx_udp=sum(v["sent_datagrams"] for v in sz.values())
                    rx_udp=sum(v["delivered"] for v in sz.values())
                    if (sum(row["sent"] for row in udp_phases.values())!=tx_udp
                        or sum(row["delivered"] for row in udp_phases.values())!=rx_udp
                        or sum(row["sent"] for row in probe_phases.values())!=src["probe_sent"]
                        or sum(row["delivered"] for row in probe_phases.values())!=src["probe_received"]):
                        raise ValueError("stage counters disagree with business size/probe totals")
                    direction[key]["phase_delivery_5205"]={
                        "udp":udp_phases,"probe":probe_phases,
                        "attribution":"actual_sender_monotonic_timestamp_not_arrival_stage",
                        "max_due_ms":3000}
                    for phase in ("pre","stress","post"):
                        udp=udp_phases[phase];probe=probe_phases[phase]
                        if udp["sent"]<100 or probe["sent"]<100:
                            issues.append("5205_STAGE_COVERAGE_"+key+"_"+phase)
                        if udp["missing"]>udp["sent"]*0.001:
                            issues.append("5205_STAGE_UDP_LOSS_OVER_0P1PCT_"+key+"_"+phase)
                        if probe["missing"]>max(1,probe["sent"]*0.01):
                            issues.append("5205_STAGE_PROBE_LOSS_OVER_1PCT_"+key+"_"+phase)
                        if udp["over_3s"] or probe["over_3s"]:
                            issues.append("5205_STAGE_DELIVERY_OVER_3S_"+key+"_"+phase)
                except ValueError as err:
                    direction[key]["phase_delivery_5205"]={"status":"INVALID",
                       "reason":str(err)}
                    issues.append("5205_PHASE_DELIVERY_INVALID_"+key)
            tx_stage=src.get("udp_stage_tx",{})
            rx_stage=dst.get("udp_stage_rx",{})
            utx=sum(v.get("count",0) for k,v in tx_stage.items() if k!="outside")
            urx=sum(v.get("count",0) for k,v in rx_stage.items() if k!="outside")
            o1=sum(v.get("over_1s",0) for k,v in rx_stage.items() if k!="outside")
            o3=sum(v.get("over_3s",0) for k,v in rx_stage.items() if k!="outside")
            direction[key]["udp_deadline"]={"sent":utx,"delivered":urx,"missing":max(0,utx-urx),
                "within_1s":max(0,urx-o1),"within_3s":max(0,urx-o3),
                "rate_all_sent_1s":(urx-o1)/utx if utx else None,
                "rate_all_sent_3s":(urx-o3)/utx if utx else None,
                "late_over_1s":o1,"late_over_3s":o3}
            if achieved<.99:issues.append("INSUFFICIENT_INJECTION_"+key)
            if src["send_errors"]:issues.append("GENERATOR_SEND_ERROR_"+key)
            if src.get("corrupt",0) or dst.get("corrupt",0):issues.append("PAYLOAD_CORRUPT_OR_MALFORMED_"+key)
            if src.get("probe_send_errors",0):issues.append("PROBE_SEND_ERROR_"+key)
            if tcp["mismatch_total"]:issues.append("TCP_HASH_MISMATCH_"+key)
            if x.workload in ("tcp","mixed") and any(str(i) not in src.get("tcp_tx",{}) for i in range(4)):
                issues.append("TCP_FOUR_SUSTAINED_FLOWS_MISSING_"+key)
            if x.loss==0 and any(v["missing"] for v in sz.values()):issues.append("LOSSLESS_UDP_MISSING_"+key)
            if x.loss==5205:
                sent_udp=sum(v["sent_datagrams"] for v in sz.values())
                missed_udp=sum(v["missing"] for v in sz.values())
                if sent_udp<1000 or missed_udp>sent_udp*0.001:
                    issues.append("5205_UDP_EVENTUAL_LOSS_OVER_0P1PCT_"+key)
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
            events=side.get("probe_events",[])
            p1=sum(0<=rec-sent<=1_000_000_000 for _,sent,rec in events)
            p3=sum(0<=rec-sent<=3_000_000_000 for _,sent,rec in events)
            probe_summary[name]["deadline"]={
                "all_sent":side["probe_sent"],"returned_within_1s":p1,
                "returned_within_3s":p3,
                "on_time_1s_rate_all_sent":p1/side["probe_sent"] if side["probe_sent"] else None,
                "on_time_3s_rate_all_sent":p3/side["probe_sent"] if side["probe_sent"] else None,
                "returned_after_3s":max(0,side["probe_received"]-p3),
                "never_returned_or_timeout":max(0,side["probe_sent"]-side["probe_received"])}
            if len(events)!=side["probe_received"]:issues.append("INCOMPLETE_PROBE_TIMING_"+name)
            if side["probe_sent"]<max(50,int(x.duration_s*5*.90)):issues.append("PROBE_COVERAGE_"+name)
            if x.loss==0 and probe_summary[name]["missing"]:issues.append("LOSSLESS_PROBE_MISSING_"+name)
            if x.loss==5205 and probe_summary[name]["missing"]>max(1,int(side["probe_sent"]*0.01)):
                issues.append("5205_PROBE_TIMEOUT_OVER_1PCT_"+name)
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
        requested_bytes=int(x.target_mbps*125000*x.duration_s)
        for side in ("c2s","s2c"):
            item=direction[side]
            item["http_https_sent_bytes_separately"]=web_sent[side]
            item["total_logical_sent_bytes_including_http"]=item["sent_bytes"]+web_sent[side]
            item["total_logical_delivered_bytes_including_verified_http"]=item["receiver_bytes"]+web_sent[side]
            item["total_logical_sent_mbps_including_http"]=item["total_logical_sent_bytes_including_http"]*8/x.duration_s/1e6
            if item["total_logical_sent_bytes_including_http"]>requested_bytes+8192:
                issues.append("LOGICAL_BUSINESS_RATE_OVER_BUDGET_"+side)
        resources=resource_report(root,int(biz["start_monotonic_ns"]),int(biz["start_monotonic_ns"])+x.duration_s*1_000_000_000)
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
            # Game2, Game3 and Game4 must be measured as REAL lanes, not
            # inferred merely from the selected command-line mode. Game
            # profile-OFF deliberately lacks these counters and therefore
            # needs an independent profile-ON qualification per lane count.
            if x.mode=="game":
                refs=[lane.get("ref") or {} for lane in lanes]
                identities=[(r.get("ID"),r.get("Generation")) for r in refs]
                valid_ids={r.get("ID") for r in refs}
                if (x.lanes not in (2,3,4) or owner.get("DesiredLanes")!=x.lanes
                    or owner.get("ActiveLogicalLanes")!=x.lanes
                    or owner.get("PhysicalLanes")!=x.lanes
                    or len(lanes)!=x.lanes or len(set(identities))!=x.lanes
                    or len(valid_ids)!=x.lanes or None in valid_ids
                    or any(not isinstance(r.get("Generation"),int) or r["Generation"]<1 for r in refs)
                    or (owner.get("GameLogicalOutbound") or 0)<=0
                    or (owner.get("GameLaneCopies") or 0)<=0):
                    issues.append("GAME_ACTUAL_LANES_NOT_QUALIFIED_"+diag_side)
        if resources["strict_resource"].get("errors"):
            issues.append("RESOURCE_AUDIT_WARNINGS")
        if resources["strict_resource"].get("socket_drop_max",0):
            issues.append("LOCAL_SOCKET_DROP")
        if any(v.get("rx",0)>0 or v.get("tx",0)>0 for v in resources["strict_resource"].get("link_drop_delta",{}).values()):
            issues.append("LOCAL_INTERFACE_DROP")
        result={"schema":"wbd-large-mtu-analysis/v1","product_source_sha":x.source,
                "helper_sha":x.helper,"workload":x.workload,"loss_percent":x.loss,"seed":x.seed,"size_profile":x.size_profile,"target_mbps":x.target_mbps,"duration_s":x.duration_s,"delay_ms":x.delay_ms,"fec_parity":x.fec_parity,
                "netem_realized":netem,"netem_stages":loss_stages,"mtu":links,"route_mode":"all",
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
    outer={}
    for side,tap in (("c2s","c2s-pre"),("s2c","s2c-pre")):
        receipts=[v for v in capture_receipts if v["filename"].startswith(tap+".pcap")]
        observed=sum((v.get("packet_distribution") or {}).get("ip_packets",0) for v in receipts)
        ipbytes=sum((v.get("packet_distribution") or {}).get("ip_bytes",0) for v in receipts)
        q=(result.get("netem_realized") or {}).get(side) or {}
        attempted=q.get("attempted")
        # Circular pcap loss/overflow can make a source trace incomplete.
        complete=bool(attempted and observed>=.95*attempted)
        outer[side]={"capture_ip_packets":observed,"capture_ip_bytes":ipbytes,
            "qdisc_attempted_packets":attempted,"qdisc_passed_packets":q.get("passed"),
            "netem_dropped_packets":q.get("dropped"),
            "wire_byte_accounting":"ESTIMATED_FULL_CAPTURE" if complete else "NOT_COLLECTED_FULL_WINDOW",
            "full_window_outer_ip_bytes":ipbytes if complete else None}
    result["outer_wire_observation"]=outer
    result.update({"capture_cleanup":{"deleted_raw_pcap":True,"capture_receipts":capture_receipts},
                   "issues":issues,
                   "classification":"INVALID" if any(y.startswith(("ANALYZER_EXCEPTION","PROCESS_IDENTITY","SOURCE_HELPER","WRONG_MANIFEST","WRONG_WORKLOAD","CAPTURE_PARSE")) for y in issues) else
                   "CAPACITY_LIMITED" if any(z.startswith("INSUFFICIENT_INJECTION") for z in issues) and result.get("resources",{}).get("capacity_limited_evidenced") else
                   "FAIL" if issues else ("VALID_OBSERVATION" if x.fec_experiment else "PASS_SCOPED_ACTIONS"),
                   "never_physical_pass":True})
    Path(x.output).write_text(json.dumps(result,indent=2,sort_keys=True))
    print("WBD_LARGE_MTU_ANALYSIS",result["classification"],"issues",issues,flush=True)
    if result["classification"] not in ("PASS_SCOPED_ACTIONS","VALID_OBSERVATION"):raise SystemExit(1)

if __name__=="__main__":main()
