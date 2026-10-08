#!/usr/bin/env python3
"""Preflight static protection for adapting preexisting dispatch-enabled strict job."""
import re
import unittest
from pathlib import Path
WF=".github/workflows/next-strict-weaknet.yml"
CTRL=".github/workflows/next-longmix-one-sample-dispatch.yml"

class StrictLongmixGate(unittest.TestCase):
    def test_existing_single_run_workflow_dispatch_reuse(self):
        t=Path(WF).read_text()
        self.assertEqual(t.count("tools/perf_sample_guard.py claim"),1)
        self.assertIn("options: [formal20, fec-profile-screen, longmix-a, longmix-b, longmix-c]",t)
        self.assertIn("inputs.qualification_kind != 'longmix-a' && inputs.qualification_kind != 'longmix-b' && inputs.qualification_kind != 'longmix-c'",t)
        self.assertIn("if: inputs.qualification_kind == 'longmix-a'",t)
        self.assertIn("inputs.qualification_kind != 'longmix-a' && inputs.qualification_kind != 'longmix-b' && inputs.qualification_kind != 'longmix-c'",t)
        self.assertIn("WBD_LONGMIX_LOSS",t)
        self.assertIn('env WBD_STRICT_ARTIFACT_DIR="$ART" GITHUB_SHA="$TESTED_SOURCE_SHA"',t)
        self.assertIn('TESTED_SOURCE_SHA',t)
        self.assertIn('WBD_HARNESS_SHA',t)
        self.assertIn("tools/check_longmix_udp_compact.py",t)
        self.assertIn("tools/prepare_longmix_udp.py",t)
        self.assertIn("tools/prepare_longmix_tcp.py",t)
        self.assertIn("tools/check_longmix_tcp_compact.py",t)
        self.assertIn('test "$LONGMIX_B_SAMPLE_OUTCOME" = success',t)
        self.assertIn('test "$LONGMIX_B_ANALYZER_OUTCOME" = success',t)
        self.assertIn("longmix-b",t)
        self.assertIn("longmix-c",t)
        self.assertIn("tools/prepare_longmix_mixed.py",t)
        self.assertIn("tools/check_longmix_mixed_compact.py",t)
        self.assertIn('test "$LONGMIX_C_SAMPLE_OUTCOME" = success',t)
        self.assertIn('test "$LONGMIX_C_ANALYZER_OUTCOME" = success',t)
        self.assertIn('test "$LONGMIX_SAMPLE_OUTCOME" = success',t)
        self.assertIn('test "$LONGMIX_ANALYZER_OUTCOME" = success',t)
        self.assertIn('test "$SAMPLE_OUTCOME" = success',t)
        self.assertNotIn("matrix:",t)
        self.assertNotIn("\n  push:",t)
    def test_push_controller_only_dispatches_existing_default_workflow(self):
        t=Path(CTRL).read_text()
        self.assertIn("gh workflow run next-strict-weaknet.yml",t)
        self.assertIn('qualification_kind="$WBD_KIND"',t)
        self.assertEqual(t.count("gh workflow run "),1)
        self.assertNotIn("tools/perf_sample_guard.py claim",t)
        self.assertIn("validate_sample(x)",t)
if __name__=="__main__":
    unittest.main(verbosity=2)
