#!/usr/bin/env python3
"""V2-M2C local DTLS + UDPspeeder + udp2raw benchmark harness.

Run inside the same isolated Linux user/network namespace used by V2-M1, with
loopback up and `iptables` resolving to a working nft/legacy frontend. The
public carrier remains exact pinned classic udp2raw FakeTCP with `-a` on both
ends. UDPspeeder source/repair datagrams are protected one-for-one by the
qualified WBD DTLS 1.3 shim before they enter udp2raw.

Impairment is injected *after DTLS encryption and before udp2raw* with 25 ms
per direction. Warmup uses a zero-loss proxy. The product proxy is then
restarted on the same UDP port with the exact V2-M1 product-seed formula so
loss patterns are directly comparable by packet index.
"""
from __future__ import annotations
import argparse, csv, hashlib, heapq, json, math, os, random, re, select, signal, socket, statistics, struct, subprocess, sys, threading, time
from pathlib import Path

CLK = os.sysconf(os.sysconf_names['SC_CLK_TCK'])
EXPECTED_UDP2RAW_SHA='c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c'
EXPECTED_SPEEDER_SHA='f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a'
# Updated by M2B trace-opt-in qualification.
EXPECTED_DTLS_SHIM_SHA='63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a'


def sha256(p: Path) -> str:
    h=hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda:f.read(1<<20), b''): h.update(b)
    return h.hexdigest()

def pct(v,p):
    if not v:return 0.0
    a=sorted(v); i=max(0,min(len(a)-1,math.ceil(p*len(a)/100)-1)); return a[i]

def proc_stats(pid):
    try:
        fields=Path(f'/proc/{pid}/stat').read_text().split(); cpu=(int(fields[13])+int(fields[14]))/CLK; rss=0
        for line in Path(f'/proc/{pid}/status').read_text().splitlines():
            if line.startswith('VmRSS:'): rss=int(line.split()[1]); break
        return cpu,rss
    except Exception:return 0.0,0

def start(cmd, log, merge=True):
    f=open(log,'wb')
    p=subprocess.Popen([str(x) for x in cmd],stdout=f,stderr=subprocess.STDOUT if merge else None,start_new_session=True)
    return p,f

def start_echo(port,log):
    code='''import socket,sys\ns=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(("127.0.0.1",int(sys.argv[1])))\nwhile True:\n d,a=s.recvfrom(65535);s.sendto(d,a)\n'''
    return start([sys.executable,'-c',code,str(port)],log)

def stop_proc(p):
    if not p:return
    try:os.killpg(p.pid,signal.SIGTERM)
    except ProcessLookupError:return
    try:p.wait(timeout=.8)
    except subprocess.TimeoutExpired:
        try:os.killpg(p.pid,signal.SIGKILL)
        except ProcessLookupError:pass
        try:p.wait(timeout=.4)
        except Exception:pass

def proxy_worker(port,target_port,loss,delay_ms,seed,stop,stats):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind(('127.0.0.1',port));s.setblocking(False)
    target=('127.0.0.1',target_port);peer=None;q=[];rng=random.Random(seed);seq=0
    stats.update({'loss_pct':loss,'forward_seen':0,'forward_dropped':0,'forward_bytes':0,'reverse_seen':0,'reverse_bytes':0})
    while not stop.is_set():
        now=time.monotonic()
        while q and q[0][0]<=now:
            _,_,d,dst=heapq.heappop(q)
            try:s.sendto(d,dst)
            except OSError:pass
        timeout=.003 if not q else max(0,min(.003,q[0][0]-now))
        try:r,_,_=select.select([s],[],[],timeout)
        except Exception:continue
        if not r:continue
        try:d,a=s.recvfrom(65535)
        except BlockingIOError:continue
        if a==target:
            stats['reverse_seen']+=1;stats['reverse_bytes']+=len(d)
            if peer is not None:
                heapq.heappush(q,(time.monotonic()+delay_ms/1000,seq,d,peer));seq+=1
        else:
            peer=a;stats['forward_seen']+=1;stats['forward_bytes']+=len(d)
            if rng.random()<loss/100:
                stats['forward_dropped']+=1;continue
            heapq.heappush(q,(time.monotonic()+delay_ms/1000,seq,d,target));seq+=1
    s.close()

