#!/usr/bin/env python3
import argparse
import json
import re
from pathlib import Path

def load(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))

def capture_drop(path):
    text=Path(path).read_text(encoding="utf-8",errors="replace")
    m=re.findall(r"(\d+) packets dropped by kernel",text)
    return int(m[-1]) if m else None

def qdisc_row(path):
    rows=load(path)
    return rows[0] if isinstance(rows,list) and rows else {}

def delta_qdisc(root, stem, duration):
    a=qdisc_row(root/f"{stem}-before.json"); b=qdisc_row(root/f"{stem}-after.json")
    out={}
    for key in ("packets","bytes","drops","overlimits","requeues"):
        out[key]=int(b.get(key,0) or 0)-int(a.get(key,0) or 0)
    out["mbps"]=out["bytes"]*8/duration/1e6
    out["pps"]=out["packets"]/duration
    out["backlog_after"]=int(b.get("backlog",0) or 0)
    out["qlen_after"]=int(b.get("qlen",0) or 0)
    return out

def link_totals(path):
    rows=load(path)
    total={"rx_dropped":0,"tx_dropped":0,"rx_errors":0,"tx_errors":0}
    for row in rows if isinstance(rows,list) else []:
        if row.get("ifname")=="lo": continue
        st=row.get("stats64") or row.get("stats") or {}
        rx=st.get("rx") or {}; tx=st.get("tx") or {}
        total["rx_dropped"] += int(rx.get("dropped",0) or 0)
        total["tx_dropped"] += int(tx.get("dropped",0) or 0)
        total["rx_errors"] += int(rx.get("errors",0) or 0)
        total["tx_errors"] += int(tx.get("errors",0) or 0)
    return total

def resource_socket_drops(path):
    max_by={}
    for line in Path(path).read_text(encoding="utf-8",errors="replace").splitlines():
        if not line.strip(): continue
        row=json.loads(line)
        for label,ns in (row.get("namespaces") or {}).items():
            text=((ns.get("ss_udp") or {}).get("stdout") or "")
            vals=[int(x) for x in re.findall(r"skmem:\([^\n)]*\bd(\d+)\)",text)]
            if vals: max_by[label]=max(max_by.get(label,0),max(vals))
    return max_by

def gen_stage(sender,receiver,target,duration):
    ss=sender["stats"]; rs=receiver["stats"]
    sent_b=sum(ss["sent_bytes_by_second"]); sent_p=sum(ss["sent_packets_by_second"])
    recv_b=sum(rs["recv_bytes_by_second"]); recv_p=sum(rs["recv_packets_by_second"])
    expected=target*1e6/8*duration
    return {
      "sent_bytes":sent_b,"sent_packets":sent_p,"recv_unique_bytes":recv_b,"recv_unique_packets":recv_p,
      "send_mbps":sent_b*8/duration/1e6,"goodput_mbps":recv_b*8/duration/1e6,
      "send_target_ratio":sent_b/expected if expected else None,
      "packet_loss_percent":100*(sent_p-recv_p)/sent_p if sent_p else None,
      "send_failures":ss.get("send_failures",0),"skipped_slots":ss.get("skipped_slots",0),
      "send_lag_p99_ns":ss.get("send_lag_p99_ns"),
    }

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--artifact-dir",required=True)
    ap.add_argument("--source-sha",required=True)
    ap.add_argument("--target-mbps",type=float,required=True)
    ap.add_argument("--output",required=True)
    args=ap.parse_args()
    root=Path(args.artifact_dir)
    biz=load(root/"biz.json"); target=load(root/"target.json")
    duration=float(biz["duration_s"])
    c2s=gen_stage(biz,target,args.target_mbps,duration)
    s2c=gen_stage(target,biz,args.target_mbps,duration)
    errors=[]
    for name,row,sender,receiver in (("c2s",c2s,biz,target),("s2c",s2c,target,biz)):
        if row["send_target_ratio"] is None or not 0.99 <= row["send_target_ratio"] <= 1.01:
            errors.append(f"{name} send_target_ratio={row['send_target_ratio']}")
        if row["send_failures"] != 0:
            errors.append(f"{name} send_failures={row['send_failures']}")
        max_skip=max(1,int(row["sent_packets"]*0.0001))
        if row["skipped_slots"] > max_skip:
            errors.append(f"{name} skipped_slots={row['skipped_slots']} > {max_skip}")
        if row["send_lag_p99_ns"] is None or row["send_lag_p99_ns"] > 10_000_000:
            errors.append(f"{name} send_lag_p99_ns={row['send_lag_p99_ns']}")
        if row["recv_unique_packets"] != row["sent_packets"] or row["recv_unique_bytes"] != row["sent_bytes"]:
            errors.append(f"{name} final_delivery sent={row['sent_packets']}/{row['sent_bytes']} recv={row['recv_unique_packets']}/{row['recv_unique_bytes']}")
        rs=receiver["stats"]
        if rs.get("corrupt",0) or rs.get("unexpected",0) or rs.get("recv_duplicates",0):
            errors.append(f"{name} receiver corrupt={rs.get('corrupt')} unexpected={rs.get('unexpected')} duplicates={rs.get('recv_duplicates')}")
    probe=biz["stats"]
    probe_loss=100*probe.get("probe_timeouts",0)/max(1,probe.get("probe_sent",0))
    if probe_loss > 1.0:
        errors.append(f"probe_loss_percent={probe_loss}")

    capture={}
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post"):
        capture[stem]=capture_drop(root/f"{stem}.tcpdump.log")
        if capture[stem] != 0:
            errors.append(f"{stem} capture_drop={capture[stem]}")

    qdisc={
      "c2s":delta_qdisc(root,"qdisc-rsrv",duration),
      "s2c":delta_qdisc(root,"qdisc-rcli",duration),
    }
    for name,row in qdisc.items():
        if row["drops"] or row["overlimits"] or row["backlog_after"] or row["qlen_after"]:
            errors.append(f"{name} qdisc={row}")

    links={}
    for label in ("biz","client","router","server","target"):
        a=link_totals(root/f"{label}-links-before.json")
        b=link_totals(root/f"{label}-links-after.json")
        links[label]={k:b[k]-a[k] for k in a}
        if any(links[label].values()):
            errors.append(f"{label} link_delta={links[label]}")

    socket_drops=resource_socket_drops(root/"resources.jsonl")
    nonzero={k:v for k,v in socket_drops.items() if v}
    if nonzero:
        errors.append(f"socket_skmem_drop={nonzero}")

    result={
      "schema":1,"source_sha":args.source_sha,"kind":"same-topology-product-bypass",
      "target_mbps_each_direction":args.target_mbps,
      "generator":{"c2s":c2s,"s2c":s2c},
      "probe_loss_percent":probe_loss,
      "qdisc":qdisc,"capture_drop":capture,"link_drop_error_delta":links,
      "socket_skmem_drop_max":socket_drops,
      "errors":errors,"result":"PASS" if not errors else "FAIL",
      "qualification_note":"diagnostic only; PASS proves the measured Actions netns/veth/netem/capture path at this offered rate, not WBD product qualification",
    }
    Path(args.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("WBD_STRICT_CAPACITY_BYPASS "+json.dumps(result,sort_keys=True))
    raise SystemExit(0 if not errors else 1)

if __name__=="__main__":
    main()
