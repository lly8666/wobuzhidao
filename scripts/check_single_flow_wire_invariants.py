#!/usr/bin/env python3
"""Qualify the public single-flow wire contract from a libpcap capture.

The checker is intentionally dependency-free so the release workflow can run it
on a stock Python installation. It proves public transport invariants directly
from captured packets instead of trusting WBD readiness logs:

* exactly one client<->server TCP 4-tuple;
* one client SYN incarnation and one server SYN-ACK incarnation (retransmits of
  the same initial sequence number are allowed);
* no second SYN after payload has started;
* no FIN or RST during the TLS->DTLS mode switch/session;
* client and server payload remain in one continuous TCP sequence space rooted
  at each side's original ISN+1, with retransmissions allowed but no reset/gap;
* the first client payload is a TLS ClientHello record;
* an optional test-only post-bootstrap marker can be required later in that same
  client sequence stream, proving bytes after the mode barrier did not use a
  second connection.

The full E2E workflow separately proves real DTLS 1.3 readiness and application
round trips. Together those facts qualify the one-public-flow/no-HOL contract.
"""

from __future__ import annotations

import argparse
import ipaddress
import json
import struct
import sys
from dataclasses import dataclass
from pathlib import Path

TCP_FIN = 0x01
TCP_SYN = 0x02
TCP_RST = 0x04
TCP_ACK = 0x10


@dataclass(frozen=True)
class Packet:
    index: int
    src: str
    dst: str
    sport: int
    dport: int
    seq: int
    ack: int
    flags: int
    payload: bytes


def fail(message: str) -> "NoReturn":
    raise SystemExit(f"single-flow wire invariant failed: {message}")


def read_pcap(path: Path) -> tuple[int, list[bytes]]:
    raw = path.read_bytes()
    if len(raw) < 24:
        fail("pcap is shorter than the global header")
    magic = raw[:4]
    if magic == b"\xd4\xc3\xb2\xa1":
        endian = "<"
    elif magic == b"\xa1\xb2\xc3\xd4":
        endian = ">"
    elif magic == b"\x4d\x3c\xb2\xa1":
        endian = "<"  # nanosecond pcap
    elif magic == b"\xa1\xb2\x3c\x4d":
        endian = ">"  # nanosecond pcap
    else:
        fail(f"unsupported pcap magic {magic.hex()}")
    linktype = struct.unpack_from(endian + "I", raw, 20)[0]
    frames: list[bytes] = []
    off = 24
    while off < len(raw):
        if off + 16 > len(raw):
            fail("truncated pcap packet header")
        _ts_sec, _ts_frac, incl_len, _orig_len = struct.unpack_from(endian + "IIII", raw, off)
        off += 16
        if off + incl_len > len(raw):
            fail("truncated pcap packet body")
        frames.append(raw[off : off + incl_len])
        off += incl_len
    return linktype, frames


def ethernet_ipv4(frame: bytes, linktype: int) -> bytes | None:
    if linktype != 1:  # DLT_EN10MB; qualification captures are on veth/Npcap Ethernet.
        fail(f"unsupported pcap linktype {linktype}; expected Ethernet(1)")
    if len(frame) < 14:
        return None
    off = 14
    ethertype = struct.unpack_from("!H", frame, 12)[0]
    while ethertype in (0x8100, 0x88A8):
        if len(frame) < off + 4:
            return None
        ethertype = struct.unpack_from("!H", frame, off + 2)[0]
        off += 4
    if ethertype != 0x0800:
        return None
    return frame[off:]


def parse_tcp(frame: bytes, linktype: int, index: int) -> Packet | None:
    ip = ethernet_ipv4(frame, linktype)
    if ip is None or len(ip) < 20 or ip[0] >> 4 != 4:
        return None
    ihl = (ip[0] & 0x0F) * 4
    if ihl < 20 or len(ip) < ihl + 20 or ip[9] != 6:
        return None
    frag = struct.unpack_from("!H", ip, 6)[0]
    if frag & 0x1FFF:
        return None
    total = struct.unpack_from("!H", ip, 2)[0]
    if total == 0 or total > len(ip):
        total = len(ip)
    if total < ihl + 20:
        return None
    tcp = ip[ihl:total]
    doff = (tcp[12] >> 4) * 4
    if doff < 20 or doff > len(tcp):
        return None
    src = str(ipaddress.ip_address(ip[12:16]))
    dst = str(ipaddress.ip_address(ip[16:20]))
    sport, dport = struct.unpack_from("!HH", tcp, 0)
    seq, ack = struct.unpack_from("!II", tcp, 4)
    return Packet(index, src, dst, sport, dport, seq, ack, tcp[13], tcp[doff:])


def relative(seq: int, base: int) -> int:
    off = (seq - base) & 0xFFFFFFFF
    if off >= 0x80000000:
        fail(f"sequence moved backwards/reset: seq={seq} base={base} relative={off}")
    return off


