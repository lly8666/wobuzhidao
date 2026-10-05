"""Actions-only isolated kernel fault semantics, not a performance workload."""
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time

ROOT = Path(__file__).resolve().parents[1]
FIXTURE = str(ROOT / 'tools/physical_egress_blackhole.py')


def run(*args):
    return subprocess.check_output(list(args), text=True)


def main():
    if os.environ.get('GITHUB_ACTIONS') != 'true' or os.geteuid() != 0:
        raise RuntimeError('Only root inside GitHub Actions qualification')
    a, b = 'wbd-tcf-a-%d' % os.getpid(), 'wbd-tcf-b-%d' % os.getpid()
    created = []
    with tempfile.TemporaryDirectory(prefix='wbd-tc-fixture-') as temp:
        try:
            for ns in (a, b):
                run('ip', 'netns', 'add', ns); created.append(ns)
            run('ip', 'link', 'add', 'wtc-a', 'type', 'veth', 'peer', 'name', 'wtc-b')
            run('ip', 'link', 'set', 'wtc-a', 'netns', a)
            run('ip', 'link', 'set', 'wtc-b', 'netns', b)
            for ns, interface, address in ((a, 'wtc-a', '10.77.0.1/24'), (b, 'wtc-b', '10.77.0.2/24')):
                run('ip', '-n', ns, 'link', 'set', 'lo', 'up')
                run('ip', '-n', ns, 'address', 'add', address, 'dev', interface)
                run('ip', '-n', ns, 'link', 'set', interface, 'up')
            run('ip', '-n', b, 'address', 'add', '10.77.0.3/32', 'dev', 'wtc-b')
            state = str(Path(temp) / 'fault.json')

            def fault(op):
                args = ['ip', 'netns', 'exec', a, sys.executable, FIXTURE, op, '--state', state]
                if op == 'enable':
                    args += ['--interface', 'wtc-a', '--peer', '10.77.0.2', '--seed', '1411']
                return json.loads(run(*args))

            def probe(index, blocked=False, protocol='tcp', port=443, peer='10.77.0.2'):
                marker = 'WBD-TC-FIXTURE-%d' % index
                ready = str(Path(temp) / ('ready-%d' % index))
                receiver = """import socket,time,pathlib,json
s=socket.socket(socket.AF_PACKET,socket.SOCK_RAW,socket.htons(0x0800));s.bind(('wtc-b',0));s.settimeout(.05)
pathlib.Path(%r).write_text('ready');deadline=time.monotonic()+.8;seen=False
while time.monotonic()<deadline:
 try:frame=s.recv(2048)
 except socket.timeout:continue
 if %r in frame:seen=True
print(json.dumps(dict(received=seen)))
""" % (ready, marker.encode())
                p = subprocess.Popen(['ip', 'netns', 'exec', b, sys.executable, '-c', receiver], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
                try:
                    for _ in range(100):
                        if Path(ready).exists(): break
                        if p.poll() is not None: raise RuntimeError('Receiver exited before ready')
                        time.sleep(.01)
                    else: raise RuntimeError('Receiver readiness timeout')
                    sender = """import socket,struct,json
payload=%r
def checksum(data):
 if len(data)%%2:data+=bytes([0])
 total=sum(struct.unpack('!%%dH'%%(len(data)//2),data))
 while total>>16:total=(total&65535)+(total>>16)
 return (~total)&65535
src=socket.inet_aton('10.77.0.1');dst=socket.inet_aton(%r)
if %r=='tcp':
 tcp=struct.pack('!HHIIBBHHH',%d,42000,100,1,80,24,4096,0,0)
 crc=checksum(src+dst+struct.pack('!BBH',0,6,len(tcp)+len(payload))+tcp+payload)
 tcp=tcp[:16]+struct.pack('!H',crc)+tcp[18:]
 ip=struct.pack('!BBHHHBBH4s4s',69,0,20+len(tcp)+len(payload),1,16384,64,6,0,src,dst)
 ip=ip[:10]+struct.pack('!H',checksum(ip))+ip[12:]
 s=socket.socket(socket.AF_INET,socket.SOCK_RAW,socket.IPPROTO_RAW);s.setsockopt(socket.IPPROTO_IP,socket.IP_HDRINCL,1)
 n=s.sendto(ip+tcp+payload,(%r,42000))
else:
 s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('10.77.0.1',%d));n=s.sendto(payload,(%r,42000))
print(json.dumps(dict(send_succeeded=True,bytes=n)))
""" % (marker.encode(), peer, protocol, port, peer, port, peer)
                    sent = json.loads(run('ip', 'netns', 'exec', a, sys.executable, '-c', sender))
                    out, error = p.communicate(timeout=3)
                    assert p.returncode == 0, error
                    received = json.loads(out)['received']
                    assert sent['send_succeeded'] and received == (not blocked), (index, sent, received)
                finally:
                    if p.poll() is None: p.kill(); p.wait()

            probe(0)
            enabled = fault('enable')
            probe(1, blocked=True)
            probe(2, protocol='udp')
            probe(3, port=55898)
            probe(4, peer='10.77.0.3')
            cleared = fault('disable')
            stats = cleared['filter'][0]['options']['actions'][0]['stats']
            assert stats['packets'] >= 1, cleared
            assert cleared['owned_filter_absent'] and cleared['root_qdisc_unchanged']
            assert not Path(state).exists()
            probe(5)
            # Existing foreign clsact is retained, never replaced or deleted.
            run('ip', 'netns', 'exec', a, 'tc', 'qdisc', 'add', 'dev', 'wtc-a', 'clsact')
            assert not fault('enable')['state']['created_clsact']
            fault('disable')
            q = json.loads(run('ip', 'netns', 'exec', a, 'tc', '-j', 'qdisc', 'show', 'dev', 'wtc-a'))
            assert any(x['kind'] == 'clsact' for x in q)
            run('ip', 'netns', 'exec', a, 'tc', 'filter', 'add', 'dev', 'wtc-a', 'egress', 'protocol', 'ip', 'pref', '41411', 'matchall', 'action', 'pass')
            conflict = subprocess.run(['ip', 'netns', 'exec', a, sys.executable, FIXTURE, 'enable', '--state', state, '--interface', 'wtc-a', '--peer', '10.77.0.2', '--seed', '1411'], capture_output=True, text=True)
            assert conflict.returncode != 0 and 'occupied' in conflict.stderr and not Path(state).exists()
            print(json.dumps(dict(result='PASS', source=os.environ.get('GITHUB_SHA'),
                                  raw_send_succeeds_during_drop=True, dropped_packets=stats['packets'],
                                  udp_other_port_other_peer_unaffected=True, restored_delivery=True,
                                  foreign_clsact_and_filter_preserved=True, kernel=os.uname().release)))
        finally:
            for ns in reversed(created):
                subprocess.run(['ip', 'netns', 'del', ns], check=True)


if __name__ == '__main__':
    main()
