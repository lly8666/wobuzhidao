#!/usr/bin/env python3
"""Fail-closed synthetic map parser contract for local recvmmsg fixtures."""
import sys, unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_recvmmsg_functional import result

GOOD="@enters: 2\n@exits: 2\n@long_inter_call_gap_bucket[123456789]: 1\n"
class FunctionalResultTest(unittest.TestCase):
    def test_exactly_two_calls_one_gap_only(self):
        self.assertEqual(result(GOOD,0),"TWO_SYSCALLS_ONE_GAP_FUNCTIONAL_ONLY")
    def test_four_real_calls_two_target_fd(self):
        self.assertEqual(result(GOOD,0,two_fd=True),"TWO_FD_TARGET_ONLY_FUNCTIONAL")
    def test_no_gap_is_not_pass(self):
        self.assertEqual(result("@enters: 2\n@exits: 2\n",0),"INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_no_calls_never_zero(self):
        self.assertEqual(result("",0),"INCONCLUSIVE_MISSING_COUNTS")
    def test_extra_non_target_counts_are_not_pass(self):
        s="@enters: 4\n@exits: 4\n@long_inter_call_gap_bucket[1]: 1\n"
        self.assertEqual(result(s,0,two_fd=True),"INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_unmatched_enter_exit_is_not_pass(self):
        s="@enters: 2\n@exits: 1\n@long_inter_call_gap_bucket[1]: 1\n"
        self.assertEqual(result(s,0,two_fd=True),"INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_unpaired_enter_marks_bad_sample(self):
        self.assertEqual(result(GOOD+"@unpaired_enter: 1\n",0,two_fd=True),
                         "INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_exact_256_target_events_with_256_other_fd_calls(self):
        expected="@enters: 256\n@exits: 256\n"
        self.assertEqual(
            result(expected,0,two_fd=True,expected_target_calls=256),
            "TARGET_256_EVENTS_MATCHED_FUNCTIONAL_ONLY")
    def test_missing_just_one_of_256_fails_not_pass(self):
        self.assertEqual(
            result("@enters: 256\n@exits: 255\n",0,two_fd=True,expected_target_calls=256),
            "INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_512_calls_on_same_tgid_would_fail_fd_filter(self):
        self.assertEqual(
            result("@enters: 512\n@exits: 512\n",0,two_fd=True,expected_target_calls=256),
            "INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")

    def test_failed_fixture_never_passes(self):
        self.assertEqual(result(GOOD,14,two_fd=True),"INCONCLUSIVE_MISSING_COUNTS")
    def test_malformed_multiple_counters_not_pass(self):
        self.assertEqual(result(GOOD+"@exits: 2\n",0,two_fd=True),"INCONCLUSIVE_MISSING_COUNTS")

if __name__=="__main__":unittest.main()
