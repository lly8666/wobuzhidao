import json,tempfile,unittest
from pathlib import Path
import adaptive_n2_mixed_nohol_realpath as fixture
import adaptive_n2_mixed_nohol_business as business

class FrozenMixedNoHOLFixture(unittest.TestCase):
    def test_exact_single_scenario_source_and_real_kernel_flows(self):
        with tempfile.TemporaryDirectory() as td:
            out=Path(td)/"derived.sh"
            receipt=Path(td)/"harness-prep.json"
            fixture.prepare(Path("."),out,receipt)
            s=out.read_text()
            self.assertEqual(s.count("loss random 1%"),2)
            self.assertEqual(s.count("--workload mixed"),2)
            self.assertEqual(s.count("--duration-s 120"),2)
            self.assertEqual(s.count("--rate-mbps 10"),2)
            self.assertEqual(s.count('GEN="$GITHUB_WORKSPACE/tools/adaptive_n2_mixed_nohol_business.py"'),1)
            self.assertEqual(s.count('--fec-mode auto --fec-min-parity 4'),1)
            self.assertEqual(s.count('--fec-parity 20'),1)
            self.assertEqual(s.count('--role target --bind 10.50.0.2:18080 --peer 10.40.0.2:28080'),1)
            self.assertEqual(s.count('--role biz --bind 10.40.0.2:28080 --peer 10.50.0.2:18080'),1)
            self.assertEqual(s.count('resource-samples.jsonl'),1)
            self.assertNotIn("netem loss gemodel",s)
            self.assertEqual(json.loads(receipt.read_text())["performance_cases"],1)

    def test_independent_10ms_packet_decode_and_corrupt_rejection(self):
        packet=business.udp_packet(10,27,96,0x565656,123456789)
        self.assertEqual(business.decode_udp(packet),(10,27,96,123456789))
        mutated=bytearray(packet);mutated[-1]^=1
        self.assertIsNone(business.decode_udp(bytes(mutated)))
        self.assertIsNone(business.decode_udp(packet[:-1]))

    def test_10ms_continuity_and_p99_gate_calculation(self):
        buckets={str(x):1 for x in range(150,11800)}
        self.assertEqual(fixture.longest_zero(buckets,150,11800),0)
        for i in range(1000,1020):buckets.pop(str(i))
        self.assertEqual(fixture.longest_zero(buckets,150,11800),20)
        self.assertEqual(fixture.percentile([100,200,300,400,500],.99),400)

if __name__=="__main__":unittest.main()
