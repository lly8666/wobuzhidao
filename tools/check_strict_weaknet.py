#!/usr/bin/env python3
import argparse
import json
import math
import os
import re
import struct
from collections import defaultdict, deque
from pathlib import Path

STAGES = {"pre": (0, 30), "stress": (30, 90), "post": (90, 120)}


def pct(values, q):
    values = sorted(v for v in values if v is not None)
    if not values:
        return None
    i = int(round((len(values) - 1) * q))
    return values[max(0, min(i, len(values) - 1))]


def read_jsonl(path):
    out = []
    p = Path(path)
    if not p.exists():
        return out
    for line in p.read_text(encoding="utf-8", errors="replace").splitlines():
        line = line.strip()
        if not line:
            continue
        try:
            out.append(json.loads(line))
        except json.JSONDecodeError:
            pass
    return out


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
    _, _, _, _, _, _, network = struct.unpack(endian + "IHHIIII", data[:24])
    if network != 1:
        raise ValueError(f"{path}: linktype={network}, want Ethernet")
    rows = []
    off = 24
    while off + 16 <= len(data):
        sec, frac, incl, _orig = struct.unpack(endian + "IIII", data[off:off+16])
        off += 16
        frame = data[off:off+incl]
        off += incl
        if len(frame) < 34 or frame[12:14] != b"\x08\x00":
            continue
        ip = frame[14:]
        if len(ip) < 20 or ip[0] >> 4 != 4:
            continue
        ihl = (ip[0] & 0x0f) * 4
        total = struct.unpack("!H", ip[2:4])[0]
        frag = struct.unpack("!H", ip[6:8])[0]
        fragmented = (frag & 0x1fff) != 0 or (frag & 0x2000) != 0
        if ip[9] != 6 or len(ip) < ihl + 20:
            continue
        tcp = ip[ihl:]
        doff = ((tcp[12] >> 4) & 0x0f) * 4
        if doff < 20 or total < ihl + doff or len(tcp) < doff:
            continue
        sport, dport, seq, ack = struct.unpack("!HHII", tcp[:12])
        flags = tcp[13]
        src = ".".join(str(x) for x in ip[12:16])
        dst = ".".join(str(x) for x in ip[16:20])
        payload_len = total - ihl - doff
        prefix = bytes(tcp[doff:min(len(tcp), doff + min(payload_len, 128))])
        rows.append({
            "ts_ns": sec * 1_000_000_000 + frac * scale,
            "src": src, "dst": dst, "sport": sport, "dport": dport,
            "seq": seq, "ack": ack, "flags": flags,
            "ip_bytes": total, "payload_bytes": payload_len,
            "payload_prefix": prefix.hex(), "fragmented": fragmented,
        })
    return rows


def pair_delay(pre, post):
    queues = defaultdict(deque)
    for row in pre:
        key = (row["src"], row["dst"], row["sport"], row["dport"],
               row["seq"], row["ack"], row["flags"], row["ip_bytes"], row["payload_prefix"])
        queues[key].append(row["ts_ns"])
    delays = []
    for row in post:
        key = (row["src"], row["dst"], row["sport"], row["dport"],
               row["seq"], row["ack"], row["flags"], row["ip_bytes"], row["payload_prefix"])
        q = queues.get(key)
        if q:
            delays.append(row["ts_ns"] - q.popleft())
    return delays


def capture_drop(path):
    text = Path(path).read_text(encoding="utf-8", errors="replace")
    matches = re.findall(r"(\d+) packets dropped by kernel", text)
    return None if not matches else int(matches[-1])


def stage_bounds(events):
    by = {x["event"]: x for x in events}
    return {
        "pre": (by["pre_start"]["unix_ns"], by["pre_end"]["unix_ns"]),
        "stress": (by["stress_start"]["unix_ns"], by["stress_end"]["unix_ns"]),
        "post": (by["post_start"]["unix_ns"], by["post_end"]["unix_ns"]),
    }


def qdisc_counter(event, direction, key):
    rows = event["qdisc"][direction]
    if not isinstance(rows, list) or not rows:
        return 0
    return int(rows[0].get(key, 0) or 0)


def qdisc_stage(events, name, direction):
    by = {x["event"]: x for x in events}
    a, b = by[name + "_start"], by[name + "_end"]
    # tc -s qdisc reports successfully dequeued/sent packets separately from
    # packets dropped by the qdisc. The offered packet denominator is therefore
    # passed + dropped, not passed alone. Using drops/passed makes a true 20%
    # loss look like 25% and a true 30% loss look like ~42.86%.
    passed = qdisc_counter(b, direction, "packets") - qdisc_counter(a, direction, "packets")
    drops = qdisc_counter(b, direction, "drops") - qdisc_counter(a, direction, "drops")
    overlimits = qdisc_counter(b, direction, "overlimits") - qdisc_counter(a, direction, "overlimits")
    attempted = passed + drops
    return {
        "packets": passed,
        "passed_packets": passed,
        "attempted_packets": attempted,
        "drops": drops,
        "overlimits": overlimits,
        "loss_denominator": "passed_plus_drops",
        "loss_percent": (100.0 * drops / attempted) if attempted > 0 else None,
    }


def generator_stage(sender, receiver, name, target_mbps):
    lo, hi = STAGES[name]
    ss, rs = sender["stats"], receiver["stats"]
    sent_b = sum(ss["sent_bytes_by_second"][lo:hi])
    sent_p = sum(ss["sent_packets_by_second"][lo:hi])
    recv_b = sum(rs["recv_bytes_by_second"][lo:hi])
    recv_p = sum(rs["recv_packets_by_second"][lo:hi])
    duration = hi - lo
    expected_b = target_mbps * 1_000_000 / 8 * duration
    return {
        "sent_bytes": sent_b, "sent_packets": sent_p,
        "unique_delivered_bytes": recv_b, "unique_delivered_packets": recv_p,
        "actual_send_mbps": sent_b * 8 / duration / 1e6,
        "eventual_goodput_mbps": recv_b * 8 / duration / 1e6,
        "send_target_ratio": sent_b / expected_b if expected_b else None,
        "packet_loss_percent": 100.0 * (sent_p - recv_p) / sent_p if sent_p else None,
        "byte_loss_percent": 100.0 * (sent_b - recv_b) / sent_b if sent_b else None,
        "wall_delivered_mbps": sum(rs.get("recv_wall_bytes_by_second", [])[lo:hi]) * 8 / duration / 1e6,
    }


