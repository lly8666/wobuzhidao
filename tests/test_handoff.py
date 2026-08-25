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
            "TCP-shaped does not mean kernel-TCP-owned",
            "DTLS 1.3",
            "wolfSSL",
            "weak-1.5x",
            "weak-2x",
            "OpenWrt/Linux ↔ Linux or Windows",
            "Android and unprivileged/no-root portability are out of scope",
            "optional TLS Persona bootstrap",
            "Persona must remain isolated from the unordered DTLS/FEC data plane",
            "Kernel TCP anchor / real-return-packet hybrid: **retired from the product roadmap",
        ):
            self.assertIn(phrase, text)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "V1 multi-ordinary-TCP is permanently rejected",
            "outer wire packets** should be TCP-shaped",
            "product payload is never committed to an ordinary kernel TCP byte stream",
            "FEC encoder",
            "DTLS application datagram",
            "Optional TLS Persona bootstrap",
            "kernel TCP anchor / real-return-packet experiment is **retired from the product roadmap**",
            "Classic udp2raw-compatible FakeTCP remains the product carrier baseline",
        ):
            self.assertIn(phrase, architecture)

        adr3 = (ROOT / "docs/architecture/ADR-0003-native-dtls.md").read_text(encoding="utf-8")
        self.assertIn("DTLS 1.3", adr3)
        self.assertIn("Do not invent a second AEAD/key schedule", adr3)
        self.assertIn("Relationship to TLS Persona", adr3)

        adr4 = (ROOT / "docs/architecture/ADR-0004-product-scope-persona.md").read_text(encoding="utf-8")
        self.assertIn("retire kernel TCP anchor from the product roadmap", adr4)
        self.assertIn("admit optional TLS Persona", adr4)
        self.assertIn("separate from the FakeTCP/DTLS data lane", adr4)

        lock = json.loads((ROOT / "deps/security-lock.json").read_text(encoding="utf-8"))
        dtls = lock["dtls"]
        self.assertEqual(dtls["tag"], "v5.9.2-stable")
        self.assertEqual(dtls["commit"], "ac01707f552c611fbd135cc723b2682b3e7f80f2")
        self.assertEqual(dtls["status"], "V2_M2_LOCALLY_QUALIFIED_RAW_FEC_COMPOSITION")
        self.assertEqual(dtls["full_m2_status"], "qualified")
        self.assertEqual(dtls["m2a_local_qualification"]["result"], "pass")
        self.assertEqual(dtls["m2b_local_qualification"]["result"], "pass")
        self.assertEqual(dtls["m2c_local_qualification"]["result"], "pass")
        self.assertEqual(dtls["initial_policy"]["zero_rtt"], "disabled")
        self.assertEqual(
            dtls["initial_policy"]["manual_openssl_compat_x509_verify_cert"],
            "forbidden_for_product_peer_auth",
        )

        roadmap = (ROOT / "ROADMAP.md").read_text(encoding="utf-8")
        self.assertIn("V2-M3 | minimal native session/control", roadmap)
        self.assertIn("V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED**", roadmap)
        self.assertIn("V2-M6 | Linux/OpenWrt native L3/TUN core | **CURRENT**", roadmap)
        self.assertIn("V2-M8A | optional TLS Persona bootstrap", roadmap)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
