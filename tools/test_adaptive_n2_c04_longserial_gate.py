import copy
import unittest

from adaptive_n2_c04_longserial_gate import score, VERSION


def fixture():
    source = "test-source-sha"
    def stats():
        return {"sent_packets_by_second": [100] * 840,
                "recv_wall_packets_by_second": [100] * 851,
                "sent_packets": 84000, "recv_unique_packets": 84000,
                "send_failures": 0, "corrupt": 0, "recv_duplicates": 0,
                "oneway_p99_ns": 603000000}
    summary = {
        "source_sha": source, "classification": "SCOPED_WAVEFORM",
        "errors": [], "correctness_errors": [], "capacity_flags": [],
        "c04_loss20_grade_screen_pass": True,
        "c04_final_recovery_grade_screen_pass": True,
        "c04_stage_last60_grade": {"recover0": {"c2s": {"status": "OBSERVED", "actual_r": [0]}, "s2c": {"status": "OBSERVED", "actual_r": [0]}}},
        "waveform": [{"name": "recover0", "start_s": 600, "end_s": 840,
                      "c2s": {"drop_counter_delta": 0, "packets_counter_delta": 1000},
                      "s2c": {"drop_counter_delta": 0, "packets_counter_delta": 1000}}],
    }
    manifest = {"source_sha": source,
                "config": {"v3_client_fec_mode": "auto",
                           "v3_client_fec_min_parity": 0,
                           "v3_client_fec_initial_parity": 0,
                           "duration_s": 840,
                           "application_mbps_each_direction": 10.0,
                           "one_way_delay_ms_each_direction": 300,
                           "mtu": 1400}}
    biz, target = {"stats": stats()}, {"stats": stats()}
    return summary, manifest, biz, target, source


class C04LongSerialGateTests(unittest.TestCase):
    def test_healthy_final_clean_can_pass_with_weak_stage_loss(self):
        summary, manifest, biz, target, sha = fixture()
        target["stats"]["recv_wall_packets_by_second"][360:480] = [70] * 120
        result = score(summary, manifest, biz, target, sha)
        self.assertEqual(result["classification"], "SCOPED_C04_LONG_LIVED_FIRST_DELIVERY_AND_GRADE_PASS")
        self.assertEqual(result["gate_version"], VERSION)
        self.assertTrue(result["not_full_n2_pass"])

    def test_original_forty_one_forty_six_second_collapse_fails(self):
        s, m, b, t, sha = fixture()
        t["stats"]["recv_wall_packets_by_second"][799:840] = [0] * 41
        b["stats"]["recv_wall_packets_by_second"][794:840] = [0] * 46
        result = score(s, m, b, t, sha)
        self.assertIn("c2s_FINAL_CLEAN_CONTINUOUS_DELIVERY_OUTAGE", result["errors"])
        self.assertIn("s2c_FINAL_CLEAN_CONTINUOUS_DELIVERY_OUTAGE", result["errors"])
        self.assertEqual(result["business"]["c2s"]["longest_wall_zero_first_delivery"]["length_seconds"], 41)
        self.assertEqual(result["business"]["s2c"]["longest_wall_zero_first_delivery"]["length_seconds"], 46)

    def test_wrong_source_and_frozen_grade_not_forgiven(self):
        s, m, b, t, sha = fixture()
        s["c04_loss20_grade_screen_pass"] = False
        result = score(s, m, b, t, sha + "-changed")
        self.assertIn("SOURCE_MISMATCH", result["errors"])
        self.assertIn("HIGH_20PCT_PROTECTION_GRADE_SCREEN_NOT_OBSERVED", result["errors"])

    def test_netem_final_zero_must_be_measured(self):
        s, m, b, t, sha = fixture()
        s["waveform"][0]["c2s"]["drop_counter_delta"] = 4
        result = score(s, m, b, t, sha)
        self.assertIn("FINAL_CLEAN_NETEM_NOT_ZERO_c2s", result["errors"])

    def test_unsupported_capacity_is_not_a_product_pass(self):
        s, m, b, t, sha = fixture()
        s["capacity_flags"] = ["source underinjected"]
        result = score(s, m, b, t, sha)
        self.assertIn("CAPACITY_OR_DELIVERY_GAP_FLAGS_REQUIRE_REVIEW", result["errors"])


if __name__ == "__main__":
    unittest.main()
