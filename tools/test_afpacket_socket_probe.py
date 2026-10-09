#!/usr/bin/env python3
"""Pure synthetic opt-in socket probe tests, Actions only."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_socket_probe import numeric_row

class TestPacketProbe(unittest.TestCase):
    def test_numeric_and_no_address(self):
        raw=("Netid State Recv-Q Send-Q Local Address:Port Peer Address:PortProcess\n"
             "p_raw UNCONN 384 0 [2048]:swan *\n"
             " skmem:(r384,rb1048576,t0,tb212992,f0,w0,o0,bl0,d86)\n")
        r=numeric_row("server",100,113,raw,0,"some avg10=6.12 avg60=5.0 total=13\n")
        self.assertEqual(r["packet_socket"],{"r":384,"rb":1048576,"d":86})
        self.assertEqual(r["ss_duration_ns"],13)
        self.assertEqual(r["cpu_psi_some_avg10_percent"],6.12)
        self.assertTrue(r["ss_ok"])
        self.assertNotIn("swan",str(r))
    def test_unavailable_not_zero(self):
        r=numeric_row("client",100,101,"",-1,"")
        self.assertIsNone(r["packet_socket"])
        self.assertFalse(r["ss_ok"])
    def test_multiple_socket_fails(self):
        body="p_raw UNCONN 0 0 [2048]:other *\n skmem:(r0,rb1000,d1)\n"
        with self.assertRaisesRegex(ValueError,"multiple packet sockets"):
            numeric_row("client",1,2,body+body,0,"")
if __name__=="__main__":
    unittest.main()
