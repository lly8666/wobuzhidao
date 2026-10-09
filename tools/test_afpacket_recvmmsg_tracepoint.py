#!/usr/bin/env python3
"""Pure syntax/guard tests: never touch BPF, product, network or process."""
import sys, unittest
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
    def test_no_packet_or_per_event_printf(self):
        s=program(731)
        for secret in ("args.","comm","str(","sys_enter_sendmsg"):
            self.assertNotIn(secret,s)
        self.assertEqual(s.count("printf("),1)
    def test_invalid_pid_and_duration_rejected(self):
        for pid in (0,-1,2147483648,1.4,"123"):
            with self.assertRaises(ValueError): program(pid)
        for seconds in (0,1,3,6,300):
            with self.assertRaises(ValueError): program(123,seconds)
    def test_five_second_functional_has_attach_ready_handshake(self):
        source=program(927,seconds=5,ready=True)
        self.assertIn("WBD_E4_RECVMMSG_FIXTURE_TRACER_READY",source)
        self.assertIn("interval:s:5",source)
    def test_cap_is_bounded(self):
        self.assertLessEqual(MAX_BPF_MAP_KEYS,4096)
    def test_exact_target_fd_only_on_enter_then_thread_exit_pairing(self):
        s=program(901,seconds=5,ready=True,recv_fd=30)
        self.assertIn("sys_enter_recvmmsg /pid == 901 && args.fd == 30/",s)
        self.assertIn("sys_exit_recvmmsg /pid == 901 && @entered_ns[tid] != 0/",s)
        self.assertEqual(s.count("args.fd"),1)
        self.assertIn("@previous_exit[tid]",s)
        self.assertNotIn("@previous_exit[pid]",s)
        self.assertIn("@enters = count()",s)
        self.assertIn("@exits = count()",s)
        self.assertIn("@unpaired_enter = count()",s)
    def test_target_fd_is_bounded_numeric_and_optional(self):
        for fd in (-1,4096,1.5,"30",True):
            with self.assertRaises(ValueError): program(901,recv_fd=fd)
        self.assertIn("args.fd == 0",program(901,recv_fd=0))
        self.assertNotIn("args.fd",program(901,recv_fd=None))
    def test_no_user_payload_or_cross_thread_gap_claim(self):
        s=program(901,recv_fd=31)
        self.assertNotIn("args.msg",s)
        self.assertNotIn("args.vlen",s)
        self.assertNotIn("printf(\"%d",s)
        self.assertIn("if (@previous_exit[tid] != 0)",s)
        self.assertIn("nsecs - @previous_exit[tid]",s)
        self.assertIn("@previous_exit[tid] = nsecs",s)
        self.assertNotIn("@previous_exit[pid]",s)

if __name__=="__main__": unittest.main()
