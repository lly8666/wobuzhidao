#!/usr/bin/env python3
"""V2-M1 local qualification harness.

Run inside a privileged Linux network namespace with loopback up and an
`iptables` command available. The exact pinned udp2raw binary is invoked with
`--raw-mode faketcp -a`; no easy-faketcp/kernel-anchor behavior is used.
Loss and 25 ms/direction delay are injected between UDPspeeder's encoded FEC
shards and udp2raw, matching the historical M10-004 reference layer.
"""
import argparse, csv, heapq, json, math, os, random, select, signal, socket, struct, subprocess, sys, threading, time
from pathlib import Path

U = None
S = None
ROOT = None
CLK = os.sysconf(os.sysconf_names['SC_CLK_TCK'])

def pct(v, p):
    if not v: return 0.0
    a=sorted(v); i=max(0,min(len(a)-1,math.ceil(p*len(a)/100)-1)); return a[i]

def proc_stats(pid):
    try:
        fields=Path(f'/proc/{pid}/stat').read_text().split()
        cpu=(int(fields[13])+int(fields[14]))/CLK
        rss=0
        for line in Path(f'/proc/{pid}/status').read_text().splitlines():
            if line.startswith('VmRSS:'):
                rss=int(line.split()[1]); break
        return cpu,rss
    except Exception:
        return 0.0,0

def start_echo(port, log):
    code='''import socket,sys\ns=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(("127.0.0.1",int(sys.argv[1])))\nwhile True:\n d,a=s.recvfrom(65535);s.sendto(d,a)\n'''
    f=open(log,'wb')
    return subprocess.Popen([sys.executable,'-c',code,str(port)],stdout=f,stderr=subprocess.STDOUT,start_new_session=True),f

def start(cmd, log):
    f=open(log,'wb')
    p=subprocess.Popen([str(x) for x in cmd],stdout=f,stderr=subprocess.STDOUT,start_new_session=True)
    return p,f

def stop_proc(p):
    if not p: return
    try: os.killpg(p.pid, signal.SIGTERM)
    except ProcessLookupError: return
    try: p.wait(timeout=.7)
    except subprocess.TimeoutExpired:
        try: os.killpg(p.pid, signal.SIGKILL)
        except ProcessLookupError: pass
        try: p.wait(timeout=.5)
        except Exception: pass

def proxy_worker(port,target_port,loss,delay_ms,seed,stop,stats):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1); s.bind(('127.0.0.1',port)); s.setblocking(False)
    target=('127.0.0.1',target_port); peer=None; q=[]; rng=random.Random(seed); seq=0
    st={'loss_pct':loss,'forward_seen':0,'forward_dropped':0,'forward_bytes':0,'reverse_seen':0,'reverse_bytes':0}
    while not stop.is_set():
        now=time.monotonic()
        while q and q[0][0]<=now:
            _,_,d,dst=heapq.heappop(q)
            try:s.sendto(d,dst)
            except OSError: pass
        timeout=.003 if not q else max(0,min(.003,q[0][0]-now))
        try:r,_,_=select.select([s],[],[],timeout)
        except Exception:continue
        if not r:continue
        try:d,a=s.recvfrom(65535)
        except BlockingIOError:continue
        if a==target:
            st['reverse_seen']+=1; st['reverse_bytes']+=len(d)
            if peer is not None:
                heapq.heappush(q,(time.monotonic()+delay_ms/1000,seq,d,peer));seq+=1
        else:
            peer=a;st['forward_seen']+=1;st['forward_bytes']+=len(d)
            if rng.random()<loss/100:
                st['forward_dropped']+=1;continue
            heapq.heappush(q,(time.monotonic()+delay_ms/1000,seq,d,target));seq+=1
    stats.update(st); s.close()

def run_proxy(loss,delay_ms,seed,base):
    stop=threading.Event(); stats={}
    t=threading.Thread(target=proxy_worker,args=(base+4,base+3,loss,delay_ms,seed,stop,stats),daemon=True);t.start();time.sleep(.03)
    return stop,t,stats

