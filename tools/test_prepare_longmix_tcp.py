#!/usr/bin/env python3
"""Pure generation + shell invariants, execute only in Actions."""
import unittest
from longmix_profile import sample_matrix
from prepare_longmix_tcp import generate
class TcpScript(unittest.TestCase):
    def test_single_fixed_source_b_tcp_product_route(self):
        sample=next(x for x in sample_matrix() if x["workload"]=="B" and x["loss_percent"]==0)
        script=generate(sample)
        self.assertEqual(script.count('python3 "$GEN"'),2)
        self.assertIn('route-mode all',script)
        self.assertIn('--workload B --seed "$SEED"',script)
        self.assertIn('ip netns exec "$SRV"',script)
        self.assertIn('server-tun-active.txt',script)
        self.assertIn('tcp-target.json',script)
        self.assertIn('tcp-biz.json',script)
        self.assertIn('10.50.0.2:18100',script)
        self.assertIn('--duration 300 --drain 10',script)
        self.assertNotIn('--duration 120',script)
        self.assertNotIn('--rate-mbps "$RATE" --bounded-stats',script)
        self.assertIn('"tcp_long_connections":3',script)
        self.assertIn('"no_tcp_backpressure_quota_reallocation":True',script)
        self.assertEqual(script.count('tcpdump -c 40000'),4)
    def test_cannot_treat_c_or_a_as_b(self):
        for name in ('A','C'):
            x=next(z for z in sample_matrix() if z["workload"]==name)
            with self.assertRaises(ValueError):
                generate(x)
if __name__=="__main__":
    unittest.main(verbosity=2)
