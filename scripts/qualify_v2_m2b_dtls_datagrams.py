#!/usr/bin/env python3
from __future__ import annotations
import argparse, hashlib, json, os, pathlib, queue, socket, subprocess, sys, threading, time

EXPECTED_SOURCE_SHA = '4a7ff40a32db0d7a262aaea2d2e674da6708250cba908441c737c981fc84f88b'
EXPECTED_COMMIT = 'ac01707f552c611fbd135cc723b2682b3e7f80f2'
EXPECTED_VERSION = 'DTLSv1.3'
EXPECTED_CIPHER = 'TLS_AES_256_GCM_SHA384'


def sha256(path: pathlib.Path) -> str:
    h=hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda:f.read(1<<20), b''): h.update(b)
    return h.hexdigest()

def free_port() -> int:
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); p=s.getsockname()[1]; s.close(); return p

def wait_ready(path:pathlib.Path, role:str, proc:subprocess.Popen, timeout=8.0):
    dead=time.monotonic()+timeout
    needle=f'READY role={role}'
    while time.monotonic()<dead:
        if path.exists() and needle in path.read_text(errors='replace'): return
        if proc.poll() is not None: raise RuntimeError(f'{role} exited rc={proc.returncode}')
        time.sleep(.02)
    raise TimeoutError(f'{role} not ready')

def terminate(p:subprocess.Popen|None):
    if p is None or p.poll() is not None: return
    p.terminate()
    try:p.wait(3)
    except subprocess.TimeoutExpired:
        p.kill(); p.wait()

class Echo(threading.Thread):
    def __init__(self,port): super().__init__(daemon=True); self.port=port; self.stop_evt=threading.Event(); self.sock=None
    def run(self):
        s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); self.sock=s; s.bind(('127.0.0.1',self.port)); s.settimeout(.1)
        while not self.stop_evt.is_set():
            try:d,a=s.recvfrom(65535)
            except socket.timeout: continue
            except OSError: break
            s.sendto(d,a)
        s.close()
    def stop(self): self.stop_evt.set(); self.join(1)

class Proxy(threading.Thread):
    def __init__(self,listen_port,server_port):
        super().__init__(daemon=True); self.listen=('127.0.0.1',listen_port); self.server=('127.0.0.1',server_port)
        self.ready=threading.Event(); self.arm=threading.Event(); self.stop_evt=threading.Event(); self.events=[]; self.drop=None; self.sock=None
    def run(self):
        s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); self.sock=s; s.bind(self.listen); s.settimeout(.1); self.ready.set(); client=None; idx=0
        while not self.stop_evt.is_set():
            try:data,src=s.recvfrom(65535)
            except socket.timeout: continue
            except OSError: break
            idx+=1; ns=time.monotonic_ns()
            if src==self.server: direction='s2c'; dst=client
            else: direction='c2s'; client=src; dst=self.server
            ev={'index':idx,'ns':ns,'direction':direction,'len':len(data),'prefix':data[:12].hex(),'action':'forward'}
            if direction=='c2s' and self.arm.is_set() and self.drop is None:
                ev['action']='drop'; self.drop=ev.copy(); self.events.append(ev); self.arm.clear(); continue
            self.events.append(ev)
            if dst is not None: s.sendto(data,dst)
        s.close()
    def stop(self): self.stop_evt.set(); self.join(1)

def start_shim(shim, args, out, err):
    return subprocess.Popen([str(shim),*map(str,args)],stdout=out.open('w'),stderr=err.open('w'))

def app_roundtrip(plain_port:int,messages:list[bytes],expect:set[bytes],timeout=2.0):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); s.settimeout(.15)
    for m in messages:
        s.sendto(m,('127.0.0.1',plain_port)); time.sleep(.03)
    seen=[]; dead=time.monotonic()+timeout
    while time.monotonic()<dead and set(seen)!=expect:
        try:d,_=s.recvfrom(65535); seen.append(d)
        except socket.timeout: pass
    s.close(); return seen

def compile_shim(src, m2a, out):
    inc=m2a/'install/include'; lib=m2a/'install/lib/libwolfssl.a'; cert=m2a/'certs/server.pem'
    if not lib.exists() or not cert.exists(): raise RuntimeError('M2A output missing build/certs')
    receipt_path=m2a/'receipt.json'
    if not receipt_path.exists(): raise RuntimeError('M2A receipt missing')
    m2a_receipt=json.loads(receipt_path.read_text())
    if m2a_receipt.get('result')!='pass': raise RuntimeError('M2A receipt is not pass')
    source=m2a_receipt.get('source') or {}; build=m2a_receipt.get('build') or {}
    if source.get('commit')!=EXPECTED_COMMIT or source.get('git_archive_sha256')!=EXPECTED_SOURCE_SHA:
        raise RuntimeError('M2A source identity mismatch')
    if build.get('flags')!=['--enable-dtls13','--disable-shared','--enable-static']:
        raise RuntimeError('M2A build flags mismatch')
    actual=sha256(lib)
    if actual!=build.get('libwolfssl_a_sha256'): raise RuntimeError(f'libwolfssl vs M2A receipt mismatch {actual}')
    shim=out/'wbd_dtls_shim'
    subprocess.run(['cc','-std=c11','-O2','-Wall','-Wextra','-Werror',f'-I{inc}',str(src),str(lib),'-lm','-o',str(shim)],check=True)
    return shim, actual

