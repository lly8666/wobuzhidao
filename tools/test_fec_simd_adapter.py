import sys, unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parent))
import fec_simd_ab as ab
class TestABBA(unittest.TestCase):
 def test_fixed_abba_oneshot(self):
  for phase in ("q120","l120","confirm300"):
   rows=ab.plan(phase)
   self.assertEqual(len(rows),12)
   for i in (0,4,8):
    group=rows[i:i+4]
    self.assertEqual([r["label"] for r in group],["A","B","B","A"])
    for key in ("context","mode","seed","rate","lanes","parity","duration","delay","loss"):
     self.assertEqual(len({r[key] for r in group}),1)
 def test_historic_q2_300s_exact_parameters(self):
  cases=ab.plan("q2historic300")
  self.assertEqual([c["label"] for c in cases],["A","B","B","A"])
  self.assertEqual([c["source"] for c in cases],[ab.A,ab.B,ab.B,ab.A])
  for c in cases:
   self.assertEqual((c["context"],c["mode"],c["lanes"],c["rate"],c["parity"],c["delay"],c["loss"],c["seed"],c["duration"]),("Q2","normal",1,10,20,300,5205,1844,300))
  self.assertEqual(ab.plan("q120")[4]["seed"],2262)
  self.assertEqual(ab.plan("confirm300")[4]["seed"],2262)
 def test_observation_exact_same_historic300_plan(self):
  self.assertEqual(ab.plan("q2observe300"),ab.plan("q2historic300"))
 def test_diagnostic_continues_only_loss_quality_fail(self):
  base={"classification":"FAIL","issues":["5205_STAGE_PROBE_LOSS_OVER_1PCT_s2c_stress"],
    "sample_exit":0,"sample_error":None,"analyzer_exit":1,"ledger_exit":0,
    "owned_cleanup":{"clean":True},
    "evidence_sha256":{k:"sha" for k in
      ("summary.json","manifest.json","efficiency-ledger.json","runtime-flags.json")}}
  self.assertTrue(ab.may_observe_next("q2observe300",base))
  self.assertFalse(ab.may_observe_next("q2historic300",base))
  for change in ({"classification":"INFRA_INVALID"},{"issues":["LOCAL_SOCKET_DROP"]},
    {"issues":["5205_STAGE_PROBE_LOSS_OVER_1PCT_s2c_stress","TCP_HASH_MISMATCH"]},
    {"sample_exit":1},{"sample_error":"TimeoutExpired"},{"analyzer_exit":0},
    {"ledger_exit":1},{"owned_cleanup":{"clean":False}},{"evidence_sha256":{}}):
   self.assertFalse(ab.may_observe_next("q2observe300",dict(base,**change)))
 def test_loss_overview_aggregates_failed_leg_instead_of_omitting_it(self):
  import json
  from tempfile import TemporaryDirectory
  cases=ab.plan("q2observe300")
  with TemporaryDirectory() as d:
   root=Path(d);rows=[]
   for c in cases:
    p=root/c["id"];p.mkdir()
    z={"sent":100,"delivered":96 if c["label"]=="A" else 99,
       "missing":4 if c["label"]=="A" else 1,"over_3s":0,"by_size":{"96":{"missing":4 if c["label"]=="A" else 1}}}
    dirs={side:{"phase_delivery_5205":{med:{k:z for k in ("pre","stress","post")}
          for med in ("udp","probe")}} for side in ("c2s","s2c")}
    (p/"summary.json").write_text(json.dumps({"direction":dirs}))
    row={"case":c,"classification":"FAIL" if c["label"]=="A" else "VALID_OBSERVATION",
     "issues":["5205_STAGE_PROBE_LOSS_OVER_1PCT_s2c_stress"] if c["label"]=="A" else [],
     "sample_exit":0,"sample_error":None,"analyzer_exit":1 if c["label"]=="A" else 0,
     "ledger_exit":0,"owned_cleanup":{"clean":True},
     "evidence_sha256":{k:"sha" for k in
       ("summary.json","manifest.json","efficiency-ledger.json","runtime-flags.json")}}
    rows.append(row)
   out=ab.loss_overview(root,cases,rows)
   self.assertTrue(out["all_four_measurable"])
   self.assertEqual(out["full_window_loss_by_label_direction_medium"]["A/s2c/probe"]["missing"],24)
   self.assertEqual(out["full_window_loss_by_label_direction_medium"]["B/s2c/probe"]["missing"],6)
   self.assertTrue((root/"loss-overview.json").is_file())
 def test_pilot(self):
  self.assertEqual([c["label"] for c in ab.plan("pilot")],["A","B"])
  self.assertNotEqual(ab.A,ab.B)
  self.assertEqual([c["duration"] for c in ab.plan("pilot")],[120,120])
  self.assertEqual([c["context"] for c in ab.plan("pilot")],["Q1","Q1"])
 def test_privileged_owned_child_is_not_silently_accepted(self):
  from unittest.mock import patch
  from types import SimpleNamespace
  case=Path("/tmp/wbd-simd-case")
  calls=[]
  def fake_run(argv,**kwargs):
   calls.append(argv)
   if argv[:3]==["ps","-eo","pid=,args="]:
    return SimpleNamespace(returncode=0,stdout=" 88 sudo bash /tmp/wbd-simd-case/generated.sh\n")
   if argv[:2]==["ps","-p"]:
    return SimpleNamespace(returncode=0,stdout="sudo bash /tmp/wbd-simd-case/generated.sh\n")
   if argv[:3]==["sudo","ip","netns"]:
    return SimpleNamespace(returncode=0,stdout="")
   if argv[:4]==["sudo","-n","kill","-TERM"]:
    return SimpleNamespace(returncode=0,stdout="")
   raise AssertionError(argv)
  with patch.object(ab.subprocess,"run",side_effect=fake_run),patch.object(ab.os,"kill",side_effect=PermissionError),patch.object(ab.time,"sleep"):
   result=ab.simd_owned(case,"901")
  self.assertFalse(result["clean"])
  self.assertEqual(result["original_leaked_pids"],[88])
  self.assertEqual(result["sudo_fallback_count"],1)
  self.assertEqual(result["cleanup_signal_failures"],[])
  self.assertIn(["sudo","-n","kill","-TERM","--","88"],calls)
 def test_unowned_pid_recheck_never_signalled(self):
  from unittest.mock import patch
  from types import SimpleNamespace
  case=Path("/tmp/wbd-simd-case")
  def fake_run(argv,**kwargs):
   if argv[:3]==["ps","-eo","pid=,args="]:
    return SimpleNamespace(returncode=0,stdout=" 89 sudo bash /tmp/wbd-simd-case/generated.sh\n")
   if argv[:2]==["ps","-p"]:
    return SimpleNamespace(returncode=0,stdout="unrelated-process\n")
   if argv[:3]==["sudo","ip","netns"]:
    return SimpleNamespace(returncode=0,stdout="")
   raise AssertionError(argv)
  with patch.object(ab.subprocess,"run",side_effect=fake_run),patch.object(ab.os,"kill") as killed,patch.object(ab.time,"sleep"):
   result=ab.simd_owned(case,"901")
  killed.assert_not_called()
  self.assertEqual(result["cleanup_signal_failures"],[89])
  self.assertFalse(result["clean"])
 def test_startup_private_error_only_yields_categories(self):
  private="WBD_STRICT_SERVER_EARLY_EXIT pid=12345\npassword=qsecret\nFEC_EXPERIMENT_SHELL_FAIL code=1 line=245\n"
  result=ab.shell_failure_classes(private)
  self.assertEqual(result,[{"class":"SHELL_FAIL","exit_code":1,"script_line":245},
                           {"class":"PRODUCT_SERVER_EARLY_EXIT"}])
  self.assertNotIn("qsecret",repr(result))

 def test_bounded_product_log_discards_secrets(self):
  from unittest.mock import patch
  from tempfile import TemporaryDirectory
  with TemporaryDirectory() as folder:
   p=Path(folder)/"server.log"
   p.write_text("2026/10/10 server stopped network closed socket password=qsecret route-key=0123456789abcdef\n")
   diag=ab.bounded_product_exit_evidence(p)
   self.assertTrue(diag["present"])
   self.assertEqual(diag["line_count"],1)
   self.assertNotIn("qsecret",repr(diag))
   self.assertNotIn("0123456789abcdef",repr(diag))
   self.assertIn("server stopped network closed socket",diag["last_error_vocabulary"][0])
 def test_bounded_error_line_redacts_credentials(self):
  raw="2026/10/10 08:08:08 connection error password=qsecret route-key=0123456789abcdef 198.18.0.1 /tmp/private/key"
  clean=ab.sanitized_product_error_line(raw)
  for secret in ("qsecret","0123456789abcdef","198.18.0.1","/tmp/private/key"):
   self.assertNotIn(secret,clean)
  self.assertIn("connection error",clean)
  self.assertEqual(ab.sanitized_product_error_line("WBD_RAW_RCVBUF secret=qsecret"),"")
if __name__=="__main__":unittest.main()
