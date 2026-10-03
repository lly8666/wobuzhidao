import json,tempfile,unittest
from pathlib import Path
from config_effective_args import cases,prepare
from prepare_config_harness import generate

class ConfigFixtures(unittest.TestCase):
    def test_all_declared_configuration_cases(self):
        inventory=cases();self.assertEqual(len(inventory),70)
        for case,p in inventory.items():
            s=generate(case);self.assertEqual(s.count('tools/config_business.py" --role biz'),1);self.assertIn(f'mtu {p["mtu"]} up',s);self.assertIn('none-functional-configuration',s)
            with tempfile.TemporaryDirectory() as d:
                for side in ('client','server'):
                    prepare(d,case,side,['--mtu','1400','--'+side+'-record-limit','1300','--raw-interface','test0'])
                    args=(Path(d)/(side+'-args.bin')).read_bytes().split(b'\0')[:-1]
                    self.assertIn(b'test0',args)
                    if p['mode']=='json':self.assertNotIn(b'--fec-parity',args)
                    if p['mode']=='priority':
                        conf=json.loads((Path(d)/(side+'-config.json')).read_text());self.assertNotEqual(conf['tls-startup-padding'],p['padding']);self.assertIn(b'--tls-startup-padding',args)

if __name__=='__main__':unittest.main()
