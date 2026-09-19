#!/usr/bin/env python3
import argparse
import ipaddress
import json
import re
import struct
import sys
from pathlib import Path

FLAG_FIN = 0x01
FLAG_SYN = 0x02
FLAG_RST = 0x04
FLAG_ACK = 0x10

def ones_sum_ok(data: bytes) -> bool:
    if len(data) & 1:
        data += b"\x00"
    total = 0
    for i in range(0, len(data), 2):
        total += (data[i] << 8) | data[i + 1]
        total = (total & 0xFFFF) + (total >> 16)
    while total >> 16:
        total = (total & 0xFFFF) + (total >> 16)
    return total == 0xFFFF

def read_pcap(path: Path):
    raw = path.read_bytes()
    if len(raw) < 24:
        raise AssertionError("pcap too short")
    magic = raw[:4]
    formats = {
        b"\xd4\xc3\xb2\xa1": ("<", 1_000_000),
        b"\xa1\xb2\xc3\xd4": (">", 1_000_000),
        b"\x4d\x3c\xb2\xa1": ("<", 1_000_000_000),
        b"\xa1\xb2\x3c\x4d": (">", 1_000_000_000),
    }
    if magic not in formats:
        raise AssertionError(f"unsupported pcap magic {magic.hex()}")
    endian, scale = formats[magic]
    network = struct.unpack(endian + "I", raw[20:24])[0]
    offset = 24
    packets = []
    while offset + 16 <= len(raw):
        sec, frac, incl, orig = struct.unpack(endian + "IIII", raw[offset:offset + 16])
        offset += 16
        if offset + incl > len(raw):
            raise AssertionError("truncated pcap packet")
        frame = raw[offset:offset + incl]
        offset += incl
        packets.append((sec + frac / scale, frame, orig))
    return network, packets

def l3_payload(network: int, frame: bytes):
    if network == 1:  # Ethernet
        if len(frame) < 14 or frame[12:14] != b"\x08\x00":
            return None
        return frame[14:]
    if network == 113:  # Linux cooked v1
        if len(frame) < 16 or frame[14:16] != b"\x08\x00":
            return None
        return frame[16:]
    if network == 276:  # Linux cooked v2
        if len(frame) < 20 or frame[:2] != b"\x08\x00":
            return None
        return frame[20:]
    if network == 101:  # raw IP
        return frame
    if network == 0 and len(frame) >= 4:  # BSD loopback/null
        return frame[4:]
    raise AssertionError(f"unsupported pcap link type {network}")

def parse_options(data: bytes):
    out = {}
    i = 0
    while i < len(data):
        kind = data[i]
        if kind == 0:
            break
        if kind == 1:
            i += 1
            continue
        if i + 2 > len(data):
            break
        n = data[i + 1]
        if n < 2 or i + n > len(data):
            break
        body = data[i + 2:i + n]
        if kind == 2 and n == 4:
            out["mss"] = int.from_bytes(body, "big")
        elif kind == 3 and n == 3:
            out["ws"] = body[0]
        elif kind == 4 and n == 2:
            out["sack"] = True
        i += n
    return out

def parse_ipv4_tcp(ts: float, ip: bytes):
    if len(ip) < 40 or ip[0] >> 4 != 4 or ip[9] != 6:
        return None
    ihl = (ip[0] & 0x0F) * 4
    total = int.from_bytes(ip[2:4], "big")
    if ihl < 20 or total < ihl + 20 or total > len(ip):
        return None
    ip = ip[:total]
    tcp = ip[ihl:]
    doff = (tcp[12] >> 4) * 4
    if doff < 20 or doff > len(tcp):
        return None
    flags_frag = int.from_bytes(ip[6:8], "big")
    src = str(ipaddress.IPv4Address(ip[12:16]))
    dst = str(ipaddress.IPv4Address(ip[16:20]))
    src_port = int.from_bytes(tcp[0:2], "big")
    dst_port = int.from_bytes(tcp[2:4], "big")
    seq = int.from_bytes(tcp[4:8], "big")
    ack = int.from_bytes(tcp[8:12], "big")
    flags = tcp[13]
    payload = tcp[doff:]
    pseudo = ip[12:20] + b"\x00\x06" + len(tcp).to_bytes(2, "big") + tcp
    return {
        "ts": ts, "src": src, "dst": dst, "sport": src_port, "dport": dst_port,
        "seq": seq, "ack": ack, "flags": flags,
        "window": int.from_bytes(tcp[14:16], "big"),
        "payload": payload, "options": parse_options(tcp[20:doff]),
        "frag_offset": flags_frag & 0x1FFF,
        "mf": bool(flags_frag & 0x2000),
        "df": bool(flags_frag & 0x4000),
        "ip_checksum_ok": ones_sum_ok(ip[:ihl]),
        "tcp_checksum_ok": ones_sum_ok(pseudo),
    }

