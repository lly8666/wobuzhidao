#!/usr/bin/env python3
"""C10 time-drive capability smoke: 16 good + 16 bad kernel-time UDP sends.

No product binaries, weaknet measurement, or simulated business throughput.
Kernel TC BPF reads CLOCK_MONOTONIC at transmission time. Python independently
timestamps intended phases; the return receipt distinguishes GOOD from BAD.
"""
import argparse
import json
import socket
import time
from pathlib import Path

NS_PER_PERIOD=200_000_000
OFFSET_GOOD=35_000_000
OFFSET_BAD=135_000_000
PACKETS_PER_STATE=16
PORT=19111

def schedule(base,cycles=PACKETS_PER_STATE):
    if cycles<1 or cycles>32:
        raise ValueError("bounded smoke cycles required")
    out=[]
    for i in range(cycles):
        out.extend([(base+i*NS_PER_PERIOD+OFFSET_GOOD,"G",i),
                    (base+i*NS_PER_PERIOD+OFFSET_BAD,"B",i)])
    return out

def score(sent,received):
    labels=[x["id"] for x in sent]
    arrivals=[x["id"] for x in received]
    if len(labels)!=32 or len(set(labels))!=32 or len(set(arrivals))!=len(arrivals):
        raise ValueError("invalid test packet count or duplicate")
    good=[x for x in labels if x.startswith("G")]
    bad=[x for x in labels if x.startswith("B")]
    good_received=[x for x in arrivals if x in good]
    bad_received=[x for x in arrivals if x in bad]
    return {"schema":1,"classification":
               "KERNEL_TIME_PHASE_EGRESS_VERIFIED" if len(good_received)>=12 and not bad_received
               else "UNSUPPORTED_OR_TIME_PHASE_MISMATCH",
            "good_sent":len(good),"bad_sent":len(bad),
            "good_received":len(good_received),"bad_received":len(bad_received),
            "no_product_performance":True,
            "time_driven_trace_equivalence":"NOT_YET_ESTABLISHED_FOR_FULL_CTMC",
            "capability_only":True}

def sender(output):
    sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    mono=time.monotonic_ns()
    base=((mono//NS_PER_PERIOD)+2)*NS_PER_PERIOD
    emitted=[]
    for target,kind,i in schedule(base):
        sleep=(target-time.monotonic_ns())/1e9
        if sleep>0:time.sleep(sleep)
        at=time.monotonic_ns()
        key=f"{kind}{i:02d}"
        sock.sendto(key.encode("ascii"),("198.19.20.2",PORT))
        emitted.append({"id":key,"intended_time_ns":target,
                        "actual_mono_ns":at,"actual_100ms_phase":(at//100_000_000)&1})
    sock.close()
    output.write_text(json.dumps({"events":emitted},indent=2)+"\n")

def receiver(output):
    sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    sock.bind(("198.19.20.2",PORT))
    sock.settimeout(.5)
    end=time.monotonic()+5.5
    arrivals=[]
    while time.monotonic()<end:
        try:
            payload,_=sock.recvfrom(128)
        except socket.timeout:
            continue
        arrivals.append({"id":payload.decode("ascii"),"received_mono_ns":time.monotonic_ns()})
    sock.close()
    output.write_text(json.dumps({"events":arrivals},indent=2)+"\n")

def main():
    p=argparse.ArgumentParser()
    p.add_argument("mode",choices=["sender","receiver","verify","unit"])
    p.add_argument("--output",type=Path)
    p.add_argument("--sent",type=Path)
    p.add_argument("--received",type=Path)
    args=p.parse_args()
    if args.mode=="unit":
        t=schedule(0)
        assert len(t)==32 and t[0]==(OFFSET_GOOD,"G",0)
        assert t[1]==(OFFSET_BAD,"B",0)
        synthetic_good=[{"id":f"G{i:02d}"} for i in range(16)]
        synthetic_bad=[{"id":f"B{i:02d}"} for i in range(16)]
        assert score(synthetic_good+synthetic_bad,synthetic_good)["classification"]=="KERNEL_TIME_PHASE_EGRESS_VERIFIED"
        assert score(synthetic_good+synthetic_bad,synthetic_good+synthetic_bad)["classification"]=="UNSUPPORTED_OR_TIME_PHASE_MISMATCH"
        print("WBD_GE_TIME_CAPABILITY_PARSER_UNIT_PASS")
        return
    if not args.output:
        p.error("--output is required")
    if args.mode=="sender":sender(args.output)
    elif args.mode=="receiver":receiver(args.output)
    else:
        sent=json.loads(args.sent.read_text())["events"]
        recv=json.loads(args.received.read_text())["events"]
        out=score(sent,recv)
        out["sender_actual_phase_mismatches"]=[
            x["id"] for x in sent
            if x["actual_100ms_phase"]!=(1 if x["id"].startswith("B") else 0)]
        if out["sender_actual_phase_mismatches"]:
            out["classification"]="UNSUPPORTED_OR_TIME_PHASE_MISMATCH"
        args.output.write_text(json.dumps(out,indent=2,sort_keys=True)+"\n")
        print("WBD_GE_TIME_CAPABILITY_"+out["classification"])
        if out["classification"]!="KERNEL_TIME_PHASE_EGRESS_VERIFIED":
            raise SystemExit(1)
if __name__=="__main__":main()
