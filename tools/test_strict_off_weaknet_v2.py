#!/usr/bin/env python3
"""Pure offline regression vectors; executed only under GitHub Actions."""
import copy
import json
import math
import pathlib
import sys
import unittest

sys.path.insert(0,str(pathlib.Path(__file__).resolve().parent))
from check_strict_off_weaknet_v2 import (
    VERSION, eligible, assess, fragmentation_reference, _raw_stage_measurements,
    _host_identity, _host_pressure,
)


def fixture():
    cfg={"fec":"off","fec_parity":0,"profile_screen":True,"mtu":1400,
         "application_mbps_each_direction":10,"one_way_delay_ms":300,
         "stages_s":[30,60,30]}
    manifest={"schema":1,"source_sha":"a"*40,"harness_sha":"h"*40,
              "mode":"normal","scenario":"5205","seed":601,
              "runner":"ubuntu-24.04","config":cfg}
    measurements={}
    for direction in ("c2s","s2c"):
        measurements[direction]={}
        for stage in ("pre","stress","post"):
            offered=200 if stage!="stress" else 400
            received=180 if stage!="stress" else 260
            measurements[direction][stage]={
                "sent_packets":offered,"unique_delivered_packets":received,
                "sent_bytes":offered*200,"unique_delivered_bytes":received*200,
                "packet_loss_percent":100*(offered-received)/offered,
                "byte_loss_percent":100*(offered-received)/offered,
                "wall_delivered_mbps_path_delay_aligned":9.0 if stage!="stress" else 6.5,
                "eventual_goodput_mbps":9.0 if stage!="stress" else 6.5,
            }
    legacy={"analysis_version":"fec-profile-screen-v1",
            "source_sha":manifest["source_sha"],"scenario":"5205","mode":"normal",
            "generator":measurements,"profile_errors":[],
            "longest_zero_delivery_seconds":{"biz":0,"target":0},
            "classifications":{"CORRECTNESS":"PASS","CAPTURE":"PASS","ENVIRONMENT":"PASS",
                               "INPUT_VALIDITY":"PASS","PERFORMANCE":"FAIL"},
            "errors":{"correctness":[],"input":[],"capture":[],"environment":[],
                      "performance":["c2s/stress packet_loss=35 > 20",
                                     "c2s/stress delay_aligned_wall_goodput=6.5 < 7.92",
                                     "post5 did not reach 3 consecutive qualifying 1s windows within 10s"]},
            "latency":{"probe_stage":{"pre":{"sent":30,"received":25,"p95_ns":100,"p99_ns":120},
                                     "stress":{"sent":60,"received":43,"p95_ns":200,"p99_ns":250},
                                     "post":{"sent":30,"received":26,"p95_ns":100,"p99_ns":120}}},
            "resource":{"process_cpu_seconds":{"client":10,"server":11},
                        "socket_drop_max":0,"link_drop_delta":{}},
            "cost":{"c2s":{"outer_ip_per_unique_delivered":1.8}},
            "drain":{"late_after_10s_bytes":0},
    }
    def app():
        # 120s fixed timelines, a stable pre/post and a damaged 60s stress.
        return {"generator_process_cpu_seconds":1.5,"stats":{
            "sent_packets_by_second":[100]*120,
            "recv_packets_by_second":[91]*30+[69]*60+[91]*30,
            "recv_wall_bytes_by_second":[1125000]*30+[820000]*60+[1125000]*30,
            "probe_sent":120,"probe_timeouts":18,
            "probe_rtt_p50_ns":600_000_000,
            "probe_rtt_p95_ns":610_000_000,
            "probe_rtt_p99_ns":950_000_000,
        }}
    return legacy,manifest,app(),app()


