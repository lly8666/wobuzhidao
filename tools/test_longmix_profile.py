#!/usr/bin/env python3
"""Actions-only deterministic planning checks; never a throughput result."""
import copy
import unittest
from longmix_profile import (
    A_UDP_SHARE, C_UDP_SHARE, DURATION_SECONDS, DRAIN_SECONDS,
    INNER_TUN_MTU, OUTER_IPV4_PACKET_BUDGET, PROBE_HZ, PROBE_PAYLOAD,
    TCP_WRITE_SIZES, WORKLOADS, WeightedUDPSlots, bytes_per_second,
    sample_matrix, udp_main_bytes_per_second, validate_sample,
)

class PlanContract(unittest.TestCase):
    def test_samples_are_twelve_independent_fixed_inputs(self):
        m = sample_matrix()
        self.assertEqual(len(m), 12)
        self.assertEqual(len({(x["workload"], x["loss_percent"]) for x in m}), 12)
        for row in m:
            w = validate_sample(row)
            self.assertEqual(w.seed, row["seed"])
            self.assertEqual(row["duration_s"], 300)
            self.assertEqual(row["drain_s"], 10)
            self.assertEqual((w.tcp_mbps + w.udp_mbps), 10)
        for w in WORKLOADS.values():
            self.assertEqual({r["seed"] for r in m if r["workload"] == w.name}, {w.seed})

    def test_invalid_conditions_rejected_not_silently_coerced(self):
        row = sample_matrix()[0]
        for key, bad in (("source_sha", "0" * 40), ("duration_s", 120),
                         ("drain_s", 0), ("seed", 17), ("workload", "D"),
                         ("loss_percent", 21), ("loss_percent", 0.0)):
            mutation = copy.deepcopy(row)
            mutation[key] = bad
            with self.subTest(key=key, bad=bad), self.assertRaises(ValueError):
                validate_sample(mutation)
        row["matrix"] = True
        with self.assertRaises(ValueError):
            validate_sample(row)

    def test_length_boundaries_are_distinct(self):
        self.assertEqual(INNER_TUN_MTU, 9000)
        self.assertEqual(OUTER_IPV4_PACKET_BUDGET, 1400)
        self.assertEqual(8972 + 8 + 20, 9000)
        self.assertEqual(8973 + 8 + 20, 9001)
        self.assertEqual(65507 + 8 + 20, 65535)
        self.assertGreater(65508 + 8 + 20, 65535)
        self.assertEqual(tuple(TCP_WRITE_SIZES), (96, 4096, 65536, 1048576))

    def test_exact_byte_budget_reserves_both_probe_directions(self):
        self.assertEqual(PROBE_HZ, 10)
        self.assertEqual(PROBE_PAYLOAD, 96)
        self.assertEqual(udp_main_bytes_per_second(WORKLOADS["A"]), 1240848)
        self.assertEqual(udp_main_bytes_per_second(WORKLOADS["C"]), 615848)
        self.assertEqual(udp_main_bytes_per_second(WORKLOADS["B"]), 0)
        self.assertEqual(bytes_per_second(10), 1250000)

    def test_weighted_scheduler_has_large_and_tiny_real_sample_counts(self):
        for name, shares in (("A", A_UDP_SHARE), ("C", C_UDP_SHARE)):
            scheduler = WeightedUDPSlots(shares)
            # Approximately one nominal second of main UDP data.
            budget = udp_main_bytes_per_second(WORKLOADS[name])
            while scheduler.total_bytes < budget:
                scheduler.next_size()
            summary = scheduler.summary()
            self.assertGreaterEqual(scheduler.total_bytes, budget)
            self.assertLess(scheduler.total_bytes, budget + 65508)
            for entry in summary["sizes"]:
                self.assertGreater(entry["packets"], 0, entry)
                # Bound terminal scheduling error by one max size (byte-fair).
                self.assertLess(abs(entry["bytes"] -
                    scheduler.total_bytes * entry["target_byte_percent"] / 100),
                    65508, entry)
            self.assertIn(65507, scheduler.counts)
            self.assertIn(8973, scheduler.counts)

if __name__ == "__main__":
    unittest.main(verbosity=2)
