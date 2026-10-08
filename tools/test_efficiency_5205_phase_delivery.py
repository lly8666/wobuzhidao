#!/usr/bin/env python3
"""Deterministic guard for actual-send-time 5205 business stage receipts.

Runs before *one* fullstack Actions sample; does not create a second sample.
"""
import unittest
from check_large_mtu_mixed import phase_delivery
from large_mtu_mixed_business import Totals

N=1_000_000_000
PRE=75*N
STRESS=225*N

class PhaseDeliveryContract(unittest.TestCase):
    def test_sender_clock_boundaries_and_deadlines(self):
        start=7_000*N
        tx=Totals(start)
        rx=Totals(start)
        samples=[
            (0, "pre"),(PRE-1,"pre"),(PRE,"stress"),
            (STRESS-1,"stress"),(STRESS,"post"),(300*N-1,"post"),
        ]
        for i,(delta,phase) in enumerate(samples):
            tx.stage_record("udp_stage_tx",start+delta,size=96)
            rx.stage_record("udp_stage_rx",start+delta,
                            received_ns=start+delta+(3*N+1 if i==3 else 250_000_000),
                            size=96)
            self.assertEqual(next(reversed(tx.d["udp_stage_tx"])),phase)
        out=phase_delivery(tx.d,rx.d,"udp")
        self.assertEqual({k:v["sent"] for k,v in out.items()},
                         {"pre":2,"stress":2,"post":2})
        self.assertEqual(sum(z["missing"] for z in out.values()),0)
        self.assertEqual(out["stress"]["over_3s"],1)
        self.assertEqual(out["stress"]["over_1s"],1)
        self.assertEqual(out["pre"]["by_size"]["96"]["delivered"],2)

    def test_missing_per_phase_remains_missing_not_survivor_p99(self):
        start=10*N;tx=Totals(start);rx=Totals(start)
        for sec in (1,2,76,77,226,227):
            tx.stage_record("udp_stage_tx",start+sec*N,size=256)
            if sec!=77:
                # Arrival during another phase still belongs to send phase.
                rx.stage_record("udp_stage_rx",start+sec*N,
                                received_ns=start+(sec+80)*N,size=256)
        out=phase_delivery(tx.d,rx.d,"udp")
        self.assertEqual(out["stress"]["missing"],1)
        self.assertEqual(out["pre"]["delivered"],2)
        self.assertEqual(out["stress"]["over_3s"],1)
        self.assertEqual(out["post"]["delivered"],2)

    def test_probe_source_and_receiver_are_separate(self):
        start=10*N;a=Totals(start);b=Totals(start)
        for sec in (1,76,226):
            a.stage_record("probe_stage_tx",start+sec*N,size=96)
            # The destination echoes kind=3 -> kind=4; its own probe
            # counters are independent. The originating socket counts RTT.
            a.stage_record("probe_stage_rx",start+sec*N,
                           received_ns=start+sec*N+700_000_000,size=96)
        with self.assertRaises(ValueError):
            phase_delivery(a.d,b.d,"probe")
        out=phase_delivery(a.d,a.d,"probe")
        for name in ("pre","stress","post"):
            self.assertEqual(out[name]["sent"],1)
            self.assertEqual(out[name]["missing"],0)

    def test_inconsistent_late_or_outside_rejected(self):
        start=10*N;a=Totals(start);b=Totals(start)
        for second in (1,76,226):
            stamp=start+second*N
            a.stage_record("udp_stage_tx",stamp,size=96)
            b.stage_record("udp_stage_rx",stamp,received_ns=stamp+300_000_000,size=96)
        # Duplicate logical receipt or inconsistent by-size must fail closed.
        b.stage_record("udp_stage_rx",start+N,received_ns=start+N+400_000_000,size=96)
        with self.assertRaises(ValueError):phase_delivery(a.d,b.d,"udp")
        b=Totals(start)
        for second in (1,76,226):
            stamp=start+second*N
            b.stage_record("udp_stage_rx",stamp,received_ns=stamp+300_000_000,size=96)
        a.stage_record("udp_stage_tx",start-1,size=96)
        with self.assertRaises(ValueError):phase_delivery(a.d,b.d,"udp")

    def test_non_instrumented_legacy_does_not_falsely_pass(self):
        with self.assertRaises(ValueError):
            phase_delivery({"udp_tx":{"96":{"packets":100}}},
                           {"udp_rx":{"96":{"packets":100}}},"udp")

if __name__=="__main__":
    unittest.main()
