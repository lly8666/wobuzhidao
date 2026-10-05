import unittest
from realpath_udp_duplex import Stats, MAX_PROBE_EVENTS, make_packet, decode_packet, KIND_PROBE


class ProbeClockEvidence(unittest.TestCase):
    def test_same_host_direction_timing_without_wire_change(self):
        packet, error = decode_packet(make_packet(KIND_PROBE, 81, 64, 99, 100))
        self.assertIsNone(error)
        target = Stats(0, 120, 10)
        target.note_probe_event(packet, 400, 405)
        client = Stats(0, 120, 10)
        client.note_probe_event(packet, 710)
        event = target.snapshot()["probe_events"][0]
        reply = client.snapshot()["probe_events"][0]
        self.assertEqual(event["received_ns"] - event["sent_ns"], 300)
        self.assertEqual(event["reply_sent_ns"] - event["received_ns"], 5)
        self.assertEqual(reply["received_ns"] - event["reply_sent_ns"], 305)
        self.assertEqual(target.snapshot()["sent_packets"], 0)
        self.assertEqual(target.snapshot()["recv_unique_packets"], 0)

    def test_bound_and_failed_echo_remain_visible(self):
        stats = Stats(0, 120, 10)
        packet = {"seq": 0, "send_ns": 0}
        for _ in range(MAX_PROBE_EVENTS + 2):
            stats.note_probe_event(packet, 100, 101, True)
        snapshot = stats.snapshot()
        self.assertEqual(len(snapshot["probe_events"]), MAX_PROBE_EVENTS)
        self.assertEqual(snapshot["probe_event_drops"], 2)
        self.assertTrue(snapshot["probe_events"][0]["reply_error"])


if __name__ == "__main__":
    unittest.main()
