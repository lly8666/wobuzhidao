#!/usr/bin/env python3
"""Genuine IPv4 UDP socket workload for one Actions A sample.

Runs on both sides in isolated netns, with independent active senders and
receivers. Payload lengths refer to the *UDP application datagram*, including
the 32-byte audit envelope; only numeric metadata leaves the runner.
"""
import argparse
import collections
import json
import math
import socket
import threading
import time
from pathlib import Path

from longmix_profile import (A_UDP_SHARE, PROBE_HZ, PROBE_PAYLOAD,
                             WeightedUDPSlots, bytes_per_second)
from realpath_udp_duplex import (
    KIND_C2S, KIND_S2C, KIND_PROBE, KIND_PROBE_REPLY, KIND_REGISTER,
    Stats, decode_packet, make_packet, parse_addr, percentile, wait_until,
)

ACK_KIND = 6
ACK_SIZE = 96
# Keep an independent reverse ACK allowance per direction; never backfill
# unused bandwidth into another workload. Main sender remains <= nominal 10M.
ACK_RESERVED_BYTES_PER_SECOND = 8192
MAX_ACK_EVENTS = 16000
MAX_SEND_LAG_NS = 10_000_000
RECV_BUFFER = 4 << 20
IP_MTU_DISCOVER = 10
IP_PMTUDISC_DONT = 0
DATA_PORT = 18080
PROBE_PORT = 18082


def whole_measured_mbps(value):
    """Normalize argparse float only after verifying exact whole-Mbps input."""
    if (type(value) not in (int, float) or not math.isfinite(value)
            or value <= 0 or value != int(value)):
        raise ValueError("rate must be a positive whole Mbps value")
    return int(value)


def new_socket(bind):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, RECV_BUFFER)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, RECV_BUFFER)
    # 8973 and 65507 must be legal IP-fragmentable UDP sends. No DF.
    sock.setsockopt(socket.IPPROTO_IP, IP_MTU_DISCOVER, IP_PMTUDISC_DONT)
    sock.bind(bind)
    return sock


