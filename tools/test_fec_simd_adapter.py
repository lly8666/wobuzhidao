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
 def test_privileged_owned_child_is_not_silently_accepted(self):
  from unittest.mock import patch
  from types import SimpleNamespace
  case=Path("/tmp/wbd-simd-case")
  calls=[]
  def fake_run(argv,**kwargs):
   calls.append(argv)
   if argv[:3]==["ps","-eo","pid=,args="]:
    return SimpleNamespace(returncode=0,stdout=" 88 sudo bash /tmp/wbd-simd-case/generated.sh\\n")
   if argv[:2]==["ps","-p"]:
    return SimpleNamespace(returncode=0,stdout="sudo bash /tmp/wbd-simd-case/generated.sh\\n")
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
    return SimpleNamespace(returncode=0,stdout=" 89 sudo bash /tmp/wbd-simd-case/generated.sh\\n")
   if argv[:2]==["ps","-p"]:
    return SimpleNamespace(returncode=0,stdout="unrelated-process\\n")
   if argv[:3]==["sudo","ip","netns"]:
    return SimpleNamespace(returncode=0,stdout="")
   raise AssertionError(argv)
  with patch.object(ab.subprocess,"run",side_effect=fake_run),patch.object(ab.os,"kill") as killed,patch.object(ab.time,"sleep"):
   result=ab.simd_owned(case,"901")
  killed.assert_not_called()
  self.assertEqual(result["cleanup_signal_failures"],[89])
  self.assertFalse(result["clean"])
if __name__=="__main__":unittest.main()
