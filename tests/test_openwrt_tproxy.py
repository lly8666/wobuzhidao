import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "openwrt_tproxy.sh"


class OpenWrtTPROXYContractTest(unittest.TestCase):
    def render(self, mode="global"):
        cp = subprocess.run(
            [
                "sh",
                str(SCRIPT),
                "render",
                "--mode",
                mode,
                "--port",
                "12345",
                "--underlay4",
                "203.0.113.7",
                "--underlay6",
                "2001:db8::7",
            ],
            cwd=ROOT,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            check=False,
        )
        self.assertEqual(cp.returncode, 0, cp.stdout)
        return cp.stdout

    def test_shell_syntax(self):
        cp = subprocess.run(["sh", "-n", str(SCRIPT)], cwd=ROOT, check=False)
        self.assertEqual(cp.returncode, 0)

    def test_underlay_escape_precedes_tproxy(self):
        out = self.render("global")
        self.assertIn("WBD_OPENWRT_TPROXY_PLAN", out)
        self.assertIn("ip daddr 203.0.113.7 return", out)
        self.assertIn("ip6 daddr 2001:db8::7 return", out)
        self.assertIn("tproxy to :12345", out)
        self.assertLess(out.index("ip daddr 203.0.113.7 return"), out.index("tproxy to :12345"))
        self.assertLess(out.index("ip6 daddr 2001:db8::7 return"), out.index("tproxy to :12345"))
        self.assertIn("ip rule add priority 1066 fwmark 0x66 lookup 1066", out)
        self.assertIn("ip route add local 0.0.0.0/0 dev lo table 1066", out)

    def test_split_modes_use_compact_sets(self):
        cn = self.render("only-cn")
        non = self.render("only-non-cn")
        self.assertIn("ip daddr @cn4", cn)
        self.assertIn("ip6 daddr @cn6", cn)
        self.assertIn("ip daddr != @cn4", non)
        self.assertIn("ip6 daddr != @cn6", non)
        self.assertLess(cn.count("tproxy to"), 4)
        self.assertLess(non.count("tproxy to"), 4)

    def test_cleanup_owns_only_wbd_table_and_policy(self):
        out = self.render("global")
        self.assertIn("nft delete table inet wbd", out)
        self.assertIn("ip rule del priority 1066 fwmark 0x66 lookup 1066", out)
        self.assertIn("ip route flush table 1066", out)


if __name__ == "__main__":
    unittest.main()
