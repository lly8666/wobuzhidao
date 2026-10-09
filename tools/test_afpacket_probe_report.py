#!/usr/bin/env python3
"""Non-network synthetic tests of fail-closed numerical 100ms trace reducer."""
import sys
import unittest
from pathlib import Path
sys.path.insert(0, str(Path(__file__).resolve().parent))
from afpacket_probe_report import summarize


def row(t, d, side="client", good=True, r=0):
    return {"schema": 1, "side": side,
            "ss_start_monotonic_ns": t,
            "ss_end_monotonic_ns": t+2_000_000,
            "ss_ok": good,
            "packet_socket": {"r": r, "rb": 1048576, "d": d} if good else None,
            "cpu_psi_some_avg10_percent": 6.12}


class NumericProbeReportTests(unittest.TestCase):
    def test_drop_is_conservatively_bracketed_not_precisely_timed(self):
        x = summarize([row(1_000_000_000, 0), row(1_100_000_000, 86, r=384)],
                      "client", 1_000_000_000, 5_000_000_000)
        self.assertEqual(x["observed_positive_drop_delta"], 86)
        self.assertEqual(x["drop_intervals"][0]["earliest_possible_s"], 0)
        self.assertEqual(x["drop_intervals"][0]["latest_possible_s"], .102)
        self.assertEqual(x["sampled_rmem_peak_bytes"], 384)
        self.assertEqual(x["quality"], "PARTIAL_NUMERIC_ONLY")

    def test_old_all_null_artifact_not_false_zero_drop(self):
        x = summarize([row(1_000_000_000 + i*100_000_000, 0, good=False)
                       for i in range(3150)], "client", 12_000_000_000, 312_000_000_000)
        self.assertEqual(x["valid_samples"], 0)
        self.assertEqual(x["invalid_samples"], 3150)
        self.assertEqual(x["quality"], "UNUSABLE")
        self.assertIsNone(x["observed_positive_drop_delta"])

    def test_valid_full_duration_is_sample_scoped_only(self):
        rows = [row(1_000_000_000 + i*100_000_000, 0) for i in range(3030)]
        x = summarize(rows, "client", 1_000_000_000, 303_000_000_000)
        self.assertEqual(x["quality"], "CONTINUOUS_NUMERIC_ONLY")
        self.assertEqual(x["observed_positive_drop_delta"], 0)
        self.assertEqual(x["original_business_gate"], "NOT_EVALUATED")

    def test_missing_window_degrades_and_brackets(self):
        x = summarize([row(1_000_000_000,0),
                       row(1_100_000_000,0,good=False),
                       row(1_400_000_000,3)], "client", 1_000_000_000, 1_300_000_000)
        self.assertEqual(x["quality"], "PARTIAL_NUMERIC_ONLY")
        self.assertTrue(x["drop_intervals"][0]["missed_sample_bracket"])
        self.assertEqual(x["invalid_samples"], 1)

    def test_counter_reset_fails_closed(self):
        with self.assertRaisesRegex(ValueError,"counter reset"):
            summarize([row(1_000_000_000,4),row(1_100_000_000,1)],
                      "client",1_000_000_000,2_000_000_000)

    def test_wrong_order_and_cap_fail(self):
        with self.assertRaisesRegex(ValueError,"duplicate"):
            summarize([row(1_000_000_000,0),row(1_000_000_000,0)],
                      "client",1_000_000_000,2_000_000_000)
        with self.assertRaisesRegex(ValueError,"row cap"):
            summarize([row(i*1_000_000+1,0) for i in range(4097)],
                      "client",1_000_000_000,2_000_000_000)

if __name__ == "__main__":
    unittest.main()
