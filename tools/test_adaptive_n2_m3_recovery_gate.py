import json
import unittest
from adaptive_n2_m3_recovery_gate import score, GATE_VERSION

def fixture():
    sha="single-source"
    freeze={"schema":1,"model_version":"n2-m3-iid-recover-high20-to5-v1",
            "scenario":"M3-R1","duration_active_s":330,
            "ordinary_recovery_independent_B_upper_s":140.4}
    cfg={"v3_client_fec_mode":"auto","v3_client_fec_min_parity":4,
         "server_config_fec_parity":20,"duration_s":330,"mtu":1400,
         "one_way_delay_ms_each_direction":300,
         "application_mbps_each_direction":10.0,"loss_percent":"waveform:20-5"}
    summary={"source_sha":sha,"errors":[],"correctness_errors":[],
             "capacity_flags":[],"m3_loss20_grade_screen_pass":True,
             "m3_final_recovery_grade_screen_pass":True,
             "waveform":[
                {"name":"loss20","start_s":0,"end_s":120,
                 "c2s":{"packets_counter_delta":8000,"drop_counter_delta":2000},
                 "s2c":{"packets_counter_delta":8000,"drop_counter_delta":2000}},
                {"name":"recover5","start_s":120,"end_s":330,
                 "c2s":{"packets_counter_delta":9500,"drop_counter_delta":500},
                 "s2c":{"packets_counter_delta":9500,"drop_counter_delta":500}}],
             "m3_stage_last60_grade":{
               "loss20":{"c2s":{"status":"OBSERVED","snapshots":60,"actual_r":[16,20],"fraction_within":1},
                         "s2c":{"status":"OBSERVED","snapshots":60,"actual_r":[16,20],"fraction_within":1}},
               "recover5":{"c2s":{"status":"OBSERVED","snapshots":60,"actual_r":[8,10],"fraction_within":1},
                           "s2c":{"status":"OBSERVED","snapshots":60,"actual_r":[8,10],"fraction_within":1}}}}
    def stats():
        return {"sent_packets_by_second":[100]*330,
                "recv_wall_packets_by_second":[100]*335,
                "sent_packets":33000,"recv_unique_packets":33000,
                "source_send_failures":0,"send_failures":0,"corrupt":0,
                "recv_duplicates":0,"oneway_p99_ns":314000000}
    return summary,{"source_sha":sha,"harness_sha":sha,"config":cfg},{"stats":stats()},{"stats":stats()},freeze,sha

class M3FrozenRecoveryGate(unittest.TestCase):
    def test_predeclared_healthy_synthetic_pass_only_scoped(self):
        s,m,b,t,f,sha=fixture()
        t["stats"]["recv_wall_packets_by_second"][:120]=[75]*120
        r=score(s,m,b,t,f,sha)
        self.assertEqual(r["classification"],"SCOPED_M3_FROZEN_PROTECTION_RECOVERY_FIRST_UNIQUE_PASS")
        self.assertTrue(r["not_full_n2"])
    def test_legacy_recover5_high_grade_fails(self):
        s,m,b,t,f,sha=fixture()
        s["m3_stage_last60_grade"]["recover5"]["c2s"]["fraction_within"]=0
        r=score(s,m,b,t,f,sha)
        self.assertIn("GRADE_FROZEN_LAST60_recover5_c2s",r["errors"])
    def test_nonlinear_source_loss_not_forced_to_zero_in_20pct(self):
        s,m,b,t,f,sha=fixture()
        t["stats"]["recv_wall_packets_by_second"][:120]=[65]*120
        r=score(s,m,b,t,f,sha)
        self.assertFalse(r["errors"])
    def test_recovery_legacy_blackout_fails(self):
        s,m,b,t,f,sha=fixture()
        t["stats"]["recv_wall_packets_by_second"][220:266]=[0]*46
        r=score(s,m,b,t,f,sha)
        self.assertIn("RECOVERY_FIRST_UNIQUE_BLACKOUT_c2s",r["errors"])
    def test_true_qdisc_required_in_each_stage(self):
        s,m,b,t,f,sha=fixture()
        s["waveform"][1]["s2c"]["drop_counter_delta"]=0
        r=score(s,m,b,t,f,sha)
        self.assertIn("ACTUAL_QDISC_LOSS_OUTSIDE_PREDECLARED_recover5_s2c",r["errors"])
    def test_wrong_sha_and_model_are_not_saved_by_business(self):
        s,m,b,t,f,sha=fixture()
        f["ordinary_recovery_independent_B_upper_s"]=140.4
        r=score(s,m,b,t,f,sha+"-changed")
        self.assertIn("SOURCE_SHA_IDENTITY_MISMATCH",r["errors"])
        f["model_version"]="invented"
        self.assertIn("FROZEN_MODEL_NOT_M3_R1",score(s,m,b,t,f,sha)["errors"])

if __name__=="__main__":
    unittest.main()
