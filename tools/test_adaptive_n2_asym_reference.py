import copy,json,unittest
from pathlib import Path
from adaptive_n2_asym_reference import evaluate,VERSION

class FrozenAsymmetricIIDReference(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.original=json.loads(Path(
            "docs/evidence/adaptive-n2-c05-c06-asym-model-freeze-v1.json").read_text())

    def test_directional_1_vs_10_percent_not_crossed(self):
        result=evaluate(copy.deepcopy(self.original))
        self.assertEqual(result["model_version"],VERSION)
        self.assertFalse(result["product_controller_imported"])
        self.assertEqual(result["measurement_status"],"MODEL_ONLY_NO_REAL_NETEM_ACTION")
        cases=result["results"]
        self.assertEqual(cases["C05"]["directions"]["c2s"]["full_k20_ideal_parity"],4)
        self.assertEqual(cases["C05"]["directions"]["s2c"]["full_k20_ideal_parity"],8)
        self.assertEqual(cases["C06"]["directions"]["c2s"]["full_k20_ideal_parity"],8)
        self.assertEqual(cases["C06"]["directions"]["s2c"]["full_k20_ideal_parity"],4)
        for case in cases.values():
            for lane in case["directions"].values():
                self.assertFalse(lane["target_unattainable"])
                self.assertLess(lane["independent_B_timeband"]["latest_screen_s"]+60,240)

    def test_band_cannot_be_postfit(self):
        manifest=copy.deepcopy(self.original)
        manifest["separate_performance_scenarios"][0]["frozen_allowed_last60"]["s2c"]=[8,20]
        with self.assertRaises(ValueError):
            evaluate(manifest)

    def test_loss_directions_cannot_swap_in_one_action(self):
        manifest=copy.deepcopy(self.original)
        manifest["separate_performance_scenarios"][0]["loss_pct_by_direction"]["c2s"]=10
        with self.assertRaises(ValueError):
            evaluate(manifest)

    def test_actual_source_window_16_and_off32_are_independent(self):
        manifest=copy.deepcopy(self.original)
        manifest["model_settings"]["history_windows_enabled"]=32
        with self.assertRaises(ValueError):
            evaluate(manifest)

    def test_config_and_version_fail_closed(self):
        manifest=copy.deepcopy(self.original)
        manifest["scenario_common"]["active_s"]=100
        with self.assertRaises(ValueError):
            evaluate(manifest)
        manifest=copy.deepcopy(self.original)
        manifest["model_version"]="invented"
        with self.assertRaises(ValueError):
            evaluate(manifest)

if __name__=="__main__":
    unittest.main()
