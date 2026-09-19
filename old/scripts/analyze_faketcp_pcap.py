#!/usr/bin/env python3
"""Analyze a client-side Ethernet pcap from WBD native FakeTCP.

The analyzer deliberately does not trust internal WBD counters. It parses the
wire image and checks TCP-shaped invariants an observer can see: SYN options,
sequence/ACK monotonicity, duplicate ACKs, persistent/merged SACK blocks,
repeated sequence ranges, fast retransmit evidence and RFC-6298-scale timeout
retransmits.
"""
from __future__ import annotations

import argparse
import json
import struct
from collections import defaultdict
from pathlib import Path

SYN = 0x02
ACK = 0x10
PSH = 0x08


def read_pcap(path: Path):
    b = path.read_bytes()
    if len(b) < 24:
        raise ValueError("short pcap")
    magic = b[:4]
    fmts = {
        b"\xd4\xc3\xb2\xa1": ("<", 1_000_000.0),
        b"\xa1\xb2\xc3\xd4": (">", 1_000_000.0),
        b"\x4d\x3c\xb2\xa1": ("<", 1_000_000_000.0),
        b"\xa1\xb2\x3c\x4d": (">", 1_000_000_000.0),
    }
    if magic not in fmts:
        raise ValueError(f"unsupported pcap magic {magic.hex()}")
    endian, frac_scale = fmts[magic]
    _, _, _, _, _, _, network = struct.unpack_from(endian + "IHHIIII", b, 0)
    if network != 1:
        raise ValueError(f"expected Ethernet pcap, linktype={network}")
    off = 24
    while off + 16 <= len(b):
        sec, frac, incl, _orig = struct.unpack_from(endian + "IIII", b, off)
        off += 16
        pkt = b[off:off + incl]
        off += incl
        yield sec + frac / frac_scale, pkt


def parse_tcp(ts: float, frame: bytes):
    if len(frame) < 14:
        return None
    ether_type = struct.unpack_from("!H", frame, 12)[0]
    l2 = 14
    if ether_type in (0x8100, 0x88A8):
        if len(frame) < 18:
            return None
        ether_type = struct.unpack_from("!H", frame, 16)[0]
        l2 = 18
    if ether_type != 0x0800 or len(frame) < l2 + 20:
        return None
    ip = frame[l2:]
    ihl = (ip[0] & 0x0F) * 4
    if ip[0] >> 4 != 4 or ihl < 20 or len(ip) < ihl + 20 or ip[9] != 6:
        return None
    total = struct.unpack_from("!H", ip, 2)[0]
    if total == 0 or total > len(ip):
        total = len(ip)
    tcp = ip[ihl:total]
    doff = (tcp[12] >> 4) * 4
    if doff < 20 or doff > len(tcp):
        return None
    srcp, dstp = struct.unpack_from("!HH", tcp, 0)
    seq, ack = struct.unpack_from("!II", tcp, 4)
    flags = tcp[13]
    win = struct.unpack_from("!H", tcp, 14)[0]
    opts = tcp[20:doff]
    mss = None
    sack_perm = False
    win_scale = None
    sacks = []
    i = 0
    while i < len(opts):
        kind = opts[i]
        if kind == 0:
            break
        if kind == 1:
            i += 1
            continue
        if i + 2 > len(opts):
            break
        ln = opts[i + 1]
        if ln < 2 or i + ln > len(opts):
            break
        val = opts[i + 2:i + ln]
        if kind == 2 and ln == 4:
            mss = struct.unpack("!H", val)[0]
        elif kind == 3 and ln == 3:
            win_scale = val[0]
        elif kind == 4 and ln == 2:
            sack_perm = True
        elif kind == 5 and ln >= 10 and (ln - 2) % 8 == 0:
            for j in range(0, len(val), 8):
                sacks.append(struct.unpack_from("!II", val, j))
        i += ln
    return {
        "ts": ts,
        "srcp": srcp,
        "dstp": dstp,
        "seq": seq,
        "ack": ack,
        "flags": flags,
        "win": win,
        "payload_len": len(tcp) - doff,
        "mss": mss,
        "sack_perm": sack_perm,
        "win_scale": win_scale,
        "sacks": sacks,
    }


