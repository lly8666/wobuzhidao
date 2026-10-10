"""Actions-only contract for isolated frozen-product crypto packet ABBA."""
import unittest
import crypto_packet_abba as ab

class TestCryptoPacketABBA(unittest.TestCase):
    def test_frozen_source_and_four_leg_order(self):
        self.assertEqual(ab.A,"7fb98fab79834a351a1dbe04eebb207f66bea28b")
        self.assertEqual(ab.B,"37e18653b0d08f4a1d932b6fd67fe081e84bda78")
        for phase in ("q1screen120","off120","game120","q120","l120","confirm300"):
            cases=ab.plan(phase)
            self.assertEqual(len(cases)%4,0)
            for i in range(0,len(cases),4):
                g=cases[i:i+4]
                self.assertEqual([x["label"] for x in g],["A","B","B","A"])
                self.assertEqual([x["source"] for x in g],[ab.A,ab.B,ab.B,ab.A])
                self.assertEqual(len({(x["mode"],x["lanes"],x["rate"],x["parity"],x["delay"],x["loss"],x["seed"],x["duration"]) for x in g}),1)
                self.assertEqual(g[0]["duration"],300 if phase=="confirm300" else 120)
    def test_gates_and_legacy_immutability(self):
        d=ab.conf()
        self.assertEqual(d["baseline_source_sha"],ab.A)
        self.assertEqual(d["candidate_source_sha"],ab.B)
        self.assertEqual(d["phase"],"preflight")
        self.assertFalse(ab.may_observe_next("q1screen120",{"classification":"FAIL","issues":["5205_STAGE_PROBE_LOSS_OVER_1PCT_c2s"]}))
        self.assertEqual(ab.plan("off120")[0]["parity"],0)
        self.assertEqual(ab.plan("off120")[0]["delay"],15)
        self.assertEqual(ab.plan("game120")[0]["rate"],3)
