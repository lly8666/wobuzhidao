import unittest
from fec_policy_batch import cases,validate,SOURCE,namespace_suffix
class ExactSerialPlan(unittest.TestCase):
    def test_plan(self):
        d=dict(schema="wbd-fec-policy-batch/v1",phase="batch_a",batch="A",source_sha=SOURCE,nonce=1)
        a=validate(d);self.assertEqual(len(a),12)
        self.assertEqual(set(c["workload"] for c in a),{"udp","tcp","mixed"})
        self.assertEqual(set(c["loss"] for c in a),{0,1})
        self.assertTrue(all(c["duration_s"]==120 and c["delay_ms"]==15 for c in a))
    def test_plan_b_300ms_same_sequential_12_cases(self):
        d=dict(schema="wbd-fec-policy-batch/v1",phase="batch_b",batch="B",source_sha=SOURCE,nonce=7)
        a=validate(d)
        self.assertEqual(len(a),12)
        self.assertTrue(all(c["duration_s"]==120 and c["drain_s"]==3 and c["delay_ms"]==300 for c in a))
        self.assertEqual([c["fec"] for c in a],["off","on","on","off","off","on","on","off","off","on","on","off"])
        for i in range(0,12,2):
            self.assertEqual(a[i]["seed"],a[i+1]["seed"])
    def test_reject_unauthorized_batch_phase(self):
        for phase,batch in (("batch_a","B"),("batch_b","A"),("pilot","B"),("preflight","B")):
            d=dict(schema="wbd-fec-policy-batch/v1",phase=phase,batch=batch,source_sha=SOURCE,nonce=7)
            with self.assertRaises(ValueError):validate(d)
    def test_reject_excess(self):
        d=dict(schema="wbd-fec-policy-batch/v1",phase="batch_a",batch="A",source_sha=SOURCE,nonce=1,extra=True)
        with self.assertRaises(ValueError):validate(d)

class NamespaceOwnedSuffix(unittest.TestCase):
    def test_numeric_only(self):
        self.assertEqual(namespace_suffix("s01"),"01")
        self.assertEqual(namespace_suffix("s12"),"12")
        self.assertEqual(namespace_suffix("pilot-1"),"901")
        with self.assertRaises(ValueError):namespace_suffix("../../../etc")
