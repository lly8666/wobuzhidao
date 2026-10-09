#!/usr/bin/env python3
import importlib.util
import pathlib
import sys
import unittest

HERE=pathlib.Path(__file__).resolve().parent
sys.path.insert(0,str(HERE))
import check_e1_lowrtt_fullstack as full
import prepare_e1_lowrtt_fullstack as gen

class LowRttSpec(unittest.TestCase):
    def test_strict_generator_rejects_drift(self):
        with self.assertRaisesRegex(ValueError,"strict upstream drift"):
            gen.derive("fake 300ms strict runner")
    def test_new_generator_never_modifies_product_or_sneaks_cpu_claim(self):
        src=pathlib.Path("scripts/strict_weaknet_sample.sh").read_text()
        derived=gen.derive(src)
        self.assertEqual(derived.count("delay 15ms"),2)
        self.assertNotIn("delay 300ms",derived)
        self.assertIn('sparse-interval-ms 45',derived)
        self.assertEqual(derived.count("--sparse-interval-ms 45"),2)
        self.assertIn("tools/e1_lowrtt_sparse_stage.py",derived)
        self.assertIn("96, 1372, 4068",derived)
        self.assertNotIn('--bounded-stats',derived)
        self.assertIn('"fec_partial_window_ms": 32',derived)
    def test_scoped_analyzer_is_zero_loss_and_size_strict(self):
        self.assertEqual(full.SIZES,(96,1372,4068))
        self.assertLessEqual(full.SMALL_P99_NS,180_000_000)
        self.assertIn("NOT_TESTED",full.review(pathlib.Path("/nonexistent-wbd-e1-proof"),full.SOURCE)["fec_repair_under_erasure"])
        self.assertEqual(full.review(pathlib.Path("/nonexistent-wbd-e1-proof"),full.SOURCE)["status"],"FAIL")
    def test_reject_wrong_product_source(self):
        self.assertEqual(len(full.SOURCE),40)
        self.assertNotEqual(full.SOURCE,"ba8ed1e656d32fa2d59cd0d5907fbc1ad10b3072")
if __name__=="__main__":unittest.main()
