"""Native capture window: <=12s/8192frames/16MiB; analyze, then delete raw.

Only the selected peer/server TCP443 is captured. No secret config reads.
"""
import argparse, json, re, signal, subprocess, time
from pathlib import Path

def main():
    def interrupt(signum, frame): raise KeyboardInterrupt('Capture interrupted;cleanup raw and child')
    signal.signal(signal.SIGTERM,interrupt)
    ap=argparse.ArgumentParser();ap.add_argument('--interface',required=True);ap.add_argument('--local-ip',required=True)
    ap.add_argument('--peer-ip',required=True);ap.add_argument('--seconds',type=int,default=6)
    ap.add_argument('--port',type=int,default=443);ap.add_argument('--output',required=True)
    args=ap.parse_args()
    if not 1<=args.seconds<=12:raise ValueError('Capture window bound1..12seconds')
    # Validate addresses before embedding them in libpcap's filter grammar.
    import ipaddress
    ipaddress.IPv4Address(args.local_ip);ipaddress.IPv4Address(args.peer_ip)
    output=Path(args.output).resolve();output.parent.mkdir(parents=True,exist_ok=True)
    raw=output.with_suffix('.temporary.pcap');raw.unlink(missing_ok=True)
    filt='tcp and host %s and host %s and port %d'%(args.local_ip,args.peer_ip,args.port)
    p=subprocess.Popen(['tcpdump','-i',args.interface,'-nn','-s','1600','-B','4096','-U','-c','8192','-w',str(raw),filt],stdout=subprocess.DEVNULL,stderr=subprocess.PIPE)
    started=time.monotonic()
    try:
        try:_,err=p.communicate(timeout=args.seconds)
        except subprocess.TimeoutExpired:
            p.send_signal(signal.SIGINT);_,err=p.communicate(timeout=5)
        from physical_wire_audit import audit
        result=audit(raw,args.local_ip,args.port)
        text=err.decode(errors='replace');dropped=re.search(r'(\d+) packets dropped by kernel',text)
        result.update(window_seconds=time.monotonic()-started,capture_returncode=p.returncode,
                      capture_kernel_drops=int(dropped.group(1)) if dropped else None,
                      capture_filter=filt,capture_stderr=text[:4096],raw_deleted_after_analysis=True)
        output.write_text(json.dumps(result,indent=2)+'\n')
    finally:
        if p.poll() is None:p.kill();p.wait(timeout=5)
        raw.unlink(missing_ok=True)
    print(json.dumps(dict(output=str(output),raw_deleted=not raw.exists())))

if __name__=='__main__':main()