class Audit:
    def __init__(self, start_ns, duration_s, drain_s):
        self.stats = Stats(start_ns, duration_s, drain_s)
        self.lock = threading.Lock()
        self.offered = collections.Counter()
        self.attempted = collections.Counter()
        self.successful = collections.Counter()
        self.send_errors = collections.Counter()
        self.received = collections.Counter()
        self.late = collections.Counter()
        self.received_latency_by_size = collections.defaultdict(list)
        self.big_sent = {}
        self.ack_count = collections.Counter()
        self.ack_rtt_ns = collections.defaultdict(list)
        self.ack_duplicate = 0
        self.ack_unknown = 0
        self.ack_emit_errors = 0
        self.ack_events_dropped = 0
        self.last_ack_seen = set()
        self.recv_header_errors = 0
        self.register_received = 0
        self.probe_request_received = 0
        self.probe_reply_errors = 0

    def offered_slot(self, size):
        with self.lock:
            self.offered[size] += 1

    def on_sent(self, size, seq, now_ns, lag_ns):
        self.stats.note_send(now_ns, size, lag_ns)
        with self.lock:
            self.successful[size] += 1
            if size in (8973, 65507):
                self.big_sent[seq] = (size, now_ns)

    def on_received(self, packet, now_ns, kind):
        # A single data receiver serializes duplicate/valid-first accounting.
        existing = packet["seq"] in self.stats.recv_seen
        self.stats.note_recv(packet, now_ns, kind)
        if existing or packet["kind"] != kind:
            return False
        with self.lock:
            self.received[packet["size"]] += 1
            self.received_latency_by_size[packet["size"]].append(
                max(0, now_ns - packet["send_ns"]))
        return True

    def on_ack(self, packet, now_ns):
        seq = packet["seq"]
        with self.lock:
            if seq in self.last_ack_seen:
                self.ack_duplicate += 1
                return
            self.last_ack_seen.add(seq)
            sent = self.big_sent.get(seq)
            if sent is None:
                self.ack_unknown += 1
                return
            size, send_ns = sent
            self.ack_count[size] += 1
            self.ack_rtt_ns[size].append(max(0, now_ns - send_ns))

    def snapshot(self):
        def fmt(cnt):
            return {str(k): int(v) for k, v in sorted(cnt.items())}
        def percentiles(values):
            return {key: percentile(values, p) for key, p in
                    (("p50_ns", .5), ("p95_ns", .95), ("p99_ns", .99))}
        st = self.stats.snapshot()
        with self.lock:
            per_size = []
            for size, pct in A_UDP_SHARE:
                good = self.received_latency_by_size.get(size, [])
                ack = self.ack_rtt_ns.get(size, [])
                per_size.append({
                    "payload_bytes": size, "expected_byte_percent": pct,
                    "offered": self.offered[size],
                    "send_success": self.successful[size],
                    "valid_first_receive": self.received[size],
                    "send_errors": self.send_errors[size],
                    "skip_late": self.late[size],
                    "oneway_returned": len(good),
                    "oneway_returned_percentiles": percentiles(good),
                    "oneway_returned_max_ns": max(good, default=None),
                    "big_ack_received": self.ack_count[size],
                    "big_ack_returned_percentiles": percentiles(ack),
                    "big_ack_max_ns": max(ack, default=None),
                    "big_ack_over_1s": sum(x > 1_000_000_000 for x in ack),
                    "big_ack_over_3s": sum(x > 3_000_000_000 for x in ack),
                })
            return {
                "stats": st, "per_size": per_size,
                "offered_count": fmt(self.offered), "sent_count": fmt(self.successful),
                "received_count": fmt(self.received), "late_slots": fmt(self.late),
                "send_errors": fmt(self.send_errors),
                "ack_duplicate": self.ack_duplicate, "ack_unknown": self.ack_unknown,
                "ack_emit_errors": self.ack_emit_errors,
                "ack_event_cap": MAX_ACK_EVENTS,
                "recv_header_errors": self.recv_header_errors,
                "register_received": self.register_received,
                "probe_request_received": self.probe_request_received,
                "probe_reply_errors": self.probe_reply_errors,
            }


def register_until_start(sock, peer, start_ns, seed, stop):
    seq = 0
    while time.monotonic_ns() < start_ns and not stop.is_set():
        try:
            sock.sendto(make_packet(KIND_REGISTER, seq, 64, seed,
                                    time.monotonic_ns()), peer)
        except OSError:
            pass
        seq += 1
        time.sleep(.2)


def active_data_sender(sock, get_peer, role, seed, rate, start_ns,
                       duration, audit, stop):
    if rate <= 0:
        return
    kind = KIND_C2S if role == "biz" else KIND_S2C
    main_per_sec = bytes_per_second(rate) - PROBE_PAYLOAD * PROBE_HZ - ACK_RESERVED_BYTES_PER_SECOND
    if main_per_sec <= 0:
        raise ValueError("nonpositive UDP main allocation")
    sizes = WeightedUDPSlots(A_UDP_SHARE)
    cumulative = 0
    seq = 0
    end_ns = start_ns + int(duration * 1e9)
    while not stop.is_set():
        size = sizes.next_size()
        target_ns = start_ns + int(cumulative * 1_000_000_000 / main_per_sec)
        if target_ns >= end_ns:
            break
        audit.offered_slot(size)
        cumulative += size
        now = time.monotonic_ns()
        if now < target_ns:
            wait_until(target_ns)
        now = time.monotonic_ns()
        if now - target_ns > MAX_SEND_LAG_NS:
            with audit.lock:
                audit.late[size] += 1
            audit.stats.note_skip(target_ns, size)
            seq += 1
            continue
        peer = get_peer()
        if peer is None:
            with audit.lock:
                audit.send_errors[size] += 1
            seq += 1
            continue
        payload = make_packet(kind, seq, size, seed, now)
        try:
            n = sock.sendto(payload, peer)
            if n != size:
                raise OSError("short UDP datagram send")
            audit.on_sent(size, seq, now, max(0, now - target_ns))
        except OSError:
            with audit.lock:
                audit.send_errors[size] += 1
        seq += 1


