#!/usr/bin/env python3
import argparse
import json
from collections import defaultdict
from pathlib import Path

def load(path):
    out = []
    for line in Path(path).read_text(errors="replace").splitlines():
        try:
            out.append(json.loads(line))
        except Exception:
            pass
    return out

def phase_labels(samples, spike):
    seen_spike = False
    labels = []
    for s in samples:
        loss = s.get("loss_pct")
        if loss is not None and abs(float(loss) - float(spike)) < 0.01:
            seen_spike = True
            labels.append(f"spike{spike}")
        elif loss is not None and abs(float(loss) - 5.0) < 0.01:
            labels.append("post5" if seen_spike else "pre5")
        else:
            labels.append("setup")
    return labels

def socket_key(s):
    return (
        s.get("role"), s.get("process"), int(s.get("pid",0) or 0),
        int(s.get("inode",0) or 0), s.get("local"), s.get("remote"),
    )

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("input")
    ap.add_argument("output")
    ap.add_argument("--spike", required=True, type=int)
    ap.add_argument("--audit")
    args = ap.parse_args()
    samples = load(args.input)
    phases = phase_labels(samples, args.spike)

    sock = {}
    phase_totals = defaultdict(lambda: {"drops_delta":0, "ss_drops_delta":0, "max_rx_queue_bytes":0})
    prev = {}
    for rec, phase in zip(samples, phases):
        for s in rec.get("sockets", []):
            key = socket_key(s)
            e = sock.setdefault(key, {
                "role":s.get("role"), "process":s.get("process"), "pid":s.get("pid"),
                "inode":s.get("inode"), "local":s.get("local"), "remote":s.get("remote"),
                "fds":s.get("fds"), "so_rcvbuf_effective_bytes":s.get("so_rcvbuf_effective_bytes"),
                "max_rx_queue_bytes":0, "max_skmem_r_bytes":0,
                "drops_delta":0, "ss_drops_delta":0,
                "phase":defaultdict(lambda: {"drops_delta":0, "ss_drops_delta":0, "max_rx_queue_bytes":0}),
            })
            e["fds"] = s.get("fds") or e.get("fds")
            rb = s.get("so_rcvbuf_effective_bytes")
            if rb is not None:
                e["so_rcvbuf_effective_bytes"] = max(int(rb), int(e.get("so_rcvbuf_effective_bytes") or 0))
            e["max_rx_queue_bytes"] = max(e["max_rx_queue_bytes"], int(s.get("rx_queue_bytes",0) or 0))
            e["max_skmem_r_bytes"] = max(e["max_skmem_r_bytes"], int(s.get("skmem_r_bytes",0) or 0))
            e["phase"][phase]["max_rx_queue_bytes"] = max(
                e["phase"][phase]["max_rx_queue_bytes"], int(s.get("rx_queue_bytes",0) or 0))
            phase_totals[phase]["max_rx_queue_bytes"] = max(
                phase_totals[phase]["max_rx_queue_bytes"], int(s.get("rx_queue_bytes",0) or 0))
            old = prev.get(key)
            if old is not None:
                d = max(0, int(s.get("drops",0) or 0) - int(old.get("drops",0) or 0))
                sd = max(0, int(s.get("ss_drops",0) or 0) - int(old.get("ss_drops",0) or 0))
                e["drops_delta"] += d
                e["ss_drops_delta"] += sd
                e["phase"][phase]["drops_delta"] += d
                e["phase"][phase]["ss_drops_delta"] += sd
                phase_totals[phase]["drops_delta"] += d
                phase_totals[phase]["ss_drops_delta"] += sd
            prev[key] = s

    thread_acc = defaultdict(lambda: {"cpu_core_seconds":0.0, "observed_seconds":0.0, "max_cores":0.0, "samples":0})
    prev_thr = {}
    prev_mono = None
    for rec in samples:
        mono = int(rec.get("monotonic_ns",0) or 0)
        dt = (mono - prev_mono)/1e9 if prev_mono else 0.0
        hz = int(rec.get("clk_tck",100) or 100)
        current = {}
        for t in rec.get("threads", []):
            key=(int(t.get("pid",0)),int(t.get("tid",0)),t.get("process"),t.get("comm"))
            current[key]=int(t.get("cpu_ticks",0) or 0)
            if dt > 0 and key in prev_thr:
                delta=max(0,current[key]-prev_thr[key])
                cores=(delta/hz)/dt
                a=thread_acc[key]
                a["cpu_core_seconds"] += cores*dt
                a["observed_seconds"] += dt
                a["max_cores"] = max(a["max_cores"], cores)
                a["samples"] += 1
        prev_thr=current
        prev_mono=mono
    threads=[]
    for (pid,tid,proc,comm),a in thread_acc.items():
        avg=a["cpu_core_seconds"]/a["observed_seconds"] if a["observed_seconds"] else 0.0
        threads.append({"pid":pid,"tid":tid,"process":proc,"comm":comm,"avg_cores":avg,
                        "max_cores":a["max_cores"],"samples":a["samples"]})
    threads.sort(key=lambda x:(x["avg_cores"],x["max_cores"]), reverse=True)

    socket_rows=[]
    for e in sock.values():
        e["phase"]={k:dict(v) for k,v in e["phase"].items()}
        socket_rows.append(e)
    socket_rows.sort(key=lambda x:(x["drops_delta"],x["ss_drops_delta"],x["max_rx_queue_bytes"]), reverse=True)
    out={
        "samples":len(samples),
        "spike_loss_pct":args.spike,
        "phase_totals":{k:dict(v) for k,v in phase_totals.items()},
        "sockets":socket_rows,
        "top_threads":threads[:40],
        "checks":{
            "per_socket_drop_diag_present": bool(socket_rows),
            "so_rcvbuf_observed": any(x.get("so_rcvbuf_effective_bytes") is not None for x in socket_rows),
            "thread_cpu_observed": bool(threads),
        },
    }
    Path(args.output).write_text(json.dumps(out, indent=2, sort_keys=True)+"\n")
    if args.audit and Path(args.audit).exists():
        d=json.loads(Path(args.audit).read_text())
        d["runtime_socket_diag"]=out
        d.setdefault("checks",{}).update(out["checks"])
        Path(args.audit).write_text(json.dumps(d,indent=2,sort_keys=True)+"\n")
    print("WBD_RUNTIME_SOCKET_SUMMARY "+json.dumps({
        "phase_totals":out["phase_totals"],
        "top_sockets":socket_rows[:8],
        "top_threads":threads[:12],
        "checks":out["checks"],
    }, sort_keys=True))

if __name__=="__main__":
    main()