def stop_proxy(x):
    stop,t,stats=x;stop.set();t.join(timeout=.5);return stats

def ping(port,count,size,window,timeout_ms):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('127.0.0.1',0));s.connect(('127.0.0.1',port));s.setblocking(False)
    sent={};done=set();samples=[];nxt=0;end=time.monotonic()+max(10,count*.02)
    payload_tail=bytes(max(0,size-4))
    def send(i): sent[i]=time.monotonic();s.send(struct.pack('!I',i)+payload_tail)
    while nxt<count and len(sent)-len(done)<window:send(nxt);nxt+=1
    while time.monotonic()<end and len(done)<count:
        rr,_,_=select.select([s],[],[],.003);now=time.monotonic()
        if rr:
            try:d=s.recv(65535)
            except BlockingIOError:d=b''
            if len(d)>=4:
                i=struct.unpack('!I',d[:4])[0]
                if i in sent and i not in done:
                    done.add(i);samples.append((now-sent[i])*1000)
        expired=[i for i,t0 in sent.items() if i not in done and (now-t0)*1000>timeout_ms]
        for i in expired:done.add(i)
        while nxt<count and len(sent)-len(done)<window:send(nxt);nxt+=1
    s.close()
    return samples

def start_stack(fec,case_dir,base):
    procs=[];files=[]
    p,f=start_echo(base,case_dir/'echo.log');procs.append(p);files.append(f)
    p,f=start([S,'-s',f'-l127.0.0.1:{base+1}',f'-r127.0.0.1:{base}',f'-f{fec}','--mode','0','--timeout','8','-k','wbdtest','--disable-color','--log-level','2'],case_dir/'speeder-server.log');procs.append(p);files.append(f)
    p,f=start([U,'-s',f'-l127.0.0.1:{base+2}',f'-r127.0.0.1:{base+1}','-k','wbdtest','--raw-mode','faketcp','-a','--disable-color','--log-level','2'],case_dir/'udp2raw-server.log');procs.append(p);files.append(f)
    time.sleep(.12)
    p,f=start([U,'-c',f'-l127.0.0.1:{base+3}',f'-r127.0.0.1:{base+2}','-k','wbdtest','--raw-mode','faketcp','-a','--source-ip','127.0.0.1','--source-port',str(base+10),'--disable-color','--log-level','2'],case_dir/'udp2raw-client.log');procs.append(p);files.append(f)
    time.sleep(.25)
    p,f=start([S,'-c',f'-l127.0.0.1:{base+5}',f'-r127.0.0.1:{base+4}',f'-f{fec}','--mode','0','--timeout','8','-k','wbdtest','--disable-color','--log-level','2'],case_dir/'speeder-client.log');procs.append(p);files.append(f)
    time.sleep(.45)
    return procs,files

def stop_stack(procs,files):
    for p in reversed(procs):stop_proc(p)
    for f in files:
        try:f.close()
        except:pass
    time.sleep(.08)

