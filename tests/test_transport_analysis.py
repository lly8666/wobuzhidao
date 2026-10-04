import importlib.util
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SPEC = importlib.util.spec_from_file_location(
    "transport_analysis", ROOT / "scripts" / "analyze_v2_transport_20x20.py"
)
MOD = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MOD)


class TransportAnalysisTest(unittest.TestCase):
    def row(self, loss, delivery=1.0, hs=3, tcp=3, udp=3, rst=0, cpu=100.0, rss=1000.0):
        return {
            "rtt_ms": 50,
            "loss_pct_per_direction": float(loss),
            "cases": 3,
            "handshake_success_cases": hs,
            "tcp_like_stable_cases": tcp,
            "udp_like_stable_cases": udp,
            "outer_rst_total": rst,
            "gap_bypass_cases": 2 if loss else 0,
            "delivery_ratio_median": delivery,
            "p99_ms_median": 70.0,
            "cpu_ms_per_delivered_mib_median": cpu,
            "rss_peak_kb_total_median": rss,
        }

    def test_delivery_and_cpu_boundary_suggest_midpoint(self):
        rows = [
            self.row(0, cpu=100),
            self.row(10, cpu=110),
            self.row(20, delivery=0.90, udp=0, cpu=170),
            self.row(30, delivery=0.80, udp=0, cpu=200),
        ]
        result = MOD.analyze(rows)
        b = result["boundaries"][0]
        self.assertEqual(b["delivery_99pct"]["last_better_loss"], 10.0)
        self.assertEqual(b["delivery_99pct"]["first_bad_loss"], 20.0)
        self.assertEqual(b["cpu_1_5x"]["first_bad_loss"], 20.0)
        picks = {(x["rtt_ms"], x["loss_pct_per_direction"]): x for x in result["targeted_followups"]}
        self.assertIn((50, 15.0), picks)
        self.assertIn("delivery_99pct", picks[(50, 15.0)]["reason"])
        self.assertIn("cpu_1_5x", picks[(50, 15.0)]["reason"])

    def test_tcp_rst_is_independent_boundary(self):
        rows = [self.row(0), self.row(1), self.row(5, tcp=2, rst=1)]
        result = MOD.analyze(rows)
        b = result["boundaries"][0]
        self.assertEqual(b["tcp_like"]["first_bad_loss"], 5.0)
        point = next(x for x in result["points"] if x["loss_pct_per_direction"] == 5.0)
        self.assertIn("tcp_like_degraded", point["labels"])

    def test_resource_ratios_use_same_rtt_loss_zero_baseline(self):
        rows = [self.row(0, cpu=80, rss=800), self.row(1, cpu=120, rss=1000)]
        result = MOD.analyze(rows)
        point = next(x for x in result["points"] if x["loss_pct_per_direction"] == 1.0)
        self.assertAlmostEqual(point["cpu_ratio_vs_loss0_same_rtt"], 1.5)
        self.assertAlmostEqual(point["rss_ratio_vs_loss0_same_rtt"], 1.25)
        self.assertIn("cpu_inflated_1_5x", point["labels"])
        self.assertIn("rss_inflated_1_25x", point["labels"])


if __name__ == "__main__":
    unittest.main()
