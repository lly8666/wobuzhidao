#!/usr/bin/env python3
import importlib.util
from pathlib import Path
import unittest
ROOT=Path(__file__).resolve().parent.parent
spec=importlib.util.spec_from_file_location("bh",ROOT/"tools/check_strict_blackhole.py")
bh=importlib.util.module_from_spec(spec); spec.loader.exec_module(bh)
class BlackholeRecoveryTest(unittest.TestCase):
    def make_stats(self):
        buckets=[0]*14000; per=3750
        for i in range(6045,len(buckets)): buckets[i]=per
        return {"recv_wall_bucket_ms":10,"recv_wall_bytes_by_bucket":buckets}
    def test_recovery_window_within_horizon(self):
        start=1_000_000_000; restored=start+60_100_000_000
        row=bh.recovery_from_wall(self.make_stats(),start,restored,300,3.0)
        self.assertTrue(row["valid"]); self.assertIsNotNone(row["qualifying_window"])
        self.assertLessEqual(row["qualifying_window"]["delay_from_expected_ms"],3000)
    def test_no_window(self):
        stats={"recv_wall_bucket_ms":10,"recv_wall_bytes_by_bucket":[0]*14000}
        start=1_000_000_000
        row=bh.recovery_from_wall(stats,start,start+60_500_000_000,300,3.0)
        self.assertTrue(row["valid"]); self.assertIsNone(row["qualifying_window"])
if __name__=="__main__": unittest.main()
