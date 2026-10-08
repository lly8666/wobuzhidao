#!/usr/bin/env python3
"""Functional planning of long/full-duplex TCP only; never performance proof."""
import hashlib
import socket
import threading
import time
import unittest
from longmix_profile import TCP_WRITE_SIZES, TCP_LONG_CONNECTIONS, TCP_SHORT_CONNECTIONS
from longmix_tcp_business import (
    HEADER,MAGIC,SHORT_BYTES,SHORT_HZ,SHORT_IN_FLIGHT_CAP,OneFlow,tcp_info,short_client
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

    def test_real_slow_1200ms_requests_still_start_every_one_second(self):
        # Functional helper test: peer waits longer than the request cadence.
        # The former serial implementation deterministically skipped 1/6.
        seed=2608102
        listener=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        listener.bind(("127.0.0.1",0))
        listener.listen(8)
        listener.settimeout(6)
        received=[]
        failures=[]
        def handle(conn):
            try:
                conn.settimeout(5)
                header=conn.recv(HEADER.size,socket.MSG_WAITALL)
                magic,seq=HEADER.unpack(header)
                if magic!=MAGIC:raise ValueError("bad TCP magic")
                sec=seq-1000
                payload=conn.recv(SHORT_BYTES,socket.MSG_WAITALL)
                if payload!=bytes([(seed+sec)&255])*SHORT_BYTES:
                    raise ValueError("request payload wrong")
                time.sleep(1.25)
                conn.sendall(bytes([(seed+sec+101)&255])*SHORT_BYTES)
                received.append(sec)
            except Exception as ex:
                failures.append(str(ex))
            finally:
                conn.close()
        handlers=[]
        def accept_three():
            for _ in range(3):
                conn,_=listener.accept()
                t=threading.Thread(target=handle,args=(conn,),daemon=True)
                t.start()
                handlers.append(t)
        accept=threading.Thread(target=accept_three,daemon=True)
        accept.start()
        samples=[]
        due=time.monotonic_ns()+100_000_000
        short_client(listener.getsockname(),due,3,seed,samples)
        accept.join(timeout=6)
        for t in handlers:t.join(timeout=6)
        listener.close()
        self.assertEqual(failures,[])
        self.assertEqual(sorted(received),[0,1,2])
        self.assertEqual([row["sec"] for row in samples],[0,1,2])
        self.assertTrue(all(row["classification"]=="RETURNED" for row in samples))
        self.assertTrue(all(row["rtt_ns"]>1_000_000_000 for row in samples))
        self.assertEqual(SHORT_IN_FLIGHT_CAP,8)

    def test_full_1m_app_write_does_not_claim_it_is_one_ip_packet(self):
        b=bytes([99])*1048576
        h=hashlib.sha256()
        h.update(memoryview(b)[:512])
        h.update(memoryview(b)[512:])
        self.assertEqual(h.hexdigest(),hashlib.sha256(b).hexdigest())
        self.assertGreater(max(TCP_WRITE_SIZES),9000)

if __name__=="__main__":unittest.main(verbosity=2)
