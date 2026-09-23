#!/usr/bin/env python3
"""Acceptance-only low-rate client-originated wake driver.

The target never replies. That is deliberate: a fully dormant product has no
out-of-band server->client wake channel. This driver proves that each new
client business event can wake the tunnel near the idle cutoff without relying
on reverse traffic or on the duplex generator's REGISTER side channel.
"""
import argparse
import json
import socket
import struct
import time
import zlib
from pathlib import Path

MAGIC=b"WBDW"
HDR=struct.Struct("!4sQII")
SIZES=(64,256,1200)

def addr(s):
    h,p=s.rsplit(":",1)
    return h,int(p)

def wait_until(ns):
    while True:
        left=ns-time.monotonic_ns()
        if left<=0:return
        if left>1_000_000:
            time.sleep((left-500_000)/1e9)

def packet(seq,size,seed):
    n=size-HDR.size
    token=struct.pack("!QI",seq ^ ((seed & 0xffffffff)<<16),size)
    body=(token*((n+len(token)-1)//len(token)))[:n]
    return HDR.pack(MAGIC,seq,seed & 0xffffffff,zlib.crc32(body)&0xffffffff)+body

def decode(data):
    if len(data)<HDR.size:return None,"short"
    magic,seq,seed,crc=HDR.unpack_from(data)
    if magic!=MAGIC:return None,"magic"
    body=data[HDR.size:]
    if zlib.crc32(body)&0xffffffff!=crc:return None,"crc"
    want=packet(seq,len(data),seed)
    if want!=data:return None,"content"
    return (seq,len(data),seed),None

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--role",choices=("biz","target"),required=True)
    ap.add_argument("--bind",required=True)
    ap.add_argument("--peer")
    ap.add_argument("--start-ns",type=int,required=True)
    ap.add_argument("--count",type=int,default=100)
    ap.add_argument("--interval",type=float,default=1.25)
    ap.add_argument("--seed",type=int,required=True)
    ap.add_argument("--min-received",type=int)
    ap.add_argument("--output",required=True)
    x=ap.parse_args()
    if x.count<=0 or x.interval<=0: raise SystemExit("invalid count/interval")
    minimum=x.count if x.min_received is None else x.min_received
    if minimum<0 or minimum>x.count: raise SystemExit("invalid min-received")
    if x.role=="biz" and not x.peer: raise SystemExit("--peer required for biz")
    sock=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    sock.setsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF,1<<20)
    sock.setsockopt(socket.SOL_SOCKET,socket.SO_SNDBUF,1<<20)
    sock.bind(addr(x.bind))
    out={"schema":1,"role":x.role,"count":x.count,"interval_s":x.interval,"seed":x.seed,
         "start_ns":x.start_ns,"sent":0,"send_errors":0,"received_unique":0,
         "duplicates":0,"corrupt":0,"unexpected":0,"min_received":minimum}
    if x.role=="biz":
        peer=addr(x.peer)
        for i in range(x.count):
            target=x.start_ns+int(i*x.interval*1e9)
            wait_until(target)
            size=SIZES[i%len(SIZES)]
            try:
                sock.sendto(packet(i,size,x.seed),peer)
                out["sent"]+=1
            except OSError:
                out["send_errors"]+=1
        # Let the final socket write leave the namespace before process exit.
        time.sleep(1)
    else:
        seen=set()
        deadline=x.start_ns+int((x.count*x.interval+20)*1e9)
        sock.settimeout(.2)
        while time.monotonic_ns()<deadline and len(seen)<minimum:
            try:data,_=sock.recvfrom(2048)
            except socket.timeout:continue
            except OSError:break
            row,err=decode(data)
            if err:
                out["corrupt"]+=1
                continue
            seq,_,seed=row
            if seed!=(x.seed & 0xffffffff) or seq>=x.count:
                out["unexpected"]+=1
            elif seq in seen:
                out["duplicates"]+=1
            else:
                seen.add(seq)
        out["received_unique"]=len(seen)
        out["missing"]=[i for i in range(x.count) if i not in seen]
    sock.close()
    Path(x.output).write_text(json.dumps(out,indent=2,sort_keys=True)+"\n")
    print("WBD_LIFECYCLE_WAKE_DRIVER "+json.dumps(out,sort_keys=True))
    ok=(out["send_errors"]==0 and out["sent"]==x.count) if x.role=="biz" else (
        out["received_unique"]>=minimum and out["corrupt"]==0 and out["unexpected"]==0)
    raise SystemExit(0 if ok else 1)
if __name__=="__main__":main()
