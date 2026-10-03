import json
import struct
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
from prepare_soak_harness import main as prepare
from soak_capture import read_chunk
from soak_weaknet_stage import plan
from check_target_soak import integrity_errors


class SoakHarnessTests(unittest.TestCase):
    def test_integrity_gate_keeps_an_error_on_a_retired_incarnation(self):
        def sample(record_errors):
            return {'product':{'lanes':[{'lane':{'RecordErrors':record_errors,'PathErrors':0},'transport':{'RecordErrors':record_errors,'PathErrors':0}}]}}
        self.assertEqual(integrity_errors([sample(0)],'client'),[])
        self.assertTrue(integrity_errors([sample(1),sample(0)],'client'))
        self.assertTrue(integrity_errors([{'product':{'lanes':[{}]}}],'client'))
        self.assertTrue(integrity_errors([],'server'))

    def test_schedules_cover_the_full_declared_run(self):
        for duration in [180,1800]:
            phases=plan(duration)
            self.assertEqual(phases[0]['start_s'],0)
            self.assertEqual(phases[-1]['end_s'],duration)
            self.assertTrue(all(a['end_s']==b['start_s'] for a,b in zip(phases,phases[1:])))
            self.assertEqual(set(p['loss_percent'] for p in phases),{5,20})

    def test_separate_generated_harness_has_no_unbounded_generator(self):
        with tempfile.TemporaryDirectory() as d:
            with patch.object(sys,'argv',['prepare','--artifact-dir',d,'--duration','1800']):prepare()
            s=(Path(d)/'generated-soak.sh').read_text()
            self.assertEqual(s.count('--duration 1800 --drain 60 --bounded-stats'),2)
            self.assertEqual(s.count('-Z root -G 15'),4)
            self.assertLess(s.index('(art / "manifest.json").write_text'),s.index('python3 tools/soak_capture.py'))
            self.assertNotIn('--duration 120',s)
            self.assertIn('--rotate-min 600s --rotate-max 600s',s)
            self.assertIn('"qualification": "FORMAL"',s)
            self.assertIn('"duration_s": 1800',s)

    def test_stream_capture_rejects_missing_data_and_counts_wire_length(self):
        with tempfile.TemporaryDirectory() as d:
            p=Path(d)/'capture.pcap'
            header=struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,128,1)
            frame=bytearray(54);frame[12:14]=b'\x08\x00';frame[14]=0x45
            frame[16:18]=struct.pack('!H',500);frame[23]=6;frame[46]=0x50
            p.write_bytes(header+struct.pack('<IIII',10,0,len(frame),514)+frame)
            r=read_chunk(p,10_000_000_000,3)
            self.assertEqual(r['ip_bytes_by_second'],[500,0,0])
            self.assertEqual(r['bad_headers'],0)
            p.write_bytes(p.read_bytes()[:-1])
            with self.assertRaises(ValueError):read_chunk(p,10_000_000_000,3)


if __name__=='__main__':unittest.main()
