"""Offline controlled fragment fixtures. No driver, socket, or workload."""
import socket
import struct
import tempfile
import unittest
from pathlib import Path
from physical_inner_fragment_watch import metadata, pcapng_rows

SOURCE, DEST = '198.18.0.1', '10.66.129.113'


def packet(offset, more, first=False):
    body = (struct.pack('!HHHH', 18446, 45000, 65515, 0)+b'P7M1'+struct.pack('<I', 1796)) if first else b'\0'*16
    flags = offset//8 | (8192 if more else 0)
    header = bytearray(struct.pack('!BBHHHBBH4s4s', 0x45, 0, 1396, 123, flags, 64, 17, 0, socket.inet_aton(SOURCE), socket.inet_aton(DEST)))
    total = sum(struct.unpack('!10H', header))
    while total >> 16:
        total = (total & 65535)+(total >> 16)
    header[10:12] = struct.pack('!H', (~total) & 65535)
    return bytes(header)+body


def block(kind, body):
    return struct.pack('<II', kind, len(body)+12)+body+struct.pack('<I', len(body)+12)


def capture(pkt):
    section = block(0x0a0d0d0a, struct.pack('<IHHq', 0x1a2b3c4d, 1, 0, -1))
    # DLT_RAW is the TUN packet representation, not Ethernet. Timestamp
    # resolution explicitly nanoseconds; EPB contains a truncated IP packet.
    interface = block(1, struct.pack('<HHI', 101, 0, 64)+struct.pack('<HH', 9, 1)+b'\x09\0\0\0'+b'\0'*4)
    body = struct.pack('<IIIII', 0, 0, 123456789, len(pkt), 1396)+pkt
    body += b'\0'*((-len(body)) % 4)
    return section+interface+block(6, body)


class FragmentFixtures(unittest.TestCase):
    def test_nonfirst_fragment_keeps_offset_without_udp_port(self):
        row = metadata(packet(1376, True), SOURCE, DEST)
        self.assertEqual((row['offset'], row['fragment_payload_length'], row['mf']), (1376, 1376, True))
        self.assertIsNone(row['sequence'])
        self.assertTrue(row['header_checksum_ok'])
        self.assertNotIn('payload', row)

    def test_controlled_first_fragment_and_wrong_scope(self):
        pkt = packet(0, True, True)
        row = metadata(pkt, SOURCE, DEST)
        self.assertEqual((row['sequence'], row['udp_length']), (1796, 65515))
        self.assertIsNone(metadata(pkt, SOURCE, '10.66.1.1'))
        self.assertIsNone(metadata(pkt, '198.18.0.2', DEST))

    def test_pcapng_raw_tun_timestamp_and_truncation(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp)/'bounded.pcapng'
            p.write_bytes(capture(packet(0, True, True)))
            result = pcapng_rows(p, SOURCE, DEST)
            self.assertEqual(len(result['rows']), 1)
            self.assertAlmostEqual(result['rows'][0]['unix_ns'], 123456789, delta=1)
            self.assertEqual(result['rows'][0]['sequence'], 1796)
            self.assertTrue(result['no_raw_payload_stored'])
            self.assertEqual(result['coverage'], 'REQUIRES_PKT_MON_COUNTERS_AND_COMPONENT_SCOPE')

    def test_damaged_capture_cannot_report_complete_coverage(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = Path(tmp)/'bounded.pcapng'
            raw = capture(packet(1376, False))
            for malformed in (raw[:-1], raw[:-4]+b'\0'*4, block(3, b'\0'*4)):
                p.write_bytes(malformed)
                with self.assertRaises(ValueError):
                    pcapng_rows(p, SOURCE, DEST)


if __name__ == '__main__':
    unittest.main()
