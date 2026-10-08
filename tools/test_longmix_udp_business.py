#!/usr/bin/env python3
"""Actions-only static proof that UDP A uses independent socket source paths."""
import unittest
from longmix_udp_business import ACK_KIND, ACK_SIZE, Audit, PROBE_PORT, DATA_PORT, whole_measured_mbps
from longmix_profile import A_UDP_SHARE, WeightedUDPSlots
from realpath_udp_duplex import decode_packet, make_packet, KIND_C2S

class LongmixUDPContract(unittest.TestCase):
    def test_argparse_rate_float_passes_exact_integer_byte_accounting(self):
        self.assertEqual(whole_measured_mbps(10.0), 10)
        self.assertEqual(whole_measured_mbps(10), 10)
        for bad in (10.001, -10.0, 0.0, float("inf"), float("nan"), True, "10"):
            with self.subTest(value=bad), self.assertRaises(ValueError):
                whole_measured_mbps(bad)

    def test_real_payload_endpoints_and_independent_probe_port(self):
        self.assertNotEqual(DATA_PORT, PROBE_PORT)
        self.assertEqual(ACK_SIZE, 96)
        self.assertNotEqual(ACK_KIND, KIND_C2S)
        for size, _ in A_UDP_SHARE:
            p = make_packet(KIND_C2S, 7, size, 8101, 111111)
            self.assertEqual(len(p), size)
            d, err = decode_packet(p)
            self.assertIsNone(err, err)
            self.assertEqual(d["size"], size)

    def test_size_scheduler_no_equal_pps_miscalculation(self):
        s = WeightedUDPSlots(A_UDP_SHARE)
        for _ in range(50000):
            s.next_size()
        summary = s.summary()
        for row in summary["sizes"]:
            self.assertLess(abs(row["actual_byte_percent"] - row["target_byte_percent"]), 0.15)
        self.assertGreater(s.counts[96], s.counts[65507] * 100)
        self.assertEqual(sum(s.bytes.values()), s.total_bytes)

    def test_late_ack_is_not_counted_as_missing_data(self):
        a = Audit(1000, 300, 10)
        a.on_sent(65507, 17, 2000, 0)
        packet, error = decode_packet(make_packet(ACK_KIND, 17, ACK_SIZE, 8101, 2000))
        self.assertIsNone(error)
        a.on_ack(packet, 1_250_002_000)
        self.assertEqual(a.ack_count[65507], 1)
        self.assertEqual(a.snapshot()["per_size"][-1]["big_ack_over_1s"], 1)
        self.assertEqual(a.snapshot()["per_size"][-1]["valid_first_receive"], 0)

if __name__ == "__main__":
    unittest.main(verbosity=2)
