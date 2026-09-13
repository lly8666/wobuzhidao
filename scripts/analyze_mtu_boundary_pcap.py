#!/usr/bin/env python3
import argparse
import json
import struct
from pathlib import Path

def parse_pcap(path):
    data = Path(path).read_bytes()
    if len(data) < 24:
        raise SystemExit("pcap too short")
    magic = data[:4]
    if magic in (b"\xd4\xc3\xb2\xa1", b"\x4d\x3c\xb2\xa1"):
        endian = "<"
    elif magic in (b"\xa1\xb2\xc3\xd4", b"\xa1\xb2\x3c\x4d"):
        endian = ">"
    else:
        raise SystemExit(f"unsupported pcap magic {magic.hex()}")
    _, _, _, _, _, network = struct.unpack(endian+"HHIIII", data[4:24])
    if network != 1:
        raise SystemExit(f"expected Ethernet pcap linktype=1 got={network}")
    off=24
    stats={
        "packets":0, "ipv4_packets":0, "fragmented_packets":0,
        "max_ip_total":0, "tcp_payload_packets":0, "max_tcp_payload":0,
        "udp_payload_packets":0, "max_udp_payload":0,
    }
    while off + 16 <= len(data):
        _, _, incl, _ = struct.unpack(endian+"IIII", data[off:off+16])
        off += 16
        frame = data[off:off+incl]
        off += incl
        stats["packets"] += 1
        if len(frame) < 14:
            continue
        pos=14
        ethertype=struct.unpack("!H", frame[12:14])[0]
        while ethertype in (0x8100,0x88a8):
            if len(frame) < pos+4:
                break
            ethertype=struct.unpack("!H", frame[pos+2:pos+4])[0]
            pos += 4
        if ethertype != 0x0800 or len(frame) < pos+20:
            continue
        ip=frame[pos:]
        ihl=(ip[0] & 0x0f)*4
        if (ip[0]>>4) != 4 or ihl < 20 or len(ip) < ihl:
            continue
        total=struct.unpack("!H", ip[2:4])[0]
        if total < ihl:
            continue
        stats["ipv4_packets"] += 1
        stats["max_ip_total"] = max(stats["max_ip_total"], total)
        frag=struct.unpack("!H", ip[6:8])[0]
        if frag & 0x3fff:
            stats["fragmented_packets"] += 1
        proto=ip[9]
        if proto == 6 and total >= ihl+20 and len(ip) >= ihl+20:
            doff=(ip[ihl+12]>>4)*4
            if doff >= 20 and total >= ihl+doff:
                payload=total-ihl-doff
                if payload:
                    stats["tcp_payload_packets"] += 1
                    stats["max_tcp_payload"] = max(stats["max_tcp_payload"], payload)
        elif proto == 17 and total >= ihl+8 and len(ip) >= ihl+8:
            udp_len=struct.unpack("!H", ip[ihl+4:ihl+6])[0]
            if udp_len >= 8:
                payload=udp_len-8
                if payload:
                    stats["udp_payload_packets"] += 1
                    stats["max_udp_payload"] = max(stats["max_udp_payload"], payload)
    return stats

ap=argparse.ArgumentParser()
ap.add_argument("pcap")
ap.add_argument("--protocol", choices=("tcp","udp"), required=True)
ap.add_argument("--max-ip-total", type=int, required=True)
ap.add_argument("--max-transport-payload", type=int, required=True)
ap.add_argument("--out", required=True)
args=ap.parse_args()
s=parse_pcap(args.pcap)
payload=s["max_tcp_payload"] if args.protocol=="tcp" else s["max_udp_payload"]
count=s["tcp_payload_packets"] if args.protocol=="tcp" else s["udp_payload_packets"]
s.update({
    "protocol":args.protocol,
    "max_ip_total_allowed":args.max_ip_total,
    "max_transport_payload_allowed":args.max_transport_payload,
    "selected_max_transport_payload":payload,
    "selected_payload_packets":count,
})
Path(args.out).write_text(json.dumps(s,sort_keys=True,indent=2)+"\n")
print("WBD_MTU_PCAP "+json.dumps(s,sort_keys=True))
if s["ipv4_packets"] == 0 or count == 0:
    raise SystemExit("no qualifying IPv4 transport payload packets")
if s["fragmented_packets"] != 0:
    raise SystemExit(f"IPv4 fragmentation observed: {s['fragmented_packets']}")
if s["max_ip_total"] > args.max_ip_total:
    raise SystemExit(f"IP total length exceeded: {s['max_ip_total']} > {args.max_ip_total}")
if payload > args.max_transport_payload:
    raise SystemExit(f"{args.protocol} payload exceeded: {payload} > {args.max_transport_payload}")
