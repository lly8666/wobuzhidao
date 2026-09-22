#!/usr/bin/env python3
import argparse
import json
import math
import re
import struct
from collections import defaultdict, deque
from pathlib import Path


def pct(values, q):
    if not values:
        return None
    values = sorted(values)
    idx = int(round((len(values) - 1) * q))
    return values[max(0, min(idx, len(values) - 1))]


def parse_pcap(path):
    data = Path(path).read_bytes()
    if len(data) < 24:
        raise ValueError(f"{path}: short pcap")
    magic = data[:4]
    if magic == b"\xd4\xc3\xb2\xa1":
        endian, scale = "<", 1000
    elif magic == b"\xa1\xb2\xc3\xd4":
        endian, scale = ">", 1000
    elif magic == b"\x4d\x3c\xb2\xa1":
        endian, scale = "<", 1
    elif magic == b"\xa1\xb2\x3c\x4d":
        endian, scale = ">", 1
    else:
        raise ValueError(f"{path}: unsupported pcap magic {magic.hex()}")
    _, _, _, _, _, snaplen, network = struct.unpack(endian + "IHHIIII", data[:24])
    if network != 1:
        raise ValueError(f"{path}: linktype={network}, want Ethernet")
    out = []
    off = 24
    while off + 16 <= len(data):
        sec, frac, incl, orig = struct.unpack(endian + "IIII", data[off:off+16])
        off += 16
        frame = data[off:off+incl]
        off += incl
        if len(frame) < 14 + 20 or frame[12:14] != b"\x08\x00":
            continue
        ip = frame[14:]
        ihl = (ip[0] & 0x0f) * 4
        if len(ip) < ihl + 20 or ip[0] >> 4 != 4 or ip[9] != 6:
            continue
        total = struct.unpack("!H", ip[2:4])[0]
        tcp = ip[ihl:]
        doff = ((tcp[12] >> 4) & 0x0f) * 4
        if doff < 20 or total < ihl + doff:
            continue
        sport, dport, seq, ack = struct.unpack("!HHII", tcp[:12])
        flags = tcp[13]
        src = ".".join(str(x) for x in ip[12:16])
        dst = ".".join(str(x) for x in ip[16:20])
        payload = total - ihl - doff
        out.append({
            "ts_ns": sec * 1_000_000_000 + frac * scale,
            "src": src, "dst": dst, "sport": sport, "dport": dport,
            "seq": seq, "ack": ack, "flags": flags,
            "ip_bytes": total, "payload_bytes": payload,
        })
    return out


def capture_drop(path):
    text = Path(path).read_text(encoding="utf-8", errors="replace")
    m = re.search(r"(\d+) packets dropped by kernel", text)
    return None if m is None else int(m.group(1))


def pair_delay(pre, post):
    queues = defaultdict(deque)
    for row in pre:
        key = (row["src"], row["dst"], row["sport"], row["dport"], row["seq"], row["ack"], row["flags"], row["ip_bytes"])
        queues[key].append(row["ts_ns"])
    delays = []
    unmatched_post = 0
    for row in post:
        key = (row["src"], row["dst"], row["sport"], row["dport"], row["seq"], row["ack"], row["flags"], row["ip_bytes"])
        q = queues.get(key)
        if not q:
            unmatched_post += 1
            continue
        delays.append(row["ts_ns"] - q.popleft())
    unmatched_pre = sum(len(q) for q in queues.values())
    return delays, unmatched_pre, unmatched_post