def start_proxy(port,target_port,loss,delay_ms,seed):
    stop=threading.Event();stats={};t=threading.Thread(target=proxy_worker,args=(port,target_port,loss,delay_ms,seed,stop,stats),daemon=True);t.start();time.sleep(.04);return stop,t,stats

def stop_proxy(px):
    stop,t,stats=px;stop.set();t.join(timeout=.6);return dict(stats)

def wait_ready(log,role,proc,timeout=12):
    end=time.monotonic()+timeout; needle=f'READY role={role}'
    while time.monotonic()<end:
        try:txt=Path(log).read_text(errors='ignore')
        except Exception:txt=''
        if needle in txt:return
        if proc.poll() is not None:raise RuntimeError(f'{role} DTLS exited rc={proc.returncode}')
        time.sleep(.04)
    raise RuntimeError(f'{role} DTLS not ready')

def ping(port,count,size,window,timeout_ms):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('127.0.0.1',0));s.connect(('127.0.0.1',port));s.setblocking(False)
    sent={};done=set();samples=[];nxt=0;end=time.monotonic()+max(10,count*.02);tail=bytes(max(0,size-4))
    def send(i):sent[i]=time.monotonic();s.send(struct.pack('!I',i)+tail)
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
    s.close();return samples

def parse_dtls_stats(path):
    txt=Path(path).read_text(errors='ignore'); ms=re.findall(r'STATS role=(\w+) plain_in=(\d+) plain_out=(\d+) dtls_writes=(\d+) dtls_records=(\d+) in_bytes=(\d+) out_bytes=(\d+)',txt)
    if not ms:return None
    role,*nums=ms[-1]
    return {'role':role,'plain_in':int(nums[0]),'plain_out':int(nums[1]),'dtls_writes':int(nums[2]),'dtls_records':int(nums[3]),'plain_in_bytes':int(nums[4]),'plain_out_bytes':int(nums[5])}

def load_m1(path):
    if not path:return {}
    rows={}
    with open(path,newline='') as f:
        for r in csv.DictReader(f):rows[(r['fec'],int(r['loss_pct']),int(r['seed']))]=r
    return rows

def start_stack(a,fec,case_dir,base):
    procs=[];files=[]
    p,f=start_echo(base,case_dir/'echo.log');procs.append(('echo',p));files.append(f)
    p,f=start([a.udpspeeder,'-s',f'-l127.0.0.1:{base+1}',f'-r127.0.0.1:{base}',f'-f{fec}','--mode','0','--timeout','8','-k','wbdtest','--disable-color','--log-level','2'],case_dir/'speeder-server.log');procs.append(('speeder-server',p));files.append(f)
    p,f=start([a.dtls_shim,'server',base+2,'127.0.0.1',base+1,a.cert_dir/'server.pem',a.cert_dir/'server.key'],case_dir/'dtls-server.log');procs.append(('dtls-server',p));files.append(f);dtls_server=p
    p,f=start([a.udp2raw,'-s',f'-l127.0.0.1:{base+3}',f'-r127.0.0.1:{base+2}','-k','wbdtest','--raw-mode','faketcp','-a','--disable-color','--log-level','2'],case_dir/'udp2raw-server.log');procs.append(('udp2raw-server',p));files.append(f)
    time.sleep(.12)
    p,f=start([a.udp2raw,'-c',f'-l127.0.0.1:{base+4}',f'-r127.0.0.1:{base+3}','-k','wbdtest','--raw-mode','faketcp','-a','--source-ip','127.0.0.1','--source-port',str(base+10),'--disable-color','--log-level','2'],case_dir/'udp2raw-client.log');procs.append(('udp2raw-client',p));files.append(f)
    time.sleep(.22)
    warm_proxy=start_proxy(base+6,base+4,0,25,260824)
    p,f=start([a.dtls_shim,'client',base+5,'127.0.0.1',base+6,a.cert_dir/'ca.pem','wbd.test'],case_dir/'dtls-client.log');procs.append(('dtls-client',p));files.append(f);dtls_client=p
    wait_ready(case_dir/'dtls-server.log','server',dtls_server);wait_ready(case_dir/'dtls-client.log','client',dtls_client)
    p,f=start([a.udpspeeder,'-c',f'-l127.0.0.1:{base+7}',f'-r127.0.0.1:{base+5}',f'-f{fec}','--mode','0','--timeout','8','-k','wbdtest','--disable-color','--log-level','2'],case_dir/'speeder-client.log');procs.append(('speeder-client',p));files.append(f)
    time.sleep(.42)
    return procs,files,warm_proxy

