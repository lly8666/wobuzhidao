import unittest
from soak_stats import ExactBitset, Histogram, slot_count, stats_class
from realpath_udp_duplex import Stats


class SoakStatsTests(unittest.TestCase):
    def test_exact_slots_match_real_sender_schedule(self):
        for duration, rate in [(1, .01), (3, .07), (1800, 3), (1800, 10), (120, 10), (1, 0)]:
            budget = duration * rate * 1_000_000 / 8
            cumulative = count = 0
            while cumulative < budget:
                cumulative += (64, 256, 1200)[count % 3]
                count += 1
            self.assertEqual(slot_count(duration, rate), count)

    def test_exact_full_run_dedupe_and_out_of_range(self):
        b = ExactBitset(1001)
        for i in [1000, 0, 500, 1000, 7, 0]: b.add(i)
        self.assertEqual(len(b), 4)
        self.assertEqual(len(b.bits), 126)
        self.assertIn(1000, b)
        for bad in [-1, 1001]:
            with self.assertRaises(ValueError): b.add(bad)

    def test_quantiles_are_conservative_and_bounded(self):
        h = Histogram(100, 1000)
        data = [0, 1, 100, 110, 199, 200, 999, 1001, 2000]
        for v in data: h.append(v)
        self.assertEqual(h.overflow, 2)
        self.assertEqual(h.maximum, 2000)
        for q in [0, .5, .9, .99, 1]:
            exact = sorted(data)[int(round((len(data)-1)*q))]
            value = h.percentile(q)
            if exact <= 1000:
                self.assertGreaterEqual(value, exact)
                self.assertLessEqual(value-exact, 100)
        self.assertEqual(len(h.bins), 11)

    def test_owned_stats_delivery_and_capacity_failure(self):
        S = stats_class(Stats)
        s = S(1_000_000_000, 1, 1, .01)
        p = dict(kind=1, seq=0, send_ns=1_000_000_000, size=64)
        s.note_recv(p, 1_300_000_000, 1)
        s.note_recv(p, 1_300_000_001, 1)
        p['seq'] = s.recv_seen.capacity
        s.note_recv(p, 1_300_000_002, 1)
        s.note_send(1_000_000_000,64,100)
        result = s.snapshot()
        self.assertEqual(result['recv_unique_packets'],1)
        self.assertEqual(result['recv_duplicates'],1)
        self.assertEqual(result['bounded_stats']['capacity_errors'],1)
        self.assertEqual(result['unexpected'],1)


if __name__ == '__main__': unittest.main()