def seq_end(pkt):
    return pkt["seq"] + len(pkt["payload"]) + (1 if pkt["flags"] & FLAG_SYN else 0) + (1 if pkt["flags"] & FLAG_FIN else 0)

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--pcap", required=True)
    ap.add_argument("--tcpdump-stderr", required=True)
    ap.add_argument("--server-ip", required=True)
    ap.add_argument("--port", required=True, type=int)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    stderr = Path(args.tcpdump_stderr).read_text(encoding="utf-8", errors="replace")
    m = re.search(r"(\d+) packets dropped by kernel", stderr)
    if not m:
        raise AssertionError("tcpdump capture-loss counter missing")
    dropped = int(m.group(1))
    if dropped != 0:
        raise AssertionError(f"tcpdump reported {dropped} packets dropped by kernel")

    network, frames = read_pcap(Path(args.pcap))
    parsed = []
    for ts, frame, _ in frames:
        l3 = l3_payload(network, frame)
        if not l3:
            continue
        pkt = parse_ipv4_tcp(ts, l3)
        if pkt and (pkt["sport"] == args.port or pkt["dport"] == args.port):
            parsed.append(pkt)
    if not parsed:
        raise AssertionError("no FakeTCP packets captured")

    syns = [p for p in parsed if p["dport"] == args.port and p["flags"] & FLAG_SYN and not p["flags"] & FLAG_ACK]
    if not syns:
        raise AssertionError("client SYN missing")
    syn = syns[0]
    client_ip, client_port = syn["src"], syn["sport"]
    flow = [p for p in parsed if
            ((p["src"], p["sport"], p["dst"], p["dport"]) == (client_ip, client_port, args.server_ip, args.port) or
             (p["src"], p["sport"], p["dst"], p["dport"]) == (args.server_ip, args.port, client_ip, client_port))]
    peers = {(p["src"], p["sport"], p["dst"], p["dport"]) for p in flow}
    if len(flow) < 8:
        raise AssertionError(f"too few packets in flow: {len(flow)}")

    if any(p["flags"] & FLAG_RST for p in flow):
        raise AssertionError("unexpected RST present in P2 fallback flow")
    if any(p["mf"] or p["frag_offset"] != 0 for p in flow):
        raise AssertionError("unexpected IPv4 fragmentation present")

    server_packets = [p for p in flow if p["src"] == args.server_ip and p["sport"] == args.port]
    client_packets = [p for p in flow if p["src"] == client_ip and p["sport"] == client_port]
    if not server_packets or not client_packets:
        raise AssertionError("flow direction missing")
    for p in server_packets:
        if not p["ip_checksum_ok"] or not p["tcp_checksum_ok"]:
            raise AssertionError(f"bad server checksum seq={p['seq']} flags={p['flags']:#x}")
        if not p["df"]:
            raise AssertionError(f"server packet lacks DF seq={p['seq']}")

    synacks = [p for p in server_packets if p["flags"] & FLAG_SYN and p["flags"] & FLAG_ACK]
    if not synacks:
        raise AssertionError("SYN-ACK missing")
    synack = synacks[0]
    if synack["ack"] != syn["seq"] + 1:
        raise AssertionError("SYN-ACK acknowledgment does not match client ISN")
    if synack["options"].get("mss") != 1360:
        raise AssertionError(f"server MSS={synack['options'].get('mss')} want 1360")
    if "ws" in syn["options"] and synack["options"].get("ws") != 8:
        raise AssertionError("window-scale negotiation mismatch")
    if bool(syn["options"].get("sack")) != bool(synack["options"].get("sack")):
        raise AssertionError("SACK-permitted negotiation mismatch")

    final_acks = [p for p in client_packets if p["flags"] & FLAG_ACK and p["ack"] == synack["seq"] + 1]
    if not final_acks:
        raise AssertionError("final handshake ACK missing")

    max_server_end = max(seq_end(p) for p in server_packets)
    max_client_end = max(seq_end(p) for p in client_packets)
    for p in client_packets:
        if p["flags"] & FLAG_ACK and not (synack["seq"] + 1 <= p["ack"] <= max_server_end):
            raise AssertionError(f"client ACK outside sent server range: {p['ack']}")
    for p in server_packets:
        if p["flags"] & FLAG_ACK and not (syn["seq"] + 1 <= p["ack"] <= max_client_end):
            raise AssertionError(f"server ACK outside sent client range: {p['ack']}")

    by_seq = {}
    retrans_gaps = []
    for p in server_packets:
        if not p["payload"]:
            continue
        old = by_seq.get(p["seq"])
        if old is None:
            by_seq[p["seq"]] = (p["payload"], p["ts"])
            continue
        old_payload, first_ts = old
        if old_payload != p["payload"]:
            raise AssertionError(f"same server TCP Seq {p['seq']} changed ciphertext")
        gap = p["ts"] - first_ts
        if gap >= 0.5:
            retrans_gaps.append((p["seq"], gap))
    if not retrans_gaps:
        raise AssertionError("no >=0.5s same-sequence server retransmission captured")

    if not any(p["flags"] & FLAG_FIN for p in server_packets):
        raise AssertionError("server FIN missing")
    if not any(p["flags"] & FLAG_FIN for p in client_packets):
        raise AssertionError("client FIN missing")

    summary = {
        "source_sha": args.source_sha,
        "link_type": network,
        "captured_flow_packets": len(flow),
        "capture_dropped_by_kernel": dropped,
        "client": f"{client_ip}:{client_port}",
        "server": f"{args.server_ip}:{args.port}",
        "server_mss": synack["options"].get("mss"),
        "server_window_scale": synack["options"].get("ws"),
        "server_sack_permitted": bool(synack["options"].get("sack")),
        "server_checksums_verified": len(server_packets),
        "no_rst": True,
        "no_ip_fragmentation": True,
        "server_fin": True,
        "client_fin": True,
        "same_seq_retransmissions": [{"seq": seq, "gap_seconds": round(gap, 6)} for seq, gap in retrans_gaps],
        "result": "PASS",
    }
    Path(args.output).write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(summary, indent=2))

if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        print(f"P2 pcap check FAIL: {exc}", file=sys.stderr)
        raise
