#!/usr/bin/env python3
"""Actions-only mixed workload script and budget invariants."""
import unittest
from longmix_profile import WORKLOADS,sample_matrix,C_UDP_SHARE
from longmix_mixed_business import specs
from prepare_longmix_mixed import generate

class MixedContract(unittest.TestCase):
    def test_real_two_socket_protocols_each_side(self):
        for role,bind,peer in (
            ("biz","10.40.0.2:28100","10.50.0.2:18100"),
            ("target","10.50.0.2:18100",None)):
            tcp,udp=specs(role,bind,peer,12345,2608103,300,10,__import__('pathlib').Path("artifacts"))
            self.assertIn("longmix_tcp_business.py",str(tcp))
            self.assertIn("longmix_udp_business.py",str(udp))
            self.assertIn("C",tcp)
            self.assertIn("C",udp)
            self.assertIn("5",udp)
            self.assertEqual(len(set((tcp[1],udp[1]))),2)
        self.assertEqual(sum(q for _,q in C_UDP_SHARE),100)
        self.assertEqual((WORKLOADS["C"].tcp_mbps,WORKLOADS["C"].udp_mbps),(5,5))

    def test_generated_shell_is_exactly_one_product_topology(self):
        x=next(x for x in sample_matrix() if x["workload"]=="C")
        s=generate(x)
        self.assertIn("route-mode all",s)
        self.assertEqual(s.count('python3 "$GEN"'),2)
        self.assertIn("longmix_mixed_business.py",s)
        self.assertIn("mixed-biz-launcher.json",s)
        self.assertIn("mixed-target-launcher.json",s)
        self.assertIn('--workload C --seed "$SEED"',s)
        self.assertNotIn('--duration 120',s)
        self.assertIn('--duration 300 --drain 10',s)
        self.assertIn('"two_protocols_one_sample":True',s)
        self.assertIn('tcp_mbps_each_direction":5',s)
        self.assertIn('udp_mbps_each_direction":5',s)
    def test_refuse_other_workloads(self):
        for w in ('A','B'):
            x=next(x for x in sample_matrix() if x["workload"]==w)
            with self.assertRaises(ValueError):generate(x)
if __name__=="__main__":unittest.main(verbosity=2)
