#!/usr/bin/env python3
"""Strict Actions-only numeric analyzer boundary/false-PASS unit checks."""
import json
import tempfile
import unittest
from pathlib import Path
from check_longmix_udp_compact import netem_stats,validate

class CompactValidator(unittest.TestCase):
    def test_counter_denominator_semantics(self):
        p,d=netem_stats([{"kind":"netem","packets":70,"drops":30}])
        self.assertEqual((p,d),(70,30))
        for broken in ([],[{"kind":"fq","packets":100,"drops":0}],
                       [{"kind":"netem","packets":1,"drops":0}]*2):
            with self.assertRaises(ValueError):
                netem_stats(broken)

    def test_missing_route_mtu_must_fail_before_business(self):
        with tempfile.TemporaryDirectory() as td:
            root=Path(td)
            with self.assertRaises((ValueError,OSError)):
                validate(root,{"workload":"A","loss_percent":0,"seed":2608101,
                   "source_sha":"b4ea061178a6e09b7e7c8587d72b4b8535492567",
                   "duration_s":300,"drain_s":10})

    def test_wrong_sample_never_passes(self):
        with tempfile.TemporaryDirectory() as td:
            root=Path(td)
            (root/"manifest.json").write_text(json.dumps({"source_sha":"bad"}))
            with self.assertRaises(ValueError):
                validate(root,{"workload":"A","loss_percent":0,"seed":2608101,
                   "source_sha":"b4ea061178a6e09b7e7c8587d72b4b8535492567",
                   "duration_s":300,"drain_s":10})

    def test_manifest_cannot_claim_unknown_helper_sha(self):
        with tempfile.TemporaryDirectory() as td:
            root=Path(td)
            (root/"manifest.json").write_text(json.dumps({
                "source_sha":"b4ea061178a6e09b7e7c8587d72b4b8535492567",
                "helper_sha":"fake","workload":"A","loss_percent":0,
                "seed":2608101,"config":{"duration_s":300}
            }))
            with self.assertRaises(ValueError):
                validate(root,{"workload":"A","loss_percent":0,"seed":2608101,
                   "source_sha":"b4ea061178a6e09b7e7c8587d72b4b8535492567",
                   "duration_s":300,"drain_s":10})
if __name__=="__main__":
    unittest.main(verbosity=2)
