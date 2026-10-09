#!/usr/bin/env python3
"""Pure static checks. No privileged access, product or performance sample."""
import sys
import unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_recvmmsg_tracepoint import program,MARKER,MAX_BPF_MAP_KEYS

class TraceDesign(unittest.TestCase):
    def test_two_pid_filtered_tracepoints(self):
        s=program(12718)
        self.assertIn("sys_enter_recvmmsg /pid == 12718/",s)
        self.assertIn("sys_exit_recvmmsg /pid == 12718 &&",s)
        self.assertEqual(s.count("tracepoint:syscalls:"),2)
    def test_strict_two_second_preflight(self):
        s=program(731)
        self.assertIn("interval:s:2",s)
        self.assertIn(MARKER,s)
    def test_bounded_latency_counts_in_100ms_bins(self):
        s=program(731)
        self.assertEqual(s.count(">= 20000000"),2)
        self.assertEqual(s.count("nsecs / 100000000"),2)
    def test_no_packet_fd_or_per_event_printf(self):
        s=program(731)
        for secret in ("args.","comm","str(","sys_enter_sendmsg"):
            self.assertNotIn(secret,s)
        self.assertEqual(s.count("printf("),1)
    def test_invalid_pid_and_duration_rejected(self):
        for pid in (0,-1,2147483648,1.4,"123"):
            with self.assertRaises(ValueError): program(pid)
        for seconds in (0,1,3,300):
            with self.assertRaises(ValueError): program(123,seconds)
    def test_cap_is_bounded(self):
        self.assertLessEqual(MAX_BPF_MAP_KEYS,4096)

if __name__=="__main__":unittest.main()
