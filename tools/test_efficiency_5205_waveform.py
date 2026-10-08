#!/usr/bin/env python3
"""Offline, deterministic validation of the one-sample 5205 netem ledger.

No sockets, no namespaces, no product binary. This is a verifier contract
test executed by Actions *before* any 300-second real business scenario.
"""
import copy
import unittest
from check_large_mtu_mixed import packet_loss

N=1_000_000_000
def row(event, second, sent, dropped, loss):
    return {
        "event":event,
        "monotonic_ns":int(second*N),
        "loss_percent":{"c2s":loss,"s2c":loss},
        "qdisc":{side:[{"kind":"netem","packets":sent,"drops":dropped}]
                 for side in ("c2s","s2c")},
    }

class OneSampleWaveformTest(unittest.TestCase):
    def rows(self):
        return [
            row("pre_start",0,0,0,5),
            row("business_start",0.001,0,0,5),
            row("pre_end",75,9500,500,5),
            # tc qdisc change may reset its counters: the verifier must
            # compare each segment with its own matched phase snapshots.
            row("stress_start",75.002,0,0,20),
            row("stress_end",225,8000,2000,20),
            row("post_start",225.002,0,0,5),
            row("post_end",300,9500,500,5),
            row("business_end",300.005,9500,500,5),
            row("drain_end",303.005,10000,530,5),
        ]

    def test_true_5205_is_three_phases_one_business(self):
        aggregate, stages=packet_loss(self.rows())
        self.assertEqual(set(stages),{"pre","stress","post"})
        for stage,want in (("pre",5),("stress",20),("post",5)):
            self.assertEqual(stages[stage]["expected_percent"],want)
            for side in ("c2s","s2c"):
                self.assertEqual(stages[stage][side]["attempted"],10000)
                self.assertEqual(stages[stage][side]["realized_percent"],want)
        for side in ("c2s","s2c"):
            self.assertEqual(aggregate[side]["attempted"],30000)
            self.assertEqual(aggregate[side]["realized_percent"],10)

    def test_phase_time_or_loss_label_fails(self):
        wrong_time=self.rows()
        wrong_time[4]["monotonic_ns"]=int(231*N)
        with self.assertRaises(ValueError):
            packet_loss(wrong_time)
        wrong_loss=self.rows()
        wrong_loss[3]["loss_percent"]["c2s"]=5
        with self.assertRaises(ValueError):
            packet_loss(wrong_loss)

    def test_fixed_0_profile_remains_unchanged(self):
        one=[row("business_start",0,0,0,0),
             row("business_end",300,10000,0,0),
             row("drain_end",303,10000,0,0)]
        aggregate,phases=packet_loss(one)
        self.assertEqual(phases,{})
        for side in ("c2s","s2c"):
            self.assertEqual(aggregate[side]["attempted"],10000)
            self.assertEqual(aggregate[side]["realized_percent"],0)

if __name__=="__main__":
    unittest.main()
