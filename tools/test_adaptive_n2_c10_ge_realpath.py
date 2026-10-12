import json,tempfile,unittest
from pathlib import Path
from adaptive_n2_c10_ge_realpath import prepare
from adaptive_n2_ge_eventmap import make_trace

class C10TrueGEPreflight(unittest.TestCase):
    def test_exact_frozen_model_and_epoch_before_business(self):
        root=Path(".")
        frozen=json.loads((root/"docs/evidence/adaptive-n2-c10-ctmc-event-trace-freeze-v1.json").read_text())
        trace=make_trace(frozen)
        self.assertEqual(len(trace["boundaries_ms"]),900)
        self.assertEqual(trace["initial_bad"],0)
        with tempfile.TemporaryDirectory() as td:
            script=Path(td)/"derived.sh"
            receipt=Path(td)/"receipt.json"
            prepare(root,script,receipt)
            text=script.read_text()
            rec=json.loads(receipt.read_text())
            self.assertEqual(rec["frozen_trace_transition_count"],900)
            self.assertEqual(rec["frozen_trace_sha256"],
                             "5fcfe423903d5b7665eb8bb341170e901757b5e6def4af1ba4ca242670237aec")
            self.assertEqual(text.count('--duration 240 --drain 10 --rate-mbps 10 --bounded-stats'),2)
            self.assertEqual(text.count('tc qdisc add dev rsrv clsact'),1)
            self.assertEqual(text.count('tc qdisc add dev rcli clsact'),1)
            self.assertEqual(text.count('tc filter add dev rsrv egress pref 1 bpf da obj'),1)
            self.assertEqual(text.count('tc filter add dev rcli egress pref 1 bpf da obj'),1)
            self.assertIn('--trace "$ART/ge-frozen-trace.json" --start-ns "$START_NS"',text)
            self.assertIn('"loss_percent": "ge-clock450-50-bad50-240s"',text)
            self.assertNotIn("loss gemodel",text)
            self.assertNotIn("loss random 5%",text)
            before=text.index('ge-maps-loaded.json')
            after=text.index('ge-maps-audited.json')
            # Real workload cannot start before exact BPF map load.
            first_generator=text.index('ip netns exec "$TGT" python3 "$GEN"')
            self.assertLess(before,first_generator)
            self.assertGreater(after,first_generator)

    def test_absent_contaminating_source_truth(self):
        with tempfile.TemporaryDirectory() as td:
            script=Path(td)/"derived.sh"
            prepare(Path("."),script,Path(td)/"prep.json")
            data=script.read_text()
            # The formal V3 client uses adaptive mode/min4, while only
            # the server advertises a fixed fec-parity 20 CLI argument.
            # Counting two fixed arguments would misclassify a healthy
            # client auto policy as invalid before any real GE traffic.
            self.assertEqual(data.count('--fec-parity 20'),1)
            self.assertEqual(data.count('--fec-mode auto --fec-min-parity 4'),1)
            self.assertNotIn('--ge-time-truth',data)
            self.assertNotIn('--loss-percent',data)

if __name__=="__main__":
    unittest.main()
