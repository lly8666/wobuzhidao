#!/usr/bin/env python3
"""Qualification false-positive regression tests; no traffic generation."""
import copy
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from aggregate_strict_weaknet_loss_tolerant_v1 import aggregate

SHA = "a" * 40


def campaign():
    rows = []
    for mode, rate, lanes in (("normal", 10, 1), ("game", 3, 4)):
        for scenario in ("lossless", "5205", "5305"):
            for seed in (101, 202, 303):
                rows.append({
                    "source_sha": SHA, "analysis_version": "loss-tolerant-v1",
                    "mode": mode, "scenario": scenario, "seed": seed,
                    "target_mbps_each_direction": rate, "lanes": lanes,
                    "classifications": {k: "PASS" for k in (
                        "CORRECTNESS", "INPUT_VALIDITY", "CAPTURE", "ENVIRONMENT", "PERFORMANCE")},
                    "latency": {"probe_stage": {stage: {
                        "sent": 30, "received": 30, "p95_ns": 600_000_000, "p99_ns": 610_000_000
                    } for stage in ("pre", "stress", "post")}},
                    "_run_receipt": {"id": len(rows) + 1, "head_sha": SHA,
                        "event": "workflow_dispatch", "run_attempt": 1,
                        "status": "completed", "conclusion": "success"},
                })
    return rows


class AggregateTest(unittest.TestCase):
    def check(self, rows):
        return aggregate(rows, source_sha=SHA, seeds=(101, 202, 303), require_run_receipts=True)

    def test_complete_campaign_passes(self):
        out = self.check(campaign())
        self.assertEqual(out["result"], "PASS")
        self.assertEqual(len(out["sample_results"]), 12)
        self.assertEqual(len(out["repeat_status"]), 6)

    def test_empty_and_incomplete_fail(self):
        for rows in ([], campaign()[:1], campaign()[:-1], campaign()[3:]):
            with self.subTest(count=len(rows)):
                self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_repeat_count_gates_default_cli_mode_too(self):
        self.assertEqual(aggregate(campaign()[:1])["result"], "FAIL")

    def test_failed_lossless_cannot_be_a_good_baseline(self):
        for key in campaign()[0]["classifications"]:
            rows = campaign()
            rows[0]["classifications"][key] = "FAIL"
            with self.subTest(key=key):
                self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_capacity_limited_and_failed_lossy_fail(self):
        rows = campaign()
        rows[3]["classifications"]["PERFORMANCE"] = "CAPACITY_LIMITED"
        self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_duplicate_identity_never_overwrites_bad_sample(self):
        rows = campaign()
        duplicate = copy.deepcopy(rows[3])
        duplicate["classifications"]["PERFORMANCE"] = "FAIL"
        rows.insert(0, duplicate)
        self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_other_source_and_config_rejected(self):
        for key, value in (("source_sha", "b" * 40), ("source_sha", "short"),
                           ("mode", "unknown"), ("scenario", "unknown"),
                           ("seed", 404), ("lanes", 2), ("target_mbps_each_direction", 5),
                           ("analysis_version", "other")):
            rows = campaign()
            rows[0][key] = value
            with self.subTest(key=key, value=value):
                self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_rtt_limits_are_inclusive_and_all_stages_gate(self):
        for stage in ("pre", "stress", "post"):
            rows = campaign()
            probe = rows[3]["latency"]["probe_stage"][stage]
            probe.update(p95_ns=800_000_000, p99_ns=1_110_000_000)
            self.assertEqual(self.check(rows)["result"], "PASS")
            probe["p95_ns"] += 1
            self.assertEqual(self.check(rows)["result"], "FAIL")
            probe["p95_ns"] -= 1
            probe["p99_ns"] += 1
            self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_missing_and_unusable_probe_evidence_fails(self):
        for value in (None, float("nan"), float("inf"), -1, "600000000"):
            rows = campaign()
            rows[0]["latency"]["probe_stage"]["stress"]["p95_ns"] = value
            with self.subTest(value=value):
                self.assertEqual(self.check(rows)["result"], "FAIL")
        rows = campaign()
        rows[3]["latency"]["probe_stage"]["pre"]["received"] = 0
        self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_receipts_must_prove_independent_first_attempts(self):
        for key, value in (("id", 2), ("run_attempt", 2), ("event", "push"),
                           ("head_sha", "b" * 40), ("status", "in_progress"),
                           ("conclusion", "failure")):
            rows = campaign()
            rows[0]["_run_receipt"][key] = value
            with self.subTest(key=key):
                self.assertEqual(self.check(rows)["result"], "FAIL")

    def test_malformed_file_outputs_failed_receipt(self):
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            (root / "summary-loss-tolerant-v1.json").write_text("{bad", encoding="utf-8")
            output = root / "aggregate.json"
            result = subprocess.run([sys.executable,
                str(Path(__file__).with_name("aggregate_strict_weaknet_loss_tolerant_v1.py")),
                "--root", temp, "--output", str(output)], capture_output=True, text=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertEqual(json.loads(output.read_text())["result"], "FAIL")


if __name__ == "__main__":
    unittest.main()
