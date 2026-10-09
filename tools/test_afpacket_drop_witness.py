#!/usr/bin/env python3
"""Small, independent no-network contract tests for E4 time/causality guards."""
import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from afpacket_drop_witness import correlate,clock_offset,diagnostic_buckets


def resources():
    out=[]
    for t in range(1,8):
        ns=t*1_000_000_000
        cg={"path":"/demo","cpu.stat":"usage_usec 100\nnr_throttled %d\n"%(t//3)}
        out.append({"monotonic_ns":ns,"unix_ns":ns+1_800_000_000_000_000_000,
                    "processes":{"client":{"cgroup":cg},"server":{"cgroup":cg}}})
    return out

def trace(side, drops=(0,0,0), good=True):
    return [{"schema":1,"side":side,"ss_start_monotonic_ns":1_000_000_000+i*100_000_000,
             "ss_end_monotonic_ns":1_002_000_000+i*100_000_000,
             "ss_ok":good,
             "packet_socket":{"r":0,"rb":1048576,"d":v} if good else None,
             "cpu_psi_some_avg10_percent":6.12} for i,v in enumerate(drops)]

def diags(side):
    out=[]
    for t in range(1,7):
        product={"raw_io":{"receive_calls":t*30,"receive_messages":t*40}}
        if side=="server":
            product["tunnel"]={"server_pipeline":{"enabled":True,"read_gap":{
                "over_10ms":t*2,"max_ns":t*50_000_000}}}
        out.append({"observed_unix_ns":t*1_000_000_000+1_800_000_000_000_000_000,
                    "product":product})
    return out

def stage():
    return [{"event":"business_start","monotonic_ns":1_000_000_000},
            {"event":"business_end","monotonic_ns":6_000_000_000}]

class SameRunWitness(unittest.TestCase):
    def test_wall_to_monotonic_calibration_correct(self):
        offset,drift=clock_offset(resources())
        self.assertEqual(offset,-1_800_000_000_000_000_000)
        self.assertEqual(drift,0)

    def test_drop_correlates_one_second_buckets_but_never_claims_cause(self):
        probes={"client":trace("client",(0,0,33)),"server":trace("server",(0,86,86))}
        ds={side:diags(side) for side in ("client","server")}
        result=correlate(probes,ds,resources(),stage())
        w=result["sides"]["server"]["socket_drop_witnesses"][0]
        self.assertEqual(w["drop_increment"],86)
        self.assertEqual(w["raw_receive_calls_in_overlapping_buckets"],30)
        self.assertEqual(w["server_read_gap_over10ms_bucket_delta"],2)
        self.assertEqual(w["business_elapsed_bound_s"],[0,0.102])
        self.assertEqual(w["verdict"],"TEMPORAL_COINCIDENCE_ONLY")
        self.assertEqual(result["causal_root"],"NOT_ESTABLISHED")
        self.assertIsNone(result["sides"]["client"]["socket_drop_witnesses"][0]["server_read_gap_over10ms_bucket_delta"])

    def test_invalid_trace_does_not_become_zero_drop(self):
        probes={side:trace(side,good=False) for side in ("client","server")}
        res=correlate(probes,{side:diags(side) for side in ("client","server")},resources(),stage())
        self.assertEqual(res["sides"]["client"]["interpretation"],"NO_VALID_SOCKET_TRACE")
        self.assertIsNone(res["sides"]["client"]["numerical_positive_drop_lower_bound"])

    def test_no_drop_is_scoped_not_a_fix(self):
        res=correlate({side:trace(side) for side in ("client","server")},
                      {side:diags(side) for side in ("client","server")},resources(),stage())
        self.assertEqual(res["sides"]["server"]["interpretation"],"NO_REPRODUCED_SOCKET_DROP_NOT_ROOT_CLOSED")

    def test_clock_drift_invalid(self):
        r=resources();r[2]["unix_ns"]+=50_000_000
        with self.assertRaisesRegex(ValueError,"drift"):
            clock_offset(r)

    def test_missing_runtime_and_counter_reset_invalid(self):
        ds=diags("server")
        ds[1]["product"]["raw_io"]["receive_calls"]=-1
        with self.assertRaisesRegex(ValueError,"counter reset"):
            diagnostic_buckets(ds,"server",-1_800_000_000_000_000_000)

    def test_sparse_context_does_not_claim_scheduling(self):
        # Two readings within a second; no full 1s read bucket crossing them.
        a=diags("server")[:1]
        report=correlate({side:trace(side,(0,2,2)) for side in ("client","server")},
                         {"client":diags("client")[:1],"server":a},resources(),stage())
        w=report["sides"]["server"]["socket_drop_witnesses"][0]
        self.assertIsNone(w["raw_receive_calls_in_overlapping_buckets"])
        self.assertEqual(w["verdict"],"INSUFFICIENT_TIME_CONTEXT")

if __name__=="__main__": unittest.main()
