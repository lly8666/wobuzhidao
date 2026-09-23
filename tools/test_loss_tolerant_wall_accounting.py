#!/usr/bin/env python3
import importlib.util
import math
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parent.parent

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod

an = load("loss_tolerant", "tools/check_strict_weaknet_loss_tolerant_v1.py")
udp = load("udp_duplex", "tools/realpath_udp_duplex.py")

class WallAccountingTest(unittest.TestCase):
    def test_stats_records_10ms_wall_bucket(self):
        start = 1_000_000_000
        s = udp.Stats(start, 120, 10)
        packet = {"kind": 7, "seq": 1, "size": 100, "send_ns": start}
        s.note_recv(packet, start + 305_000_000, 7)
        snap = s.snapshot()
        self.assertEqual(snap["recv_wall_bucket_ms"], 10)
        self.assertEqual(snap["recv_wall_bytes_by_bucket"][30], 100)
        self.assertEqual(sum(snap["recv_wall_bytes_by_bucket"]), 100)

    def test_delay_aligned_window_keeps_threshold_unchanged(self):
        seconds = 120
        sender = {"stats": {
            "sent_bytes_by_second": [1_250_000] * seconds,
            "sent_packets_by_second": [1000] * seconds,
        }}
        receiver = {"stats": {
            "recv_bytes_by_second": [1_250_000] * seconds,
            "recv_packets_by_second": [1000] * seconds,
            # Legacy unshifted 30s pre wall window is intentionally just below 9.9 Mbps.
            "recv_wall_bytes_by_second": [1_237_375] * 30 + [1_250_000] * 101,
            "recv_wall_bucket_ms": 10,
            "recv_wall_bytes_by_bucket": [0] * 13001,
        }}
        # Healthy 10 Mbps arrivals occupy exactly the 30s window shifted by fixed 300ms path delay.
        for i in range(30, 3030):
            receiver["stats"]["recv_wall_bytes_by_bucket"][i] = 12_500
        row = an.generator_stage(sender, receiver, "pre", 10.0, 300)
        self.assertLess(row["wall_delivered_mbps"], 9.9)
        self.assertTrue(math.isclose(row["wall_delivered_mbps_path_delay_aligned"], 10.0, rel_tol=0, abs_tol=1e-12))
        self.assertGreaterEqual(row["wall_delivered_mbps_path_delay_aligned"], 10.0 * 0.99)

if __name__ == "__main__":
    unittest.main()