def tcp_seq_before(a, b):
    delta = (int(a) - int(b)) & 0xffffffff
    return delta != 0 and (delta & 0x80000000) != 0


def outer_stage_ledger(rows, bounds):
    seen = set()
    stages = {name: {
        "packets": 0, "outer_ip_bytes": 0, "fresh_payload_bytes": 0,
        "repair_payload_bytes": 0, "fresh_packet_ip_bytes": 0,
        "repair_packet_ip_bytes": 0, "ack_control_ip_bytes": 0,
        "duplicate_ack_packets": 0, "fresh_seq_regressions": 0,
        "protocol_header_bytes": 0, "flows": defaultdict(lambda: {"packets": 0, "ip_bytes": 0}),
    } for name in STAGES}
    diff_cipher = 0
    seq_prefix = {}
    fragments = 0
    last_ack = {}
    highest_fresh_seq = {}
    for row in rows:
        if row["fragmented"]:
            fragments += 1
        if row["payload_bytes"]:
            k = (row["src"], row["dst"], row["sport"], row["dport"], row["seq"], row["payload_bytes"])
            old = seq_prefix.get(k)
            if old is None:
                seq_prefix[k] = row["payload_prefix"]
            elif old != row["payload_prefix"]:
                diff_cipher += 1
        stage = None
        for name, (a, b) in bounds.items():
            if a <= row["ts_ns"] < b:
                stage = name
                break
        key = (row["src"], row["dst"], row["sport"], row["dport"], row["seq"], row["payload_bytes"])
        is_repair = row["payload_bytes"] > 0 and key in seen
        if row["payload_bytes"] > 0:
            seen.add(key)
        if stage is None:
            continue
        s = stages[stage]
        s["packets"] += 1
        s["outer_ip_bytes"] += row["ip_bytes"]
        s["protocol_header_bytes"] += row["ip_bytes"] - row["payload_bytes"]
        flow = f"{row['src']}:{row['sport']}->{row['dst']}:{row['dport']}"
        s["flows"][flow]["packets"] += 1
        s["flows"][flow]["ip_bytes"] += row["ip_bytes"]
        flow_key = (row["src"], row["dst"], row["sport"], row["dport"])
        if row["payload_bytes"] == 0:
            s["ack_control_ip_bytes"] += row["ip_bytes"]
            if (row["flags"] & 0x10) and last_ack.get(flow_key) == row["ack"]:
                s["duplicate_ack_packets"] += 1
            if row["flags"] & 0x10:
                last_ack[flow_key] = row["ack"]
        elif is_repair:
            s["repair_payload_bytes"] += row["payload_bytes"]
            s["repair_packet_ip_bytes"] += row["ip_bytes"]
        else:
            prev = highest_fresh_seq.get(flow_key)
            if prev is not None and tcp_seq_before(row["seq"], prev):
                s["fresh_seq_regressions"] += 1
            elif prev is None or tcp_seq_before(prev, row["seq"]):
                highest_fresh_seq[flow_key] = row["seq"]
            s["fresh_payload_bytes"] += row["payload_bytes"]
            s["fresh_packet_ip_bytes"] += row["ip_bytes"]
    for name, s in stages.items():
        dur = STAGES[name][1] - STAGES[name][0]
        s["outer_mbps"] = s["outer_ip_bytes"] * 8 / dur / 1e6
        s["pps"] = s["packets"] / dur
        payload_ip = s["fresh_packet_ip_bytes"] + s["repair_packet_ip_bytes"]
        s["repair_outer_share"] = s["repair_packet_ip_bytes"] / payload_ip if payload_ip else 0.0
        s["flows"] = dict(s["flows"])
    return stages, fragments, diff_cipher


def product(sample, side):
    p = sample.get("product") or {}
    if side == "server":
        if not p.get("present"):
            return None
        return p.get("tunnel")
    return p


def lane_key(lane):
    ref = lane.get("ref") or {}
    return f"{ref.get('ID', 0)}:{ref.get('Generation', 0)}"


def counters(diag):
    if not diag:
        return {"owner": {}, "lanes": {}}
    owner = diag.get("owner") or {}
    out = {
        "owner": {
            "game_logical_packets": owner.get("GameLogicalOutbound", 0),
            "game_logical_bytes": owner.get("GameLogicalOutboundBytes", 0),
            "game_lane_copies": owner.get("GameLaneCopies", 0),
            "game_lane_copy_bytes": owner.get("GameLaneCopyBytes", 0),
            "game_delivered": owner.get("GameDelivered", 0),
            "game_duplicates": owner.get("GameDuplicates", 0),
            "game_stale": owner.get("GameStale", 0),
            "game_lane_mismatches": owner.get("GameLaneMismatches", 0),
            "source_discards": owner.get("SourceDiscards", 0),
            "padding_bytes": (owner.get("Padding") or {}).get("PaddingBytes", 0),
        },
        "lanes": {},
    }
    for lane in diag.get("lanes") or []:
        ls = lane.get("lane") or {}
        tx = ls.get("TxPath") or {}
        rx = ls.get("RxPath") or {}
        enc = tx.get("Encoder") or {}
        dec = rx.get("Decoder") or {}
        tr = lane.get("transport") or {}
        out["lanes"][lane_key(lane)] = {
            "fec_source_shards": enc.get("source_shards", 0),
            "fec_source_bytes": enc.get("source_bytes", 0),
            "fec_parity_shards": enc.get("parity_shards", 0),
            "fec_parity_bytes": enc.get("parity_bytes", 0),
            "fec_reconstruction_events": dec.get("reconstruction_events", 0),
            "fec_recovered_sources": dec.get("recovered_sources", 0),
            "padding_bytes": ls.get("PaddingBytes", 0),
            "fresh_segments": tr.get("FreshSent", 0),
            "health_sent": tr.get("HealthSent", 0),
            "health_received": tr.get("HealthReceived", 0),
            "repair_segments": tr.get("Retransmitted", 0),
            "fast_repairs": tr.get("FastRepairs", 0),
            "rto_repairs": tr.get("RTORepairs", 0),
            "repair_succeeded": tr.get("RepairSucceeded", 0),
            "repair_selected": tr.get("RepairSelected", 0),
            "repair_attempts": tr.get("RepairAttempts", 0),
            "repair_failures": tr.get("RepairFailures", 0),
            "repair_deferred": tr.get("RepairDeferred", 0),
            "abandoned": tr.get("Abandoned", 0),
            "forgiven_gaps": tr.get("ForgivenGaps", 0),
            "received_segments": tr.get("Received", 0),
            "transport_duplicates": tr.get("Duplicates", 0),
            "late_first_arrival": tr.get("LateFirstArrival", 0),
            "decoder_failed": (ls.get("Decoder") or {}).get("Failed", 0),
            "decoder_duplicates": (ls.get("Decoder") or {}).get("Duplicates", 0),
        }
    return out


