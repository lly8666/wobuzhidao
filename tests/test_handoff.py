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

    def test_v23_single_flow_architecture_invariants_are_persisted(self):
        constitution = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "exactly one public TCP-shaped raw/FakeTCP association",
            "short real-TLS Reality-like bootstrap carried inside that same FakeTCP association",
            "temporary reliable ordered stream is permitted only during the bounded TLS/bootstrap phase",
            "later independent authenticated datagrams must be able to complete while an earlier FakeTCP sequence range is missing",
            "DTLS 1.3",
            "wolfSSL",
            "fixed systematic `20:20`",
            "40 Mbit/s aggregate inner payload",
            "OpenWrt final transparent capture through **TPROXY**",
            "Windows final client capture through a **TUN/Wintun-class L3 adapter**",
            "Reality TCP -> close -> new FakeTCP SYN",
        ):
            self.assertIn(phrase, constitution)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "one session = one public 4-tuple + one SYN lineage + one continuous FakeTCP sequence space",
            "temporary reliable ordered bootstrap stream",
            "SAME 4-tuple / SAME sequence space / NO new SYN",
            "The product must never route sustained VPN payload through an ordinary kernel TCP byte stream",
            "real TLS 1.3 records and configured SNI",
            "later independent DTLS/FEC datagrams may complete while an earlier FakeTCP sequence range is missing",
            "40 Mbit/s aggregate inner release operating point",
        ):
            self.assertIn(phrase, architecture)

        adr11 = (ROOT / "docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md").read_text(encoding="utf-8")
        for phrase in (
            "one public TCP-shaped 4-tuple and one continuous TCP sequence space",
            "No ordinary kernel TCP socket owns WBD product payload",
            "Temporary stream semantics are allowed only during bootstrap",
            "Reality-like recognition moves inside the raw association",
            "same 4-tuple and same sequence space, no FIN/RST/new SYN",
            "exactly one client SYN",
            "post-switch test",
        ):
            self.assertIn(phrase, adr11)

        adr10 = (ROOT / "docs/architecture/ADR-0010-v2-protocol-freeze-40m-release-cap.md").read_text(encoding="utf-8")
        self.assertIn("AMENDED BY ADR-0011", adr10)
        self.assertIn("40 Mbit/s aggregate inner offered payload", adr10)
        self.assertIn("No second public SYN is permitted", adr10)

        adr4 = (ROOT / "docs/architecture/ADR-0004-product-scope-persona.md").read_text(encoding="utf-8")
        self.assertIn("PARTIALLY SUPERSEDED BY ADR-0011", adr4)
        self.assertIn("separate Persona/preflight public connection is retired", adr4)

        adr8 = (ROOT / "docs/architecture/ADR-0008-reality-target-mirror-diagnostic.md").read_text(encoding="utf-8")
        self.assertIn("AMENDED BY ADR-0011", adr8)
        self.assertIn("one public TCP-shaped FakeTCP 4-tuple and one continuous sequence space", adr8)
        self.assertIn("unrecognized ClientHello bytes are forwarded byte-for-byte", adr8)
        self.assertIn("later DTLS/FEC datagram can bypass an earlier missing FakeTCP payload", adr8)

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

        roadmap = (ROOT / "ROADMAP.md").read_text(encoding="utf-8")
        for phrase in (
            "V2.3 SINGLE-FLOW CORRECTION ACTIVE",
            "V2-M8A-old | separate ordinary-TCP Reality-like front | **SUPERSEDED BY ADR-0011**",
            "V2-M8A-SF1 | temporary reliable FakeTCP bootstrap stream",
            "V2-M8A-SF2 | real TLS 1.3 / Reality-like auth over same FakeTCP association",
            "V2-M8A-SF3 | raw-listener fallback/decoy proxy + fingerprint qualification",
            "post-switch no-HOL hole-bypass test green",
            "one public WBD SYN lineage",
        ):
            self.assertIn(phrase, roadmap)

        bench = (ROOT / "docs/benchmarks/v2-transport-20x20-matrix.md").read_text(encoding="utf-8")
        self.assertIn("later-datagram bypass", bench)
        self.assertIn("CPU ms per delivered MiB", bench)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
