import threading
import unittest
from unittest.mock import patch

import realpath_udp_duplex as generator


class AbsolutePacingTests(unittest.TestCase):
    def test_early_wakeup_rechecks_absolute_deadline(self):
        clock = [100_000]
        waits = []

        def sleep(seconds):
            waits.append(seconds)
            requested = round(seconds * 1e9)
            clock[0] += requested // 2 if len(waits) == 1 else requested

        with patch.object(generator.time, "monotonic_ns", side_effect=lambda: clock[0]), \
                patch.object(generator.time, "sleep", side_effect=sleep):
            observed = generator.wait_until(500_000)
        self.assertEqual(observed, 500_000)
        self.assertEqual(waits, [0.0004, 0.0002])

    def test_late_deadline_returns_actual_clock_without_sleep(self):
        with patch.object(generator.time, "monotonic_ns", return_value=900_000), \
                patch.object(generator.time, "sleep") as sleep:
            self.assertEqual(generator.wait_until(500_000), 900_000)
        sleep.assert_not_called()

    def test_late_slot_stays_missing_and_later_slots_keep_original_schedule(self):
        # A 15ms scheduling delay misses slot0; sleep overshoots the next
        # deadlines by 100us. Neither delay can become hidden injection or
        # shift the absolute bandwidth schedule of slot1/slot2.
        clock = [15_000_000]
        stop = threading.Event()
        packets = []
        stats = generator.Stats(0, 1, 0)

        def sleep(seconds):
            clock[0] += round(seconds * 1e9) + 100_000

        class Socket:
            def sendto(self, data, peer):
                packets.append(data)
                if len(packets) == 2:
                    stop.set()

        with patch.object(generator.time, "monotonic_ns", side_effect=lambda: clock[0]), \
                patch.object(generator.time, "sleep", side_effect=sleep):
            generator.run_sender(Socket(), lambda: ("127.0.0.1", 1),
                                 generator.KIND_C2S, 0.03, 17, stats, stop)
        decoded = [generator.decode_packet(data) for data in packets]
        self.assertTrue(all(error is None for _, error in decoded))
        self.assertEqual([packet["seq"] for packet, _ in decoded], [1, 2])
        self.assertEqual([packet["size"] for packet, _ in decoded], [256, 1200])
        self.assertEqual([packet["send_ns"] for packet, _ in decoded],
                         [17_166_666, 85_433_333])
        snapshot = stats.snapshot()
        self.assertEqual(snapshot["skipped_slots"], 1)
        self.assertEqual(snapshot["skipped_bytes"], 64)
        self.assertEqual(snapshot["sent_bytes"], 1456)
        self.assertEqual(snapshot["send_failures"], 0)
        self.assertEqual(snapshot["send_lag_p99_ns"], 100_000)


if __name__ == "__main__":
    unittest.main()
