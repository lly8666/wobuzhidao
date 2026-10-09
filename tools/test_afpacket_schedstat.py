#!/usr/bin/env python3
"""Pure deterministic tests: no privileged procfs or performance sample."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_schedstat import parse_starttime,parse_schedstat,aggregate

def rec(at,birth,threads,status="OK"):
    return {"at_ns":at,"birth":birth,"threads":threads,"status":status}

class SchedstatContract(unittest.TestCase):
    def test_linux_comm_field22(self):
        fields=["S"]+["0"]*18+["12345"]+["0"]*8
        self.assertEqual(parse_starttime("71 (wbd raw (loop)) "+" ".join(fields)),12345)
    def test_basic_schedstat(self):
        self.assertEqual(parse_schedstat("123 456 7\n"),(123,456,7))
    def test_matched_deltas_only(self):
        a=rec(10,99,{(1,2):(100,4,1),(3,4):(200,5,2)})
        b=rec(20,99,{(1,2):(300,10,2),(3,4):(250,12,3)})
        x=aggregate(a,b)
        self.assertEqual(x["status"],"MATCHED_COMPLETE")
        self.assertEqual(x["runqueue_wait_ns_delta"],13)
        self.assertEqual(x["runtime_ns_delta"],250)
        self.assertNotIn("threads",x)
    def test_thread_reuse_ignored(self):
        a=rec(1,99,{(2,3):(1000,900,9),(4,1):(10,20,1)})
        b=rec(2,99,{(2,7):(1,0,1),(4,1):(20,30,2)})
        x=aggregate(a,b)
        self.assertEqual(x["status"],"MATCHED_PARTIAL")
        self.assertEqual(x["runqueue_wait_ns_delta"],10)
        self.assertEqual(x["new_threads"],1)
    def test_process_change_or_missing_never_zero(self):
        a=rec(1,98,{(3,1):(1,1,1)})
        b=rec(2,99,{(3,1):(2,2,2)})
        self.assertEqual(aggregate(a,b)["status"],"IDENTITY_UNKNOWN")
        self.assertIsNone(aggregate(a,b)["runqueue_wait_ns_delta"])
        self.assertEqual(aggregate(None,b)["status"],"BASELINE_ONLY")
    def test_counter_reset_drops_attribution(self):
        a=rec(1,99,{(3,1):(10,5,1)})
        b=rec(2,99,{(3,1):(11,1,2)})
        self.assertEqual(aggregate(a,b)["status"],"COUNTER_RESET")
        self.assertIsNone(aggregate(a,b)["runqueue_wait_ns_delta"])
    def test_proc_format_invalid(self):
        for t in ("foo","1 (name) R 2","1 (name) "+" ".join(["S"]+["n"]*23)):
            with self.assertRaises(ValueError):parse_starttime(t)
        with self.assertRaises(ValueError):parse_schedstat("1 2")
if __name__=="__main__":unittest.main()
