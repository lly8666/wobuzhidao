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
            "20:10",
            "20:20",
            "OpenWrt/Linux ↔ Linux or Windows",
            "Android and unprivileged/no-root portability are out of scope",
            "optional TLS Persona bootstrap",
            "Persona must remain isolated from the unordered DTLS/FEC data plane",
            "Kernel TCP anchor / real-return-packet hybrid: retired",
            "Phase B — fixed FEC / immutable setup / periodic refresh — CURRENT",
            "optional fixed FEC",
            "same account username",
            "only-cn",
            "Do not delay an available systematic source merely to fill a FEC block",
            "periodic fixed-profile refresh",
            "There is no runtime FEC config epoch",
            "changing parameters means a fresh association",
            "configured-capacity rate guard",
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
            "Transport-only characterization — current priority",
            "FEC fixed at `20:20`",
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

        adr5 = (ROOT / "docs/architecture/ADR-0005-adaptive-fec-multisession-routing.md").read_text(encoding="utf-8")
        for phrase in (
            "fec.mode = off | fixed",
            "multiple simultaneous sessions/devices",
            "capture.mode = off | global | only-cn | only-non-cn",
            "client selects `persona = off | native | chrome | firefox | safari | edge`",
            "dual lane is an optional survival mode",
            "alpha > p/(1-p)",
            "Auto FEC remains future advanced research",
            "speed-test sites as **measurement baselines**",
        ):
            self.assertIn(phrase, adr5)

        adr6 = (ROOT / "docs/architecture/ADR-0006-immutable-link-setup.md").read_text(encoding="utf-8")
        for phrase in (
            "Immutable per-association link setup",
            "LINK_INIT(client proposal)",
            "LINK_ACCEPT(server exact acceptance)",
            "does **not** silently clamp or rewrite",
            "There is no config epoch",
            "reconnect required",
            "fixed systematic `20:20` tail-RS",
        ):
            self.assertIn(phrase, adr6)

        adr7 = (ROOT / "docs/architecture/ADR-0007-periodic-fixed-fec-refresh.md").read_text(encoding="utf-8")
        for phrase in (
            "Low-frequency fixed-FEC refresh",
            "LossMarked",
            "Wilson 95%",
            "window = 20 s",
            "periodic fixed-profile refresh",
            "association rotation",
            "B_inner_max",
            "public control service",
            "CertificateVerify",
        ):
            self.assertIn(phrase, adr7)

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
        self.assertIn("V2-M3A-E | minimal native session/control + bearer auth + legacy fixed config foundation | **DONE AS FOUNDATION**", roadmap)
        self.assertIn("V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED**", roadmap)
        self.assertIn("V2-M6A | Linux/OpenWrt packet-preserving L3/TUN core | **IMPLEMENTED**", roadmap)
        self.assertIn("V2-M8A | optional TLS Persona bootstrap + network-treatment diagnostic | **PLANNED AFTER CURRENT TRANSPORT SETUP**", roadmap)
        self.assertIn("V2-M8B-T2 | fixed FEC presets + immutable setup + periodic low-load refresh | **CURRENT**", roadmap)
        self.assertIn("V2-M8C | account + per-device credential + concurrent multi-session server state | **PLANNED**", roadmap)
        self.assertIn("V2-X1 | advanced continuously learning Auto FEC / automatic capacity inference | **FUTURE RESEARCH; NOT REQUIRED**", roadmap)
        self.assertIn("link-defining parameters never change in place", roadmap)

        bench = (ROOT / "docs/benchmarks/v2-transport-20x20-matrix.md").read_text(encoding="utf-8")
        self.assertIn("Nominal matrix size: **126 cases**", bench)
        self.assertIn("later-datagram bypass", bench)
        self.assertIn("CPU ms per delivered MiB", bench)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
