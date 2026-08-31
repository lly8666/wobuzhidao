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

    def test_v24_lane_and_tunnel_architecture_invariants_are_persisted(self):
        constitution = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "Each Transport Lane has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage",
            "Reality-like TLS is the first protected payload phase of that same FakeTCP association",
            "A temporary reliable ordered stream is permitted only during bounded TLS/bootstrap for each lane",
            "later independent authenticated datagrams must be able to complete while an earlier FakeTCP sequence range is missing",
            "A Logical Tunnel may intentionally own multiple independent lanes",
            "DTLS 1.3",
            "wolfSSL",
            "fixed systematic `20:20`",
            "40 Mbit/s aggregate inner payload",
            "OpenWrt final transparent capture through **TPROXY**",
            "Windows final client capture through a **TUN/Wintun-class L3 adapter**",
            "Reality TCP -> close -> new FakeTCP payload SYN",
        ):
            self.assertIn(phrase, constitution)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "one raw FakeTCP SYN lineage / 4-tuple / sequence space",
            "bounded reliable bootstrap on that same association",
            "same association, no second WBD payload SYN",
            "one lane has one public FakeTCP association from its SYN through Reality-like bootstrap and steady payload",
            "no separate ordinary kernel-TCP WBD payload connection exists",
            "real TLS 1.3 ClientHello/ServerHello/Finished on the same lane sequence space",
            "post-bootstrap earliest-complete datagram behavior",
            "40 Mbit/s aggregate-inner conservative release operating point",
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

        adr12 = (ROOT / "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md").read_text(encoding="utf-8")
        for phrase in (
            "Logical Tunnel",
            "Transport Lane",
            "server-assigned",
            "make-before-break",
        ):
            self.assertIn(phrase, adr12)

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
            "V2.4 LOGICAL-TUNNEL / MULTIPATH PIVOT ACTIVE",
            "V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association per lane",
            "V2-M8-old | per-LiveID raw-IP netns + veth + double NAT",
            "V2-M9B | shared Linux TUN + one host NAT + lease demux",
            "one raw FakeTCP SYN lineage",
            "NO second WBD payload SYN",
            "packet/datagram VPN payload without ordinary-TCP HOL",
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
