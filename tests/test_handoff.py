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
            "DTLS 1.3",
            "wolfSSL",
            "weak-1.5x",
            "weak-2x",
            "Android and unprivileged mobile compatibility are explicitly out of scope",
            "Xray, VLESS, Vision, REALITY and WireGuard are not part of the V2 product stack",
            "Local sandbox/host execution remains qualification authority",
        ):
            self.assertIn(phrase, text)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        self.assertIn("V1 multi-ordinary-TCP is permanently rejected", architecture)
        self.assertIn("ADR-0003", architecture)
        self.assertIn("FEC encoder **before each DTLS application-record encryption**", architecture)
        self.assertIn("one independent DTLS association per raw lane", architecture)
        self.assertIn("Xray is removed", architecture)
        self.assertIn("Kernel-anchor / real-return-packet experiment", architecture)

        adr = (ROOT / "docs/architecture/ADR-0003-native-dtls.md").read_text(encoding="utf-8")
        self.assertIn("There is no post-handshake transition to a custom cipher", adr)
        self.assertIn("two independent DTLS associations", adr)
        self.assertIn("Do not tune record length, timing, handshake extensions", adr)

        lock = json.loads((ROOT / "deps/security-lock.json").read_text(encoding="utf-8"))
        self.assertEqual(lock["dtls"]["tag"], "v5.9.2-stable")
        self.assertEqual(lock["dtls"]["commit"], "ac01707f552c611fbd135cc723b2682b3e7f80f2")
        self.assertEqual(lock["dtls"]["status"], "architecture_pinned_not_locally_qualified")

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
