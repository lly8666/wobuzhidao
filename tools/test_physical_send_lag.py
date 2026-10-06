"""Fixed vectors for native input qualification; no network workload."""
import math
import unittest

from physical_udp_server import SendLag


class SendLagTests(unittest.TestCase):
    def test_percentile_is_an_upper_bound_with_exact_coverage(self):
        values = [0, .0001, .05, .1, 1.05, 10, 10.001]
        histogram = SendLag()
        for value in values:
            histogram.record(value)
        self.assertEqual(histogram.samples, len(values))
        self.assertEqual(sum(histogram.bins), len(values))
        for percentile in (.01, .5, .95, .99, 1):
            exact = sorted(values)[math.ceil(len(values) * percentile)-1]
            self.assertGreaterEqual(histogram.upper_percentile(percentile), exact)
        self.assertEqual(histogram.upper_percentile(.5), .1)
        self.assertEqual(histogram.overflow, 1)

    def test_one_outlier_does_not_replace_p99_with_max(self):
        histogram = SendLag()
        for _ in range(99):
            histogram.record(1.05)
        histogram.record(200)
        self.assertEqual(histogram.upper_percentile(.99), 1.1)
        self.assertEqual(histogram.upper_percentile(1), 200)
        self.assertEqual(histogram.maximum, 200)

    def test_overflow_percentile_remains_conservative(self):
        histogram = SendLag()
        for _ in range(98):
            histogram.record(1.05)
        histogram.record(20)
        histogram.record(200)
        self.assertEqual(histogram.upper_percentile(.99), 200)
        self.assertEqual(histogram.overflow, 2)

    def test_empty_negative_finite_and_memory_bound(self):
        histogram = SendLag()
        self.assertEqual(histogram.upper_percentile(.99), 0)
        histogram.record(-1)
        self.assertEqual(histogram.upper_percentile(.99), 0)
        for _ in range(100_000):
            histogram.record(10)
        self.assertEqual(len(histogram.bins), 102)
        self.assertEqual(histogram.samples, 100_001)
        self.assertEqual(sum(histogram.bins), 100_001)
        self.assertEqual(histogram.upper_percentile(.99), 10)
        for bad in (math.nan, math.inf, -math.inf):
            with self.assertRaises(ValueError):
                histogram.record(bad)
        for bad in (0, -1, 1.001, math.nan, math.inf):
            with self.assertRaises(ValueError):
                histogram.upper_percentile(bad)


if __name__ == '__main__':
    unittest.main()
