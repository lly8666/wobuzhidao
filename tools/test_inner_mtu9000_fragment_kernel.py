#!/usr/bin/env python3
"""Actions-only real Linux IPv4 fragmentation boundary at inner MTU 9000.

This is a functional kernel fixture, not a throughput or product performance
sample. It records only bounded numeric metadata, never UDP payload bytes.
"""
import errno
import fcntl
import json
import os
import socket
import struct
import subprocess
import sys
import time

TUNSETIFF = 0x400454CA
IFF_TUN_NO_PI = 0x1001
IP_MTU_DISCOVER = 10
IP_PMTUDISC_DONT = 0
IP_PMTUDISC_DO = 2
SOURCE = "198.18.0.1"
DEST = "10.66.1.1"
SPORT = 18446
DPORT = 45000
ROUNDS = 24
MAX_TIMELINE = 512


def checksum_ok(header):
    words = struct.unpack("!%dH" % (len(header) // 2), header)
    total = sum(words)
    while total >> 16:
        total = (total & 0xffff) + (total >> 16)
    return total == 0xffff


def packet_meta(packet):
    if len(packet) < 20 or packet[0] >> 4 != 4:
        raise AssertionError("non-IPv4 TUN packet")
    ihl = (packet[0] & 15) * 4
    total = int.from_bytes(packet[2:4], "big")
    if ihl < 20 or total < ihl or len(packet) != total:
        raise AssertionError("malformed/truncated TUN IPv4 packet")
    flags = int.from_bytes(packet[6:8], "big")
    row = {
        "ip_id": int.from_bytes(packet[4:6], "big"),
        "offset": (flags & 8191) * 8,
        "mf": bool(flags & 8192),
        "df": bool(flags & 16384),
        "ip_length": total,
        "header_length": ihl,
        "fragment_payload_length": total - ihl,
        "header_checksum_ok": checksum_ok(packet[:ihl]),
        "sequence": None,
        "udp_length": None,
    }
    if row["offset"] == 0 and len(packet) >= ihl + 16:
        udp = packet[ihl:]
        if int.from_bytes(udp[:2], "big") == SPORT and int.from_bytes(udp[2:4], "big") == DPORT and udp[8:12] == b"P7M1":
            row["sequence"] = int.from_bytes(udp[12:16], "little")
            row["udp_length"] = int.from_bytes(udp[4:6], "big")
    return row


def expected_fragments(payload):
    if payload == 96:
        return 1
    if payload == 8972:
        return 1
    if payload == 8973:
        return 2
    if payload == 65507:
        return 8
    raise AssertionError("unexpected successful payload")


def validate_fragments(rows, payload, df):
    expected = expected_fragments(payload)
    if len(rows) != expected:
        raise AssertionError((payload, df, "fragment_count", len(rows), expected))
    if any(not r["header_checksum_ok"] for r in rows):
        raise AssertionError((payload, df, "bad_header_checksum"))
    rows = sorted(rows, key=lambda r: r["offset"])
    if rows[0]["offset"] != 0 or rows[0]["udp_length"] != payload + 8:
        raise AssertionError((payload, df, "missing first UDP fragment"))
    end = 0
    for index, row in enumerate(rows):
        if row["offset"] != end:
            raise AssertionError((payload, df, "fragment_gap", end, row["offset"]))
        end += row["fragment_payload_length"]
        if index + 1 < len(rows) and not row["mf"]:
            raise AssertionError((payload, df, "early_last_fragment"))
    if rows[-1]["mf"]:
        raise AssertionError((payload, df, "missing_last_fragment"))
    if end != payload + 8:
        raise AssertionError((payload, df, "payload_total", end, payload + 8))
    if df and any(not r["df"] for r in rows):
        raise AssertionError((payload, df, "DF_not_set"))
    if payload == 8973 and not df:
        if [r["ip_length"] for r in rows] != [8996, 25]:
            raise AssertionError((payload, "unexpected_9001_shape", [r["ip_length"] for r in rows]))


def inside():
    fd = os.open("/dev/net/tun", os.O_RDWR | os.O_NONBLOCK)
    timeline = []
    try:
        fcntl.ioctl(fd, TUNSETIFF, struct.pack("16sH22x", b"wbdg0", IFF_TUN_NO_PI))
        for cmd in [
            ["ip", "link", "set", "lo", "up"],
            ["ip", "address", "add", SOURCE + "/32", "dev", "lo"],
            ["ip", "link", "set", "wbdg0", "mtu", "9000", "up"],
            ["ip", "route", "add", "10.66.0.0/16", "dev", "wbdg0"],
        ]:
            subprocess.run(cmd, check=True)

        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        try:
            sock.bind((SOURCE, SPORT))
            target = (DEST, DPORT)
            seq = 1

            def one(payload, df, expect_error=False):
                nonlocal seq
                if len(timeline) >= MAX_TIMELINE:
                    raise AssertionError("timeline bound exceeded")
                current = seq
                seq += 1
                body = bytearray(payload)
                body[:4] = b"P7M1"
                body[4:8] = current.to_bytes(4, "little")
                for i in range(8, payload):
                    body[i] = (i - 8) & 255
                sock.setsockopt(socket.IPPROTO_IP, IP_MTU_DISCOVER, IP_PMTUDISC_DO if df else IP_PMTUDISC_DONT)
                started = time.monotonic_ns()
                try:
                    sent = sock.sendto(body, target)
                    send_end = time.monotonic_ns()
                    error_no = None
                except OSError as exc:
                    sent = 0
                    send_end = time.monotonic_ns()
                    error_no = exc.errno
                row = {
                    "sequence": current,
                    "udp_payload": payload,
                    "ipv4_total": payload + 28,
                    "dont_fragment": df,
                    "sent": sent == payload,
                    "send_call_ns": send_end - started,
                    "error_errno": error_no,
                    "fragment_count": 0,
                    "first_tun_arrival_ns": None,
                    "complete_tun_arrival_ns": None,
                    "ip_lengths": [],
                    "offsets": [],
                }
                if expect_error:
                    if sent or error_no != errno.EMSGSIZE:
                        raise AssertionError((payload, df, "expected_EMSGSIZE", sent, error_no))
                    timeline.append(row)
                    return

                if sent != payload or error_no is not None:
                    raise AssertionError((payload, df, "unexpected_send_failure", sent, error_no))
                expected = expected_fragments(payload)
                fragments = []
                first_arrival = None
                deadline = time.monotonic() + 0.25
                ip_id = None
                while len(fragments) < expected and time.monotonic() < deadline:
                    try:
                        packet = os.read(fd, 65535)
                    except BlockingIOError:
                        time.sleep(0.0005)
                        continue
                    arrived = time.monotonic_ns()
                    meta = packet_meta(packet)
                    if meta["sequence"] == current:
                        ip_id = meta["ip_id"]
                        first_arrival = arrived
                    if ip_id is None or meta["ip_id"] != ip_id:
                        raise AssertionError((payload, df, "unexpected_interleaved_TUN_packet", meta))
                    fragments.append(meta)
                    if first_arrival is None:
                        first_arrival = arrived
                if len(fragments) != expected:
                    raise AssertionError((payload, df, "TUN_timeout", len(fragments), expected))
                validate_fragments(fragments, payload, df)
                row.update({
                    "fragment_count": len(fragments),
                    "first_tun_arrival_ns": first_arrival - started,
                    "complete_tun_arrival_ns": time.monotonic_ns() - started,
                    "ip_lengths": [r["ip_length"] for r in sorted(fragments, key=lambda x: x["offset"])],
                    "offsets": [r["offset"] for r in sorted(fragments, key=lambda x: x["offset"])],
                })
                timeline.append(row)

            for _ in range(ROUNDS):
                one(8972, False)
                one(96, False)
                one(8973, False)
                one(96, False)
                one(65507, False)
                one(96, False)
                one(65508, False, True)
                one(8972, True)
                one(8973, True, True)
                one(65507, True, True)
                one(65508, True, True)
        finally:
            sock.close()
    finally:
        os.close(fd)

    counts = {}
    for row in timeline:
        key = "%d/df%d" % (row["udp_payload"], int(row["dont_fragment"]))
        item = counts.setdefault(key, {"attempts": 0, "sent": 0, "message_size_error": 0, "fragments": 0})
        item["attempts"] += 1
        item["sent"] += int(row["sent"])
        item["message_size_error"] += int(row["error_errno"] == errno.EMSGSIZE)
        item["fragments"] += row["fragment_count"]
    result = {
        "schema": "wbd-inner-mtu9000-kernel/v1",
        "state": "PASS",
        "scope": "real Linux kernel IPv4 fragmentation functional fixture; no WBD performance claim",
        "inner_mtu": 9000,
        "rounds": ROUNDS,
        "counts": counts,
        "timeline": timeline,
        "no_raw_payload_stored": True,
    }
    print(json.dumps(result, separators=(",", ":")))


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true" or os.geteuid() != 0:
        raise RuntimeError("Only privileged Actions functional fixture permitted")
    if "--inside" in sys.argv:
        inside()
        return
    name = "wbd-mtu9000-" + str(os.getpid())
    subprocess.run(["ip", "netns", "add", name], check=True)
    try:
        subprocess.run(["ip", "netns", "exec", name, sys.executable, os.path.abspath(__file__), "--inside"], check=True, timeout=20)
    finally:
        subprocess.run(["ip", "netns", "delete", name], check=True)


if __name__ == "__main__":
    main()
