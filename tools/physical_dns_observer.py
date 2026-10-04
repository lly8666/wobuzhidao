"""Bounded DNS IP/transport counters on the tunnel;no DNS bodies saved."""
import argparse,collections,ipaddress,json,re,signal,subprocess
from pathlib import Path
def main():
    ap=argparse.ArgumentParser();ap.add_argument('--lease',required=True);ap.add_argument('--output',required=True);args=ap.parse_args()
    ipaddress.IPv4Address(args.lease)
    p=subprocess.Popen(['tcpdump','-i','wbdg0','-n','-l','-q','-s','128','-B','1024','-c','1000',
        '(src host %s or dst host %s) and port 53'%(args.lease,args.lease)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
    try:
        try:text,err=p.communicate(timeout=335)
        except subprocess.TimeoutExpired:p.send_signal(signal.SIGINT);text,err=p.communicate(timeout=5)
        counts=collections.Counter();total=0
        for line in text.splitlines():
            m=re.search(r'IP ([0-9.]+)\.(\d+) > ([0-9.]+)\.(\d+):',line)
            if m:
                counts[(m[1],m[3],'UDP' if 'UDP' in line else 'TCP')]+=1;total+=1
        d=re.search(r'(\d+) packets dropped by kernel',err)
        Path(args.output).write_text(json.dumps(dict(counted_frames=total,
            flows=[dict(src=k[0],dst=k[1],transport=k[2],packets=v) for k,v in counts.items()],
            observer_drops=int(d[1]) if d else None,payload_stored=False,pcap_written=False),indent=2)+'\n')
    finally:
        if p.poll() is None:p.send_signal(signal.SIGINT);p.wait(timeout=5)
if __name__=='__main__':main()
