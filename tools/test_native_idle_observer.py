import unittest
from native_idle_boundary_observer import parse_event


class IdleBoundaryMetadata(unittest.TestCase):
    def test_dns_and_tcp_keep_only_numeric_header_fields(self):
        dns = parse_event("1791167151.123 IP 10.66.0.2.53000 > 1.1.1.1.53: UDP, length 42 secret-question")
        self.assertEqual(dns["dst_port"], 53)
        self.assertEqual(dns["transport"], "UDP")
        self.assertNotIn("secret", str(dns))
        tcp = parse_event("1791167152.123 IP 10.66.0.2.49100 > 8.8.8.8.53: tcp 0")
        self.assertEqual(tcp["transport"], "TCP")
        self.assertEqual(set(tcp), {"unix_s", "src", "src_port", "dst", "dst_port", "transport"})

    def test_unparsed_headers_are_not_guessed(self):
        self.assertIsNone(parse_event("1791167152.123 IP 10.66.0.2 > 8.8.8.8: ICMP"))
        self.assertIsNone(parse_event("tcpdump: listening on wbdg0"))
        self.assertIsNone(parse_event("1791167152.123 IP 999.66.0.2.12 > 8.8.8.8.53: UDP, length 42"))
        self.assertIsNone(parse_event("1791167152.123 IP 10.66.0.2.65536 > 8.8.8.8.53: UDP, length 42"))
        self.assertIsNone(parse_event("1791167152.123 IP 10.66.0.2.12 > 8.8.8.8.53: unknown secret-tcp"))


if __name__ == "__main__":
    unittest.main()
