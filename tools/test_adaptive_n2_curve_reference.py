"""No product package imports: independent mathematical and timeline checks."""
import itertools
import json
import math
import unittest
from pathlib import Path

from adaptive_n2_curve_reference import (
    MODEL_VERSION, ideal, mixed_residual, residual, time_band, generate)


class IndependentFECReferenceTest(unittest.TestCase):
    def test_mds_by_exhaustive_erasure_enumeration(self):
        for k in (1, 2, 3, 4):
            for r in (0, 4, 8, 20):
                effective = min(k, r)
                for p in (0., .01, .05, .2, .5, 1.):
                    exact = 0.
                    for lost in itertools.product((0, 1), repeat=k + effective):
                        if lost[0] and sum(lost) > effective:
                            n = sum(lost)
                            exact += p ** n * (1 - p) ** (len(lost) - n)
                    self.assertAlmostEqual(residual(k, r, p), exact, places=11)

    def test_boundary_off_and_unattainable_partial(self):
        for r in (0, 4, 20):
            self.assertEqual(residual(20, r, 0), 0)
            self.assertEqual(residual(20, r, 1), 1)
        self.assertAlmostEqual(residual(1, 4, .05), .0025)
        self.assertAlmostEqual(residual(1, 20, .05), .0025)
        k1 = [{"k": 1, "count": 100}]
        self.assertIsNone(ideal(k1, .05, 0)["ideal_parity"])
        self.assertEqual(ideal([{"k": 20, "count": 100}], 0, 0)["ideal_parity"], 0)

    def test_iid_full20_reference_ladder(self):
        full = [{"k": 20, "count": 30}]
        expected = {0: 0, .01: 4, .05: 8, .2: 16, .3: None}
        for p, grade in expected.items():
            self.assertEqual(ideal(full, p, 0)["ideal_parity"], grade)
        self.assertAlmostEqual(mixed_residual(full, 4, .01),
                               residual(20, 4, .01))

    def test_byte_vs_source_weighting_and_invalid(self):
        shape = [{"k": 1, "count": 10, "source_bytes": 100},
                 {"k": 20, "count": 10, "source_bytes": 100000}]
        self.assertNotAlmostEqual(mixed_residual(shape, 8, .05),
                                  mixed_residual(shape, 8, .05, True))
        with self.assertRaises(ValueError):
            residual(0, 4, .05)
        with self.assertRaises(ValueError):
            mixed_residual([{"k": 20, "count": 0}], 4, .05)

    def test_independent_time_budget_and_fixed_freeze(self):
        payload = json.loads(
            Path("docs/evidence/adaptive-n2-curve-freeze-v1.json").read_text())
        self.assertEqual(payload["model_version"], MODEL_VERSION)
        got = generate(payload)
        self.assertEqual(len(got["scenarios"]), len(payload["scenarios"]))
        timing = time_band(rtt_s=.6, from_r=20, to_r=4, **payload["timing"])
        self.assertGreaterEqual(timing["earliest_s"], 70)
        self.assertGreater(timing["expected_s"], timing["earliest_s"])
        self.assertGreater(timing["latest_screen_s"], timing["expected_s"])
        tampered = json.loads(json.dumps(payload))
        tampered["scenarios"][0]["stages"][0]["frozen_ideal_parity"] = 20
        with self.assertRaises(ValueError):
            generate(tampered)


if __name__ == "__main__":
    unittest.main()