def analyze(path: Path, client_port: int, server_port: int):
    packets = []
    for ts, frame in read_pcap(path):
        p = parse_tcp(ts, frame)
        if p and {p["srcp"], p["dstp"]} == {client_port, server_port}:
            packets.append(p)
    if not packets:
        raise AssertionError("no target TCP packets in pcap")

    c2s = [p for p in packets if p["srcp"] == client_port]
    s2c = [p for p in packets if p["srcp"] == server_port]
    syn = next((p for p in c2s if p["flags"] & SYN and not (p["flags"] & ACK)), None)
    synack = next((p for p in s2c if p["flags"] & SYN and p["flags"] & ACK), None)
    third = next((p for p in c2s if p["flags"] & ACK and not (p["flags"] & SYN) and p["payload_len"] == 0), None)
    assert syn and synack and third, "3-way handshake not visible"
    assert syn["mss"] and syn["sack_perm"] and syn["win_scale"] is not None, syn
    assert synack["mss"] and synack["sack_perm"] and synack["win_scale"] is not None, synack
    assert synack["ack"] == (syn["seq"] + 1) & 0xFFFFFFFF
    assert third["ack"] == (synack["seq"] + 1) & 0xFFFFFFFF

    data = [p for p in c2s if p["payload_len"] > 0]
    acks = [p for p in s2c if p["flags"] & ACK and p["payload_len"] == 0]
    assert data and acks, "missing steady-state data/ACK packets"

    first_by_seq = {}
    sends_by_seq = defaultdict(list)
    first_segments = []
    for p in data:
        sends_by_seq[p["seq"]].append(p)
        if p["seq"] not in first_by_seq:
            first_by_seq[p["seq"]] = p
            first_segments.append(p)
    first_segments.sort(key=lambda p: p["seq"])
    for a, b in zip(first_segments, first_segments[1:]):
        expected = (a["seq"] + a["payload_len"]) & 0xFFFFFFFF
        assert b["seq"] == expected, ("non-contiguous first-send sequence", a, b)

    ack_values = [p["ack"] for p in acks]
    for old, new in zip(ack_values, ack_values[1:]):
        # Test capture is short enough not to wrap 32-bit sequence space.
        assert new >= old, ("cumulative ACK moved backwards", old, new)

    sack_packets = [p for p in acks if p["sacks"]]
    max_sack_blocks = max((len(p["sacks"]) for p in sack_packets), default=0)
    merged_sack_packets = 0
    for p in sack_packets:
        for left, right in p["sacks"]:
            assert left >= p["ack"] and right > left, ("invalid SACK block", p)
            if right - left > 1200:
                merged_sack_packets += 1
                break

    dup_acks = 0
    run = 0
    last_ack = None
    for p in acks:
        if p["ack"] == last_ack:
            run += 1
            dup_acks += 1
        else:
            last_ack = p["ack"]
            run = 0

    retrans = {seq: ps for seq, ps in sends_by_seq.items() if len(ps) > 1}
    assert retrans, "no repeated sequence range seen on wire"
    assert dup_acks >= 3, f"too few duplicate ACKs: {dup_acks}"
    assert sack_packets, "no SACK blocks observed on wire"

    # Classify evidence from packet chronology rather than WBD's counters.
    ack_run = 0
    current_ack = None
    fast = []
    rto_like = []
    previous_send = {}
    retrans_events = []
    for p in packets:
        if p["srcp"] == server_port and p["payload_len"] == 0 and p["flags"] & ACK:
            if p["ack"] == current_ack:
                ack_run += 1
            else:
                current_ack = p["ack"]
                ack_run = 0
            continue
        if p["srcp"] != client_port or p["payload_len"] == 0:
            continue
        seq = p["seq"]
        prev = previous_send.get(seq)
        if prev is not None:
            delta = p["ts"] - prev
            ev = {"seq": seq, "delta_ms": delta * 1000.0, "ack": current_ack, "dup_ack_run": ack_run}
            retrans_events.append(ev)
            if current_ack == seq and ack_run >= 3 and delta < 0.9:
                fast.append(ev)
            if delta >= 0.9:
                rto_like.append(ev)
        previous_send[seq] = p["ts"]

    assert fast, "pcap has retransmissions but no third-duplicate-ACK fast retransmit evidence"

    first_ts = packets[0]["ts"]
    out = {
        "pcap": str(path),
        "packets": len(packets),
        "handshake": {
            "syn_mss": syn["mss"],
            "syn_sack_permitted": syn["sack_perm"],
            "syn_window_scale": syn["win_scale"],
            "synack_mss": synack["mss"],
            "synack_sack_permitted": synack["sack_perm"],
            "synack_window_scale": synack["win_scale"],
            "effective_advertised_window_bytes": 65535 << synack["win_scale"],
            "third_ack_delay_ms": (third["ts"] - syn["ts"]) * 1000.0,
        },
        "data_first_transmissions": len(first_segments),
        "wire_data_transmissions": len(data),
        "retransmitted_seq_ranges": len(retrans),
        "retransmission_events": len(retrans_events),
        "fast_retransmit_evidence": len(fast),
        "rto_scale_retransmit_evidence": len(rto_like),
        "duplicate_acks": dup_acks,
        "sack_ack_packets": len(sack_packets),
        "max_sack_blocks": max_sack_blocks,
        "merged_sack_packets": merged_sack_packets,
        "first_fast": fast[0] if fast else None,
        "first_rto_like": rto_like[0] if rto_like else None,
        "capture_duration_ms": (packets[-1]["ts"] - first_ts) * 1000.0,
    }
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("pcap", type=Path)
    ap.add_argument("--client-port", type=int, required=True)
    ap.add_argument("--server-port", type=int, required=True)
    ap.add_argument("--out", type=Path)
    args = ap.parse_args()
    out = analyze(args.pcap, args.client_port, args.server_port)
    text = json.dumps(out, sort_keys=True)
    print("FAKETCP_PCAP", text)
    if args.out:
        args.out.write_text(text + "\n")


if __name__ == "__main__":
    main()
