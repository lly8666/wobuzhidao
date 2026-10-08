#!/usr/bin/env python3
import json
import tempfile
import unittest
from pathlib import Path
from check_longmix_tcp_compact import verify, percent
from longmix_profile import sample_matrix

class BAnalyzerContract(unittest.TestCase):
    def test_conditional_rtt(self):
        self.assertEqual(percent([40,10,20],.5),20)
        self.assertEqual(percent([40,10,20],.99),40)
        self.assertIsNone(percent([],.99))
    def test_no_evidence_cannot_pass(self):
        req=next(x for x in sample_matrix() if x["workload"]=="B")
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            with self.assertRaises(OSError):
                verify(root,req)
            (root/"manifest.json").write_text(json.dumps({"source_sha":"wrong"}))
            with self.assertRaises(ValueError):
                verify(root,req)
    def test_c_does_not_pass_B(self):
        req=next(x for x in sample_matrix() if x["workload"]=="C")
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(ValueError):
                verify(Path(d),req)
if __name__=="__main__":
    unittest.main(verbosity=2)
