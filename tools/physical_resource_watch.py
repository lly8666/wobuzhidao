"""1Hz scoped socket/CPU/RSS sampler; bounded metadata, no packet bytes."""
import argparse,json,os,re,subprocess,time
from pathlib import Path

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--pid',type=int,required=True);ap.add_argument('--seconds',type=int,default=330)
    ap.add_argument('--output',required=True);args=ap.parse_args()
    if not 1<=args.seconds<=660:raise ValueError('bounded1..660seconds')
    hz=os.sysconf('SC_CLK_TCK');pages=os.sysconf('SC_PAGE_SIZE');samples=[];started=time.monotonic()
    while time.monotonic()-started<args.seconds:
        sample_started_unix_ns=time.time_ns()
        sample_started_monotonic_ns=time.monotonic_ns()
        stat=Path('/proc/%d/stat'%args.pid).read_text().split(') ',1)[1].split()
        sockets=subprocess.run(['ss','-0apnm'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,timeout=3).stdout
        own=[line.strip() for line in sockets.splitlines() if 'pid=%d,'%args.pid in line]
        udp=subprocess.run(['ss','-uapnm','sport = :18445 or sport = :18446'],stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True,timeout=3)
        samples.append(dict(unix_ns=sample_started_unix_ns,monotonic_ns=sample_started_monotonic_ns,
            sampled_unix_ns=time.time_ns(),elapsed_seconds=time.monotonic()-started,cpu_seconds=(int(stat[11])+int(stat[12]))/hz,
            rss_bytes=int(stat[21])*pages, socket_lines=own, fixture_udp_socket_lines=udp.stdout.splitlines()[:12],
            fixture_udp_sampler_error=udp.stderr[:256],
            host_cpu=Path('/proc/stat').read_text().splitlines()[0],
            cpu_pressure=Path('/proc/pressure/cpu').read_text().strip(),
            softnet=Path('/proc/net/softnet_stat').read_text().strip()))
        time.sleep(max(0,1-(time.monotonic()-started)%1))
    Path(args.output).write_text(json.dumps(dict(pid=args.pid,seconds=args.seconds,samples=samples),indent=2)+'\n')

if __name__=='__main__':main()
