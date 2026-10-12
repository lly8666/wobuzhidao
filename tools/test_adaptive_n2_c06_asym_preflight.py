"""C06 preflight tests: generated formal realpath tc impairment direction split."""
import json, tempfile, unittest
from pathlib import Path
from adaptive_n2_c06_asym10_1 import prepare, grades
from adaptive_n2_asym_reference import evaluate

class C06RealpathPreflight(unittest.TestCase):
    def test_freeze_and_derived_realpath_not_two_configs(self):
        root=Path(".")
        frozen=json.loads((root/"docs/evidence/adaptive-n2-c05-c06-asym-model-freeze-v1.json").read_text())
        prediction=evaluate(frozen)["results"]["C06"]["directions"]
        self.assertEqual((prediction["c2s"]["full_k20_ideal_parity"],
                          prediction["s2c"]["full_k20_ideal_parity"]),(8,4))
        with tempfile.TemporaryDirectory() as td:
            script=Path(td)/"derived.sh"
            receipt=Path(td)/"receipt.json"
            prepare(root,script,receipt)
            source=script.read_text()
            self.assertEqual(source.count('tc qdisc add dev rsrv root netem limit 200000 delay 300ms loss random 10%'),1)
            self.assertEqual(source.count('tc qdisc add dev rcli root netem limit 200000 delay 300ms loss random 1%'),1)
            self.assertEqual(source.count('--duration 240 --drain 10 --rate-mbps 10 --bounded-stats'),2)
            self.assertNotIn('loss random 10% loss random',source)
            r=json.loads(receipt.read_text())
            self.assertEqual(r["frozen_model_version"],"n2-asymmetric-iid-timeband-v1")
            self.assertFalse(r["netem_seed_applied"])
    def test_directional_grade_from_real_source_diags_not_nominal_loss(self):
        with tempfile.TemporaryDirectory() as td:
            p=Path(td)/"lane.jsonl"
            for server,parity in ((False,8),(True,4)):
                lane={"control":{"n2_controller_enabled":True},
                      "actual_tx_parity":parity,
                      "lane":{"TxPath":{"Encoder":{"source_bytes":512,"parity_bytes":256}}}}
                prod={"tunnel":{"lanes":[lane]}} if server else {"lanes":[lane]}
                samples=[json.dumps({"unix_ns":int((181+i)*1e9),"product":prod})
                         for i in range(58)]
                p.write_text("\n".join(samples)+"\n")
                out=grades(Path(td),p.name,server,0)
                self.assertTrue(out["stable_meets_90pct_screen"])
                self.assertEqual(out["final60_grade_screen"],[4,8] if server else [8,10])
                self.assertEqual(out["encoder_cumulative"]["parity_bytes"],256)
            p.write_text(json.dumps({"unix_ns":0,"product":{"tunnel":{"lanes":None}}})+"\n")
            self.assertEqual(grades(Path(td),p.name,True,0)["stable_active"],0)

if __name__=="__main__":
    unittest.main()
