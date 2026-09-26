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
    needle=f'READY role={role}'
    end=time.monotonic()+timeout
    while time.monotonic()<end:
        if path.exists() and needle in path.read_text(errors='replace'): return
        if proc.poll() is not None: raise RuntimeError(f'{role} shim exited rc={proc.returncode}: {path.read_text(errors="replace") if path.exists() else ""}')
        time.sleep(.02)
    raise TimeoutError(f'{role} READY timeout')

def stop(p):
    if p is None or p.poll() is not None: return
    p.terminate()
    try: p.wait(2)
    except subprocess.TimeoutExpired: p.kill(); p.wait()

def run_case(root:pathlib.Path, shim:pathlib.Path, certs:pathlib.Path, out:pathlib.Path, name:str, minv:int, maxv:int, expected:str):
    target, transport, plain = free_port(), free_port(), free_port()
    files={k:out/f'{name}.{k}.log' for k in ('control-server','shim-server-out','shim-server-err','shim-client-out','shim-client-err','control-client')}
    server_probe=shim_server=shim_client=None
    try:
        with files['control-server'].open('w') as so:
            server_probe=subprocess.Popen([str(root/'wbd-control-probe'),'-mode','server','-addr',f'127.0.0.1:{target}'],stdout=so,stderr=subprocess.STDOUT)
        time.sleep(.05)
        sso=files['shim-server-out'].open('w'); sse=files['shim-server-err'].open('w')
        shim_server=subprocess.Popen([str(shim),'server',str(transport),'127.0.0.1',str(target),str(certs/'server.pem'),str(certs/'server.key')],stdout=sso,stderr=sse)
        time.sleep(.05)
        cso=files['shim-client-out'].open('w'); cse=files['shim-client-err'].open('w')
        shim_client=subprocess.Popen([str(shim),'client',str(plain),'127.0.0.1',str(transport),str(certs/'ca.pem'),'wbd.test'],stdout=cso,stderr=cse)
        wait_ready(files['shim-server-out'],'server',shim_server)
        wait_ready(files['shim-client-out'],'client',shim_client)
        cp=subprocess.run([str(root/'wbd-control-probe'),'-mode','client','-addr',f'127.0.0.1:{plain}','-min',str(minv),'-max',str(maxv)],text=True,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,timeout=5)
        files['control-client'].write_text(cp.stdout)
        if cp.returncode != 0: raise RuntimeError(f'control client rc={cp.returncode}: {cp.stdout}')
        server_probe.wait(5)
        if server_probe.returncode != 0: raise RuntimeError(f'control server rc={server_probe.returncode}: {files["control-server"].read_text(errors="replace")}')
        if expected not in cp.stdout: raise RuntimeError(f'{name}: expected {expected!r}, got {cp.stdout!r}')
        for p in (files['shim-server-out'],files['shim-client-out']):
            txt=p.read_text(errors='replace')
            if EXPECTED_VERSION not in txt or EXPECTED_CIPHER not in txt: raise RuntimeError(f'{name}: wrong DTLS identity in {p}: {txt}')
        return {'name':name,'min':minv,'max':maxv,'expected':expected,'client_output':cp.stdout.strip(),'server_output':files['control-server'].read_text(errors='replace').strip(),'dtls_ready_before_control_client':True}
    finally:
        stop(shim_client); stop(shim_server); stop(server_probe)

def main():
    ap=argparse.ArgumentParser(); ap.add_argument('repo_root'); ap.add_argument('shim'); ap.add_argument('cert_dir'); ap.add_argument('out_dir'); ns=ap.parse_args()
    root=pathlib.Path(ns.repo_root).resolve(); shim=pathlib.Path(ns.shim).resolve(); certs=pathlib.Path(ns.cert_dir).resolve(); out=pathlib.Path(ns.out_dir).resolve(); out.mkdir(parents=True,exist_ok=True)
    actual=sha256(shim)
    if actual!=EXPECTED_SHIM_SHA: raise SystemExit(f'shim SHA mismatch {actual}')
    subprocess.run(['go','test','./internal/control','./cmd/wbd-control-probe'],cwd=root,check=True)
    subprocess.run(['go','build','-o',str(root/'wbd-control-probe'),'./cmd/wbd-control-probe'],cwd=root,check=True)
    cases=[
        run_case(root,shim,certs,out,'accept',1,1,'CLIENT reply=ACCEPT protocol=1'),
        run_case(root,shim,certs,out,'unsupported',2,2,'CLIENT reply=ERROR code=1'),
    ]
    receipt={'schema':'wbd-v2-m3a-control-qualification/v1','result':'pass','authority':'local sandbox execution','shim_sha256':actual,'dtls':{'version':EXPECTED_VERSION,'cipher':EXPECTED_CIPHER,'control_started_only_after_both_ready':True},'cases':cases,'scope':'direct DTLS loopback only; no credentials, keepalive, TUN, FEC or FakeTCP changes'}
    (out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
    print(json.dumps(receipt,indent=2))
if __name__=='__main__': main()
