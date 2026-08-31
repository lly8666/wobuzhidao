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

    def test_adr0014_single_flow_architecture_invariants_are_persisted(self):
        constitution = (ROOT / "PROJECT_CONSTITUTION.md").read_text(encoding="utf-8")
        for phrase in (
            "One connected Logical Tunnel has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage.",
            "Reality-like TLS is the first protected payload phase of that same FakeTCP association.",
            "A temporary reliable ordered adapter is permitted only during bounded TLS/bootstrap.",
            "later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing",
            "A simultaneous second WBD public transport for the same Logical Tunnel is forbidden.",
            "DTLS 1.3",
            "wolfSSL",
            "fixed systematic `20:20`",
            "40 Mbit/s aggregate-inner release operating point",
            "Reality TCP -> close -> new FakeTCP payload SYN",
        ):
            self.assertIn(phrase, constitution)

        architecture = (ROOT / "ARCHITECTURE.md").read_text(encoding="utf-8")
        for phrase in (
            "one raw FakeTCP SYN lineage / 4-tuple / sequence space",
            "bounded reliable ordered bootstrap on that same association",
            "explicit barrier; no FIN/RST/new WBD payload SYN",
            "One connected Logical Tunnel has one public FakeTCP association from SYN through Reality-like bootstrap and steady payload.",
            "During product operation no separate ordinary kernel-TCP WBD Reality/payload connection exists.",
            "Bootstrap carries real TLS 1.3 ClientHello/ServerHello/Finished on the same FakeTCP sequence space.",
            "post-bootstrap earliest-complete datagram behavior",
            "40 Mbit/s aggregate inner payload",
        ):
            self.assertIn(phrase, architecture)

        adr14 = (ROOT / "docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md").read_text(encoding="utf-8")
        for phrase in (
            "exactly one WBD TCP-shaped connection lineage at a time",
            "one FakeTCP SYN / SYN-ACK / ACK lineage",
            "no preliminary ordinary kernel-TCP Reality connection",
            "real TLS 1.3 Reality-like ClientHello / ServerHello / Finished",
            "explicit bootstrap barrier, with no FIN/RST/new WBD payload SYN",
            "a later independently complete datagram may progress while an earlier FakeTCP sequence range is missing",
            "The mature FakeTCP recovery/FEC core is frozen",
            "ADR-0014 controls",
        ):
            self.assertIn(phrase, adr14)

        adr12 = (ROOT / "docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md").read_text(encoding="utf-8")
        self.assertIn("PARTIALLY SUPERSEDED BY ADR-0014", adr12)
        self.assertIn("1..4 active public Transport Lanes", adr12)
        self.assertIn("The second is no longer product policy", adr12)
        self.assertIn("stable InstallationID / Logical Tunnel identity", adr12)

        roadmap = (ROOT / "ROADMAP.md").read_text(encoding="utf-8")
        for phrase in (
            "V2.6 GLOBAL SINGLE-FLOW / ADR-0014 ACTIVE",
            "exactly **one** public WBD TCP-shaped lineage",
            "no preliminary ordinary kernel-TCP Reality product connection",
            "A connected tunnel may not own a simultaneous second public WBD transport",
            "The mature TCP-like/FakeTCP recovery/FEC core is frozen",
            "40 Mbit/s aggregate-inner conservative release operating point",
        ):
            self.assertIn(phrase, roadmap)

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

        bench = (ROOT / "docs/benchmarks/v2-transport-20x20-matrix.md").read_text(encoding="utf-8")
        self.assertIn("later-datagram bypass", bench)
        self.assertIn("CPU ms per delivered MiB", bench)

    def test_no_required_binary_state_at_bootstrap(self):
        dp = json.loads((ROOT / ".wbd/handoff/data-plane.json").read_text())
        self.assertEqual(dp["assets"], [])


if __name__ == "__main__":
    unittest.main()
