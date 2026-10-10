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
