"""Functional DNS probe only; public addresses exist in isolated namespaces."""
import argparse,json,socket,struct,threading,time
from pathlib import Path

def wire(i):
    return struct.pack('!6H',i,0x100,1,0,0,0)+b'\x07example\x03com\x00'+struct.pack('!HH',1,1)

def answer(q):
    return q[:2]+struct.pack('!5H',0x8180,1,1,0,0)+q[12:]+b'\xc0\x0c'+struct.pack('!HHIH',1,1,30,4)+socket.inet_aton('203.0.113.77')

def exact(c,n):
    b=b''
    while len(b)<n:
        x=c.recv(n-len(b))
        if not x:raise EOFError('short DNS TCP response')
        b+=x
    return b

def serve(root,addresses):
    lock=threading.Lock()
    def allowed(addr):return (root/'mode').read_text().strip() not in ('drop-'+addr,'drop-all')
    def record(addr,peer,protocol):
        with lock:
            with (root/'upstream.jsonl').open('a') as f:f.write(json.dumps(dict(server=addr,peer=peer[0],protocol=protocol))+'\n')
    def udp(addr):
        s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind((addr,53))
        while True:
            q,p=s.recvfrom(65535);record(addr,p,'udp')
            if allowed(addr):s.sendto(answer(q),p)
    def tcp_client(c,addr,p):
        with c:
            c.settimeout(10)
            try:
                q=exact(c,struct.unpack('!H',exact(c,2))[0]);record(addr,p,'tcp')
                if allowed(addr):a=answer(q);c.sendall(struct.pack('!H',len(a))+a)
            except (OSError,EOFError):pass
    def tcp(addr):
        s=socket.socket();s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);s.bind((addr,53));s.listen()
        while True:
            c,p=s.accept();threading.Thread(target=tcp_client,args=(c,addr,p),daemon=True).start()
    for a in addresses:
        for fn in (udp,tcp):threading.Thread(target=fn,args=(a,),daemon=True).start()
    while True:time.sleep(1)

def query(root,target,protocol,label,source_port):
    q=wire(1234);start=time.monotonic()
    if protocol=='udp':
        with socket.socket(socket.AF_INET,socket.SOCK_DGRAM) as s:
            s.settimeout(8)
            if source_port:s.bind(('0.0.0.0',source_port))
            s.sendto(q,(target,53));a,p=s.recvfrom(65535)
            assert p==(target,53),('original destination not restored',p,target)
    else:
        with socket.create_connection((target,53),timeout=8) as c:
            c.sendall(struct.pack('!H',len(q))+q);a=exact(c,struct.unpack('!H',exact(c,2))[0])
    assert a==answer(q),('DNS integrity',a.hex())
    with (root/'queries.jsonl').open('a') as f:f.write(json.dumps(dict(label=label,target=target,protocol=protocol,elapsed=time.monotonic()-start,result='PASS'))+'\n')

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('role',choices=['target','query']);p.add_argument('--root',required=True);p.add_argument('--addresses',default='1.1.1.1,8.8.8.8');p.add_argument('--target');p.add_argument('--protocol',choices=['udp','tcp'],default='udp');p.add_argument('--label',default='query');p.add_argument('--source-port',type=int,default=0);a=p.parse_args();r=Path(a.root)
    if a.role=='target':serve(r,a.addresses.split(','))
    else:query(r,a.target,a.protocol,a.label,a.source_port)