def subtract(b, a):
    if isinstance(b, dict) and isinstance(a, dict):
        keys = set(b) | set(a)
        return {k: subtract(b.get(k, 0), a.get(k, 0)) for k in keys}
    if isinstance(b, (int, float)) and isinstance(a, (int, float)):
        return b - a
    return b


def nearest_diag(samples, side, unix_ns):
    good = [(abs(int(s.get("unix_ns", 0)) - unix_ns), s) for s in samples if product(s, side)]
    if not good:
        return None
    return min(good, key=lambda x: x[0])[1]


def diag_stage_deltas(samples, side, bounds):
    out = {}
    for name, (a, b) in bounds.items():
        sa, sb = nearest_diag(samples, side, a), nearest_diag(samples, side, b)
        out[name] = subtract(counters(product(sb, side)), counters(product(sa, side))) if sa and sb else None
    return out


def directional_recovery(sender_row, receiver_row):
    """Build one business-direction cross-layer ledger.

    sender_row supplies outbound Game/FEC/transport counters for the direction.
    receiver_row supplies receive-side FEC reconstruction/duplicate/gap counters.
    A Lane diagnostic contains both TxPath and RxPath, so using one endpoint for
    both silently attributes reverse-direction recovery to the wrong direction.
    """
    sender_row = sender_row or {"owner": {}, "lanes": {}}
    receiver_row = receiver_row or {"owner": {}, "lanes": {}}
    sender_lanes = sender_row.get("lanes") or {}
    receiver_lanes = receiver_row.get("lanes") or {}
    zero = {
        "tx_fec_source_shards": 0,
        "tx_fec_source_bytes": 0,
        "tx_fec_parity_shards": 0,
        "tx_fec_parity_bytes": 0,
        "tx_padding_bytes": 0,
        "tx_fresh_segments": 0,
        "tx_health_records": 0,
        "tx_repair_segments": 0,
        "tx_repair_selected": 0,
        "tx_repair_attempts": 0,
        "tx_repair_succeeded": 0,
        "tx_repair_failures": 0,
        "tx_repair_deferred": 0,
        "tx_fast_repairs": 0,
        "tx_rto_repairs": 0,
        "tx_abandoned": 0,
        "rx_fec_reconstruction_events": 0,
        "rx_fec_recovered_sources": 0,
        "rx_received_segments": 0,
        "rx_transport_duplicates": 0,
        "rx_decoder_duplicates": 0,
        "rx_decoder_failed": 0,
        "rx_late_first_arrival": 0,
        "rx_forgiven_gaps": 0,
    }
    lanes = {}
    for ref in sorted(set(sender_lanes) | set(receiver_lanes)):
        tx = sender_lanes.get(ref) or {}
        rx = receiver_lanes.get(ref) or {}
        row = dict(zero)
        row.update({
            "tx_fec_source_shards": int(tx.get("fec_source_shards", 0) or 0),
            "tx_fec_source_bytes": int(tx.get("fec_source_bytes", 0) or 0),
            "tx_fec_parity_shards": int(tx.get("fec_parity_shards", 0) or 0),
            "tx_fec_parity_bytes": int(tx.get("fec_parity_bytes", 0) or 0),
            "tx_padding_bytes": int(tx.get("padding_bytes", 0) or 0),
            "tx_fresh_segments": int(tx.get("fresh_segments", 0) or 0),
            "tx_health_records": int(tx.get("health_sent", 0) or 0),
            "tx_repair_segments": int(tx.get("repair_segments", 0) or 0),
            "tx_repair_selected": int(tx.get("repair_selected", 0) or 0),
            "tx_repair_attempts": int(tx.get("repair_attempts", 0) or 0),
            "tx_repair_succeeded": int(tx.get("repair_succeeded", 0) or 0),
            "tx_repair_failures": int(tx.get("repair_failures", 0) or 0),
            "tx_repair_deferred": int(tx.get("repair_deferred", 0) or 0),
            "tx_fast_repairs": int(tx.get("fast_repairs", 0) or 0),
            "tx_rto_repairs": int(tx.get("rto_repairs", 0) or 0),
            "tx_abandoned": int(tx.get("abandoned", 0) or 0),
            "rx_fec_reconstruction_events": int(rx.get("fec_reconstruction_events", 0) or 0),
            "rx_fec_recovered_sources": int(rx.get("fec_recovered_sources", 0) or 0),
            "rx_received_segments": int(rx.get("received_segments", 0) or 0),
            "rx_transport_duplicates": int(rx.get("transport_duplicates", 0) or 0),
            "rx_decoder_duplicates": int(rx.get("decoder_duplicates", 0) or 0),
            "rx_decoder_failed": int(rx.get("decoder_failed", 0) or 0),
            "rx_late_first_arrival": int(rx.get("late_first_arrival", 0) or 0),
            "rx_forgiven_gaps": int(rx.get("forgiven_gaps", 0) or 0),
        })
        lanes[ref] = row

    total = dict(zero)
    for row in lanes.values():
        for key in total:
            total[key] += row[key]

    owner = sender_row.get("owner") or {}
    tx_owner = {
        "game_logical_packets": int(owner.get("game_logical_packets", 0) or 0),
        "game_logical_bytes": int(owner.get("game_logical_bytes", 0) or 0),
        "game_lane_copies": int(owner.get("game_lane_copies", 0) or 0),
        "game_lane_copy_bytes": int(owner.get("game_lane_copy_bytes", 0) or 0),
        "padding_bytes": int(owner.get("padding_bytes", 0) or 0),
    }
    return {"total": total, "lanes": lanes, "tx_owner": tx_owner}


