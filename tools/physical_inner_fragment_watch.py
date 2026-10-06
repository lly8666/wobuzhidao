"""Controlled IPv4 fragment metadata only; no retained packet body.

Linux observes the server TUN before product ingestion. Windows consumes a
bounded Pktmon pcapng from the selected Wintun component. Capture coverage and
drops must be checked separately; a row is not proof of application delivery.
"""
import argparse
import ipaddress
import json
import signal
import socket
import struct
import time
from pathlib import Path

MAX_BYTES = 16 * 1024 * 1024
MAX_ROWS = 40000


def metadata(packet, source, destination):
    if len(packet) < 20 or packet[0] >> 4 != 4:
        return None
    ihl = (packet[0] & 15) * 4
    total = int.from_bytes(packet[2:4], 'big')
    if ihl < 20 or len(packet) < ihl or total < ihl:
        raise ValueError('Malformed IPv4 header')
    src, dst = socket.inet_ntoa(packet[12:16]), socket.inet_ntoa(packet[16:20])
    if src != source or dst != destination or packet[9] != 17:
        return None
    flags = int.from_bytes(packet[6:8], 'big')
    words = struct.unpack('!%dH' % (ihl // 2), packet[:ihl])
    checksum = sum(words)
    while checksum >> 16:
        checksum = (checksum & 65535) + (checksum >> 16)
    row = dict(ip_id=int.from_bytes(packet[4:6], 'big'), offset=(flags & 8191) * 8,
               mf=bool(flags & 8192), ip_length=total, header_length=ihl,
               fragment_payload_length=total-ihl, header_checksum_ok=checksum == 65535,
               captured_ip_bytes=len(packet), sequence=None, udp_length=None)
    if row['offset'] == 0 and len(packet) >= ihl+16:
        udp = packet[ihl:]
        # Only identify our existing controlled echo. Other body bytes are
        # neither decoded nor retained; nonfirst fragments have no UDP header.
        if int.from_bytes(udp[:2], 'big') == 18446 and udp[8:12] == b'P7M1':
            row['sequence'] = int.from_bytes(udp[12:16], 'little')
            row['udp_length'] = int.from_bytes(udp[4:6], 'big')
    return row


def l3(frame, linktype):
    if linktype == 1:
        if len(frame) < 14 or frame[12:14] != b'\x08\x00':
            return None
        return frame[14:]
    if linktype in (101, 228):
        return frame
    if linktype == 0 and len(frame) >= 4:
        return frame[4:]
    raise ValueError('Unsupported component link type: %d' % linktype)


def pcapng_rows(path, source, destination):
    if not 0 < path.stat().st_size <= MAX_BYTES:
        raise ValueError('Capture size outside bound')
    raw = path.read_bytes()
    pos, endian, interfaces, rows, blocks = 0, None, [], [], 0
    while pos < len(raw):
        if len(raw)-pos < 12:
            raise ValueError('Truncated pcapng block')
        if raw[pos:pos+4] == b'\x0a\x0d\x0d\x0a':
            magic = raw[pos+8:pos+12]
            if magic not in (b'\x4d\x3c\x2b\x1a', b'\x1a\x2b\x3c\x4d'):
                raise ValueError('Invalid pcapng endian marker')
            endian = '<' if magic == b'\x4d\x3c\x2b\x1a' else '>'
            interfaces = []
        if endian is None:
            raise ValueError('Missing section header')
        kind, size = struct.unpack_from(endian+'II', raw, pos)
        if size < 12 or size % 4 or size > len(raw)-pos or struct.unpack_from(endian+'I', raw, pos+size-4)[0] != size:
            raise ValueError('Invalid pcapng block size/trailer')
        body = raw[pos+8:pos+size-4]
        if kind == 1:
            if len(body) < 8:
                raise ValueError('Short interface block')
            linktype = struct.unpack_from(endian+'H', body)[0]
            resolution = 1e-6
            at = 8
            while at+4 <= len(body):
                code, n = struct.unpack_from(endian+'HH', body, at)
                at += 4
                if n > len(body)-at:
                    raise ValueError('Short interface option')
                if code == 0:
                    break
                if code == 9 and n == 1:
                    val = body[at]
                    resolution = 2 ** -(val & 127) if val & 128 else 10 ** -val
                at += (n+3)//4*4
            interfaces.append((linktype, resolution))
        elif kind == 6:
            if len(body) < 20:
                raise ValueError('Short enhanced packet')
            index, high, low, captured, original = struct.unpack_from(endian+'IIIII', body)
            if index >= len(interfaces) or captured > len(body)-20 or captured > original:
                raise ValueError('Invalid enhanced packet inventory')
            packet = l3(body[20:20+captured], interfaces[index][0])
            row = metadata(packet, source, destination) if packet else None
            if row is not None:
                row.update(unix_ns=int(((high << 32)+low)*interfaces[index][1]*1e9), interface_index=index)
                rows.append(row)
                if len(rows) > MAX_ROWS:
                    raise ValueError('Fragment row bound exceeded')
        elif kind in (2, 3):
            raise ValueError('Unsupported packet block; do not silently lose coverage')
        pos += size
        blocks += 1
    return dict(schema='wbd-inner-fragments/v1', source_ip=source, destination_ip=destination,
                no_raw_payload_stored=True, rows=rows, blocks=blocks,
                coverage='REQUIRES_PKT_MON_COUNTERS_AND_COMPONENT_SCOPE')


def watch(interface, source, destination, seconds, output, ready=None):
    if not 1 <= seconds <= 390:
        raise ValueError('Bounded duration required')
    stopping = False
    def stop(signum, frame):
        nonlocal stopping
        stopping = True
    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    started, rows, error, drops, packets = time.monotonic(), [], None, None, None
    hardware = int(Path('/sys/class/net/%s/type' % interface).read_text())
    if hardware not in (1, 65534):
        raise ValueError('Unknown shared TUN link format')
    # ETH_P_IP observes only packets entering the kernel on this TUN. Kernel
    # replies leave via PACKET_OUTGOING; ETH_P_ALL is required to see them.
    with socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x0003)) as sock:
        sock.bind((interface, 0))
        sock.settimeout(.5)
        if ready is not None:
            if ready.exists():
                raise ValueError('Existing readiness artifact')
            ready.write_text(json.dumps(dict(interface=interface,started_unix_ns=time.time_ns()))+'\n')
        # Existing default receive buffer, diagnostic only. No product change.
        while not stopping and time.monotonic()-started < seconds:
            try:
                frame, _ = sock.recvfrom(96)
            except socket.timeout:
                continue
            try:
                packet = l3(frame, 1 if hardware == 1 else 101)
                row = metadata(packet, source, destination) if packet else None
                if row is None:
                    continue
                row['unix_ns'] = time.time_ns()
                rows.append(row)
                if len(rows) >= MAX_ROWS:
                    error = 'ROW_BOUND_EXCEEDED'
                    break
            except ValueError as e:
                error = str(e)
                break
        packets, drops = struct.unpack('II', sock.getsockopt(263, 6, 8))
    result = dict(schema='wbd-inner-fragments/v1', source_ip=source, destination_ip=destination,
                  no_raw_payload_stored=True, interface=interface, rows=rows,
                  observer_packets=packets, observer_drops=drops, error=error,
                  actual_seconds=time.monotonic()-started, completed_window=not stopping and error is None)
    text = json.dumps(result, separators=(',', ':'))+'\n'
    if len(text.encode()) > MAX_BYTES:
        raise ValueError('Metadata output bound exceeded')
    output.write_text(text)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('--source', default='198.18.0.1')
    ap.add_argument('--destination', required=True)
    ap.add_argument('--output', type=Path, required=True)
    ap.add_argument('--pcapng', type=Path)
    ap.add_argument('--interface', default='wbdg0')
    ap.add_argument('--seconds', type=int, default=330)
    ap.add_argument('--ready', type=Path)
    args = ap.parse_args()
    if args.source != '198.18.0.1' or not ipaddress.IPv4Address(args.destination) in ipaddress.IPv4Network('10.66.0.0/16'):
        raise ValueError('Only controlled test target and leased destination allowed')
    if args.pcapng:
        args.output.write_text(json.dumps(pcapng_rows(args.pcapng, args.source, args.destination), separators=(',', ':'))+'\n')
    else:
        if args.interface != 'wbdg0':
            raise ValueError('Only shared test TUN permitted')
        watch(args.interface, args.source, args.destination, args.seconds, args.output, args.ready)


if __name__ == '__main__':
    main()
