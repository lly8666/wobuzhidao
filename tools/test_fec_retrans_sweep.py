#!/usr/bin/env python3
"""Static tests in GitHub Actions only, no local traffic/build."""
import os, subprocess, tempfile, unittest
from pathlib import Path
from fec_retrans_sweep import DELAYS, LOSSES, RATES, cases, exact

class FixedThirtySix(unittest.TestCase):
    def test_exact_plan_and_isolation(self):
        plan=exact()
        self.assertEqual(len(plan),36)
        self.assertEqual([x["id"] for x in plan],["s%02d"%i for i in range(1,37)])
        self.assertEqual({(x["delay_ms"],x["loss"],x["rate_mbps"]) for x in plan},
            {(d,l,r) for d in DELAYS for l in LOSSES for r in RATES})
        self.assertTrue(all(x["duration_s"]==120 and x["drain_s"]==3
            and x["fec"]=="off" and x["parity"]==0 and x["mode"]=="normal"
            and x["lanes"]==1 and x["workload"]=="udp" for x in plan))
        self.assertEqual(len({x["id"] for x in plan}),36)
    def test_highest_rate_and_loss_generation_is_real_not_label_only(self):
        case=next(x for x in cases() if (x["delay_ms"],x["loss"],x["rate_mbps"])==(150,10,50))
        with tempfile.TemporaryDirectory() as root:
            sh=Path(root)/"sample.sh"
            subprocess.run(["python3","tools/prepare_large_mtu_harness.py",
              "--fec-experiment","--sweep-experiment","--workload","udp",
              "--loss",str(case["loss"]),"--delay-ms",str(case["delay_ms"]),
              "--duration-s","120","--output",str(sh)],
              env={**os.environ,"WBD_EFF_DIAGNOSTIC":"0"},check=True)
            subprocess.run(["bash","-n",str(sh)],check=True)
            s=sh.read_text()
            for fragment in ('normal:1:50','WBD_STRICT_RATE_MBPS','WBD_FEC_DELAY_MS',
              'WBD_LARGE_LOSS','fec-retrans-sweep.yml','fec_wire_retrans_observer.py'):
                self.assertIn(fragment,s)
    def test_unrequested_cases_rejected_by_generator(self):
        with tempfile.TemporaryDirectory() as root:
            cmd=["python3","tools/prepare_large_mtu_harness.py",
                "--fec-experiment","--sweep-experiment","--workload","tcp",
                "--loss","10","--delay-ms","150","--duration-s","120",
                "--output",str(Path(root)/"bad.sh")]
            result=subprocess.run(cmd,env={**os.environ,"WBD_EFF_DIAGNOSTIC":"0"},
                capture_output=True)
            self.assertNotEqual(result.returncode,0)
if __name__=="__main__": unittest.main()