def queue_age_at(samples_by_side, unix_ns):
    ages = []
    for side, samples in samples_by_side.items():
        s = nearest_diag(samples, side, unix_ns)
        d = product(s, side) if s else None
        if not d:
            continue
        for lane in d.get("lanes") or []:
            tr = lane.get("transport") or {}
            ages.append(max(int(tr.get("OldestOutstandingAge", 0) or 0),
                            int(tr.get("OldestOutOfOrderAge", 0) or 0)))
    return max(ages) if ages else None


def parse_skmem_stats(text):
    out = {"rmem_alloc": 0, "rcv_buf": 0, "drops": 0}
    for body in re.findall(r"skmem:\(([^\n)]*)\)", text or ""):
        fields = {}
        for token in body.split(","):
            token = token.strip()
            match = re.fullmatch(r"(rb|tb|bl|r|t|f|w|o|d)(-?\d+)", token)
            if match:
                fields[match.group(1)] = int(match.group(2))
        out["rmem_alloc"] = max(out["rmem_alloc"], fields.get("r", 0))
        out["rcv_buf"] = max(out["rcv_buf"], fields.get("rb", 0))
        out["drops"] = max(out["drops"], fields.get("d", 0))
    return out


def parse_skmem_drop(text):
    return parse_skmem_stats(text)["drops"]


def link_drop_totals(sample):
    out = {}
    ns = sample.get("namespaces") or {}
    for label, data in ns.items():
        links = data.get("links")
        if not isinstance(links, list):
            continue
        rx = tx = 0
        for link in links:
            st = link.get("stats64") or link.get("stats") or {}
            rx += int((st.get("rx") or {}).get("dropped", 0) or 0)
            tx += int((st.get("tx") or {}).get("dropped", 0) or 0)
        out[label] = {"rx": rx, "tx": tx}
    return out


def resource_summary(samples, start_unix, end_unix):
    during = [s for s in samples if start_unix <= s.get("unix_ns", 0) <= end_unix]
    errors = []
    if len(during) < 100:
        errors.append(f"resource samples during injection={len(during)}")
    gaps = [(b["monotonic_ns"] - a["monotonic_ns"]) / 1e9 for a, b in zip(during, during[1:])]
    max_gap = max(gaps) if gaps else None
    if max_gap is None or max_gap > 2.5:
        errors.append(f"resource max sample gap={max_gap}")
    missing = {}
    for label in ("client", "server"):
        count = sum(1 for s in during if not ((s.get("processes") or {}).get(label) or {}).get("present"))
        missing[label] = count
        if count:
            errors.append(f"{label} missing in {count} resource samples")
    socket_drop_max = 0
    socket_drop_by_endpoint = {}
    packet_socket_by_endpoint = {}
    packet_socket_command_errors = {}
    for s in during:
        for label, data in (s.get("namespaces") or {}).items():
            for key in ("ss_udp", "ss_raw", "ss_packet"):
                item = data.get(key) or {}
                stats = parse_skmem_stats(item.get("stdout", ""))
                value = stats["drops"]
                endpoint = f"{label}/{key}"
                socket_drop_by_endpoint[endpoint] = max(socket_drop_by_endpoint.get(endpoint, 0), value)
                # UDP/raw-IP sockets keep their historical gate in every
                # namespace. AF_PACKET is the WBD raw receive socket only in
                # client/server namespaces; router AF_PACKET belongs to
                # tcpdump and is gated separately by capture_drop().
                if key != "ss_packet" or label in ("client", "server"):
                    socket_drop_max = max(socket_drop_max, value)
                if key == "ss_packet":
                    row = packet_socket_by_endpoint.setdefault(endpoint, {
                        "max_rmem_alloc": 0,
                        "max_rcv_buf": 0,
                        "max_drops": 0,
                        "max_rmem_ratio": 0.0,
                    })
                    row["max_rmem_alloc"] = max(row["max_rmem_alloc"], stats["rmem_alloc"])
                    row["max_rcv_buf"] = max(row["max_rcv_buf"], stats["rcv_buf"])
                    row["max_drops"] = max(row["max_drops"], stats["drops"])
                    if stats["rcv_buf"] > 0:
                        row["max_rmem_ratio"] = max(
                            row["max_rmem_ratio"],
                            stats["rmem_alloc"] / stats["rcv_buf"],
                        )
                    if label in ("client", "server") and (
                        item.get("error") or item.get("returncode", 0) != 0
                    ):
                        packet_socket_command_errors[endpoint] = (
                            item.get("error") or item.get("stderr") or
                            f"returncode={item.get('returncode')}"
                        )
    if socket_drop_max:
        offenders = {
            k: v for k, v in socket_drop_by_endpoint.items()
            if v and (not k.endswith("/ss_packet") or k.startswith(("client/", "server/")))
        }
        errors.append(f"socket skmem drop max={socket_drop_max} endpoints={offenders}")
    if packet_socket_command_errors:
        errors.append(f"packet socket sampler errors={packet_socket_command_errors}")

    link_delta = {}
    if during:
        first, last = link_drop_totals(during[0]), link_drop_totals(during[-1])
        for label in set(first) | set(last):
            link_delta[label] = {
                "rx": last.get(label, {}).get("rx", 0) - first.get(label, {}).get("rx", 0),
                "tx": last.get(label, {}).get("tx", 0) - first.get(label, {}).get("tx", 0),
            }
        unexpected = sum(max(0, v["rx"]) + max(0, v["tx"]) for v in link_delta.values())
        if unexpected:
            errors.append(f"interface drop delta={unexpected}")

    cpu_seconds = {}
    thread_cpu_seconds = {}
    hz = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
    if during:
        for label in ("client", "server"):
            firstp = ((during[0].get("processes") or {}).get(label) or {})
            lastp = ((during[-1].get("processes") or {}).get(label) or {})
            if firstp.get("present") and lastp.get("present"):
                a, b = firstp["stat"], lastp["stat"]
                cpu_seconds[label] = ((b["utime_ticks"] + b["stime_ticks"]) -
                                      (a["utime_ticks"] + a["stime_ticks"])) / hz
                ta = {x["tid"]: x for x in firstp.get("threads") or []}
                tb = {x["tid"]: x for x in lastp.get("threads") or []}
                thread_cpu_seconds[label] = {
                    str(tid): ((x["utime_ticks"] + x["stime_ticks"]) -
                               (ta.get(tid, x)["utime_ticks"] + ta.get(tid, x)["stime_ticks"])) / hz
                    for tid, x in tb.items()
                }
    return {
        "samples": len(during), "max_gap_s": max_gap, "missing_process_samples": missing,
        "socket_drop_max": socket_drop_max, "socket_drop_by_endpoint": socket_drop_by_endpoint,
        "packet_socket_by_endpoint": packet_socket_by_endpoint,
        "packet_socket_command_errors": packet_socket_command_errors,
        "link_drop_delta": link_delta,
        "process_cpu_seconds": cpu_seconds, "thread_cpu_seconds": thread_cpu_seconds,
        "errors": errors,
    }



