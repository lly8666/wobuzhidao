#!/usr/bin/env python3
"""Actions-only synthetic contract tests: no product sample, no privileges."""
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
from packet_socket_drop_forensics import analyze, packet_socket


def sample(ns, client=0, server=0, psi="6.12", missing_client=False):
    def socket(label, count):
        return {"returncode": 0, "stdout":
                "Netid State Recv-Q Send-Q Local Address:Port Peer Address:PortProcess\n"
                + "p_raw UNCONN 0 0 [2048]:" + label + " *\n"
                + " skmem:(r0,rb1048576,t0,tb212992,f0,w0,o0,bl0,d%d)\n" % count}
    entry = {"monotonic_ns": ns,
             "namespaces": {"client": {"ss_packet": {} if missing_client else socket("cwan", client)},
                            "server": {"ss_packet": socket("swan", server)}},
             "pressure": {"cpu": "some avg10=%s avg60=3.0 total=100\n" % psi},
             "processes": {"client": {"stat": {"utime_ticks": 10, "stime_ticks": 20}},
                           "server": {"stat": {"utime_ticks": 12, "stime_ticks": 30}}}}
    return entry


def stages():
    return [{"event": "business_start", "monotonic_ns": 20_000_000_000},
            {"event": "business_end", "monotonic_ns": 120_000_000_000}]


class SocketClockTests(unittest.TestCase):
    def test_business_not_resource_anchor_and_independent_sides(self):
        rows = [sample(5_000_000_000), sample(30_000_000_000),
                sample(31_000_000_000, server=86),
                sample(86_000_000_000, server=86),
                sample(87_000_000_000, client=33, server=86)]
        report = analyze(rows, stages())
        self.assertEqual(report["resource_first_to_business_start_s"], 15)
        self.assertEqual(report["sides"]["server"]["drop_intervals"][0]["business_elapsed_interval_s"], [10., 11.])
        self.assertEqual(report["sides"]["client"]["drop_intervals"][0]["business_elapsed_interval_s"], [66., 67.])
        self.assertEqual(report["sides"]["server"]["all_adjacent_drop_increments"], 86)
        self.assertEqual(report["sides"]["client"]["all_adjacent_drop_increments"], 33)
        self.assertEqual(report["sides"]["client"]["drop_intervals"][0]["rmem_before_after_bytes"], [0, 0])
        self.assertTrue(report["original_classifier_unchanged"])

    def test_zero_drop_not_empty_sample(self):
        report = analyze([sample(2_000_000_000), sample(31_000_000_000)], stages())
        self.assertEqual(report["sides"]["client"]["all_adjacent_drop_increments"], 0)
        self.assertTrue(report["sides"]["client"]["time_axis_complete"])

    def test_missing_packet_socket_does_not_claim_complete(self):
        report = analyze([sample(1_000_000_000, missing_client=True),
                          sample(31_000_000_000, client=90)], stages())
        self.assertFalse(report["sides"]["client"]["time_axis_complete"])
        self.assertEqual(report["sides"]["client"]["all_adjacent_drop_increments"], 0)
        self.assertEqual(report["sides"]["client"]["first_observed_drops"], 90)

    def test_multi_packet_socket_fails_closed(self):
        row = sample(1)
        row["namespaces"]["client"]["ss_packet"]["stdout"] += (
            "p_raw UNCONN 0 0 [2048]:other *\n skmem:(r0,rb1048576,t0,tb212992,f0,w0,o0,bl0,d1)\n")
        with self.assertRaisesRegex(ValueError, "multiple packet sockets"):
            packet_socket(row, "client")

    def test_counter_reset_fails_closed(self):
        with self.assertRaisesRegex(ValueError, "counter reset"):
            analyze([sample(1_000_000_000, client=5), sample(2_000_000_000, client=0)], stages())

    def test_duplicate_monotonic_or_missing_start_fails(self):
        with self.assertRaisesRegex(ValueError, "duplicate resource"):
            analyze([sample(1), sample(1)], stages())
        with self.assertRaisesRegex(ValueError, "business_start"):
            analyze([sample(1)], [{"event": "business_end", "monotonic_ns": 10}])


if __name__ == "__main__":
    unittest.main()
