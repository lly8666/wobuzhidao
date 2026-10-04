"""Controlled variable-size UDP echo target for native MTU boundary tests."""
import argparse,hashlib,json,socket,time
from pathlib import Path
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--output',required=True);args=ap.parse_args()
    sizes={96,1371,1372,1373,1472,1972,4068,8972};counts={};bad=0;started=time.monotonic();peer=None
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as s:
        s.bind(('198.18.0.1',18446));s.settimeout(.5)
        while time.monotonic()-started<390:
            try:b,address=s.recvfrom(65535)
            except socket.timeout:continue
            if b==b'P7M-DONE' and address==peer:break
            if peer is not None and peer!=address:continue
            if len(b) not in sizes or b[:4]!=b'P7M1' or b[8:]!=bytes((i%256 for i in range(len(b)-8))):bad+=1;continue
            peer=address;key=str(len(b));counts[key]=counts.get(key,0)+1;s.sendto(b,address)
    Path(args.output).write_text(json.dumps(dict(result='MEASURED',peer_ipv4=peer[0] if peer else None,counts=counts,bad_payload=bad,elapsed_seconds=time.monotonic()-started),indent=2)+'\n')
if __name__=='__main__':main()
