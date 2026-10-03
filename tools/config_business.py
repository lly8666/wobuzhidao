"""Actual DNS, plain TCP and inner HTTPS over the product TPROXY path."""
import argparse,hashlib,json,socket,ssl,threading,time
from pathlib import Path

BODY=bytes(range(256))*400
QUERY=bytes.fromhex('123401000001000000000000047465737404776264300000010001')
ANSWER=QUERY[:2]+bytes.fromhex('81800001000100000000')+QUERY[12:]+bytes.fromhex('c00c000100010000003c00040a320002')

def server(bind,cert,key):
    ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER);ctx.load_cert_chain(cert,key)
    peers={}
    def dns():
        with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as s:
            s.bind((bind,15353));s.settimeout(40);q,peer=s.recvfrom(4096)
            peers['dns']=peer[0]
            if q!=QUERY:raise ValueError('DNS query differs')
            s.sendto(ANSWER,peer)
    def plain():
        with socket.socket() as s:
            s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
            s.bind((bind,18444));s.listen();s.settimeout(40);c,peer=s.accept();peers['tcp']=peer[0]
            with c:
                c.settimeout(15);data=c.recv(1024)
                if data!=b'wbd-plain-tcp-probe':raise ValueError('TCP payload differs')
                c.sendall(data)
    errors=[]
    def guarded(fn):
        try:fn()
        except Exception as e:errors.append(str(e))
    ts=[threading.Thread(target=guarded,args=(fn,)) for fn in (dns,plain)]
    for t in ts:t.start()
    with socket.socket() as s:
        s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind((bind,18443));s.listen();s.settimeout(40);c,peer=s.accept();peers['https']=peer[0]
        with ctx.wrap_socket(c,server_side=True) as c:
            c.settimeout(20);request=b''
            while b'\r\n\r\n' not in request:
                request+=c.recv(4096)
                if len(request)>8192:raise ValueError('HTTP request too long')
            if not request.startswith(b'GET /qualification HTTP/1.1\r\n'):raise ValueError('HTTP request differs')
            c.sendall(b'HTTP/1.1 200 OK\r\nContent-Length: '+str(len(BODY)).encode()+b'\r\nConnection: close\r\n\r\n'+BODY)
    for t in ts:t.join()
    if errors:raise ValueError(errors)
    return dict(result='PASS',role='target',body_sha256=hashlib.sha256(BODY).hexdigest(),peers=peers)

def client(peer,cert):
    with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as s:
        s.settimeout(15);s.sendto(QUERY,(peer,15353));answer,_=s.recvfrom(4096)
        if answer!=ANSWER:raise ValueError('DNS answer differs')
    with socket.create_connection((peer,18444),15) as s:
        s.sendall(b'wbd-plain-tcp-probe');data=b''
        while len(data)<19:
            part=s.recv(1024)
            if not part:break
            data+=part
        if data!=b'wbd-plain-tcp-probe':raise ValueError('plain TCP answer differs')
    ctx=ssl.create_default_context(cafile=cert)
    with socket.create_connection((peer,18443),15) as raw:
        with ctx.wrap_socket(raw,server_hostname='qual.test') as s:
            s.settimeout(20);s.sendall(b'GET /qualification HTTP/1.1\r\nHost: qual.test\r\nConnection: close\r\n\r\n');data=b''
            while True:
                part=s.recv(16384)
                if not part:break
                data+=part
                if len(data)>len(BODY)+8192:raise ValueError('HTTP response too large')
            protocol=s.version()
    head,body=data.split(b'\r\n\r\n',1)
    if not head.startswith(b'HTTP/1.1 200 OK') or body!=BODY:
        mismatch=next((i for i,(a,b) in enumerate(zip(body,BODY)) if a!=b),None)
        raise ValueError(f'HTTPS content differs: received={len(body)} expected={len(BODY)} first_mismatch={mismatch} body_sha256={hashlib.sha256(body).hexdigest()}')
    return dict(result='PASS',role='biz',dns_exact=True,tcp_exact=True,https_exact=True,tls_version=protocol,body_bytes=len(body),body_sha256=hashlib.sha256(body).hexdigest())

if __name__=='__main__':
    a=argparse.ArgumentParser();a.add_argument('--role',choices=('target','biz'),required=True);a.add_argument('--address',required=True);a.add_argument('--cert',required=True);a.add_argument('--key');a.add_argument('--output',required=True);x=a.parse_args();r=server(x.address,x.cert,x.key) if x.role=='target' else client(x.address,x.cert);Path(x.output).write_text(json.dumps(r,indent=2)+'\n')
