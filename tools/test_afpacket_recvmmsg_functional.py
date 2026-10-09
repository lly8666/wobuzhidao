#!/usr/bin/env python3
"""Pure parser tests for two synthetic recvmmsg call exits and one gap."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_recvmmsg_functional import result
class FunctionalResultTest(unittest.TestCase):
    def test_exactly_two_calls_one_gap_only(self):
        s="@exits: 2\n@long_inter_call_gap_bucket[123456789]: 1\n"
        self.assertEqual(result(s,0),"TWO_SYSCALLS_ONE_GAP_FUNCTIONAL_ONLY")
    def test_no_gap_is_not_pass(self):
        self.assertEqual(result("@exits: 2\n",0),"INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_no_calls_never_zero(self):
        self.assertEqual(result("",0),"INCONCLUSIVE_MISSING_EXITS")
    def test_extra_counts_are_not_pass(self):
        self.assertEqual(result("@exits: 3\n@long_inter_call_gap_bucket[1]: 1\n",0),
                         "INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE")
    def test_failed_fixture_not_pass(self):
        self.assertEqual(result("@exits: 2\n@long_inter_call_gap_bucket[2]: 1\n",14),
                         "INCONCLUSIVE_MISSING_EXITS")
if __name__=="__main__":
    unittest.main()