def stop_stack(procs,files):
    for _,p in reversed(procs):stop_proc(p)
    for f in files:
        try:f.close()
        except Exception:pass
    time.sleep(.06)

def run_case(a,fec,loss,seed,m1):
    case_dir=a.out/f'{fec.replace(":","-")}_loss{loss}_seed{seed}';case_dir.mkdir(parents=True,exist_ok=True)
    parity=int(fec.split(':')[1]);fec_idx=0 if fec=='20:10' else 1;base=42000+fec_idx*6000+loss*100+(seed-260824)*20
    procs=files=None;warm_proxy=None;product_proxy=None;row=None
    try:
        procs,files,warm_proxy=start_stack(a,fec,case_dir,base)
        warm=ping(base+7,64,256,1,1500)
        stop_proxy(warm_proxy);warm_proxy=None;time.sleep(.04)
        if len(warm)!=64:raise RuntimeError(f'warmup delivery {len(warm)}/64')
        product_seed=seed+loss*1009+parity*17
        product_proxy=start_proxy(base+6,base+4,loss,25,product_seed)
        product=[(n,p) for n,p in procs if n!='echo'];dtls=[p for n,p in product if n.startswith('dtls-')];other=[p for n,p in product if not n.startswith('dtls-')]
        cpu0={p.pid:proc_stats(p.pid)[0] for _,p in product};maxrss=[];monstop=threading.Event()
        def monitor():
            while not monstop.is_set():maxrss.append(sum(proc_stats(p.pid)[1] for _,p in product));time.sleep(.01)
        mt=threading.Thread(target=monitor,daemon=True);mt.start();samples=ping(base+7,200,256,32,1500);monstop.set();mt.join(.2)
        cpu1={p.pid:proc_stats(p.pid)[0] for _,p in product};st=stop_proxy(product_proxy);product_proxy=None
        cpu_total=max(0,sum(cpu1.values())-sum(cpu0.values()));cpu_dtls=max(0,sum(cpu1[p.pid]-cpu0[p.pid] for p in dtls));cpu_other=max(0,sum(cpu1[p.pid]-cpu0[p.pid] for p in other))
        delivered=len(samples);src=200*256
        row={'fec':fec,'loss_pct':loss,'seed':seed,'samples':200,'delivered':delivered,'delivery_ratio':delivered/200,'mean_ms':sum(samples)/delivered if delivered else 0,'p50_ms':pct(samples,50),'p95_ms':pct(samples,95),'p99_ms':pct(samples,99),'late_ratio':sum(x>100 for x in samples)/delivered if delivered else 1,'encrypted_packets_seen':st.get('forward_seen',0),'encrypted_packets_dropped':st.get('forward_dropped',0),'encrypted_forward_bytes':st.get('forward_bytes',0),'encrypted_reverse_bytes':st.get('reverse_bytes',0),'encrypted_forward_traffic_x':st.get('forward_bytes',0)/src,'cpu_ms_product':cpu_total*1000,'cpu_ms_dtls':cpu_dtls*1000,'cpu_ms_raw_fec':cpu_other*1000,'rss_peak_kb_product':max(maxrss) if maxrss else 0,'warmup_delivery_ratio':1.0,'product_proxy_seed':product_seed}
        mr=m1.get((fec,loss,seed))
        if mr:
            m1bytes=int(float(mr['fec_forward_bytes']));row['m1_fec_forward_bytes']=m1bytes;row['security_bytes_delta_vs_m1']=st.get('forward_bytes',0)-m1bytes;row['security_overhead_vs_m1_fec_pct']=(st.get('forward_bytes',0)-m1bytes)/m1bytes*100 if m1bytes else 0;row['m1_p99_ms']=float(mr['p99_ms']);row['p99_delta_vs_m1_ms']=row['p99_ms']-float(mr['p99_ms'])
    finally:
        if product_proxy:stop_proxy(product_proxy)
        if warm_proxy:stop_proxy(warm_proxy)
        if procs:stop_stack(procs,files)
    dc=parse_dtls_stats(case_dir/'dtls-client.log');ds=parse_dtls_stats(case_dir/'dtls-server.log')
    if row is not None:
        row['dtls_client_total_records']=dc['dtls_records'] if dc else 0;row['dtls_server_total_records']=ds['dtls_records'] if ds else 0
        (case_dir/'result.json').write_text(json.dumps(row,indent=2)+'\n')
    return row

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--udp2raw',required=True,type=Path);ap.add_argument('--udpspeeder',required=True,type=Path);ap.add_argument('--dtls-shim',required=True,type=Path);ap.add_argument('--cert-dir',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);ap.add_argument('--fec',default='20:20');ap.add_argument('--losses',default='0,1,5,10,15');ap.add_argument('--seeds',default='260824,260825,260826');ap.add_argument('--m1-results',type=Path)
    a=ap.parse_args();a.udp2raw=a.udp2raw.resolve();a.udpspeeder=a.udpspeeder.resolve();a.dtls_shim=a.dtls_shim.resolve();a.cert_dir=a.cert_dir.resolve();a.out=a.out.resolve();a.out.mkdir(parents=True,exist_ok=True)
    if sha256(a.udp2raw)!=EXPECTED_UDP2RAW_SHA:raise SystemExit('udp2raw sha mismatch')
    if sha256(a.udpspeeder)!=EXPECTED_SPEEDER_SHA:raise SystemExit('UDPspeeder sha mismatch')
    if sha256(a.dtls_shim)!=EXPECTED_DTLS_SHIM_SHA:raise SystemExit('DTLS shim sha mismatch')
    if a.fec not in ('20:10','20:20'):raise SystemExit('unsupported fec')
    m1=load_m1(a.m1_results);rows=[]
    for loss in [int(x) for x in a.losses.split(',') if x.strip()]:
        for seed in [int(x) for x in a.seeds.split(',') if x.strip()]:
            r=run_case(a,a.fec,loss,seed,m1);rows.append(r);print(json.dumps(r),flush=True)
    fields=[]
    for r in rows:
        for k in r:
            if k not in fields:fields.append(k)
    with open(a.out/'results.csv','w',newline='') as f:w=csv.DictWriter(f,fieldnames=fields);w.writeheader();w.writerows(rows)
    metrics=['mean_ms','p50_ms','p95_ms','p99_ms','late_ratio','delivery_ratio','encrypted_forward_traffic_x','cpu_ms_product','cpu_ms_dtls','rss_peak_kb_product']
    groups={}
    for r in rows:groups.setdefault((r['fec'],r['loss_pct']),[]).append(r)
    with open(a.out/'median.csv','w',newline='') as f:
        w=csv.writer(f);w.writerow(['fec','loss_pct']+metrics)
        for k in sorted(groups,key=lambda x:(x[0],x[1])):
            rs=groups[k];w.writerow([k[0],k[1]]+[statistics.median(float(r[m]) for r in rs) for m in metrics])
    print('---MEDIAN---');print((a.out/'median.csv').read_text())
if __name__=='__main__':main()
