import json
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]


class HandoffContractTest(unittest.TestCase):
    def test_verifier_passes(self):
        cp = subprocess.run(
            [sys.executable, str(ROOT / "scripts/verify_handoff.py")],
            cwd=ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        self.assertEqual(cp.returncode, 0, cp.stdout)
        self.assertIn("HANDOFF_VERIFY_PASS", cp.stdout)

    def test_architecture_invariants_are_persisted(self):
        text = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "udp2raw-compatible FakeTCP",
            "weak-1.5x",
            "weak-2x",
            "Android and unprivileged mobile compatibility are explicitly out of scope",
            "It must not become the public outer ordinary-TCP carrier",
            "Local sandbox/host execution remains qualification authority",
        ):
            self.assertIn(phrase, text)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        self.assertIn("V1 multi-ordinary-TCP is permanently rejected", architecture)
        self.assertIn("kernel TCP anchor", architecture)
        self.assertIn("payload is not placed in an ordinary kernel TCP byte stream", architecture)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
