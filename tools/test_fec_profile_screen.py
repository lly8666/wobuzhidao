import copy
import unittest

from check_strict_fec_profile_screen import (longest_zero_delivery,
                                            source_loss_reference, validate_profile)


class ProfileScreenTests(unittest.TestCase):
    def test_source_reference_not_block_failure_probability(self):
        self.assertAlmostEqual(source_loss_reference(20, 20, .3), 0.0013010796404352904, places=10)
        self.assertAlmostEqual(source_loss_reference(20, 4, .2), 0.14069371780302395, places=10)

    def test_partial_uses_actual_k_and_min_k_r(self):
        self.assertAlmostEqual(source_loss_reference(1, 20, .3), .09)
        self.assertAlmostEqual(source_loss_reference(20, 0, .3), .3)
        self.assertEqual(source_loss_reference(3, 4, .2), source_loss_reference(3, 20, .2))

    def test_invalid_geometry_rejected(self):
        for args in ((0, 4, .2), (21, 4, .2), (20, 6, .2), (20, 4, 1.1)):
            with self.assertRaises(ValueError):
                source_loss_reference(*args)

    def make_receipts(self, parity):
        path = {"FECEnabled": parity != 0, "ParityShards": parity}
        lane = {"parity_shards": parity, "lane": {"TxPath": path, "RxPath": path}}
        product = {"lanes": [lane]}
        manifest = {"source_sha": "a" * 40, "config": {
            "fec": "off" if parity == 0 else "20:" + str(parity),
            "fec_parity": parity, "profile_screen": True}}
        diagnostics = {"client": [{"product": product}],
                       "server": [{"product": {"present": True, "tunnel": product}}]}
        return manifest, diagnostics

    def test_every_profile_verified_on_both_paths(self):
        for parity in (0, 4, 8, 10, 12, 16, 20):
            manifest, diag = self.make_receipts(parity)
            errors, counts = validate_profile(manifest, diag, parity, "a" * 40)
            self.assertEqual(errors, [])
            self.assertEqual(counts, {"client": 1, "server": 1})

    def test_claimed_profile_does_not_hide_actual_mismatch(self):
        manifest, diag = self.make_receipts(4)
        diag = copy.deepcopy(diag)
        diag["server"][0]["product"]["tunnel"]["lanes"][0]["parity_shards"] = 20
        self.assertTrue(validate_profile(manifest, diag, 4, "a" * 40)[0])
        self.assertTrue(validate_profile(manifest, diag, 4, "b" * 40)[0])
        self.assertTrue(validate_profile(manifest, {"client": [], "server": []}, 4, "a" * 40)[0])

    def test_not_a_formal20_receipt(self):
        manifest, diag = self.make_receipts(20)
        manifest["config"]["profile_screen"] = False
        self.assertTrue(validate_profile(manifest, diag, 20, "a" * 40)[0])

    def test_continuity_does_not_ignore_middle_stall(self):
        wall = [1] * 130
        wall[50:53] = [0] * 3
        self.assertEqual(longest_zero_delivery({"recv_wall_bytes_by_second": wall}), 3)
        self.assertIsNone(longest_zero_delivery({"recv_wall_bytes_by_second": [1] * 50}))


if __name__ == "__main__":
    unittest.main()