def main():
    ap=argparse.ArgumentParser(); ap.add_argument('m2a_out'); ap.add_argument('out_dir'); ap.add_argument('--shim-src',default=None); ns=ap.parse_args()
    root=pathlib.Path(__file__).resolve().parents[1] if 'scripts' in pathlib.Path(__file__).parts else pathlib.Path.cwd()
    src=pathlib.Path(ns.shim_src) if ns.shim_src else root/'native/dtls/wbd_dtls_shim.c'
    m2a=pathlib.Path(ns.m2a_out).resolve(); out=pathlib.Path(ns.out_dir).resolve(); out.mkdir(parents=True,exist_ok=True)
    shim,lib_sha=compile_shim(src,m2a,out); shim_sha=sha256(shim); certs=m2a/'certs'

    # direct gate
    echo_port, srv_port, plain_port = free_port(), free_port(), free_port(); echo=Echo(echo_port); echo.start(); time.sleep(.05)
    sd=out/'direct.server.out'; se=out/'direct.server.err'; cd=out/'direct.client.out'; ce=out/'direct.client.err'; sp=cp=None
    try:
        sp=start_shim(shim,['server',srv_port,'127.0.0.1',echo_port,certs/'server.pem',certs/'server.key'],sd,se); time.sleep(.05)
        cp=start_shim(shim,['client',plain_port,'127.0.0.1',srv_port,certs/'ca.pem','wbd.test'],cd,ce)
        wait_ready(sd,'server',sp); wait_ready(cd,'client',cp)
        msgs=[f'direct-{i:02d}'.encode() for i in range(1,6)]; seen=app_roundtrip(plain_port,msgs,set(msgs))
        if set(seen)!=set(msgs): raise RuntimeError(f'direct mismatch {seen!r}')
    finally:
        terminate(cp); terminate(sp); echo.stop()

    # deterministic drop gate
    echo_port, srv_port, proxy_port, plain_port = free_port(), free_port(), free_port(), free_port(); echo=Echo(echo_port); echo.start(); proxy=Proxy(proxy_port,srv_port); proxy.start();
    if not proxy.ready.wait(2): raise RuntimeError('proxy not ready')
    sd2=out/'drop.server.out'; se2=out/'drop.server.err'; cd2=out/'drop.client.out'; ce2=out/'drop.client.err'; sp=cp=None
    try:
        sp=start_shim(shim,['server',srv_port,'127.0.0.1',echo_port,certs/'server.pem',certs/'server.key'],sd2,se2); time.sleep(.05)
        cp=start_shim(shim,['client',plain_port,'127.0.0.1',proxy_port,certs/'ca.pem','wbd.test'],cd2,ce2)
        wait_ready(sd2,'server',sp); wait_ready(cd2,'client',cp)
        time.sleep(.5); before=len(proxy.events); proxy.arm.set()
        msgs=[b'msg-01',b'msg-02',b'msg-03']; seen=app_roundtrip(plain_port,msgs,{b'msg-02',b'msg-03'})
        if b'msg-01' in seen or b'msg-02' not in seen or b'msg-03' not in seen: raise RuntimeError(f'drop independence failed {seen!r}')
        dead=time.monotonic()+1
        while proxy.drop is None and time.monotonic()<dead: time.sleep(.01)
        if proxy.drop is None: raise RuntimeError('proxy never dropped')
        drop=proxy.drop.copy(); drop['events_before_arm']=before
    finally:
        terminate(cp); terminate(sp); proxy.stop(); echo.stop()

    for p in (cd,sd,cd2,sd2):
        txt=p.read_text(errors='replace')
        if EXPECTED_VERSION not in txt or EXPECTED_CIPHER not in txt: raise RuntimeError(f'wrong DTLS identity in {p}')
    direct_client_err=ce.read_text(); direct_server_err=se.read_text(); drop_client_err=ce2.read_text(); drop_server_err=se2.read_text()
    for needle in ('plain_in=5 plain_out=5 dtls_writes=5 dtls_records=5',):
        if needle not in direct_client_err or needle not in direct_server_err: raise RuntimeError('direct stats mismatch')
    if 'plain_in=3 plain_out=2 dtls_writes=3 dtls_records=2' not in drop_client_err: raise RuntimeError('drop client stats mismatch')
    if 'plain_in=2 plain_out=2 dtls_writes=2 dtls_records=2' not in drop_server_err: raise RuntimeError('drop server stats mismatch')
    (out/'proxy-events.json').write_text(json.dumps(proxy.events,indent=2)+'\n')
    receipt={
      'schema':'wbd-v2-m2b-dtls-datagram-qualification/v1','result':'pass','authority':'local sandbox execution',
      'wolfssl':{'libwolfssl_a_sha256':lib_sha,'version':EXPECTED_VERSION,'cipher':EXPECTED_CIPHER},
      'shim':{'source_sha256':sha256(src),'binary_sha256':shim_sha,'mapping':'one plaintext UDP recv -> one wolfSSL_write; one successful wolfSSL_read -> one plaintext UDP send'},
      'direct':{'sent':5,'delivered':5,'bidirectional_echo':True},
      'drop_gate':{'sent':['msg-01','msg-02','msg-03'],'delivered':[x.decode() for x in seen],'dropped_application_datagram':drop,'conclusion':'later DTLS application records delivered despite one missing earlier encrypted application datagram'},
      'scope':'plain UDP only; no UDPspeeder/FEC/FakeTCP in M2B'
    }
    (out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    print(json.dumps(receipt,indent=2))

if __name__=='__main__': main()