def rebuild_payload(packets: list[Packet], base: int, label: str) -> bytes:
    payload_packets = [p for p in packets if p.payload]
    if not payload_packets:
        fail(f"{label} carried no payload")
    cells: dict[int, int] = {}
    max_end = 0
    for p in payload_packets:
        off = relative(p.seq, base)
        end = off + len(p.payload)
        if end >= 0x80000000:
            fail(f"{label} payload sequence span is implausibly large")
        max_end = max(max_end, end)
        for i, byte in enumerate(p.payload, off):
            old = cells.get(i)
            if old is not None and old != byte:
                fail(f"{label} retransmission changed payload byte at relative seq {i}")
            cells[i] = byte
    if 0 not in cells:
        fail(f"{label} first payload does not begin at ISN+1")
    missing = next((i for i in range(max_end) if i not in cells), None)
    if missing is not None:
        fail(f"{label} sequence space has an unrecovered payload gap at relative seq {missing}")
    return bytes(cells[i] for i in range(max_end))


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("pcap", type=Path)
    ap.add_argument("--client-ip", required=True)
    ap.add_argument("--server-ip", required=True)
    ap.add_argument("--client-port", required=True, type=int)
    ap.add_argument("--server-port", required=True, type=int)
    ap.add_argument("--min-client-payload", type=int, default=256)
    ap.add_argument("--min-server-payload", type=int, default=256)
    ap.add_argument("--require-client-marker", default="", help="ASCII marker that must occur after the TLS ClientHello in the same client seq-space")
    args = ap.parse_args()

    linktype, frames = read_pcap(args.pcap)
    parsed = [p for i, f in enumerate(frames) if (p := parse_tcp(f, linktype, i)) is not None]

    relevant: list[Packet] = []
    stray: list[Packet] = []
    for p in parsed:
        client_to_server_ip = p.src == args.client_ip and p.dst == args.server_ip and p.dport == args.server_port
        server_to_client_ip = p.src == args.server_ip and p.dst == args.client_ip and p.sport == args.server_port
        if not (client_to_server_ip or server_to_client_ip):
            continue
        expected = (
            client_to_server_ip and p.sport == args.client_port
        ) or (
            server_to_client_ip and p.dport == args.client_port
        )
        (relevant if expected else stray).append(p)
    if stray:
        p = stray[0]
        fail(f"second public 4-tuple observed at packet {p.index}: {p.src}:{p.sport}->{p.dst}:{p.dport}")
    if not relevant:
        fail("no packets for the expected public 4-tuple")

    c2s = [p for p in relevant if p.src == args.client_ip]
    s2c = [p for p in relevant if p.src == args.server_ip]
    client_syn = [p for p in c2s if p.flags & TCP_SYN and not p.flags & TCP_ACK]
    server_synack = [p for p in s2c if (p.flags & (TCP_SYN | TCP_ACK)) == (TCP_SYN | TCP_ACK)]
    if not client_syn:
        fail("no client SYN")
    if not server_synack:
        fail("no server SYN-ACK")
    client_isns = {p.seq for p in client_syn}
    server_isns = {p.seq for p in server_synack}
    if len(client_isns) != 1:
        fail(f"multiple client SYN incarnations: {sorted(client_isns)}")
    if len(server_isns) != 1:
        fail(f"multiple server SYN-ACK incarnations: {sorted(server_isns)}")

    first_payload_index = min((p.index for p in relevant if p.payload), default=None)
    if first_payload_index is None:
        fail("flow never carried payload")
    if any(p.index > first_payload_index for p in client_syn):
        fail("a new client SYN appeared after application payload began")
    if any(p.flags & (TCP_FIN | TCP_RST) for p in relevant):
        bad = next(p for p in relevant if p.flags & (TCP_FIN | TCP_RST))
        fail(f"FIN/RST observed during single-flow session at packet {bad.index} flags=0x{bad.flags:02x}")

    client_isn = next(iter(client_isns))
    server_isn = next(iter(server_isns))
    client_stream = rebuild_payload(c2s, (client_isn + 1) & 0xFFFFFFFF, "client")
    server_stream = rebuild_payload(s2c, (server_isn + 1) & 0xFFFFFFFF, "server")
    if len(client_stream) < args.min_client_payload:
        fail(f"client payload too short to cover bootstrap+post-switch traffic: {len(client_stream)}")
    if len(server_stream) < args.min_server_payload:
        fail(f"server payload too short to cover bootstrap+post-switch traffic: {len(server_stream)}")
    if len(client_stream) < 6 or client_stream[0] != 22 or client_stream[1] != 3 or client_stream[5] != 1:
        fail("client sequence space does not start with a TLS ClientHello record")

    marker_offset = -1
    if args.require_client_marker:
        marker = args.require_client_marker.encode("ascii")
        marker_offset = client_stream.find(marker)
        if marker_offset < 0:
            fail(f"required post-bootstrap client marker {args.require_client_marker!r} was not captured")
        if marker_offset < 6:
            fail("post-bootstrap marker appeared before the TLS ClientHello")

    summary = {
        "tuple": f"{args.client_ip}:{args.client_port}-{args.server_ip}:{args.server_port}",
        "client_isn": client_isn,
        "server_isn": server_isn,
        "client_syn_packets": len(client_syn),
        "server_synack_packets": len(server_synack),
        "client_payload_bytes": len(client_stream),
        "server_payload_bytes": len(server_stream),
        "client_marker_offset": marker_offset,
        "fin_rst": 0,
        "seq_spaces": 1,
    }
    print("SINGLE_FLOW_WIRE_INVARIANT_PASS " + json.dumps(summary, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
