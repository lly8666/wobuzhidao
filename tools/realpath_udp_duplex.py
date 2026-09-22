#!/usr/bin/env python3
import argparse
import json
import math
import socket
import struct
import threading
import time
import zlib
from pathlib import Path

MAGIC = b"WBDQ"
VERSION = 1
KIND_C2S = 1
KIND_S2C = 2
KIND_PROBE = 3
KIND_PROBE_REPLY = 4
KIND_REGISTER = 5
HEADER = struct.Struct("!4sBBHQQII")
SIZES = (64, 256, 1200)


def parse_addr(text):
    host, port = text.rsplit(":", 1)
    return host, int(port)


def percentile(values, q):
    if not values:
        return None
    values = sorted(values)
    idx = int(round((len(values) - 1) * q))
    return values[max(0, min(idx, len(values) - 1))]


def make_body(seq, size, seed):
    n = size - HEADER.size
    if n <= 0:
        return b""
    token = struct.pack("!QI", seq ^ ((seed & 0xffffffff) << 16), size)
    return (token * ((n + len(token) - 1) // len(token)))[:n]


def make_packet(kind, seq, size, seed, send_ns):
    body = make_body(seq, size, seed)
    crc = zlib.crc32(body) & 0xffffffff
    return HEADER.pack(MAGIC, VERSION, kind, size, seq, send_ns, seed & 0xffffffff, crc) + body


def decode_packet(data):
    if len(data) < HEADER.size:
        return None, "short"
    magic, version, kind, declared, seq, send_ns, seed, crc = HEADER.unpack_from(data)
    if magic != MAGIC or version != VERSION or declared != len(data):
        return None, "header"
    body = data[HEADER.size:]
    if zlib.crc32(body) & 0xffffffff != crc:
        return None, "crc"
    if body != make_body(seq, declared, seed):
        return None, "content"
    return {"kind": kind, "seq": seq, "size": declared, "send_ns": send_ns, "seed": seed}, None


class Stats:
    def __init__(self, start_ns, duration_s, drain_s):
        self.start_ns = start_ns
        self.duration_s = duration_s
        self.drain_s = drain_s
        self.seconds = int(duration_s)
        self.wall_seconds = int(math.ceil(duration_s + drain_s)) + 1
        self.lock = threading.Lock()
        self.sent_packets = [0] * self.seconds
        self.sent_bytes = [0] * self.seconds
        self.recv_packets = [0] * self.seconds
        self.recv_bytes = [0] * self.seconds
        self.recv_wall_packets = [0] * self.wall_seconds
        self.recv_wall_bytes = [0] * self.wall_seconds
        self.recv_seen = set()
        self.recv_duplicates = 0
        self.corrupt = 0
        self.unexpected = 0
        self.send_failures = 0
        self.skipped_slots = 0
        self.skipped_bytes = 0
        self.skipped_slots_by_second = [0] * self.seconds
        self.skipped_bytes_by_second = [0] * self.seconds
        self.send_lag_ns = []
        self.oneway_ns = []
        self.probe_sent = 0
        self.probe_recv = 0
        self.probe_rtt_ns = []
        self.probe_rtt_ns_by_second = [None] * self.seconds
        self.peer_ready_ns = None

    def second_for(self, send_ns):
        sec = int((send_ns - self.start_ns) // 1_000_000_000)
        if 0 <= sec < self.seconds:
            return sec
        return None

    def note_send(self, actual_send_ns, size, lag_ns):
        sec = self.second_for(actual_send_ns)
        with self.lock:
            if sec is not None:
                self.sent_packets[sec] += 1
                self.sent_bytes[sec] += size
            self.send_lag_ns.append(max(0, lag_ns))

    def wall_second_for(self, now_ns):
        sec = int((now_ns - self.start_ns) // 1_000_000_000)
        if 0 <= sec < self.wall_seconds:
            return sec
        return None

    def note_skip(self, target_ns, size):
        sec = self.second_for(target_ns)
        with self.lock:
            self.skipped_slots += 1
            self.skipped_bytes += size
            if sec is not None:
                self.skipped_slots_by_second[sec] += 1
                self.skipped_bytes_by_second[sec] += size

    def note_recv(self, packet, now_ns, expected_kind):
        if packet["kind"] != expected_kind:
            with self.lock:
                self.unexpected += 1
            return
        sec = self.second_for(packet["send_ns"])
        if sec is None:
            return
        key = packet["seq"]
        with self.lock:
            if key in self.recv_seen:
                self.recv_duplicates += 1
                return
            self.recv_seen.add(key)
            self.recv_packets[sec] += 1
            self.recv_bytes[sec] += packet["size"]
            wall_sec = self.wall_second_for(now_ns)
            if wall_sec is not None:
                self.recv_wall_packets[wall_sec] += 1
                self.recv_wall_bytes[wall_sec] += packet["size"]
            self.oneway_ns.append(max(0, now_ns - packet["send_ns"]))

    def snapshot(self):
        with self.lock:
            return {
                "sent_packets_by_second": list(self.sent_packets),
                "sent_bytes_by_second": list(self.sent_bytes),
                "recv_packets_by_second": list(self.recv_packets),
                "recv_bytes_by_second": list(self.recv_bytes),
                "recv_wall_packets_by_second": list(self.recv_wall_packets),
                "recv_wall_bytes_by_second": list(self.recv_wall_bytes),
                "sent_packets": sum(self.sent_packets),
                "sent_bytes": sum(self.sent_bytes),
                "recv_unique_packets": len(self.recv_seen),
                "recv_unique_bytes": sum(self.recv_bytes),
                "recv_duplicates": self.recv_duplicates,
                "corrupt": self.corrupt,
                "unexpected": self.unexpected,
                "send_failures": self.send_failures,
                "skipped_slots": self.skipped_slots,
                "skipped_bytes": self.skipped_bytes,
                "skipped_slots_by_second": list(self.skipped_slots_by_second),
                "skipped_bytes_by_second": list(self.skipped_bytes_by_second),
                "send_lag_p99_ns": percentile(self.send_lag_ns, 0.99),
                "oneway_p50_ns": percentile(self.oneway_ns, 0.50),
                "oneway_p95_ns": percentile(self.oneway_ns, 0.95),
                "oneway_p99_ns": percentile(self.oneway_ns, 0.99),
                "probe_sent": self.probe_sent,
                "probe_recv": self.probe_recv,
                "probe_rtt_p50_ns": percentile(self.probe_rtt_ns, 0.50),
                "probe_rtt_p95_ns": percentile(self.probe_rtt_ns, 0.95),
                "probe_rtt_p99_ns": percentile(self.probe_rtt_ns, 0.99),
                "probe_rtt_ns_by_second": list(self.probe_rtt_ns_by_second),
                "probe_timeouts": max(0, self.probe_sent - self.probe_recv),
                "peer_ready_ns": self.peer_ready_ns,
            }


def wait_until(target_ns):
    while True:
        now = time.monotonic_ns()
        remaining = target_ns - now
        if remaining <= 0:
            return now
        if remaining > 1_000_000:
            time.sleep((remaining - 500_000) / 1e9)


def run_sender(sock, peer_getter, kind, rate_mbps, seed, stats, stop_event):
    seq = 0
    cumulative = 0
    end_ns = stats.start_ns + int(stats.duration_s * 1e9)
    max_slot_lag_ns = 10_000_000
    wait_until(stats.start_ns)
    if rate_mbps <= 0:
        wait_until(end_ns)
        return
    bytes_per_second = rate_mbps * 1_000_000.0 / 8.0
    while not stop_event.is_set():
        size = SIZES[seq % len(SIZES)]
        target_ns = stats.start_ns + int((cumulative / bytes_per_second) * 1e9)
        if target_ns >= end_ns:
            break

        now_ns = time.monotonic_ns()
        if now_ns - target_ns > max_slot_lag_ns:
            stats.note_skip(target_ns, size)
            cumulative += size
            seq += 1
            continue

        wait_until(target_ns)
        actual_send_ns = time.monotonic_ns()
        if actual_send_ns - target_ns > max_slot_lag_ns:
            stats.note_skip(target_ns, size)
            cumulative += size
            seq += 1
            continue

        peer = peer_getter()
        if peer is None:
            with stats.lock:
                stats.send_failures += 1
            stats.note_skip(target_ns, size)
            cumulative += size
            seq += 1
            continue

        packet = make_packet(kind, seq, size, seed, actual_send_ns)
        try:
            sock.sendto(packet, peer)
            stats.note_send(actual_send_ns, size, actual_send_ns - target_ns)
        except OSError:
            with stats.lock:
                stats.send_failures += 1
        cumulative += size
        seq += 1


def receiver_loop(role, sock, expected_kind, stats, peer_holder, stop_ns, stop_event):
    sock.settimeout(0.1)
    while not stop_event.is_set() and time.monotonic_ns() < stop_ns:
        try:
            data, addr = sock.recvfrom(2048)
        except socket.timeout:
            continue
        except OSError:
            break
        now_ns = time.monotonic_ns()
        packet, err = decode_packet(data)
        if err:
            with stats.lock:
                stats.corrupt += 1
            continue
        kind = packet["kind"]
        if role == "target" and kind == KIND_REGISTER:
            with peer_holder["lock"]:
                peer_holder["peer"] = addr
            with stats.lock:
                if stats.peer_ready_ns is None:
                    stats.peer_ready_ns = now_ns
            continue
        if role == "target" and kind == KIND_PROBE:
            try:
                reply = make_packet(KIND_PROBE_REPLY, packet["seq"], packet["size"], packet["seed"], packet["send_ns"])
                sock.sendto(reply, addr)
            except OSError:
                pass
            continue
        if role == "biz" and kind == KIND_PROBE_REPLY:
            with stats.lock:
                rtt = max(0, now_ns - packet["send_ns"])
                stats.probe_recv += 1
                stats.probe_rtt_ns.append(rtt)
                sec = stats.second_for(packet["send_ns"])
                if sec is not None and stats.probe_rtt_ns_by_second[sec] is None:
                    stats.probe_rtt_ns_by_second[sec] = rtt
            continue
        stats.note_recv(packet, now_ns, expected_kind)


def register_loop(sock, peer, start_ns, seed, stop_event):
    seq = 0
    while not stop_event.is_set() and time.monotonic_ns() < start_ns:
        try:
            sock.sendto(make_packet(KIND_REGISTER, seq, 64, seed, time.monotonic_ns()), peer)
        except OSError:
            pass
        seq += 1
        time.sleep(0.2)


def probe_loop(sock, peer, start_ns, duration_s, seed, stats, stop_event, interval_s=1.0):
    if interval_s <= 0:
        return
    seq = 0
    end_ns = start_ns + int(duration_s * 1e9)
    step_ns = int(interval_s * 1e9)
    target = start_ns + min(500_000_000, step_ns)
    while target < end_ns and not stop_event.is_set():
        wait_until(target)
        send_ns = time.monotonic_ns()
        try:
            sock.sendto(make_packet(KIND_PROBE, seq, 64, seed ^ 0x51A7, send_ns), peer)
            with stats.lock:
                stats.probe_sent += 1
        except OSError:
            with stats.lock:
                stats.send_failures += 1
        seq += 1
        target += step_ns


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--role", choices=("biz", "target"), required=True)
    ap.add_argument("--bind", required=True)
    ap.add_argument("--peer")
    ap.add_argument("--start-ns", type=int, required=True)
    ap.add_argument("--duration", type=float, required=True)
    ap.add_argument("--drain", type=float, default=10.0)
    ap.add_argument("--rate-mbps", type=float, required=True)
    ap.add_argument("--probe-interval", type=float, default=1.0,
                    help="biz RTT probe interval in seconds; 0 disables probes")
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    bind = parse_addr(args.bind)
    peer = parse_addr(args.peer) if args.peer else None
    if args.role == "biz" and peer is None:
        ap.error("--peer is required for biz")

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 << 20)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 4 << 20)
    sock.bind(bind)
    rcvbuf = sock.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF)
    sndbuf = sock.getsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF)

    stats = Stats(args.start_ns, args.duration, args.drain)
    peer_holder = {"peer": peer if args.role == "biz" else None, "lock": threading.Lock()}
    stop_event = threading.Event()
    stop_ns = args.start_ns + int((args.duration + args.drain) * 1e9)
    expected_kind = KIND_S2C if args.role == "biz" else KIND_C2S

    recv_thread = threading.Thread(
        target=receiver_loop,
        args=(args.role, sock, expected_kind, stats, peer_holder, stop_ns, stop_event),
        daemon=True,
    )
    recv_thread.start()

    extra_threads = []
    if args.role == "biz":
        t = threading.Thread(target=register_loop, args=(sock, peer, args.start_ns, args.seed, stop_event), daemon=True)
        t.start()
        extra_threads.append(t)
        if args.probe_interval > 0:
            p = threading.Thread(target=probe_loop, args=(sock, peer, args.start_ns, args.duration, args.seed, stats, stop_event, args.probe_interval), daemon=True)
            p.start()
            extra_threads.append(p)
        sender_kind = KIND_C2S
        peer_getter = lambda: peer
    else:
        sender_kind = KIND_S2C

        def peer_getter():
            with peer_holder["lock"]:
                return peer_holder["peer"]

    run_sender(sock, peer_getter, sender_kind, args.rate_mbps, args.seed, stats, stop_event)
    wait_until(stop_ns)
    stop_event.set()
    recv_thread.join(timeout=1.0)
    for t in extra_threads:
        t.join(timeout=0.2)
    sock.close()

    result = {
        "schema": 1,
        "role": args.role,
        "bind": args.bind,
        "peer": args.peer,
        "start_ns": args.start_ns,
        "duration_s": args.duration,
        "drain_s": args.drain,
        "rate_mbps": args.rate_mbps,
        "probe_interval_s": args.probe_interval,
        "seed": args.seed,
        "socket_rcvbuf": rcvbuf,
        "socket_sndbuf": sndbuf,
        "stats": stats.snapshot(),
    }
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_REALPATH_UDP_DONE " + json.dumps(result["stats"], sort_keys=True))


if __name__ == "__main__":
    main()