def connection_cost(rows, start_unix_ns, lanes):
    """Separate startup handshake bytes and reconnect-flow wire evidence.

    New-flow total bytes are reported as an upper carrier total, not claimed as
    pure handshake bytes. ACK/SYN/control bytes on new ports are the conservative
    reconnect-control lower bound.
    """
    ordered = sorted(rows, key=lambda r: r["ts_ns"])
    port_first = {}
    def client_port(row):
        if row["src"] == "198.18.0.2":
            return row["sport"]
        if row["dst"] == "198.18.0.2":
            return row["dport"]
        return None
    for row in ordered:
        p = client_port(row)
        if p is not None and p not in port_first:
            port_first[p] = row["ts_ns"]
    ports = [p for p, _ in sorted(port_first.items(), key=lambda kv: kv[1])]
    initial = set(ports[:lanes])
    reconnect = set(ports[lanes:])
    startup = [r for r in ordered if r["ts_ns"] < start_unix_ns]
    reconnect_rows = [r for r in ordered if client_port(r) in reconnect]
    return {
        "initial_client_ports": sorted(initial),
        "reconnect_client_ports": sorted(reconnect),
        "reconnect_flow_count": len(reconnect),
        "startup_outer_ip_bytes": sum(r["ip_bytes"] for r in startup),
        "startup_tcp_payload_bytes": sum(r["payload_bytes"] for r in startup),
        "startup_tcp_control_ip_bytes": sum(r["ip_bytes"] for r in startup if r["payload_bytes"] == 0),
        "reconnect_flow_outer_ip_bytes_upper_carrier_total": sum(r["ip_bytes"] for r in reconnect_rows),
        "reconnect_tcp_control_ip_bytes_lower_bound": sum(
            r["ip_bytes"] for r in reconnect_rows if r["payload_bytes"] == 0
        ),
        "note": "reconnect flow total can include steady encrypted business after promotion; control bytes are a lower-bound reconnect cost",
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--artifact-dir", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--mode", choices=("normal", "game"), required=True)
    ap.add_argument("--scenario", choices=("lossless", "5205", "5305"), required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--target-mbps", type=float, required=True)
    ap.add_argument("--lanes", type=int, required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()
    root = Path(args.artifact_dir)
    biz = json.loads((root / "biz.json").read_text())
    target = json.loads((root / "target.json").read_text())
    events = read_jsonl(root / "stage-events.jsonl")
    required_events = ("pre_start", "pre_end", "stress_start", "stress_end", "post_start", "post_end", "drain_start")
    event_names = {x.get("event") for x in events}
    missing_events = [name for name in required_events if name not in event_names]
    harness_errors = [x for x in events if x.get("event") == "harness_error"]
    if missing_events or harness_errors:
        errors = []
        if missing_events:
            errors.append(f"missing stage events={missing_events}")
        errors.extend(f"stage harness error={x.get('error')}" for x in harness_errors)
        result = {
            "schema": 1, "source_sha": args.source_sha, "mode": args.mode,
            "scenario": args.scenario, "seed": args.seed, "lanes": args.lanes,
            "target_mbps_each_direction": args.target_mbps,
            "harness_validity": "FAIL",
            "classifications": {
                "CORRECTNESS": "NOT_EVALUATED",
                "INPUT_VALIDITY": "FAIL",
                "CAPTURE": "NOT_EVALUATED",
                "ENVIRONMENT": "FAIL",
                "PERFORMANCE": "NOT_EVALUATED",
            },
            "errors": {
                "correctness": [], "input": errors, "capture": [],
                "environment": errors, "performance": [],
            },
            "stage_events": events,
            "transport_hygiene": {
                "gating": False, "status": "NOT_EVALUATED", "observations": [],
            },
            "result": "HARNESS_INVALID",
        }
        Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
        print("WBD_STRICT_WEAKNET_HARNESS_INVALID " + json.dumps({
            "mode": args.mode, "scenario": args.scenario, "seed": args.seed, "errors": errors,
        }, sort_keys=True))
        raise SystemExit(1)
    bounds = stage_bounds(events)
    c2s_pre, c2s_post = parse_pcap(root / "c2s-pre.pcap"), parse_pcap(root / "c2s-post.pcap")
    s2c_pre, s2c_post = parse_pcap(root / "s2c-pre.pcap"), parse_pcap(root / "s2c-post.pcap")
    c2s_outer, c2s_frag, c2s_diff = outer_stage_ledger(c2s_pre, bounds)
    s2c_outer, s2c_frag, s2c_diff = outer_stage_ledger(s2c_pre, bounds)
    c2s_delay = pair_delay(c2s_pre, c2s_post)
    s2c_delay = pair_delay(s2c_pre, s2c_post)
    client_diag, server_diag = read_jsonl(root / "client-diag.jsonl"), read_jsonl(root / "server-diag.jsonl")
    resources = read_jsonl(root / "resources.jsonl")
    start_unix = bounds["pre"][0]
    end_unix = bounds["post"][1]

    gen = {
        "c2s": {name: generator_stage(biz, target, name, args.target_mbps) for name in STAGES},
        "s2c": {name: generator_stage(target, biz, name, args.target_mbps) for name in STAGES},
    }
    desired = {
        "lossless": {"pre": 0.0, "stress": 0.0, "post": 0.0},
        "5205": {"pre": 5.0, "stress": 20.0, "post": 5.0},
        "5305": {"pre": 5.0, "stress": 30.0, "post": 5.0},
    }[args.scenario]
    injection = {d: {name: qdisc_stage(events, name, d) for name in STAGES} for d in ("c2s", "s2c")}

    correctness_errors = []
    for name, sender, receiver in (("c2s", biz, target), ("s2c", target, biz)):
        ss, rs = sender["stats"], receiver["stats"]
        if rs.get("corrupt", 0) or rs.get("unexpected", 0) or rs.get("recv_duplicates", 0):
            correctness_errors.append(
                f"{name} app corrupt={rs.get('corrupt')} unexpected={rs.get('unexpected')} duplicates={rs.get('recv_duplicates')}"
            )
    if c2s_frag + s2c_frag:
        correctness_errors.append(f"unexpected IPv4 fragments={c2s_frag+s2c_frag}")
    if c2s_diff + s2c_diff:
        correctness_errors.append(f"same TCP seq/len different ciphertext prefix={c2s_diff+s2c_diff}")

    final_client = counters(product(client_diag[-1], "client")) if client_diag else {"owner": {}, "lanes": {}}
    final_server = counters(product(server_diag[-1], "server")) if server_diag and product(server_diag[-1], "server") else {"owner": {}, "lanes": {}}
    transport_observations = []
    for endpoint, final in (("client", final_client), ("server", final_server)):
        own = final["owner"]
        if own.get("game_lane_mismatches", 0) or own.get("source_discards", 0):
            correctness_errors.append(f"{endpoint} lane/source mismatch stats={own}")
        outbound_direction = "c2s" if endpoint == "client" else "s2c"
        inbound_direction = "s2c" if endpoint == "client" else "c2s"
        for ref, lane in final["lanes"].items():
            if lane.get("decoder_failed", 0):
                correctness_errors.append(f"{endpoint} lane={ref} tlsrecord decoder_failed={lane['decoder_failed']}")
            for key in ("abandoned", "repair_segments", "repair_deferred"):
                if lane.get(key, 0):
                    transport_observations.append({
                        "endpoint": endpoint, "business_direction": outbound_direction,
                        "lane": ref, "kind": key, "count": lane.get(key, 0),
                    })
            for key in ("decoder_duplicates", "transport_duplicates", "forgiven_gaps", "late_first_arrival"):
                if lane.get(key, 0):
                    transport_observations.append({
                        "endpoint": endpoint, "business_direction": inbound_direction,
                        "lane": ref, "kind": key, "count": lane.get(key, 0),
                    })

    input_errors = []
    for direction in ("c2s", "s2c"):
        sender = biz if direction == "c2s" else target
        st = sender["stats"]
        if st.get("send_failures", 0) != 0:
            input_errors.append(f"{direction} send_failures={st.get('send_failures')}")
        skipped = st.get("skipped_slots", 0)
        if skipped > max(1, st.get("sent_packets", 0) * 0.0001):
            input_errors.append(f"{direction} skipped_slots={skipped}")
        if st.get("send_lag_p99_ns") is None or st["send_lag_p99_ns"] > 10_000_000:
            input_errors.append(f"{direction} send_lag_p99_ns={st.get('send_lag_p99_ns')}")
        for name in STAGES:
            ratio = gen[direction][name]["send_target_ratio"]
            if ratio is None or not (0.99 <= ratio <= 1.01):
                input_errors.append(f"{direction}/{name} send_target_ratio={ratio}")
            actual_loss = injection[direction][name]["loss_percent"]
            want = desired[name]
            if actual_loss is None or abs(actual_loss - want) > 2.0:
                input_errors.append(f"{direction}/{name} actual_loss={actual_loss} want={want}")

    capture = {}
    capture_errors = []
    for stem in ("c2s-pre", "c2s-post", "s2c-pre", "s2c-post"):
        value = capture_drop(root / f"{stem}.tcpdump.log")
        capture[stem] = value
        if value != 0:
            capture_errors.append(f"{stem} capture_drop={value}")

    resource = resource_summary(resources, start_unix, end_unix)
    environment_errors = list(resource["errors"])

    performance_errors = []
    def enforce(direction, stage, max_loss, min_gp):
        row = gen[direction][stage]
        if row["packet_loss_percent"] is None or row["packet_loss_percent"] > max_loss:
            performance_errors.append(f"{direction}/{stage} packet_loss={row['packet_loss_percent']} > {max_loss}")
        if row["eventual_goodput_mbps"] < args.target_mbps * min_gp:
            performance_errors.append(
                f"{direction}/{stage} goodput={row['eventual_goodput_mbps']} target={args.target_mbps} ratio={min_gp}"
            )

    for direction in ("c2s", "s2c"):
        if args.scenario == "lossless":
            for stage in STAGES:
                enforce(direction, stage, 0.0, 0.99)
        else:
            enforce(direction, "pre", 0.1, 0.99)
            if args.scenario == "5205":
                enforce(direction, "stress", 0.1, 0.99)
            else:
                enforce(direction, "stress", 1.0, 0.98)
            enforce(direction, "post", 0.1, 0.99)

    probe = biz["stats"]
    probe_loss = 100.0 * probe.get("probe_timeouts", 0) / probe.get("probe_sent", 1)
    if probe_loss > 1.0:
        performance_errors.append(f"probe loss={probe_loss}%")

    late_after_2s = 0
    for receiver in (biz, target):
        wall = receiver["stats"].get("recv_wall_bytes_by_second", [])
        late_after_2s += sum(wall[122:]) if len(wall) > 122 else 0
    if late_after_2s:
        performance_errors.append(f"delivery persisted >2s into drain bytes={late_after_2s}")

    samples_by_side = {"client": client_diag, "server": server_diag}
    queue_pre = []
    for sec in range(10, 30):
        age = queue_age_at(samples_by_side, start_unix + int((sec + 0.5) * 1e9))
        if age is not None:
            queue_pre.append(age)
    queue_pre_p95 = pct(queue_pre, 0.95)
    post5_window = None
    if args.scenario != "lossless" and queue_pre_p95 is not None:
        for offset in range(0, 8):
            ok = True
            details = []
            for sec in range(90 + offset, 93 + offset):
                for direction, sender, receiver in (
                    ("c2s", biz, target), ("s2c", target, biz)
                ):
                    sent_p = sender["stats"]["sent_packets_by_second"][sec]
                    recv_p = receiver["stats"]["recv_packets_by_second"][sec]
                    sent_b = sender["stats"]["sent_bytes_by_second"][sec]
                    recv_b = receiver["stats"]["recv_bytes_by_second"][sec]
                    loss = 100 * (sent_p - recv_p) / sent_p if sent_p else 100
                    gp = recv_b * 8 / 1e6
                    if gp < args.target_mbps * 0.98 or loss > 0.1:
                        ok = False
                    details.append({"direction": direction, "second": sec, "loss_percent": loss, "goodput_mbps": gp})
                age = queue_age_at(samples_by_side, start_unix + int((sec + 0.5) * 1e9))
                if age is None or age > queue_pre_p95 + 200_000_000:
                    ok = False
                details.append({"queue_age_ns": age, "second": sec})
            if ok:
                post5_window = {"offset_s": offset, "windows": details}
                break
        if post5_window is None:
            performance_errors.append("post5 did not reach 3 consecutive qualifying 1s windows within 10s")
    elif args.scenario != "lossless":
        performance_errors.append("post5 queue baseline unavailable")

    product_stage_sender = {
        "c2s": diag_stage_deltas(client_diag, "client", bounds),
        "s2c": diag_stage_deltas(server_diag, "server", bounds),
    }
    product_stage_receiver = {
        "c2s": diag_stage_deltas(server_diag, "server", bounds),
        "s2c": diag_stage_deltas(client_diag, "client", bounds),
    }

    recovery_accounting = {
        direction: {
            name: directional_recovery(
                product_stage_sender[direction].get(name),
                product_stage_receiver[direction].get(name),
            )
            for name in STAGES
        }
        for direction in ("c2s", "s2c")
    }
    for direction, rows in (("c2s", c2s_outer), ("s2c", s2c_outer)):
        for name, row in rows.items():
            if row.get("duplicate_ack_packets", 0):
                transport_observations.append({
                    "wire_direction": direction, "stage": name, "kind": "pcap_duplicate_ack",
                    "count": row["duplicate_ack_packets"],
                })
            if row.get("fresh_seq_regressions", 0):
                transport_observations.append({
                    "wire_direction": direction, "stage": name, "kind": "pcap_fresh_seq_regression",
                    "count": row["fresh_seq_regressions"],
                })

    transport_hygiene = {
        "gating": False,
        "status": "REVIEW" if transport_observations else "PASS",
        "observations": transport_observations,
        "policy": (
            "Transport appearance is explanatory. Low-loss runs should minimize unnecessary "
            "repair/gap forgiveness; stressed runs prioritize unique business goodput, latency, "
            "continuity and bounded resources. Same-seq different ciphertext remains a hard "
            "correctness error and is not waived here."
        ),
    }

    outer_total = {
        "c2s": sum(v["outer_ip_bytes"] for v in c2s_outer.values()),
        "s2c": sum(v["outer_ip_bytes"] for v in s2c_outer.values()),
    }
    app_input = {
        "c2s": sum(biz["stats"]["sent_bytes_by_second"][:120]),
        "s2c": sum(target["stats"]["sent_bytes_by_second"][:120]),
    }
    unique = {
        "c2s": sum(target["stats"]["recv_bytes_by_second"][:120]),
        "s2c": sum(biz["stats"]["recv_bytes_by_second"][:120]),
    }
    cost = {}
    connection = {
        "c2s": connection_cost(c2s_pre, start_unix, args.lanes),
        "s2c": connection_cost(s2c_pre, start_unix, args.lanes),
    }
    for d in ("c2s", "s2c"):
        rec = recovery_accounting[d]
        fec_parity_bytes = sum(int((rec[name].get("total") or {}).get("tx_fec_parity_bytes", 0) or 0) for name in STAGES)
        padding_bytes = sum(int((rec[name].get("total") or {}).get("tx_padding_bytes", 0) or 0) for name in STAGES)
        health_records = sum(int((rec[name].get("total") or {}).get("tx_health_records", 0) or 0) for name in STAGES)
        repair_payload_bytes = sum(int((c2s_outer if d == "c2s" else s2c_outer)[name].get("repair_payload_bytes", 0) or 0) for name in STAGES)
        repair_outer_ip_bytes = sum(int((c2s_outer if d == "c2s" else s2c_outer)[name].get("repair_packet_ip_bytes", 0) or 0) for name in STAGES)
        game_lane_copy_bytes = sum(int((rec[name].get("tx_owner") or {}).get("game_lane_copy_bytes", 0) or 0) for name in STAGES)
        game_logical_bytes = sum(int((rec[name].get("tx_owner") or {}).get("game_logical_bytes", 0) or 0) for name in STAGES)
        cost[d] = {
            "outer_ip_bytes": outer_total[d],
            "app_raw_input_bytes": app_input[d],
            "unique_delivered_app_bytes": unique[d],
            "outer_ip_per_app_raw_input": outer_total[d] / app_input[d] if app_input[d] else None,
            "outer_ip_per_unique_delivered": outer_total[d] / unique[d] if unique[d] else None,
            "inner_tcp_retransmission_bytes": 0,
            "inner_tcp_retransmission_note": "strict main load is UDP; HTTPS/TCP retransmission is a separate specialty",
            "components": {
                "fec_parity_bytes": fec_parity_bytes,
                "game_logical_bytes": game_logical_bytes,
                "game_lane_copy_bytes": game_lane_copy_bytes,
                "game_replication_extra_bytes": max(0, game_lane_copy_bytes - game_logical_bytes),
                "repair_tcp_payload_bytes": repair_payload_bytes,
                "repair_outer_ip_bytes": repair_outer_ip_bytes,
                "health_records": health_records,
                "health_tlslike_wire_bytes": health_records * 40,
                "health_wire_note": "HEALTH plaintext is 9 bytes; fixed TLS-like overhead is 31 bytes; health bypasses FEC/padding/repair",
                "padding_bytes": padding_bytes,
                "handshake_reconnect": connection[d],
            },
            "stages": c2s_outer if d == "c2s" else s2c_outer,
            "product_stage_sender_endpoint": product_stage_sender[d],
            "product_stage_receiver_endpoint": product_stage_receiver[d],
            "product_stage_direction_note": (
                "sender endpoint supplies TX Game/FEC/repair; receiver endpoint supplies "
                "RX FEC reconstruction/duplicates/gap forgiveness. Do not sum overlapping "
                "FEC recovery, repair and final application delivery as independent benefit."
            ),
        }

    classifications = {
        "CORRECTNESS": "PASS" if not correctness_errors else "FAIL",
        "INPUT_VALIDITY": "PASS" if not input_errors else "FAIL",
        "CAPTURE": "PASS" if not capture_errors else "FAIL",
        "ENVIRONMENT": "PASS" if not environment_errors else "FAIL",
    }
    if correctness_errors:
        perf_class = "FAIL"
    elif input_errors or capture_errors or environment_errors:
        perf_class = "CAPACITY_LIMITED"
    else:
        perf_class = "PASS" if not performance_errors else "FAIL"
    classifications["PERFORMANCE"] = perf_class

    probe_by_second = probe.get("probe_rtt_ns_by_second", [])
    probe_stage = {
        name: {
            "sent": hi - lo,
            "received": sum(x is not None for x in probe_by_second[lo:hi]),
            "p95_ns": pct(probe_by_second[lo:hi], 0.95),
            "p99_ns": pct(probe_by_second[lo:hi], 0.99),
        } for name, (lo, hi) in STAGES.items()
    }

    result = {
        "schema": 1, "source_sha": args.source_sha, "mode": args.mode,
        "scenario": args.scenario, "seed": args.seed, "lanes": args.lanes,
        "target_mbps_each_direction": args.target_mbps,
        "classifications": classifications,
        "errors": {
            "correctness": correctness_errors, "input": input_errors,
            "capture": capture_errors, "environment": environment_errors,
            "performance": performance_errors,
        },
        "generator": gen, "injection": injection,
        "latency": {
            "probe_loss_percent": probe_loss, "probe_stage": probe_stage,
            "c2s_qdisc_delay_p50_ns": pct(c2s_delay, 0.50),
            "c2s_qdisc_delay_p95_ns": pct(c2s_delay, 0.95),
            "s2c_qdisc_delay_p50_ns": pct(s2c_delay, 0.50),
            "s2c_qdisc_delay_p95_ns": pct(s2c_delay, 0.95),
            "queue_pre_p95_ns": queue_pre_p95,
        },
        "post5": post5_window,
        "capture": capture, "resource": resource, "cost": cost,
        "qualification_priority": {
            "high_loss": "business_goodput_latency_continuity_resource_first",
            "low_loss": "tcp_like_hygiene_review_without_reintroducing_hol",
            "fixed_loss_rate_product_switch": False,
        },
        "transport_hygiene": transport_hygiene,
        "recovery_accounting": recovery_accounting,
        "fec_recovery_final": {
            "client": final_client["lanes"], "server": final_server["lanes"],
            "note": "reconstruction/recovered-source counters overlap final app recovery and are not additive benefit bytes",
        },
        "game_final": {"client": final_client["owner"], "server": final_server["owner"]},
        "drain": {"late_after_2s_bytes": late_after_2s},
        "rtt_baseline_gate": "DEFER_TO_AGGREGATE_SAME_MODE_SEED_LOSSLESS",
    }
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_STRICT_WEAKNET_SAMPLE " + json.dumps({
        "mode": args.mode, "scenario": args.scenario, "seed": args.seed,
        "classifications": classifications, "post5": post5_window is not None,
        "transport_hygiene": transport_hygiene["status"],
        "cost": {d: {
            "outer_ip_per_app_raw_input": cost[d]["outer_ip_per_app_raw_input"],
            "outer_ip_per_unique_delivered": cost[d]["outer_ip_per_unique_delivered"],
        } for d in cost},
    }, sort_keys=True))
    ok = all(v == "PASS" for v in classifications.values())
    raise SystemExit(0 if ok else 1)


if __name__ == "__main__":
    main()
