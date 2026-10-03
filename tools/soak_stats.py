"""Bounded exact sequence accounting and declared-resolution latency histograms."""
from array import array
from fractions import Fraction


def slot_count(duration, rate):
    budget = Fraction(str(duration)) * Fraction(str(rate)) * 1_000_000 / 8
    cycles, remainder = divmod(budget, 1520)
    return int(cycles) * 3 + sum(p < remainder for p in (0, 64, 320))


class ExactBitset:
    def __init__(self, capacity):
        self.capacity = capacity
        self.bits = bytearray((capacity + 7) // 8)
        self.count = 0

    def __contains__(self, seq):
        if not 0 <= seq < self.capacity:
            raise ValueError("sequence outside declared run capacity")
        return bool(self.bits[seq // 8] & (1 << (seq % 8)))

    def add(self, seq):
        if seq not in self:
            self.bits[seq // 8] |= 1 << (seq % 8)
            self.count += 1

    def __len__(self):
        return self.count


class Histogram:
    def __init__(self, width_ns, ceiling_ns):
        self.width_ns = width_ns
        self.ceiling_ns = ceiling_ns
        self.bins = array('Q', [0]) * (ceiling_ns // width_ns + 1)
        self.count = self.overflow = self.maximum = 0

    def append(self, value):
        value = max(0, int(value))
        self.count += 1
        self.maximum = max(self.maximum, value)
        if value > self.ceiling_ns:
            self.overflow += 1
        self.bins[min(value // self.width_ns, len(self.bins) - 1)] += 1

    def percentile(self, q):
        if not self.count:
            return None
        rank = int(round((self.count - 1) * q)) + 1
        seen = 0
        for i, count in enumerate(self.bins):
            seen += count
            if seen >= rank:
                return min(self.maximum, (i + 1) * self.width_ns)

    def receipt(self):
        return dict(resolution_ns=self.width_ns, ceiling_ns=self.ceiling_ns,
                    count=self.count, overflow=self.overflow, maximum_ns=self.maximum,
                    allocated_bytes=len(self.bins) * self.bins.itemsize)


def stats_class(base):
    class BoundedStats(base):
        def __init__(self, start_ns, duration_s, drain_s, rate):
            super().__init__(start_ns, duration_s, drain_s)
            self.recv_seen = ExactBitset(slot_count(duration_s, rate))
            self.send_lag_ns = Histogram(100_000, 100_000_000)
            self.oneway_ns = Histogram(500_000, 10_000_000_000)
            self.probe_rtt_ns = Histogram(500_000, 10_000_000_000)
            self.capacity_errors = 0

        def note_recv(self, packet, now_ns, expected_kind):
            if not 0 <= packet['seq'] < self.recv_seen.capacity:
                with self.lock:
                    self.capacity_errors += 1
                    self.unexpected += 1
                return
            super().note_recv(packet, now_ns, expected_kind)

        def snapshot(self):
            result = super().snapshot()
            result['bounded_stats'] = dict(
                version=1, exact_dedupe=True, sequence_capacity=self.recv_seen.capacity,
                bitset_bytes=len(self.recv_seen.bits), capacity_errors=self.capacity_errors,
                histograms={name: getattr(self, name).receipt() for name in
                            ['send_lag_ns', 'oneway_ns', 'probe_rtt_ns']})
            return result
    return BoundedStats
