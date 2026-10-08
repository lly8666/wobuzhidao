#!/usr/bin/env python3
"""Generation safety tests, run only in Actions."""
import unittest
from longmix_profile import sample_matrix
from prepare_longmix_udp import generate

class Prepare(unittest.TestCase):
    def test_single_fixed_300s_true_product_path(self):
        a = sample_matrix()[0]
        s = generate(a)
        self.assertIn('route-mode all', s)
        self.assertIn('ip netns exec "$SRV"', s)
        self.assertIn('server-tun-active.txt', s)
        self.assertIn('client-tproxy-ingress-mtu-active.txt', s)
        self.assertIn('mtu=9000', s)
        self.assertIn('--duration 300 --drain 10', s)
        self.assertNotIn('--duration 120', s)
        self.assertIn('loss-percent "$WBD_LONGMIX_LOSS"', s)
        self.assertNotIn('--pre-loss "$PRE_LOSS"', s)
        self.assertEqual(s.count('tcpdump -c 40000'), 4)
        self.assertEqual(s.count('python3 "$GEN"'), 2)
        self.assertNotIn(' --route-mode bypass-lan-cn', s)
        self.assertNotIn('ip -n "$TGT" addr add 8.8.8.8', s)
        self.assertIn('WBD_HARNESS_SHA', s)
        self.assertIn('GITHUB_SHA', s)
    def test_non_udp_driver_rejected(self):
        with self.assertRaises(ValueError):
            generate([x for x in sample_matrix() if x["workload"] == "B"][0])

if __name__ == "__main__":
    unittest.main(verbosity=2)