def active_data_receiver(sock, get_peer, set_peer, role, audit, stop_ns, stop):
    expected_kind = KIND_S2C if role == "biz" else KIND_C2S
    sock.settimeout(.1)
    while not stop.is_set() and time.monotonic_ns() < stop_ns:
        try:
            data, addr = sock.recvfrom(65535)
        except socket.timeout:
            continue
        except OSError:
            break
        now = time.monotonic_ns()
        p, err = decode_packet(data)
        if err:
            with audit.lock:
                audit.recv_header_errors += 1
            continue
        if p["kind"] == KIND_REGISTER and role == "target":
            set_peer(addr)
            with audit.lock:
                audit.register_received += 1
            continue
        if p["kind"] == ACK_KIND:
            audit.on_ack(p, now)
            continue
        if p["kind"] != expected_kind:
            with audit.lock:
                audit.stats.unexpected += 1
            continue
        accepted = audit.on_received(p, now, expected_kind)
        if accepted and p["size"] in (8973, 65507):
            # A 96-byte independent ACK to correlate the large datagram's RTT.
            # ACK loss is separate from the target's valid-first receive.
            try:
                sock.sendto(make_packet(ACK_KIND, p["seq"], ACK_SIZE, p["seed"],
                                        p["send_ns"]), addr)
            except OSError:
                with audit.lock:
                    audit.ack_emit_errors += 1


def probe_receiver(sock, role, audit, stop_ns, stop):
    sock.settimeout(.1)
    while not stop.is_set() and time.monotonic_ns() < stop_ns:
        try:
            data, addr = sock.recvfrom(512)
        except socket.timeout:
            continue
        except OSError:
            break
        now = time.monotonic_ns()
        p, err = decode_packet(data)
        if err:
            with audit.lock:
                audit.stats.corrupt += 1
            continue
        if role == "target" and p["kind"] == KIND_PROBE:
            with audit.lock:
                audit.probe_request_received += 1
            try:
                sock.sendto(make_packet(KIND_PROBE_REPLY, p["seq"], PROBE_PAYLOAD,
                                        p["seed"], p["send_ns"]), addr)
            except OSError:
                with audit.lock:
                    audit.probe_reply_errors += 1
        elif role == "biz" and p["kind"] == KIND_PROBE_REPLY:
            with audit.stats.lock:
                if p["seq"] in audit.stats.recv_seen:
                    audit.stats.recv_duplicates += 1
                    continue
                audit.stats.recv_seen.add(p["seq"])
                audit.stats.probe_recv += 1
                audit.stats.probe_rtt_ns.append(max(0, now - p["send_ns"]))
            audit.stats.note_probe_event(p, now)
        else:
            with audit.stats.lock:
                audit.stats.unexpected += 1


