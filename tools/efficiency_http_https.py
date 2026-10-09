#!/usr/bin/env python3
"""HTTP and certificate-checked HTTPS sidecars for one real fullstack E0 sample.

This runs only inside existing biz/target network namespaces. It does not
create a second measurement or proxy endpoint; short flows coexist with
normal independently paced TCP/UDP and do not gate first-arrival traffic.
"""
import argparse
import hashlib
import http.server
import json
import socket
import ssl
import struct
import threading
import time
from pathlib import Path

REQUESTS = 20
BODY_BYTES = 2048
DURATION_NS = 300_000_000_000
DRAIN_NS = 3_000_000_000

def now():
    return time.monotonic_ns()

def wait_until(deadline):
    delay=(deadline-now())/1e9
    if delay>0: time.sleep(delay)

def payload(seq, seed):
    chunk=struct.pack("!QQ",seq,seed)
    return (chunk*((BODY_BYTES+15)//16))[:BODY_BYTES]

class Receiver(http.server.BaseHTTPRequestHandler):
    protocol_version="HTTP/1.1"
    def do_GET(self):
        try:
            seq=int(self.path.removeprefix("/p/"))
            if self.path != f"/p/{seq}" or not 0<=seq<REQUESTS or self.headers.get("Host")!="qual.test":
                raise ValueError("invalid URL")
            data=payload(seq,self.server.seed)
            self.send_response(200)
            self.send_header("Content-Length",str(len(data)))
            self.send_header("Content-Type","application/octet-stream")
            self.send_header("Connection","close")
            self.end_headers()
            self.wfile.write(data)
            with self.server.lock:self.server.served+=1
        except (ValueError,OSError):
            with self.server.lock:self.server.errors+=1
            try:self.send_error(400)
            except OSError:pass
        self.close_connection=True
    def log_message(self, format, *args):
        return

def target(args):
    servers=[]
    for secure in (False,True):
        port=args.port+(1 if secure else 0)
        server=http.server.ThreadingHTTPServer((args.bind,port),Receiver)
        server.daemon_threads=True
        server.timeout=.2
        server.seed=args.seed
        server.served=0;server.errors=0;server.lock=threading.Lock()
        if secure:
            ctx=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
            ctx.load_cert_chain(args.tls_cert,args.tls_key)
            server.socket=ctx.wrap_socket(server.socket,server_side=True)
        servers.append((server,secure))
    deadline=args.start_ns+args.duration_s*1_000_000_000+DRAIN_NS
    while now()<deadline:
        for server,_ in servers:server.handle_request()
    result={"role":"target","http_served":servers[0][0].served,
            "https_served":servers[1][0].served,
            "server_errors":sum(server.errors for server,_ in servers)}
    for server,_ in servers:server.server_close()
    return result

def business(args):
    ctx=ssl.create_default_context(cafile=args.tls_cert)
    ctx.check_hostname=True
    completed=[];failures=[]
    for seq in range(REQUESTS):
        wait_until(args.start_ns+seq*args.duration_s*1_000_000_000//REQUESTS)
        secure=bool(seq&1)
        started=now()
        sock=None
        try:
            sock=socket.create_connection((args.peer,args.port+int(secure)),timeout=10)
            sock.settimeout(10)
            if secure:
                sock=ctx.wrap_socket(sock,server_hostname="qual.test")
            req=f"GET /p/{seq} HTTP/1.1\r\nHost: qual.test\r\nConnection: close\r\n\r\n".encode()
            sock.sendall(req)
            data=bytearray()
            while len(data)<8192:
                part=sock.recv(8192-len(data))
                if not part:break
                data.extend(part)
            header,sep,body=bytes(data).partition(b"\r\n\r\n")
            if (not sep or not header.startswith(b"HTTP/1.1 200")
                or b"Content-Length: 2048" not in header
                or body!=payload(seq,args.seed)):
                raise ValueError("HTTP status/body/length mismatch")
            completed.append({"sequence":seq,"tls_verified":secure,
                              "response_bytes":len(body),
                              "wire_response_bytes":len(data),
                              "request_bytes":len(req),
                              "duration_ns":now()-started,
                              "sha256":hashlib.sha256(body).hexdigest()})
        except (OSError,ssl.SSLError,ValueError) as exc:
            failures.append({"sequence":seq,"tls":secure,"type":type(exc).__name__})
        finally:
            if sock is not None:
                try:sock.close()
                except OSError:pass
    return {"role":"biz","requests":REQUESTS,"returned":len(completed),
            "tls_verified":sum(item["tls_verified"] for item in completed),
            "failed":failures,"completed":completed}

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--role",choices=["target","biz"],required=True)
    p.add_argument("--bind",default="8.8.8.8")
    p.add_argument("--peer",default="8.8.8.8")
    p.add_argument("--port",type=int,default=18083)
    p.add_argument("--start-ns",type=int,required=True)
    p.add_argument("--seed",type=int,required=True)
    p.add_argument("--tls-cert",required=True)
    p.add_argument("--tls-key",required=True)
    p.add_argument("--source",required=True)
    p.add_argument("--helper",required=True)
    p.add_argument("--output",required=True)
    p.add_argument("--duration-s",type=int,default=300,choices=[15,120,300])
    a=p.parse_args()
    result=target(a) if a.role=="target" else business(a)
    result.update({"schema":"wbd-efficiency-http/v1","seed":a.seed,
                   "product_source_sha":a.source,"helper_sha":a.helper,
                   "duration_seconds":a.duration_s,"drain_seconds":3,
                   "certificate_validation":"system roots + pinned fixture CA, hostname qual.test" if a.role=="biz" else "self-signed fixture only"})
    Path(a.output).write_text(json.dumps(result,indent=2))
    print("E0_HTTP_FINISHED",a.role,result.get("returned",result.get("https_served")),flush=True)
if __name__=="__main__":
    main()
