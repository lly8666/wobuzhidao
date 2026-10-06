"""Actions-only isolated real TUN reverse fragment coverage, no benchmark."""
import fcntl
import json
import os
import socket
import struct
import subprocess
import sys
import tempfile
import time
from pathlib import Path


def inside():
    fd=os.open('/dev/net/tun',os.O_RDWR|os.O_NONBLOCK)
    try:
        fcntl.ioctl(fd,0x400454ca,struct.pack('16sH22x',b'wbdg0',0x1001))
        for cmd in [ ['ip','link','set','lo','up'],['ip','address','add','198.18.0.1/32','dev','lo'],['ip','link','set','wbdg0','mtu','1400','up'],['ip','route','add','10.66.0.0/16','dev','wbdg0'] ]:
            subprocess.run(cmd,check=True)
        with tempfile.TemporaryDirectory() as tmp:
            output=Path(tmp)/'fragment.json';ready=Path(tmp)/'ready.json'
            observer=subprocess.Popen([sys.executable,str(Path(__file__).with_name('physical_inner_fragment_watch.py')),'--destination','10.66.1.1','--seconds','2','--output',str(output),'--ready',str(ready)])
            try:
                deadline=time.monotonic()+3
                while not ready.exists():
                    if observer.poll() is not None or time.monotonic()>deadline:raise AssertionError('Observer not bound')
                    time.sleep(.01)
                with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as udp:
                    udp.setsockopt(socket.IPPROTO_IP,10,0)
                    udp.bind(('198.18.0.1',18446))
                    for seq,size in [(100,65507),(101,8973),(102,96)]:
                        data=b'P7M1'+struct.pack('<I',seq)+bytes((n%256 for n in range(size-8)))
                        assert udp.sendto(data,('10.66.1.1',45000))==size
                observer.wait(timeout=5)
                assert observer.returncode==0
                result=json.loads(output.read_text());rows=result['rows']
                assert result['completed_window'] and result['observer_drops']==0 and not result['error']
                assert len(rows)==56,('Reverse fragments missing',len(rows))
                for seq,expected,size in [(100,48,65507),(101,7,8973),(102,1,96)]:
                    first=[r for r in rows if r['sequence']==seq];assert len(first)==1
                    fragments=sorted([r for r in rows if r['ip_id']==first[0]['ip_id']],key=lambda r:r['offset'])
                    assert len(fragments)==expected
                    end=0
                    for r in fragments:
                        assert r['offset']==end and r['header_checksum_ok'];end+=r['fragment_payload_length']
                    assert end==size+8 and not fragments[-1]['mf']
                print(json.dumps(dict(state='PASS',reverse_fragments=len(rows),observer_drops=0,no_raw_payload_stored=True)))
            finally:
                if observer.poll() is None:observer.terminate();observer.wait(timeout=5)
    finally:
        os.close(fd)


def main():
    if os.environ.get('GITHUB_ACTIONS')!='true' or os.geteuid()!=0:
        raise RuntimeError('Only privileged Actions functional fixture permitted')
    if '--inside' in sys.argv:
        inside();return
    name='wbd-frag-'+str(os.getpid())
    subprocess.run(['ip','netns','add',name],check=True)
    try:
        subprocess.run(['ip','netns','exec',name,sys.executable,str(Path(__file__).resolve()),'--inside'],check=True,timeout=12)
    finally:
        subprocess.run(['ip','netns','delete',name],check=True)


if __name__=='__main__':main()
