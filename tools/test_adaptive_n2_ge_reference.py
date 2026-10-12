import json
import math
import unittest
from pathlib import Path

from adaptive_n2_ge_reference import (
    VERSION, PROFILES, params, state_transition,
    exact_source_residual, group_timestamps, mixture, freeze)


class IndependentCTMCFECReferenceTests(unittest.TestCase):
    def setUp(self):
        self.ge={"good_mean_ms":450,"bad_mean_ms":50,
                 "good_loss":0,"bad_loss":0.5}

    def test_stationary_average_loss_is_five_percent(self):
        rg,rb,pg,pb,bad=params(self.ge)
        self.assertAlmostEqual(bad,.1,places=12)
        self.assertAlmostEqual((1-bad)*pg+bad*pb,.05,places=12)
        t=state_transition(rg,rb,.42/1000)
        for row in t:
            self.assertAlmostEqual(sum(row),1.,places=12)
        self.assertGreater(t[1][1],t[0][1])

    def test_k1_same_time_source_and_parity_are_correlated(self):
        self.assertEqual(group_timestamps(1,4,.42,0),[0,0])
        self.assertAlmostEqual(exact_source_residual(1,0,self.ge,.42,0),.05)
        self.assertAlmostEqual(exact_source_residual(1,4,self.ge,.42,0),.025)
        self.assertAlmostEqual(exact_source_residual(1,20,self.ge,.42,0),.025)

    def test_iid_constant_state_conditional_losses_matches_binomial(self):
        p=.05
        independent={"good_mean_ms":450,"bad_mean_ms":50,
                     "good_loss":p,"bad_loss":p}
        for k in (1,2,5,20):
            for r in PROFILES:
                eff=min(k,r)
                n=k+eff-1
                want=p if not eff else p*sum(
                    math.comb(n,j)*p**j*(1-p)**(n-j) for j in range(eff,n+1))
                got=exact_source_residual(k,r,independent,.42,.1)
                self.assertAlmostEqual(got,want,places=9)

    def test_timing_and_actual_partial_shapes_matter(self):
        loss_full=exact_source_residual(20,8,self.ge,.42,.1)
        loss_partial=exact_source_residual(1,8,self.ge,.42,.1)
        self.assertNotAlmostEqual(loss_full,loss_partial)
        loss_wide=exact_source_residual(20,8,self.ge,10,2)
        self.assertNotAlmostEqual(loss_full,loss_wide,places=5)
        self.assertGreaterEqual(mixture([{"k":20,"count":50},{"k":1,"count":4}],
                                         8,self.ge,.42,.1),0)

    def test_frozen_manifest_and_unattainable_flag(self):
        path=Path("docs/evidence/adaptive-n2-ge-ctmc-freeze-v1.json")
        payload=json.loads(path.read_text())
        self.assertEqual(payload["model_version"],VERSION)
        result=freeze(payload)
        self.assertFalse(result["production_selection_from_netem_truth"])
        self.assertAlmostEqual(result["stationary_mean_loss"],.05)
        self.assertEqual(len(result["results"]),len(payload["assumptions"]))
        altered=json.loads(json.dumps(payload))
        altered["stationary_mean_loss"]=0.01
        with self.assertRaises(ValueError):
            freeze(altered)

    def test_nonmonotonic_events_are_rejected(self):
        r,b,*_=params(self.ge)
        with self.assertRaises(ValueError):
            state_transition(r,b,-1)


if __name__=="__main__":
    unittest.main()
