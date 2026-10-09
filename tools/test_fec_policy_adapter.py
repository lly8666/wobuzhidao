import unittest
from fec_policy_batch import cases,validate,SOURCE,namespace_suffix
class ExactSerialPlan(unittest.TestCase):
    def test_plan(self):
        d=dict(schema="wbd-fec-policy-batch/v1",phase="batch_a",batch="A",source_sha=SOURCE,nonce=1)
        a=validate(d);self.assertEqual(len(a),12)
        self.assertEqual(set(c["workload"] for c in a),{"udp","tcp","mixed"})
        self.assertEqual(set(c["loss"] for c in a),{0,1})
        self.assertTrue(all(c["duration_s"]==120 and c["delay_ms"]==15 for c in a))
    def test_reject_excess(self):
        d=dict(schema="wbd-fec-policy-batch/v1",phase="batch_a",batch="A",source_sha=SOURCE,nonce=1,extra=True)
        with self.assertRaises(ValueError):validate(d)

class NamespaceOwnedSuffix(unittest.TestCase):
    def test_numeric_only(self):
        self.assertEqual(namespace_suffix("s01"),"01")
        self.assertEqual(namespace_suffix("s12"),"12")
        self.assertEqual(namespace_suffix("pilot-1"),"901")
        with self.assertRaises(ValueError):namespace_suffix("../../../etc")