class OffWeaknetV2Test(unittest.TestCase):
    def test_eligible_scope_does_not_touch_lossless_nonoff_or_formal20(self):
        self.assertTrue(eligible("fec-profile-screen",0,"5205"))
        self.assertTrue(eligible("fec-profile-screen",0,"5305"))
        for kind,parity,scenario in [
            ("fec-profile-screen",0,"lossless"),("fec-profile-screen",20,"5205"),
            ("formal20",0,"5205"),("formal20",20,"5205"),
            ("fec-profile-screen",4,"5205"),
        ]:
            self.assertFalse(eligible(kind,parity,scenario))
        l,m,b,t=fixture()
        m["config"]["profile_screen"]=False
        with self.assertRaises(ValueError):
            assess(l,m,b,t)

    def test_valid_fragment_loss_and_missing_baseline_never_automatically_pass(self):
        l,m,b,t=fixture()
        r=assess(l,m,b,t)
        self.assertEqual(r["policy_version"],VERSION)
        self.assertEqual(r["qualification_status"],"MEASURED_NOT_QUALIFIED")
        self.assertEqual(r["legacy_reference"]["classifications"]["PERFORMANCE"],"FAIL")
        self.assertEqual(r["legacy_reference"]["errors"]["performance"],l["errors"]["performance"])
        self.assertEqual(r["fragmentation_reference"]["state"],"UNAVAILABLE")
        self.assertEqual(r["comparison"]["state"],"UNAVAILABLE")
        self.assertEqual(r["continuity"]["severe_stall_roles"],[])
        self.assertEqual(r["relative_pre5_recovery"]["c2s"]["state"],"RECOVERED_IN_INITIAL_10S")
        self.assertEqual(r["latency"]["total_probe_timeouts"],18)
        self.assertIsNone(r["measurements"]["c2s"]["stress"]["stage_probe_returned_only_p50_ns"])

    def test_corruption_and_effective_config_never_hidden_by_legacy_perf_relaxation(self):
        l,m,b,t=fixture()
        l["classifications"]["CORRECTNESS"]="FAIL"
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"FAIL_HARD_GATE")
        l,m,b,t=fixture()
        l["profile_errors"]=["negotiated parity mismatch"]
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"FAIL_HARD_GATE")

    def test_injection_input_invalid_and_capacity_not_measured_qualified(self):
        l,m,b,t=fixture()
        l["classifications"]["INPUT_VALIDITY"]="FAIL"
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"INVALID_INPUT")
        l,m,b,t=fixture()
        l["classifications"]["ENVIRONMENT"]="FAIL"
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"CAPACITY_LIMITED")
        l,m,b,t=fixture()
        l["resource"]["socket_drop_max"]=120
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"CAPACITY_LIMITED")

    def test_two_second_business_stall_remains_absolute_hard_failure(self):
        l,m,b,t=fixture()
        l["longest_zero_delivery_seconds"]["target"]=2
        self.assertEqual(assess(l,m,b,t)["qualification_status"],"FAIL_HARD_GATE")
        l,m,b,t=fixture()
        # Explicit slowdown in post5 cannot be excused by removing 1-p gate.
        t["stats"]["recv_wall_bytes_by_second"][90:]=[0]*30
        t["stats"]["recv_packets_by_second"][90:]=[0]*30
        res=assess(l,m,b,t)
        self.assertEqual(res["qualification_status"],"REVIEW_REQUIRED")
        self.assertEqual(res["relative_pre5_recovery"]["c2s"]["state"],"NOT_CONFIRMED_WITHIN_OBSERVATION")

    def test_mixed_fragment_q_weights_by_packet_and_byte_not_average_m(self):
        l,m,b,t=fixture()
        mapping={"schema":1,"source_sha":m["source_sha"],"harness_sha":m["harness_sha"],
                 "evidence":{"kind":"authenticated_link_counts"},
                 "size_distribution":{}}
        for direction in ("c2s","s2c"):
            mapping["size_distribution"][direction]={}
            for stage in ("pre","stress","post"):
                observed=l["generator"][direction][stage]
                n=observed["sent_packets"]
                mapping["size_distribution"][direction][stage]=[
                    {"business_packets":n//2,"business_bytes_per_packet":100,"link_fragments_per_business_packet":1},
                    {"business_packets":n//2,"business_bytes_per_packet":300,"link_fragments_per_business_packet":2},
                ]
        ref=fragmentation_reference(mapping,m,_raw_stage_measurements(l))
        self.assertEqual(ref["state"],"REFERENCE_ONLY")
        stress=ref["by_direction_stage"]["c2s"]["stress"]
        self.assertAlmostEqual(stress["iid_no_repair_packet_loss_reference_percent"],28.0)
        self.assertAlmostEqual(stress["iid_no_repair_byte_loss_reference_percent"],32.0)
        mapping["size_distribution"]["c2s"]["stress"][1]["link_fragments_per_business_packet"]=0
        self.assertEqual(fragmentation_reference(mapping,m,_raw_stage_measurements(l))["state"],"INVALID_MAP")
        mapping["size_distribution"]["c2s"]["stress"][1]["link_fragments_per_business_packet"]=2
        mapping["size_distribution"]["c2s"]["stress"][0]["business_packets"]=1
        self.assertEqual(fragmentation_reference(mapping,m,_raw_stage_measurements(l))["state"],"INVALID_MAP")

    def test_incomparable_baseline_cannot_relabel_as_pass(self):
        l,m,b,t=fixture()
        current=assess(l,m,b,t)
        baseline=copy.deepcopy(current)
        baseline["product_source_sha"]="b"*40
        baseline["harness_sha"]="different"
        result=assess(l,m,b,t,baseline_receipts=[baseline])
        self.assertEqual(result["comparison"]["state"],"INCOMPARABLE")
        self.assertNotIn("PASS",result["qualification_status"])
        baseline["harness_sha"]=current["harness_sha"]
        baseline["resource"]["cpu_seconds_per_delivered_gib"]*=0.5
        hw={"model":"synthetic EPYC","logical_cpus":4,"instructions":["aes","avx2"],"cpu_max":"max 100000"}
        baseline["runner_hardware"]=hw
        result=assess(l,m,b,t,baseline_receipts=[baseline],runner_hardware=hw)
        self.assertEqual(result["qualification_status"],"REVIEW_REQUIRED")
        self.assertIn("CPU/GiB",str(result["comparison"]["review_flags"]))

    def test_cross_cpu_runner_comparison_is_incomparable(self):
        l,m,b,t=fixture()
        host='Model name: AMD EPYC 9V45\nCPU(s): 4\nFlags: aes avx2 vaes\n'
        rows=[{'processes':{'client':{'cgroup':{'cpu.max':'max 100000\n'}}},
               'pressure':{'cpu':'some avg10=5.50 avg60=3.0'},
               'proc_stat':{'cpu':[1,2,3,4,5,6,7,8,9,10]}},
              {'pressure':{'cpu':'some avg10=9.00 avg60=4.0'},
               'proc_stat':{'cpu':[9,10,12,14,16,18,20,24,26,30]}}]
        hw=_host_identity(host,rows)
        self.assertEqual(hw['logical_cpus'],4)
        self.assertEqual(hw['cpu_max'],'max 100000')
        self.assertEqual(_host_pressure(rows)['cpu_psi_some_avg10_peak_percent'],9.0)
        self.assertGreater(_host_pressure(rows)['host_cpu_steal_delta_percent'],0)
        curr=assess(l,m,b,t,runner_hardware=hw)
        old=copy.deepcopy(curr)
        old['product_source_sha']='b'*40
        old['runner_hardware']['model']='AMD EPYC 7763'
        check=assess(l,m,b,t,baseline_receipts=[old],runner_hardware=hw)
        self.assertEqual(check['comparison']['state'],'INCOMPARABLE')
        self.assertIsNone(_host_identity(host,[]))

if __name__ == "__main__":
    unittest.main()
