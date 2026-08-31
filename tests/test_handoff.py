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

    def test_v3_architecture_invariants_are_persisted(self):
        constitution = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "exactly one public TCP-shaped raw/FakeTCP flow per WBD session",
            "V3 one-public-flow law",
            "same 4-tuple, no FIN/RST/close_notify/new SYN",
            "raw FakeTCP mux is the sole public owner",
            "Reality-like parsing/authentication is an in-flow phase",
            "No-HOL steady-state law",
            "destroy ordered bootstrap assemblers",
            "DTLS 1.3",
            "wolfSSL",
            "fixed systematic `20:20`",
            "Do not delay an available systematic source merely to fill a FEC block",
            "100 Mbit/s physical link capacity",
            "OpenWrt/Linux ↔ Linux or Windows",
            "Windows final client capture through a **TUN/Wintun-class L3 adapter**",
        ):
            self.assertIn(phrase, constitution)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "one public raw FakeTCP flow",
            "one raw SYN",
            "same public 4-tuple",
            "Reality-like TLS 1.3 bootstrap",
            "encrypted switch",
            "DTLS 1.3",
            "no-HOL",
            "sole public",
        ):
            self.assertIn(phrase, architecture)

        adr11 = (ROOT / "docs/architecture/ADR-0011-v3-single-public-flow-realitylike-switch.md").read_text(encoding="utf-8")
        for phrase in (
            "V3 PRODUCT AUTHORITY",
            "one public raw FakeTCP association",
            "There is no second public SYN",
            "raw FakeTCP mux is the sole WBD owner",
            "encrypted SWITCH_REQ / SWITCH_ACK",
            "No-HOL switch law",
            "Npcap captures an adapter rather than a socket",
            "Physical Windows 11 + Npcap",
        ):
            self.assertIn(phrase, adr11)

        # Historical ADRs remain available as evidence but must clearly tell a
        # future recovery session that their separate-public-flow decisions are
        # no longer product authority.
        adr4 = (ROOT / "docs/architecture/ADR-0004-product-scope-persona.md").read_text(encoding="utf-8")
        self.assertIn("SUPERSEDED IN PART BY ADR-0011 FOR V3", adr4)
        self.assertIn("Kernel TCP anchor remains retired", adr4)

        adr8 = (ROOT / "docs/architecture/ADR-0008-reality-target-mirror-diagnostic.md").read_text(encoding="utf-8")
        self.assertIn("SUPERSEDED IN PART BY ADR-0011 FOR V3", adr8)
        self.assertIn("Do not implement or package this sequence for V3", adr8)

        adr3 = (ROOT / "docs/architecture/ADR-0003-native-dtls.md").read_text(encoding="utf-8")
        self.assertIn("DTLS 1.3", adr3)
        self.assertIn("Do not invent a second AEAD/key schedule", adr3)

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
        for phrase in (
            "V3 SINGLE-PUBLIC-FLOW ACTIVE",
            "one raw FakeTCP SYN / SYN-ACK / ACK",
            "same public 4-tuple and FakeTCP sequence space",
            "destroy ordered bootstrap state",
            "V3-M4",
            "Windows Npcap WBD-flow demux",
            "Linux V3 path",
            "V3 one-public-owner firewall/manager composition",
            "Physical Windows 11 + Npcap",
            "ADR-0011 is authoritative",
        ):
            self.assertIn(phrase, roadmap)

        # Retain transport evidence locks that remain valid under V3. The
        # connection-establishment diagrams in historical benchmark prose are
        # not architecture authority.
        bench = (ROOT / "docs/benchmarks/v2-transport-20x20-matrix.md").read_text(encoding="utf-8")
        self.assertIn("Nominal matrix size: **126 cases**", bench)
        self.assertIn("later-datagram bypass", bench)
        self.assertIn("CPU ms per delivered MiB", bench)

        mux_bench = (ROOT / "docs/benchmarks/v2-mux-two-session-100m.md").read_text(encoding="utf-8")
        self.assertIn("32918572671", mux_bench)
        self.assertIn("measurement loss: random 20%", mux_bench)
        self.assertIn("0.8022 -> **1.0000**", mux_bench)
        self.assertIn("0.8062 -> **1.0000**", mux_bench)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
