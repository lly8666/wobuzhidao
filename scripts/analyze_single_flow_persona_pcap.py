#!/usr/bin/env python3
"""Report the public SYN and first TLS ClientHello persona from a classic pcap.

The tool intentionally has no third-party dependencies so qualification can run
on stock GitHub runners. It understands Ethernet + IPv4 + TCP classic pcap,
reassembles the first client byte stream by TCP sequence number, and reports
both raw ClientHello ordering and a GREASE-stripped JA3 value.
"""

import argparse
import hashlib
import json
import struct

GREASE = {0x0A0A + 0x1010 * i for i in range(16)}


def read_pcap(path):
    b = open(path, "rb").read()
    if len(b) < 24:
        raise ValueError("pcap is truncated")
    magic = b[:4]
    if magic == b"\xd4\xc3\xb2\xa1":
        endian = "<"
    elif magic == b"\xa1\xb2\xc3\xd4":
        endian = ">"
    else:
        raise ValueError("only classic microsecond pcap is supported")
    _, _, _, _, _, _, linktype = struct.unpack(endian + "IHHIIII", b[:24])
    if linktype != 1:
        raise ValueError("only Ethernet pcap is supported")
    pos = 24
    out = []
    while pos + 16 <= len(b):
        ts_sec, ts_usec, incl_len, _ = struct.unpack(endian + "IIII", b[pos:pos + 16])
        pos += 16
        if pos + incl_len > len(b):
            raise ValueError("truncated pcap packet")
        out.append((ts_sec, ts_usec, b[pos:pos + incl_len]))
        pos += incl_len
    return out


def parse_ipv4_tcp(frame):
    if len(frame) < 14:
        return None
    ethertype = struct.unpack("!H", frame[12:14])[0]
    off = 14
    if ethertype == 0x8100:
        if len(frame) < 18:
            return None
        ethertype = struct.unpack("!H", frame[16:18])[0]
        off = 18
    if ethertype != 0x0800 or len(frame) < off + 20:
        return None
    ip = frame[off:]
    if ip[0] >> 4 != 4:
        return None
    ihl = (ip[0] & 0x0F) * 4
    if ihl < 20 or len(ip) < ihl or ip[9] != 6:
        return None
    total = struct.unpack("!H", ip[2:4])[0]
    if total < ihl + 20 or len(ip) < total:
        return None
    tcp = ip[ihl:total]
    sport, dport, seq, ack, off_flags, window, _, _ = struct.unpack("!HHIIHHHH", tcp[:20])
    data_off = ((off_flags >> 12) & 0x0F) * 4
    if data_off < 20 or len(tcp) < data_off:
        return None
    return {
        "ttl": ip[8],
        "src": ".".join(str(x) for x in ip[12:16]),
        "dst": ".".join(str(x) for x in ip[16:20]),
        "sport": sport,
        "dport": dport,
        "seq": seq,
        "ack": ack,
        "flags": off_flags & 0x01FF,
        "window": window,
        "options": tcp[20:data_off],
        "payload": tcp[data_off:],
    }


def parse_tcp_options(raw):
    out = []
    p = 0
    while p < len(raw):
        kind = raw[p]
        if kind == 0:
            out.append({"kind": 0, "name": "EOL"})
            break
        if kind == 1:
            out.append({"kind": 1, "name": "NOP"})
            p += 1
            continue
        if p + 2 > len(raw):
            raise ValueError("truncated TCP option")
        ln = raw[p + 1]
        if ln < 2 or p + ln > len(raw):
            raise ValueError("invalid TCP option length")
        body = raw[p + 2:p + ln]
        ent = {"kind": kind, "length": ln, "hex": body.hex()}
        if kind == 2 and len(body) == 2:
            ent.update(name="MSS", value=struct.unpack("!H", body)[0])
        elif kind == 3 and len(body) == 1:
            ent.update(name="WS", value=body[0])
        elif kind == 4 and len(body) == 0:
            ent.update(name="SACK_PERMITTED")
        elif kind == 8 and len(body) == 8:
            ent.update(name="TIMESTAMP")
        else:
            ent.update(name="OPTION_%d" % kind)
        out.append(ent)
        p += ln
    return out


def contiguous_stream(segments):
    first = {}
    for seq, payload in segments:
        if payload and seq not in first:
            first[seq] = payload
    if not first:
        raise ValueError("no client TCP payload found")
    pos = min(first)
    result = bytearray()
    while pos in first:
        payload = first[pos]
        result.extend(payload)
        pos += len(payload)
    return bytes(result)


def u16_list(raw):
    if len(raw) % 2:
        raise ValueError("odd uint16 vector")
    return [struct.unpack("!H", raw[i:i + 2])[0] for i in range(0, len(raw), 2)]


