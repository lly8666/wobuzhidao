#!/usr/bin/env python3
"""No-product C10 BPF GE event-map 1ms-boundary kernel capability probe.

Preloaded 64 alternating 100ms event boundaries, NOT exponential CTMC.
GOOD accepts all 32 packets; BAD accepts about half of 32 packets via
independent kernel PRNG. Wall clock, never packet count, chooses the state.
"""
import argparse,json,socket,time
from pathlib import Path
CYCLES=32
PERIOD=200_000_000
PORT=19112

def send(epoch,output):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    packets=[]
    if epoch<time.monotonic_ns()+150_000_000:
        raise ValueError("kernel map epoch must be loaded well before first sender packet")
    for i in range(CYCLES):
        for tag,offset in (("G",35_000_000),("B",135_000_000)):
            goal=epoch+i*PERIOD+offset
            delay=(goal-time.monotonic_ns())/1e9
            if delay>0:time.sleep(delay)
            at=time.monotonic_ns()
            label=f"{tag}{i:02d}"
            s.sendto(label.encode(),("198.19.20.2",PORT))
            packets.append({"id":label,"actual_ns":at,
                            "phase_ms":(at-epoch)//1_000_000})
    s.close()
    output.write_text(json.dumps({"start_ns":epoch,"events":packets},indent=2)+"\n")

def receive(output,epoch):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    s.bind(("198.19.20.2",PORT))
    s.settimeout(.5)
    deadline=(epoch+7_000_000_000)/1e9
    seen=[]
    while time.monotonic()<deadline:
        try:payload,_=s.recvfrom(128)
        except socket.timeout:continue
        seen.append({"id":payload.decode(),"received_ns":time.monotonic_ns()})
    s.close()
    output.write_text(json.dumps({"events":seen},indent=2)+"\n")

def gate(sent,recv):
    epoch=sent["start_ns"]
    good_bad={x["id"] for x in sent["events"]}
    actual={x["id"] for x in recv["events"]}
    good=sum(x.startswith("G") for x in actual)
    bad=sum(x.startswith("B") for x in actual)
    deviations=[x["id"] for x in sent["events"]
        if (x["phase_ms"]//100)%2 != (1 if x["id"][0]=="B" else 0)]
    return {"schema":1,"classification":
            "C10_BPF_EVENT_BOUNDARY_1MS_AND_BAD50PCT_CAPABILITY_PASS"
            if len(good_bad)==64 and actual<=good_bad and
               len(actual)==len(recv["events"]) and
               good>=30 and 7<=bad<=25 and not deviations
            else "C10_BPF_EVENT_BOUNDARY_CAPABILITY_FAIL",
            "good_sent":32,"bad_sent":32,"good_received":good,
            "bad_received":bad,"phase_mismatch_labels":deviations,
            "not_exponential_CTMC":True,
            "product_performance_jobs":0}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("mode",choices=["send","receive","verify","unit"])
    p.add_argument("--start-ns",type=int)
    p.add_argument("--sent",type=Path)
    p.add_argument("--received",type=Path)
    p.add_argument("--output",type=Path)
    a=p.parse_args()
    if a.mode=="unit":
        fake={"start_ns":100,"events":[{"id":f"{k}{i:02d}",
              "phase_ms":200*i+(35 if k=="G" else 135)}
              for i in range(CYCLES) for k in ("G","B")]}
        rec={"events":[{"id":f"G{i:02d}"} for i in range(32)]+
                       [{"id":f"B{i:02d}"} for i in range(16)]}
        assert gate(fake,rec)["classification"].endswith("_PASS")
        rec={"events":[{"id":f"G{i:02d}"} for i in range(32)]+
                       [{"id":f"B{i:02d}"} for i in range(32)]}
        assert gate(fake,rec)["classification"].endswith("_FAIL")
        print("WBD_C10_GE_MAP_SMOKE_GATE_UNIT_PASS")
    elif a.mode=="send":send(a.start_ns,a.output)
    elif a.mode=="receive":receive(a.output,a.start_ns)
    else:
        result=gate(json.loads(a.sent.read_text()),json.loads(a.received.read_text()))
        a.output.write_text(json.dumps(result,sort_keys=True,indent=2)+"\n")
        print("WBD_"+result["classification"])
        if not result["classification"].endswith("_PASS"):raise SystemExit(1)
if __name__=="__main__":main()