def run_case(fec,loss,seed):
    case_dir=ROOT/f'{fec.replace(":","-")}_loss{loss}_seed{seed}';case_dir.mkdir(exist_ok=True)
    fec_idx=0 if fec=='20:10' else 1
    base=45000 + fec_idx*5000 + loss*100 + (seed-260824)*20
    procs,files=start_stack(fec,case_dir,base)
    try:
        px=run_proxy(0,25,seed,base)
        warm=ping(base+5,64,256,1,1000)
        stop_proxy(px); time.sleep(.04)
        if len(warm)!=64:
            raise RuntimeError(f'warmup delivery {len(warm)}/64')
        px=run_proxy(loss,25,seed+loss*1009+int(fec.split(':')[1])*17,base)
        product=procs[1:]
        cpu0={p.pid:proc_stats(p.pid)[0] for p in product}
        maxrss=[sum(proc_stats(p.pid)[1] for p in product)]
        monstop=threading.Event()
        def monitor():
            while not monstop.is_set():
                maxrss.append(sum(proc_stats(p.pid)[1] for p in product));time.sleep(.01)
        mt=threading.Thread(target=monitor,daemon=True);mt.start()
        samples=ping(base+5,200,256,32,1000)
        monstop.set();mt.join(timeout=.2)
        cpu1={p.pid:proc_stats(p.pid)[0] for p in product}
        st=stop_proxy(px)
        cpu=max(0,sum(cpu1.values())-sum(cpu0.values()))
        delivered=len(samples);src=200*256
        row={
          'fec':fec,'loss_pct':loss,'seed':seed,'samples':200,'delivered':delivered,
          'delivery_ratio':delivered/200,'mean_ms':sum(samples)/delivered if delivered else 0,
          'p50_ms':pct(samples,50),'p95_ms':pct(samples,95),'p99_ms':pct(samples,99),
          'late_ratio':sum(x>100 for x in samples)/delivered if delivered else 1,
          'fec_packets_seen':st.get('forward_seen',0),'fec_packets_dropped':st.get('forward_dropped',0),
          'fec_forward_bytes':st.get('forward_bytes',0),'fec_reverse_bytes':st.get('reverse_bytes',0),
          'fec_forward_traffic_x':st.get('forward_bytes',0)/src,
          'cpu_ms_product':cpu*1000,'rss_peak_kb_product':max(maxrss) if maxrss else 0,
          'warmup_delivery_ratio':len(warm)/64,
        }
        (case_dir/'result.json').write_text(json.dumps(row,indent=2))
        return row
    finally:
        stop_stack(procs,files)

def main():
    global U, S, ROOT
    ap=argparse.ArgumentParser(description="Reproduce the pinned V2-M1 one-lane udp2raw + UDPspeeder baseline")
    ap.add_argument('--udp2raw', required=True, type=Path)
    ap.add_argument('--udpspeeder', required=True, type=Path)
    ap.add_argument('--out', type=Path, default=Path('v2-m1-results'))
    ap.add_argument('--fecs', default='20:10,20:20')
    ap.add_argument('--losses', default='0,1,5,10,15')
    ap.add_argument('--seeds', default='260824,260825,260826')
    a=ap.parse_args()
    U=a.udp2raw.resolve(); S=a.udpspeeder.resolve(); ROOT=a.out.resolve(); ROOT.mkdir(parents=True,exist_ok=True)
    fecs=[x.strip() for x in a.fecs.split(',') if x.strip()]
    losses=[int(x) for x in a.losses.split(',') if x.strip()]
    seeds=[int(x) for x in a.seeds.split(',') if x.strip()]
    for fec in fecs:
        if fec not in ('20:10','20:20'): raise SystemExit(f'unsupported V2-M1 FEC profile: {fec}')
    rows=[]
    for fec in fecs:
      for loss in losses:
       for seed in seeds:
        r=run_case(fec,loss,seed);rows.append(r);print(json.dumps(r),flush=True)
    fields=list(rows[0].keys())
    with open(ROOT/'results.csv','w',newline='') as f:
        w=csv.DictWriter(f,fieldnames=fields);w.writeheader();w.writerows(rows)
    groups={}
    for r in rows:groups.setdefault((r['fec'],r['loss_pct']),[]).append(r)
    med_fields=['mean_ms','p50_ms','p95_ms','p99_ms','late_ratio','delivery_ratio','fec_forward_traffic_x','cpu_ms_product','rss_peak_kb_product']
    import statistics
    with open(ROOT/'median.csv','w',newline='') as f:
      w=csv.writer(f);w.writerow(['fec','loss_pct']+med_fields)
      for k in sorted(groups,key=lambda x:(x[0],x[1])):
        rs=groups[k];w.writerow([k[0],k[1]]+[statistics.median(float(r[x]) for r in rs) for x in med_fields])
    print('---MEDIAN---');print((ROOT/'median.csv').read_text())
if __name__=='__main__':main()