def parse_client_hello(stream):
    if len(stream) < 9 or stream[0] != 22:
        raise ValueError("first reassembled payload is not a TLS handshake record")
    record_version = struct.unpack("!H", stream[1:3])[0]
    record_len = struct.unpack("!H", stream[3:5])[0]
    if len(stream) < 5 + record_len:
        raise ValueError("TLS record is incomplete")
    record = stream[5:5 + record_len]
    if len(record) < 4 or record[0] != 1:
        raise ValueError("first TLS handshake message is not ClientHello")
    hello_len = int.from_bytes(record[1:4], "big")
    if len(record) < 4 + hello_len:
        raise ValueError("ClientHello is incomplete")
    b = record[4:4 + hello_len]
    p = 0
    legacy_version = struct.unpack("!H", b[p:p + 2])[0]
    p += 2
    random = b[p:p + 32]
    p += 32
    sid_len = b[p]
    p += 1
    session_id = b[p:p + sid_len]
    p += sid_len
    cs_len = struct.unpack("!H", b[p:p + 2])[0]
    p += 2
    ciphers = u16_list(b[p:p + cs_len])
    p += cs_len
    comp_len = b[p]
    p += 1 + comp_len
    ext_total = struct.unpack("!H", b[p:p + 2])[0]
    p += 2
    ext_end = p + ext_total
    extensions = []
    ext_data = {}
    while p < ext_end:
        typ, ln = struct.unpack("!HH", b[p:p + 4])
        p += 4
        body = b[p:p + ln]
        p += ln
        extensions.append(typ)
        ext_data.setdefault(typ, []).append(body)
    if p != ext_end:
        raise ValueError("invalid ClientHello extension vector")

    groups = []
    if 10 in ext_data:
        raw = ext_data[10][0]
        ln = struct.unpack("!H", raw[:2])[0]
        groups = u16_list(raw[2:2 + ln])
    point_formats = []
    if 11 in ext_data:
        raw = ext_data[11][0]
        if raw:
            point_formats = list(raw[1:1 + raw[0]])
    sig_algs = []
    if 13 in ext_data:
        raw = ext_data[13][0]
        ln = struct.unpack("!H", raw[:2])[0]
        sig_algs = u16_list(raw[2:2 + ln])
    supported_versions = []
    if 43 in ext_data:
        raw = ext_data[43][0]
        if raw:
            supported_versions = u16_list(raw[1:1 + raw[0]])
    key_shares = []
    if 51 in ext_data:
        raw = ext_data[51][0]
        if len(raw) >= 2:
            end = 2 + struct.unpack("!H", raw[:2])[0]
            q = 2
            while q + 4 <= end:
                group, ln = struct.unpack("!HH", raw[q:q + 4])
                q += 4
                key_shares.append({"group": group, "bytes": ln})
                q += ln

    sni = ""
    if 0 in ext_data:
        raw = ext_data[0][0]
        if len(raw) >= 5:
            total = struct.unpack("!H", raw[:2])[0]
            q = 2
            while q + 3 <= min(len(raw), 2 + total):
                name_type = raw[q]
                ln = struct.unpack("!H", raw[q + 1:q + 3])[0]
                q += 3
                name = raw[q:q + ln]
                q += ln
                if name_type == 0:
                    sni = name.decode("ascii", "replace")
                    break

    alpn = []
    if 16 in ext_data:
        raw = ext_data[16][0]
        if len(raw) >= 2:
            end = 2 + struct.unpack("!H", raw[:2])[0]
            q = 2
            while q < min(end, len(raw)):
                ln = raw[q]
                q += 1
                alpn.append(raw[q:q + ln].decode("ascii", "replace"))
                q += ln

    ja3_ciphers = [x for x in ciphers if x not in GREASE]
    ja3_exts = [x for x in extensions if x not in GREASE]
    ja3_groups = [x for x in groups if x not in GREASE]
    ja3 = "%d,%s,%s,%s,%s" % (
        legacy_version,
        "-".join(map(str, ja3_ciphers)),
        "-".join(map(str, ja3_exts)),
        "-".join(map(str, ja3_groups)),
        "-".join(map(str, point_formats)),
    )
    return {
        "record_version": record_version,
        "record_bytes": record_len + 5,
        "hello_bytes": hello_len + 4,
        "legacy_version": legacy_version,
        "random_hex": random.hex(),
        "session_id_bytes": len(session_id),
        "ciphers": ciphers,
        "extensions": extensions,
        "supported_groups": groups,
        "signature_algorithms": sig_algs,
        "supported_versions": supported_versions,
        "key_shares": key_shares,
        "sni": sni,
        "alpn": alpn,
        "grease_cipher_count": sum(x in GREASE for x in ciphers),
        "grease_extension_count": sum(x in GREASE for x in extensions),
        "grease_group_count": sum(x in GREASE for x in groups),
        "ja3": ja3,
        "ja3_md5": hashlib.md5(ja3.encode()).hexdigest(),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("pcap")
    ap.add_argument("--client-ip", required=True)
    ap.add_argument("--server-ip", required=True)
    ap.add_argument("--client-port", required=True, type=int)
    ap.add_argument("--server-port", required=True, type=int)
    ap.add_argument("--out")
    args = ap.parse_args()

    packets = [parse_ipv4_tcp(frame) for _, _, frame in read_pcap(args.pcap)]
    packets = [p for p in packets if p]
    lineage = [p for p in packets if p["src"] == args.client_ip and p["dst"] == args.server_ip and p["sport"] == args.client_port and p["dport"] == args.server_port]
    syns = [p for p in lineage if p["flags"] & 0x02 and not p["flags"] & 0x10]
    if not syns:
        raise SystemExit("client SYN not found")
    syn = syns[0]
    payloads = [(p["seq"], p["payload"]) for p in lineage if p["payload"]]
    stream = contiguous_stream(payloads)
    report = {
        "flow": "%s:%d-%s:%d" % (args.client_ip, args.client_port, args.server_ip, args.server_port),
        "syn": {
            "unique_syn_sequences": len(set(p["seq"] for p in syns)),
            "ttl": syn["ttl"],
            "window": syn["window"],
            "options_hex": syn["options"].hex(),
            "options": parse_tcp_options(syn["options"]),
        },
        "client_hello": parse_client_hello(stream),
    }
    text = json.dumps(report, indent=2, sort_keys=True) + "\n"
    if args.out:
        open(args.out, "w").write(text)
    print(text, end="")


if __name__ == "__main__":
    main()
