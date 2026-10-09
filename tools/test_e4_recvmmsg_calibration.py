#!/usr/bin/env python3
"""No BPF and no benchmark: synthetic fail-closed host-stratum contract."""
import sys, unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
from e4_recvmmsg_calibration_compare import compare,valid_case,host_psi_avg10
from afpacket_recvmmsg_tracepoint import program
from e4_recvmmsg_calibration import assess_host_quality

def item(mode, cpu=1.0, p99=1000, model="AMD EPYC 9V45", sha="f"*64):
    d={"mode":mode,"status":"ONE_SYNTHETIC_CASE_COMPLETE_NOT_CALIBRATED",
       "measurement_sec":12,"target_rate_hz":4000,"product_used":False,
       "tracer_overhead_qualified":False,
       "fixture_sha256":sha,"cgroup_throttled_delta":0,
       "host_steal_ticks_delta":0,
       "host_before":{"model":model,"vcpus":4,"cgroup_cpu_max":"max 100000","cpu_pressure_first_line":"some avg10=2.00 avg60=2.00 total=1000"},
       "host_after":{"model":model,"vcpus":4,"cgroup_cpu_max":"max 100000","cpu_pressure_first_line":"some avg10=3.00 avg60=2.00 total=2000"},
       "fixture":{"target_calls":48000,"decoy_calls":48000,
                  "cpu_total_s":cpu,"recv_p99_ns":p99,"recv_p999_ns":p99+1000},
       "kernel_counters":"NOT_MEASURED_OBSERVER_OFF"}
    if mode=="on":
        d["kernel_counters"]={"entered":48000,"exited":48000,"unpaired":0}
        d["tracer"]={"exit_code":0,"maxrss_kib":200000}
    return d

class CalibrationContract(unittest.TestCase):
    def test_legacy_short_probe_cannot_run_30s_without_opt_in(self):
        with self.assertRaisesRegex(ValueError,"bounded"):
            program(123,seconds=30,ready=True)
        s=program(123,seconds=30,recv_fd=30,ready=True,calibration=True)
        self.assertIn("args.fd == 30",s)
        self.assertIn("interval:s:30",s)
    def test_only_one_per_workflow_mode(self):
        self.assertTrue(valid_case(item("off"),"off"))
        self.assertTrue(valid_case(item("on"),"on"))
        self.assertFalse(valid_case(item("on"),"off"))
    def test_one_each_is_not_calibrated(self):
        self.assertEqual(compare([item("off")],[item("on")])["status"],"NOT_CALIBRATED")
    def test_different_virtual_cpu_no_false_normalization(self):
        x=compare([item("off")]*3,[item("on",model="AMD EPYC 9V74")]*3)
        self.assertEqual(x["reason"],"NO_MATCHING_CPU_VCPU_QUOTA_FIXTURE_STRATUM")
    def test_missing_or_extra_events_cannot_be_discarded(self):
        on=item("on")
        on["kernel_counters"]["exited"]=47999
        x=compare([item("off")]*3,[on]*3)
        self.assertEqual(x["reason"],"RED_OR_UNKNOWN_SAMPLES_CANNOT_BE_DISCARDED")
    def test_host_steal_and_throttling_unknown_ineligible(self):
        x=item("off");x["cgroup_throttled_delta"]=None
        self.assertFalse(valid_case(x,"off"))
        x=item("off");x["host_steal_ticks_delta"]=1
        self.assertFalse(valid_case(x,"off"))
    def test_missing_or_high_cpu_psi_never_qualifies(self):
        x=item("on")
        x["host_before"]["cpu_pressure_first_line"]=None
        self.assertFalse(valid_case(x,"on"))
        x=item("on")
        x["host_after"]["cpu_pressure_first_line"]="some avg10=18.3 avg60=9.0 total=250"
        self.assertFalse(valid_case(x,"on"))
        self.assertEqual(host_psi_avg10(x["host_after"]),18.3)
    def test_midrun_host_stratum_change_rejected(self):
        x=item("off")
        x["host_after"]["cgroup_cpu_max"]="200000 100000"
        self.assertFalse(valid_case(x,"off"))
    def test_three_each_on_same_host_stratum_needed(self):
        x=compare([item("off")]*3,[item("on")]*2)
        self.assertEqual(x["reason"],"AT_LEAST_THREE_INDEPENDENT_ACTIONS_EACH_MODE_REQUIRED")
    def test_synthetic_only_budget_can_hold_but_never_product_pass(self):
        x=compare([item("off",1.0,1000)]*3,[item("on",1.03,1050)]*3)
        self.assertEqual(x["status"],"SYNTHETIC_SCOPE_ONLY_WITHIN_CONSERVATIVE_BUDGET")
        self.assertEqual(x["product_qualification"],"NOT_RUN")
    def test_budget_breach_does_not_pass(self):
        x=compare([item("off")]*3,[item("on",1.10,1150)]*3)
        self.assertEqual(x["status"],"SYNTHETIC_OVERHEAD_BUDGET_NOT_PROVEN")
    def test_more_than_one_host_stratum_never_mixes(self):
        off=[item("off")]*3+[item("off",model="EPYC 9V74")]*3
        on=[item("on")]*3+[item("on",model="EPYC 9V74")]*3
        self.assertEqual(compare(off,on)["reason"],"ONLY_ONE_EXACT_STRATUM_SUPPORTED")
    def test_single_case_host_quality_gate_is_independent_of_workload_success(self):
        x=item("off")
        x["host_before"]["cpu_pressure_first_line"]="some avg10=17.06 total=100"
        x["host_after"]["cpu_pressure_first_line"]="some avg10=5.14 total=200"
        x["host_before"]["cgroup_cpu_max"]=None
        x["host_after"]["cgroup_cpu_max"]=None
        quality=assess_host_quality(x)
        self.assertFalse(quality["eligible"])
        self.assertIn("CPU_PSI_UNKNOWN_OR_OVER_10_PCT_BEFORE",quality["reasons"])
        self.assertIn("CPU_QUOTA_UNKNOWN_BEFORE",quality["reasons"])
    def test_truly_known_stable_host_quality(self):
        x=item("off")
        quality=assess_host_quality(x)
        self.assertTrue(quality["eligible"])
        self.assertEqual(quality["reasons"],[])
    def test_no_implicit_total_cpu_gain(self):
        x=compare([item("off")]*3,[item("on")]*3)
        self.assertEqual(x["product_cpu_gain"],"UNPROVEN")

if __name__=="__main__":unittest.main()
