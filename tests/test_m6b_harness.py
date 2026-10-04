import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "qualify_v2_m6b_tun.py"


class M6BHarnessContractTest(unittest.TestCase):
    def test_help_is_safe_and_parses(self):
        cp = subprocess.run(
            [sys.executable, str(SCRIPT), "--help"],
            cwd=ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        self.assertEqual(cp.returncode, 0, cp.stdout)
        self.assertIn("--wbd-tun", cp.stdout)
        self.assertIn("--dtls-shim", cp.stdout)
        self.assertIn("--fec", cp.stdout)
        self.assertIn("--mtu", cp.stdout)

    def test_harness_pins_qualified_transport_assets(self):
        text = SCRIPT.read_text(encoding="utf-8")
        self.assertIn("c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c", text)
        self.assertIn("f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a", text)
        self.assertIn("63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a", text)
        self.assertIn("/dev/net/tun is missing", text)
        self.assertIn("tcp_inside_vpn", text)


if __name__ == "__main__":
    unittest.main()
