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
from check_target_soak import integrity_errors,short_window_loss,internal_queue_drops


class SoakHarnessTests(unittest.TestCase):
    def test_kernel_zero_does_not_hide_internal_queue_overflow(self):
        rows=[{'product':{'present':True,'tunnel':{'server_pipeline':{'overflow_drops':13482}}}}]
        self.assertTrue(internal_queue_drops(rows,'server')['errors'])
        rows[0]['product']['tunnel']['server_pipeline']['overflow_drops']=0
        self.assertFalse(internal_queue_drops(rows,'server')['errors'])
        self.assertTrue(internal_queue_drops([],'client')['errors'])

    def test_short_loss_gate_rejects_a_burst_hidden_by_stage_average(self):
        phases=[dict(name='post5',start_s=0,end_s=60,loss_percent=5)]
        sent={'sent_packets_by_second':[2000]*60}
        received={'recv_packets_by_second':[2000]*60}
        received['recv_packets_by_second'][30]=800
        # Overall stage loss is1%, yet that send-second lost60%.
        result=short_window_loss(sent,received,phases,60)
        self.assertTrue(result['errors'])
        self.assertEqual(result['phases'][0]['worst']['second'],30)
        self.assertEqual(result['phases'][0]['worst']['packet_loss_percent'],60)
        self.assertEqual(result['phases'][0]['violations'],1)

    def test_short_loss_uses_eventual_send_buckets_and_checks_counts(self):
        phases=[dict(name='stress20',start_s=0,end_s=2,loss_percent=20)]
        sender={'sent_packets_by_second':[2000,2000]}
        receiver={'recv_packets_by_second':[1600,2000],'recv_wall_packets_by_second':[0,3600]}
        self.assertFalse(short_window_loss(sender,receiver,phases,2)['errors'])
        receiver['recv_packets_by_second']=[2001,2000]
        self.assertTrue(short_window_loss(sender,receiver,phases,2)['errors'])
        self.assertTrue(short_window_loss({},receiver,phases,2)['errors'])

    def test_integrity_gate_keeps_an_error_on_a_retired_incarnation(self):
        def sample(record_errors):
            return {'product':{'lanes':[{'lane':{'RecordErrors':record_errors,'PathErrors':0},'transport':{'RecordErrors':record_errors,'PathErrors':0}}]}}
        self.assertEqual(integrity_errors([sample(0)],'client'),[])
        self.assertTrue(integrity_errors([sample(1),sample(0)],'client'))
        self.assertTrue(integrity_errors([{'product':{'lanes':[{}]}}],'client'))
        self.assertTrue(integrity_errors([],'server'))
        self.assertTrue(integrity_errors([{'product':{'lanes':[], 'retiring_lanes':sample(1)['product']['lanes']}}],'client'))

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
            self.assertIn('"harness_sha": os.environ.get("WBD_HARNESS_SHA", source)',s)

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