def probe_sender(sock, peer, start_ns, duration, seed, audit, stop):
    # Distinct receiving socket/thread from data traffic; probe budget is
    # deducted from BOTH active directions regardless of actual echo count.
    stop_ns = start_ns + int(duration * 1e9)
    seq = 0
    target_ns = start_ns
    step_ns = int(1e9 / PROBE_HZ)
    while target_ns < stop_ns and not stop.is_set():
        wait_until(target_ns)
        now_ns = time.monotonic_ns()
        with audit.stats.lock:
            audit.stats.probe_sent += 1
        try:
            sock.sendto(make_packet(KIND_PROBE, seq, PROBE_PAYLOAD,
                                    seed ^ 0x51A7, now_ns), peer)
        except OSError:
            with audit.stats.lock:
                audit.stats.send_failures += 1
        seq += 1
        target_ns += step_ns


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--role", choices=("biz", "target"), required=True)
    ap.add_argument("--bind", required=True)
    ap.add_argument("--peer")
    ap.add_argument("--start-ns", type=int, required=True)
    ap.add_argument("--duration", type=float, required=True)
    ap.add_argument("--drain", type=float, default=10)
    ap.add_argument("--rate-mbps", type=float, required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--bounded-stats", action="store_true")
    args = ap.parse_args()
    if args.duration != 300 or args.drain != 10 or args.rate_mbps != 10:
        ap.error("A qualification must be 300s+10s and 10Mbps each direction")
    if args.role == "biz" and not args.peer:
        ap.error("biz requires an explicit tunnel destination")
    bind = parse_addr(args.bind)
    if bind[1] not in (DATA_PORT, 28080):
        ap.error("unexpected data port")
    base_peer = parse_addr(args.peer) if args.peer else None
    data = new_socket(bind)
    probe_bind = (bind[0], PROBE_PORT if args.role == "target" else 28082)
    probe = new_socket(probe_bind)
    peer = [base_peer]
    peer_lock = threading.Lock()

    def get_peer():
        with peer_lock:
            return peer[0]

    def set_peer(address):
        with peer_lock:
            peer[0] = address

    start = args.start_ns
    stop_ns = start + int((args.duration + args.drain) * 1e9)
    stop = threading.Event()
    audit = Audit(start, args.duration, args.drain)
    probe_audit = Audit(start, args.duration, args.drain)
    thread_list = [
        threading.Thread(target=active_data_receiver,
                         args=(data, get_peer, set_peer, args.role, audit, stop_ns, stop)),
        threading.Thread(target=probe_receiver,
                         args=(probe, args.role, probe_audit, stop_ns, stop)),
    ]
    for t in thread_list:
        t.start()
    if args.role == "biz":
        thread_list.append(threading.Thread(target=register_until_start,
                           args=(data, base_peer, start, args.seed, stop)))
        thread_list[-1].start()
        probe_dest = (base_peer[0], PROBE_PORT)
        thread_list.append(threading.Thread(target=probe_sender,
                           args=(probe, probe_dest, start, args.duration, args.seed,
                                 probe_audit, stop)))
        thread_list[-1].start()
    cpu0 = time.process_time()
    active_data_sender(data, get_peer, args.role, args.seed,
                       whole_measured_mbps(args.rate_mbps),
                       start, args.duration, audit, stop)
    wait_until(stop_ns)
    stop.set()
    for t in thread_list:
        t.join(timeout=1)
    result = {
        "schema": "wbd-longmix-udp-a/v1",
        "role": args.role, "source_seed": args.seed,
        "start_monotonic_ns": start, "duration_s": args.duration,
        "drain_s": args.drain, "rate_target_mbps": args.rate_mbps,
        "outer_direction": "c2s" if args.role == "biz" else "s2c",
        "socket_effective": {
            "data_rcvbuf": data.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF),
            "data_sndbuf": data.getsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF),
            "probe_rcvbuf": probe.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF),
            "probe_sndbuf": probe.getsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF),
        },
        "generator_cpu_seconds": time.process_time() - cpu0,
        "data": audit.snapshot(),
        "probes": probe_audit.stats.snapshot(),
        "probe_request_received": probe_audit.probe_request_received,
        "probe_reply_errors": probe_audit.probe_reply_errors,
        "byte_quota": {
            "total": int(bytes_per_second(10)),
            "probe_reserved_per_second": PROBE_PAYLOAD * PROBE_HZ,
            "large_ack_reserved_per_second": ACK_RESERVED_BYTES_PER_SECOND,
            "main_sender_limit_per_second":
                int(bytes_per_second(10) - PROBE_PAYLOAD * PROBE_HZ -
                    ACK_RESERVED_BYTES_PER_SECOND),
            "unused_reserved_budget_not_reallocated": True,
        }
    }
    data.close()
    probe.close()
    Path(args.output).write_text(json.dumps(result, separators=(",", ":")) + "\n")
    print("WBD_LONGMIX_UDP_A_RECORDED", args.role,
          "sent", result["data"]["stats"]["sent_bytes"],
          "received", result["data"]["stats"]["recv_unique_bytes"])

if __name__ == "__main__":
    main()
