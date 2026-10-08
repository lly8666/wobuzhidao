#!/usr/bin/env python3
import json,tempfile,unittest
from pathlib import Path
from longmix_profile import sample_matrix
from check_longmix_mixed_compact import analyze

class CAnalyzer(unittest.TestCase):
    def test_reject_missing_raw_evidence(self):
        c=next(x for x in sample_matrix() if x["workload"]=="C")
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(OSError):analyze(Path(d),c)
    def test_reject_other_workload_as_c(self):
        a=next(x for x in sample_matrix() if x["workload"]=="A")
        with tempfile.TemporaryDirectory() as d:
            with self.assertRaises(ValueError):analyze(Path(d),a)
    def test_reject_wrong_product(self):
        c=next(x for x in sample_matrix() if x["workload"]=="C")
        with tempfile.TemporaryDirectory() as d:
            root=Path(d)
            (root/"manifest.json").write_text(json.dumps({"source_sha":"not_b4","workload":"C"}))
            with self.assertRaises(ValueError):analyze(root,c)
if __name__=="__main__":unittest.main(verbosity=2)
