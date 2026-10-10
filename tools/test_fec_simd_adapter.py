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
if __name__=="__main__":unittest.main()
