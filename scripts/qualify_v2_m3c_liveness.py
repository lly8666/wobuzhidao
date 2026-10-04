#!/usr/bin/env python3
from __future__ import annotations
import argparse, hashlib, json, pathlib, socket, subprocess, time

EXPECTED_SHIM_SHA='63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a'
EXPECTED_VERSION='DTLSv1.3'
EXPECTED_CIPHER='TLS_AES_256_GCM_SHA384'

def sha256(p:pathlib.Path)->str:
    h=hashlib.sha256()
    with p.open('rb') as f:
        for b in iter(lambda:f.read(1<<20), b''): h.update(b)
    return h.hexdigest()

def free_port()->int:
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); p=s.getsockname()[1]; s.close(); return p

def wait_ready(path:pathlib.Path, role:str, proc:subprocess.Popen, timeout=8.0):
    end=time.monotonic()+timeout; needle=f'READY role={role}'
    while time.monotonic()<end:
        if path.exists() and needle in path.read_text(errors='replace'): return
        if proc.poll() is not None: raise RuntimeError(f'{role} shim exited rc={proc.returncode}')
        time.sleep(.02)
    raise TimeoutError(f'{role} READY timeout')

def stop(p):
    if p is None or p.poll() is not None: return
    p.terminate()
    try:p.wait(2)
    except subprocess.TimeoutExpired:p.kill();p.wait()

def run_case(root,shim,certs,out,name,token,nonce,expect_auth_required):
    target,transport,plain=free_port(),free_port(),free_port()
    logs={k:out/f'{name}.{k}.log' for k in ('control-server','shim-server-out','shim-server-err','shim-client-out','shim-client-err','control-client')}
    server=ss=sc=None
    try:
        server_cmd=[str(root/'wbd-control-probe'),'-mode','server','-addr',f'127.0.0.1:{target}','-serve-ping']
        if token: server_cmd += ['-expected-token',token]
        with logs['control-server'].open('w') as f: server=subprocess.Popen(server_cmd,stdout=f,stderr=subprocess.STDOUT)
        time.sleep(.05)
        sso=logs['shim-server-out'].open('w');sse=logs['shim-server-err'].open('w')
        ss=subprocess.Popen([str(shim),'server',str(transport),'127.0.0.1',str(target),str(certs/'server.pem'),str(certs/'server.key')],stdout=sso,stderr=sse)
        time.sleep(.05)
        cso=logs['shim-client-out'].open('w');cse=logs['shim-client-err'].open('w')
        sc=subprocess.Popen([str(shim),'client',str(plain),'127.0.0.1',str(transport),str(certs/'ca.pem'),'wbd.test'],stdout=cso,stderr=cse)
        wait_ready(logs['shim-server-out'],'server',ss);wait_ready(logs['shim-client-out'],'client',sc)
        cmd=[str(root/'wbd-control-probe'),'-mode','client','-addr',f'127.0.0.1:{plain}','-ping-nonce',str(nonce)]
        if token: cmd += ['-token',token]
        cp=subprocess.run(cmd,text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=5)
        logs['control-client'].write_text(cp.stdout)
        if cp.returncode!=0: raise RuntimeError(f'{name}: client rc={cp.returncode}: {cp.stdout}')
        server.wait(5)
        if server.returncode!=0: raise RuntimeError(f'{name}: server rc={server.returncode}: {logs["control-server"].read_text(errors="replace")}')
        if f'CLIENT reply=PONG nonce={nonce}' not in cp.stdout: raise RuntimeError(f'{name}: missing pong: {cp.stdout}')
        st=logs['control-server'].read_text(errors='replace')
        if 'pings=1 pongs=1' not in st: raise RuntimeError(f'{name}: bad stats: {st}')
        marker=f'auth_required={str(expect_auth_required).lower()} authenticated=true'
        if marker not in st: raise RuntimeError(f'{name}: auth stats missing {marker}: {st}')
        for p in (logs['shim-server-out'],logs['shim-client-out']):
            txt=p.read_text(errors='replace')
            if EXPECTED_VERSION not in txt or EXPECTED_CIPHER not in txt: raise RuntimeError(f'{name}: wrong DTLS identity')
        return {'name':name,'auth_required':expect_auth_required,'nonce':nonce,'client_output':cp.stdout.strip(),'server_output':st.strip(),'dtls_ready_before_session_control':True}
    finally:
        stop(sc);stop(ss);stop(server)

def main():
    ap=argparse.ArgumentParser();ap.add_argument('repo_root');ap.add_argument('shim');ap.add_argument('cert_dir');ap.add_argument('out_dir');ns=ap.parse_args()
    root=pathlib.Path(ns.repo_root).resolve();shim=pathlib.Path(ns.shim).resolve();certs=pathlib.Path(ns.cert_dir).resolve();out=pathlib.Path(ns.out_dir).resolve();out.mkdir(parents=True,exist_ok=True)
    actual=sha256(shim)
    if actual!=EXPECTED_SHIM_SHA: raise SystemExit(f'shim SHA mismatch {actual}')
    subprocess.run(['go','test','./internal/control','./cmd/wbd-control-probe'],cwd=root,check=True)
    subprocess.run(['go','build','-o',str(root/'wbd-control-probe'),'./cmd/wbd-control-probe'],cwd=root,check=True)
    token='m3c-static-test-token'
    cases=[run_case(root,shim,certs,out,'auth-disabled','',0x1111222233334444,False),run_case(root,shim,certs,out,'auth-token',token,0x5555666677778888,True)]
    receipt={'schema':'wbd-v2-m3c-liveness-qualification/v1','result':'pass','authority':'local sandbox execution','shim_sha256':actual,'dtls':{'version':EXPECTED_VERSION,'cipher':EXPECTED_CIPHER,'session_control_started_only_after_both_ready':True},'liveness':{'ping_body_bytes':8,'pong_echoes_nonce':True,'server_stats_include_wire_bytes':True,'idle_policy':'caller-supplied monotonic ticks; no hidden timer/retransmission'},'cases':cases,'scope':'M3C liveness/stats only; no reconnect, TUN, FEC or carrier changes'}
    (out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n');print(json.dumps(receipt,indent=2))
if __name__=='__main__':main()
