#!/usr/bin/env python3
"""Frozen workload contract for the long UDP/TCP Actions qualification.

Pure planning and input rejection only. This is not a measurement driver.
The per-run runtime and tunnel provenance MUST be qualified separately.
"""
from dataclasses import dataclass
from fractions import Fraction
import heapq
import json

PRODUCT_SOURCE = "b4ea061178a6e09b7e7c8587d72b4b8535492567"
DURATION_SECONDS = 300
DRAIN_SECONDS = 10
ONE_WAY_DELAY_MS = 300
OUTER_IPV4_PACKET_BUDGET = 1400
INNER_TUN_MTU = 9000
FEC_DATA = 20
FEC_PARITY = 20
PROBE_PAYLOAD = 96
PROBE_HZ = 10
LARGE_ACK_RESERVE_BPS = 8192
C_LARGE_ACK_RESERVE_BPS = 4096
LOSS_PERCENTS = (0, 5, 20, 30)
A_UDP_SHARE = ((96, 5), (256, 5), (512, 5), (1000, 5),
               (1372, 5), (4068, 10), (8972, 15), (8973, 20), (65507, 30))
C_UDP_SHARE = ((96, 10), (512, 10), (1372, 10),
               (8972, 15), (8973, 25), (65507, 30))
TCP_WRITE_SIZES = (96, 4096, 65536, 1048576)
TCP_LONG_CONNECTIONS = 3
TCP_SHORT_CONNECTIONS = 1

@dataclass(frozen=True)
class Workload:
    name: str
    udp_mbps: int
    tcp_mbps: int
    udp_sizes: tuple
    seed: int

WORKLOADS = {
    "A": Workload("A", 10, 0, A_UDP_SHARE, 2608101),
    "B": Workload("B", 0, 10, (), 2608102),
    "C": Workload("C", 5, 5, C_UDP_SHARE, 2608103),
}

def bytes_per_second(mbps):
    if not isinstance(mbps, int) or mbps <= 0:
        raise ValueError("invalid Mbps")
    return Fraction(mbps * 1_000_000, 8)

def udp_main_bytes_per_second(workload):
    if not workload.udp_mbps:
        return Fraction(0)
    # A separate, always-on 96-byte probe stream and its reverse replies
    # consume identical explicit reservations in BOTH directional UDP quotas.
    return (bytes_per_second(workload.udp_mbps) - PROBE_PAYLOAD * PROBE_HZ
            - (C_LARGE_ACK_RESERVE_BPS if workload.name == 'C' else LARGE_ACK_RESERVE_BPS))

class WeightedUDPSlots:
    """Deterministic virtual byte-fair queue, independent of loss/replies.

    The returned size is a UDP *payload*, never an IP packet or TCP write.
    The caller paces against total scheduled payload bytes, not packets/s.
    """
    def __init__(self, shares):
        if not shares or sum(p for _, p in shares) != 100:
            raise ValueError("UDP byte shares must total 100")
        if len(set(n for n, _ in shares)) != len(shares):
            raise ValueError("duplicate UDP size")
        self.shares = tuple(shares)
        self.heap = []
        self.counts = {n: 0 for n, _ in shares}
        self.bytes = {n: 0 for n, _ in shares}
        self.total_bytes = 0
        for i, (size, share) in enumerate(shares):
            if size < 32 or size > 65507 or share <= 0:
                raise ValueError("illegal UDP payload size or share")
            # Half-slot offset distributes first transmissions fairly.
            heapq.heappush(self.heap, (Fraction(size, 2 * share), i))
    def next_size(self):
        score, i = heapq.heappop(self.heap)
        size, share = self.shares[i]
        heapq.heappush(self.heap, (score + Fraction(size, share), i))
        self.counts[size] += 1
        self.bytes[size] += size
        self.total_bytes += size
        return size
    def summary(self):
        return {
            "sizes": [{"payload_bytes": n, "target_byte_percent": p,
                       "packets": self.counts[n], "bytes": self.bytes[n],
                       "actual_byte_percent": (100 * self.bytes[n] / self.total_bytes
                                               if self.total_bytes else 0)}
                      for n, p in self.shares],
            "total_payload_bytes": self.total_bytes,
            "total_payload_packets": sum(self.counts.values()),
            "probe_reservation_payload_bytes_per_second": PROBE_PAYLOAD * PROBE_HZ,
        }

def validate_sample(sample):
    if not isinstance(sample, dict):
        raise ValueError("sample must be an object")
    expected = {"workload", "loss_percent", "seed", "source_sha",
                "duration_s", "drain_s"}
    if set(sample) != expected:
        raise ValueError("sample keys differ from immutable specification")
    workload = WORKLOADS.get(sample["workload"])
    if workload is None:
        raise ValueError("unknown workload")
    if type(sample["loss_percent"]) is not int or sample["loss_percent"] not in LOSS_PERCENTS:
        raise ValueError("unsupported fixed loss")
    if type(sample["seed"]) is not int or sample["seed"] != workload.seed:
        raise ValueError("paired workload seed mismatch")
    if sample["source_sha"] != PRODUCT_SOURCE:
        raise ValueError("wrong exact source")
    if sample["duration_s"] != DURATION_SECONDS or sample["drain_s"] != DRAIN_SECONDS:
        raise ValueError("duration/drain mismatch")
    if workload.udp_mbps + workload.tcp_mbps != 10:
        raise ValueError("per-direction combined quota is not 10Mbps")
    return workload

def sample_matrix():
    return [
        {"workload": w.name, "loss_percent": loss, "seed": w.seed,
         "source_sha": PRODUCT_SOURCE, "duration_s": DURATION_SECONDS,
         "drain_s": DRAIN_SECONDS}
        for w in WORKLOADS.values() for loss in LOSS_PERCENTS
    ]

if __name__ == "__main__":
    print(json.dumps({"schema": "wbd-longmix-plan/v1",
                      "source_sha": PRODUCT_SOURCE, "samples": sample_matrix()},
                     indent=2))
