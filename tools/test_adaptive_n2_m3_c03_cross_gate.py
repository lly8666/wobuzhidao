import unittest
from adaptive_n2_m3_c03_cross_gate import score, VERSION

def fixture():
    sha="exact-model-source"
    model={"model_version":"n2-independent-iid-timeband-v1",
           "scenarios":[{"id":"C03","stages":[{"name":"recover5",
             "loss_percent":5,"duration_s":120,
             "frozen_ideal_parity":8,"frozen_grade_screen":[8,10],
             "must_settle":False}]}]}
    mani={"source_sha":sha,"config":{"v3_client_fec_mode":"auto",
          "v3_client_fec_min_parity":4,"fec_parity":20,"duration_s":780,
          "one_way_delay_ms_each_direction":300,"application_mbps_each_direction":10.0,
          "loss_percent":"waveform:0-1-5-20-5-0"}}
    prep={"c03_frozen":{"model_version":"n2-independent-iid-timeband-v1"}}
    summary={"source_sha":sha,"errors":[],"capacity_flags":[],
             "correctness_errors":[],"c03_loss20_grade_screen_pass":True,
             "c03_final_recovery_grade_screen_pass":True,
             "c03_stage_last60_grade":{"recover5":{
               "c2s":{"status":"OBSERVED","snapshots":60,"actual_r":[8,10],"fraction_within":1.0},
               "s2c":{"status":"OBSERVED","snapshots":60,"actual_r":[8],"fraction_within":1.0}}}}
    return summary,mani,prep,model,sha

class C03CrossGrade(unittest.TestCase):
    def test_scoped_true_not_full_N2(self):
        args=fixture()
        r=score(*args)
        self.assertEqual(r["classification"],"C03_RECOVER5_SCOPED_GRADE_SCREEN_PASS")
        self.assertTrue(r["not_full_n2"])
        self.assertTrue(r["B_upper_plus_60_exceeds_stage_length"])

    def test_old_upstream_no_r8_is_recorded_not_reclassified_ideal(self):
        s,m,p,ref,sha=fixture()
        s["c03_stage_last60_grade"]["recover5"]["c2s"]["fraction_within"]=0.0
        s["c03_stage_last60_grade"]["recover5"]["c2s"]["actual_r"]=[12,16,20]
        r=score(s,m,p,ref,sha)
        self.assertIn("c2s_RECOVER5_GRADE_SCREEN_UNMET",r["efficiency_screen_limitations"])
        self.assertFalse(r["hard_failures"])

    def test_true_source_mismatch_is_invalid(self):
        s,m,p,ref,sha=fixture()
        r=score(s,m,p,ref,sha+"_other")
        self.assertIn("BAD_SOURCE_IDENTITY",r["hard_failures"])

    def test_frozen_grade_and_must_settle_cannot_change(self):
        s,m,p,ref,sha=fixture()
        ref["scenarios"][0]["stages"][0]["frozen_grade_screen"]=[4,20]
        r=score(s,m,p,ref,sha)
        self.assertIn("C03_PREDECLARED_RECOVER5_GRADE_CHANGED",r["hard_failures"])

    def test_bad_business_or_capacity_blocks_scope(self):
        s,m,p,ref,sha=fixture()
        s["capacity_flags"]=["5s first unique delivery hole"]
        r=score(s,m,p,ref,sha)
        self.assertIn("SUMMARY_ERRORS_CORRUPTION_OR_CAPACITY",r["hard_failures"])
if __name__=="__main__":
    unittest.main()