def outer_ledger(rows):
    flows = {}
    fresh_payload = 0
    repair_payload = 0
    ack_control_ip = 0
    total_ip = 0
    payload_ip = 0
    seen = set()
    syns = set()
    for row in rows:
        total_ip += row["ip_bytes"]
        if row["payload_bytes"] == 0:
            ack_control_ip += row["ip_bytes"]
        else:
            payload_ip += row["ip_bytes"]
            key = (row["src"], row["dst"], row["sport"], row["dport"], row["seq"], row["payload_bytes"])
            if key in seen:
                repair_payload += row["payload_bytes"]
            else:
                seen.add(key)
                fresh_payload += row["payload_bytes"]
        if row["flags"] & 0x02 and not (row["flags"] & 0x10):
            syns.add((row["src"], row["dst"], row["sport"], row["dport"]))
        flow = (row["src"], row["dst"], row["sport"], row["dport"])
        f = flows.setdefault(str(flow), {"packets": 0, "ip_bytes": 0, "payload_bytes": 0})
        f["packets"] += 1
        f["ip_bytes"] += row["ip_bytes"]
        f["payload_bytes"] += row["payload_bytes"]
    return {
        "packets": len(rows),
        "outer_ip_bytes": total_ip,
        "payload_bearing_ip_bytes": payload_ip,
        "ack_control_ip_bytes": ack_control_ip,
        "fresh_tcp_payload_bytes": fresh_payload,
        "repair_tcp_payload_bytes": repair_payload,
        "outer_syn_flows": len(syns),
        "flows": flows,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--artifact-dir", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()
    root = Path(args.artifact_dir)
    biz = json.loads((root / "biz.json").read_text())
    target = json.loads((root / "target.json").read_text())
    c2s_pre = parse_pcap(root / "c2s-pre.pcap")
    c2s_post = parse_pcap(root / "c2s-post.pcap")
    s2c_pre = parse_pcap(root / "s2c-pre.pcap")
    s2c_post = parse_pcap(root / "s2c-post.pcap")
    c2s_delay, c2s_missing_pre, c2s_extra_post = pair_delay(c2s_pre, c2s_post)
    s2c_delay, s2c_missing_pre, s2c_extra_post = pair_delay(s2c_pre, s2c_post)

    errors = []
    for name, sender, receiver in [
        ("c2s", biz["stats"], target["stats"]),
        ("s2c", target["stats"], biz["stats"]),
    ]:
        if sender["send_failures"] != 0:
            errors.append(f"{name}: send_failures={sender['send_failures']}")
        if sender["sent_packets"] == 0:
            errors.append(f"{name}: no data sent")
        if receiver["corrupt"] != 0 or receiver["recv_duplicates"] != 0:
            errors.append(f"{name}: corrupt={receiver['corrupt']} duplicates={receiver['recv_duplicates']}")
        if receiver["recv_unique_packets"] != sender["sent_packets"]:
            errors.append(f"{name}: delivered={receiver['recv_unique_packets']} sent={sender['sent_packets']}")
        if receiver["recv_unique_bytes"] != sender["sent_bytes"]:
            errors.append(f"{name}: delivered_bytes={receiver['recv_unique_bytes']} sent_bytes={sender['sent_bytes']}")
        if sender["send_lag_p99_ns"] is None or sender["send_lag_p99_ns"] > 10_000_000:
            errors.append(f"{name}: send_lag_p99_ns={sender['send_lag_p99_ns']}")

    if biz["stats"]["probe_sent"] == 0 or biz["stats"]["probe_recv"] != biz["stats"]["probe_sent"]:
        errors.append(f"probe: recv={biz['stats']['probe_recv']} sent={biz['stats']['probe_sent']}")

    for name, delays, missing, extra in [
        ("c2s", c2s_delay, c2s_missing_pre, c2s_extra_post),
        ("s2c", s2c_delay, s2c_missing_pre, s2c_extra_post),
    ]:
        median = pct(delays, 0.50)
        if missing or extra:
            errors.append(f"{name}: capture pairing missing_pre={missing} extra_post={extra}")
        if median is None or not (280_000_000 <= median <= 340_000_000):
            errors.append(f"{name}: netem median delay={median}")

    capture_drops = {}
    for stem in ("c2s-pre", "c2s-post", "s2c-pre", "s2c-post"):
        value = capture_drop(root / f"{stem}.tcpdump.log")
        capture_drops[stem] = value
        if value not in (0, None):
            errors.append(f"{stem}: capture dropped={value}")
        if value is None:
            errors.append(f"{stem}: missing tcpdump drop accounting")

    c2s_ledger = outer_ledger(c2s_pre)
    s2c_ledger = outer_ledger(s2c_pre)
    if c2s_ledger["outer_syn_flows"] != 1:
        errors.append(f"outer connections={c2s_ledger['outer_syn_flows']} want=1")
    if c2s_ledger["repair_tcp_payload_bytes"] != 0 or s2c_ledger["repair_tcp_payload_bytes"] != 0:
        errors.append(
            "lossless outer repair payload nonzero: "
            f"c2s={c2s_ledger['repair_tcp_payload_bytes']} s2c={s2c_ledger['repair_tcp_payload_bytes']}"
        )

    app_input = biz["stats"]["sent_bytes"] + target["stats"]["sent_bytes"]
    outer_ip = c2s_ledger["outer_ip_bytes"] + s2c_ledger["outer_ip_bytes"]
    result = {
        "schema": 1,
        "source_sha": args.source_sha,
        "result": "PASS" if not errors else "FAIL",
        "errors": errors,
        "topology": "biz-netns -> OpenWrt TPROXY wbd-client process -> raw/veth -> router netns netem -> raw wbd-server process -> shared TUN -> target-netns",
        "config": {
            "lanes": 1, "fec": "20:20", "padding": "off", "mtu": 1400,
            "one_way_delay_ms": 300, "loss": "0%", "hidden_rate_limit": False,
        },
        "application": {
            "c2s_sent_bytes": biz["stats"]["sent_bytes"],
            "c2s_unique_delivered_bytes": target["stats"]["recv_unique_bytes"],
            "s2c_sent_bytes": target["stats"]["sent_bytes"],
            "s2c_unique_delivered_bytes": biz["stats"]["recv_unique_bytes"],
            "app_input_bytes": app_input,
        },
        "injection": {
            "c2s_send_lag_p99_ns": biz["stats"]["send_lag_p99_ns"],
            "s2c_send_lag_p99_ns": target["stats"]["send_lag_p99_ns"],
        },
        "latency": {
            "probe_rtt_p50_ns": biz["stats"]["probe_rtt_p50_ns"],
            "probe_rtt_p95_ns": biz["stats"]["probe_rtt_p95_ns"],
            "probe_rtt_p99_ns": biz["stats"]["probe_rtt_p99_ns"],
            "c2s_qdisc_delay_p50_ns": pct(c2s_delay, 0.50),
            "c2s_qdisc_delay_p95_ns": pct(c2s_delay, 0.95),
            "s2c_qdisc_delay_p50_ns": pct(s2c_delay, 0.50),
            "s2c_qdisc_delay_p95_ns": pct(s2c_delay, 0.95),
        },
        "capture": {"drops": capture_drops},
        "outer": {
            "c2s": c2s_ledger,
            "s2c": s2c_ledger,
            "outer_ip_bytes": outer_ip,
            "outer_ip_per_app_input": (outer_ip / app_input) if app_input else None,
        },
    }
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_REALPATH_CALIBRATION_" + result["result"] + " " + json.dumps(result, sort_keys=True))
    raise SystemExit(0 if not errors else 1)


if __name__ == "__main__":
    main()
