#!/usr/bin/env python3
"""Functional planning of long/full-duplex TCP only; never performance proof."""
import hashlib
import unittest
from longmix_profile import TCP_WRITE_SIZES, TCP_LONG_CONNECTIONS, TCP_SHORT_CONNECTIONS
from longmix_tcp_business import (
    HEADER,MAGIC,SHORT_BYTES,SHORT_HZ,OneFlow,tcp_info
)

class DummySocket:
    def getsockopt(self, level, opt, length=None):
        if length is None:
            return 1400
        return bytes(length)

class TcpWorkloadContract(unittest.TestCase):
    def test_3_distinct_full_duplex_long_connections_and_short_control(self):
        self.assertEqual(TCP_LONG_CONNECTIONS,3)
        self.assertEqual(TCP_SHORT_CONNECTIONS,1)
        self.assertEqual((SHORT_BYTES,SHORT_HZ),(96,1))
        self.assertEqual(TCP_WRITE_SIZES,(96,4096,65536,1048576))
        for i in range(TCP_LONG_CONNECTIONS):
            self.assertEqual(HEADER.unpack(HEADER.pack(MAGIC,i)),(MAGIC,i))

    def test_oneflow_bounded_metadata_and_independent_direction_hash(self):
        fs=[OneFlow("biz",i,DummySocket(),100000000,300,10,1_250_000/3)
            for i in range(3)]
        for f in fs:
            r=f.snapshot()
            self.assertEqual(r["send_bytes"],0)
            self.assertEqual(r["recv_bytes"],0)
            self.assertEqual(r["tcp_info_initial"]["tcp_maxseg_socket"],1400)
            self.assertEqual(len(r["send_by_second"]),300)
            self.assertEqual(len(r["recv_by_second"]),311)
            self.assertEqual(r["recv_longest_zero_delivery_ms"],300000)
        self.assertEqual(len({f.byte_value for f in fs}),3)
        self.assertNotEqual(fs[0].byte_value,
                            OneFlow("target",0,DummySocket(),100000000,300,10,1_250_000/3).byte_value)

    def test_full_1m_app_write_does_not_claim_it_is_one_ip_packet(self):
        b=bytes([99])*1048576
        h=hashlib.sha256()
        h.update(memoryview(b)[:512])
        h.update(memoryview(b)[512:])
        self.assertEqual(h.hexdigest(),hashlib.sha256(b).hexdigest())
        self.assertGreater(max(TCP_WRITE_SIZES),9000)

if __name__=="__main__":unittest.main(verbosity=2)
