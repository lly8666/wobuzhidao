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

    def test_per_lane_single_flow_multipath_authority_is_persisted(self):
        constitution = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "PER TRANSPORT LANE",
            "Logical Tunnel may own 1..4 independent complete WBD Transport Lanes",
            "Game Lane is a product multipath mechanism",
            "not research-only",
            "A -> A+B -> B",
            "pinned wolfSSL DTLS 1.3",
            "fixed systematic `20:20`",
            "40 Mbit/s aggregate-inner",
            "shared WBD TUN",
        ):
            self.assertIn(phrase, constitution)
        self.assertNotIn("A simultaneous second WBD public transport for the same Logical Tunnel is forbidden", constitution)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "each independent Transport Lane",
            "one raw FakeTCP SYN lineage / 4-tuple / sequence space",
            "no FIN/RST/reconnect/new WBD payload SYN inside that lane",
            "Game/race operates above independent complete WBD lanes",
            "first valid arrival is delivered once",
            "A ACTIVE",
            "one shared WBD TUN",
        ):
            self.assertIn(phrase, architecture)

        adr12 = (ROOT / "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md").read_text(encoding="utf-8")
        for phrase in (
            "CURRENT LIFECYCLE AND MULTIPATH AUTHORITY",
            "per-Transport-Lane invariant",
            "not a global one-flow-per-Logical-Tunnel invariant",
            "MaxProductPublicTransportLanes = 4",
            "Game / weak-network mode",
            "A -> A+B -> B",
        ):
            self.assertIn(phrase, adr12)

        adr14 = (ROOT / "docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md").read_text(encoding="utf-8")
        self.assertIn("WITHDRAWN / INVALIDATED", adr14)
        self.assertIn("incorrectly expanded", adr14)
        self.assertNotIn("Status: **ACCEPTED / PRODUCT-OWNER FINAL FREEZE", adr14)

        handoff = json.loads((ROOT / ".wbd/handoff/current.json").read_text(encoding="utf-8"))
        authority = handoff["architecture_override"]["authority"]
        guard = handoff["architecture_override"]["critical_guard"]
        replacement = handoff["architecture_override"]["replacement"]
        self.assertIn("ADR-0012", authority)
        self.assertIn("single-flow is PER TRANSPORT LANE", guard)
        self.assertIn("1..4 logical lanes", guard)
        self.assertIn("A -> A+B -> B", replacement)
        self.assertIn("never as a fifth logical lane", replacement)
        self.assertIn("source == server-issued Logical Tunnel lease", handoff["architecture_override"]["lease_source_boundary"])
        self.assertFalse(handoff["qualification_snapshot"]["release_authorized"])

        lock = json.loads((ROOT / "deps/security-lock.json").read_text(encoding="utf-8"))
        dtls = lock["dtls"]
        self.assertEqual(dtls["tag"], "v5.9.2-stable")
        self.assertEqual(dtls["commit"], "ac01707f552c611fbd135cc723b2682b3e7f80f2")
        self.assertEqual(dtls["status"], "V2_M2_LOCALLY_QUALIFIED_RAW_FEC_COMPOSITION")
        self.assertEqual(dtls["full_m2_status"], "qualified")
        self.assertEqual(dtls["initial_policy"]["zero_rtt"], "disabled")

        bench = (ROOT / "docs/benchmarks/v2-transport-20x20-matrix.md").read_text(encoding="utf-8")
        self.assertIn("later-datagram bypass", bench)
        self.assertIn("CPU ms per delivered MiB", bench)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
