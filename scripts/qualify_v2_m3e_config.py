#!/usr/bin/env python3
from __future__ import annotations
import argparse, hashlib, json, pathlib, socket, subprocess, time

EXPECTED_SHIM_SHA='63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a'
EXPECTED_VERSION='DTLSv1.3'; EXPECTED_CIPHER='TLS_AES_256_GCM_SHA384'

def sha256(p):
    h=hashlib.sha256()
    with pathlib.Path(p).open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def free_port():
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('127.0.0.1',0));p=s.getsockname()[1];s.close();return p
def wait_ready(path,role,proc):
    end=time.monotonic()+8;needle=f'READY role={role}'
    while time.monotonic()<end:
        if path.exists() and needle in path.read_text(errors='replace'):return
        if proc.poll() is not None:raise RuntimeError(f'{role} exited {proc.returncode}')
        time.sleep(.02)
    raise TimeoutError(role)
def stop(p):
    if p is None or p.poll() is not None:return
    p.terminate()
    try:p.wait(2)
    except subprocess.TimeoutExpired:p.kill();p.wait()
def main():
    ap=argparse.ArgumentParser();ap.add_argument('repo_root');ap.add_argument('shim');ap.add_argument('cert_dir');ap.add_argument('out_dir');ns=ap.parse_args()
    root=pathlib.Path(ns.repo_root).resolve();shim=pathlib.Path(ns.shim).resolve();certs=pathlib.Path(ns.cert_dir).resolve();out=pathlib.Path(ns.out_dir).resolve();out.mkdir(parents=True,exist_ok=True)
    actual=sha256(shim)
    if actual!=EXPECTED_SHIM_SHA:raise SystemExit(f'shim SHA mismatch {actual}')
    subprocess.run(['go','test','./internal/control','./cmd/wbd-config-probe'],cwd=root,check=True)
    probe=root/'wbd-config-probe';subprocess.run(['go','build','-o',str(probe),'./cmd/wbd-config-probe'],cwd=root,check=True)
    target,transport,plain=free_port(),free_port(),free_port();token='m3e-static-test-token';mode='weak-1.5x'
    logs={k:out/f'{k}.log' for k in ('control-server','shim-server-out','shim-server-err','shim-client-out','shim-client-err','control-client')}
    server=ss=sc=None; opened=[]
    try:
        f=logs['control-server'].open('w');opened.append(f);server=subprocess.Popen([str(probe),'-mode','server','-addr',f'127.0.0.1:{target}','-expected-token',token],stdout=f,stderr=subprocess.STDOUT)
        time.sleep(.05)
        sso=logs['shim-server-out'].open('w');sse=logs['shim-server-err'].open('w');opened += [sso,sse]
        ss=subprocess.Popen([str(shim),'server',str(transport),'127.0.0.1',str(target),str(certs/'server.pem'),str(certs/'server.key')],stdout=sso,stderr=sse)
        time.sleep(.05)
        cso=logs['shim-client-out'].open('w');cse=logs['shim-client-err'].open('w');opened += [cso,cse]
        sc=subprocess.Popen([str(shim),'client',str(plain),'127.0.0.1',str(transport),str(certs/'ca.pem'),'wbd.test'],stdout=cso,stderr=cse)
        wait_ready(logs['shim-server-out'],'server',ss);wait_ready(logs['shim-client-out'],'client',sc)
        cp=subprocess.run([str(probe),'-mode','client','-addr',f'127.0.0.1:{plain}','-token',token,'-config-mode',mode],text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=5)
        logs['control-client'].write_text(cp.stdout)
        if cp.returncode!=0:raise RuntimeError(cp.stdout)
        server.wait(5)
        st=logs['control-server'].read_text(errors='replace')
        if 'CLIENT configured=true protection_mode=weak-1.5x' not in cp.stdout or f'CLIENT configured=true protection_mode={mode}' not in cp.stdout:raise RuntimeError(cp.stdout)
        if 'configured=true protection_mode=weak-1.5x' not in st:raise RuntimeError(st)
        for p in (logs['shim-server-out'],logs['shim-client-out']):
            txt=p.read_text(errors='replace')
            if EXPECTED_VERSION not in txt or EXPECTED_CIPHER not in txt:raise RuntimeError('DTLS identity')
        # CLI policy check: auto must be rejected before any frame is emitted.
        auto=subprocess.run([str(probe),'-mode','client','-addr','127.0.0.1:9','-config-mode','auto'],text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=2)
        if auto.returncode==0 or 'auto protection mode is not admitted' not in auto.stdout:
            raise RuntimeError(f'auto policy not rejected: {auto.returncode} {auto.stdout!r}')
        receipt={'schema':'wbd-v2-m3e-config-qualification/v1','result':'pass','authority':'local sandbox execution','shim_sha256':actual,'dtls':{'version':EXPECTED_VERSION,'cipher':EXPECTED_CIPHER,'session_control_started_only_after_both_ready':True},'sequence':['HELLO/ACCEPT','AUTH/AUTH_OK','CONFIG weak-1.5x/CONFIG_OK'],'selected_mode':mode,'server_output':st.strip(),'client_output':cp.stdout.strip(),'auto':'reserved and rejected; not admitted','scope':'M3E one-time fixed-mode negotiation only; no UDPspeeder reconfiguration, TUN, two-lane or Auto'}
        (out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt,indent=2))
    finally:
        stop(sc);stop(ss);stop(server)
        for f in opened:
            try:f.close()
            except Exception:pass
if __name__=='__main__':main()
