#!/usr/bin/env python3
"""One-run, two-real-socket-endpoint large-inner-MTU traffic generator.

The business and target processes run in separate network namespaces. Each
originates its own UDP traffic; TCP uses simultaneous full-duplex senders on
multiple independent kernel TCP connections. No mocked tunnel APIs.
"""
import argparse
import collections
import hashlib
import json
import socket
import struct
import threading
import time
import zlib
from pathlib import Path

U = struct.Struct("!4sBBHQQII")
T = struct.Struct("!4sIIII")
MAGIC = b"WMIX"
A_JUMBO = [96]*30+[256]*20+[512]*14+[1000]*10+[1372]*9+[4068]*6+[8972]*5+[8973]*4+[65507]*2
C_JUMBO = [96]*50+[256]*20+[512]*10+[1372]*8+[8972]*5+[8973]*5+[65507]*2
A_ORDINARY = [96]*30+[256]*20+[512]*15+[1000]*15+[1372]*10+[4068]*10
C_ORDINARY = [96]*50+[256]*20+[512]*10+[1000]*5+[1372]*10+[4068]*5
BOUNDARY = [96]*50+[256]*10+[512]*10+[1372]*10+[4068]*10+[8936]*5+[8937]*4+[65507]*1
assert len(BOUNDARY)==100
TCP_WRITES = (256, 4096, 65536, 1048576)
PROBE_BYTES_PER_SECOND = 96*5
SHORT_BYTES_PER_SECOND = 24576
DRAIN_S = 3

def ns():
    return time.monotonic_ns()

def wait(when):
    left = (when-ns())/1e9
    if left>0: time.sleep(left)

def body(seq, size, seed):
    token = struct.pack("!QII", seq, size, seed & 0xffffffff)
    return (token*((size+15)//16))[:size]

def udp_packet(kind, seq, size, seed, timestamp):
    data = body(seq, size-U.size, seed)
    return U.pack(MAGIC, 1, kind, size, seq, timestamp, seed & 0xffffffff, zlib.crc32(data)) + data

def decode_udp(data):
    if len(data)<U.size: return None
    magic, ver, kind, length, seq, sent, seed, crc = U.unpack_from(data)
    if magic!=MAGIC or ver!=1 or length!=len(data): return None
    payload=data[U.size:]
    if zlib.crc32(payload)!=crc or payload!=body(seq,len(payload),seed): return None
    return kind,seq,length,sent

def addr(text):
    ip, port=text.rsplit(":",1)
    return ip,int(port)

class Totals:
    def __init__(self, start):
        self.lock=threading.Lock()
        self.start=start
        self.d={"offered_bytes":0,"sent_bytes":0,"received_bytes":0,"send_errors":0,
                "skipped_bytes":0,"corrupt":0,"duplicates":0,"probe_sent":0,
                "probe_received":0,"probe_send_errors":0,
                "probe_rtt_ns":[],"probe_events":[],
                "udp_tx":{},"udp_rx":{},"tcp_tx":{},"tcp_rx":{},
                "receive_bucket_10ms":{},"send_bucket_10ms":{},
                "tcp_connections_failed":0,"tcp_connect_attempts":0,
                "tcp_mss":[],"tcp_backpressure_events":0,"tcp_write_block_ns":0,
                "tcp_write_max_ns":0,"tcp_rx_corrupt":0,"tcp_rx_seq_errors":0}
        self.seen=set()
        self.probes=set()
    def add(self,key,n=1):
        with self.lock: self.d[key]+=n
    def record(self,key, sub, num, length):
        with self.lock:
            q=self.d[key].setdefault(str(sub),{"packets":0,"bytes":0})
            q["packets"]+=num;q["bytes"]+=length
    def bucket(self,key,t,n):
        index=(t-self.start)//10_000_000
        if index<0 or index>33000: return
        with self.lock:
            s=self.d[key]; k=str(index);s[k]=s.get(k,0)+n
    def snap(self):
        with self.lock:
            out=json.loads(json.dumps(self.d))
            out["unique_udp_received"]=len(self.seen)
            return out

def setup_udp(ip,port):
    s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
    s.setsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF,4<<20)
    s.setsockopt(socket.SOL_SOCKET,socket.SO_SNDBUF,4<<20)
    # Linux IP_MTU_DISCOVER=10, IP_PMTUDISC_DONT=0. Never rely on defaults.
    s.setsockopt(socket.IPPROTO_IP,getattr(socket,"IP_MTU_DISCOVER",10),getattr(socket,"IP_PMTUDISC_DONT",0))
    s.bind((ip,port))
    return s

def udp_recv(s,role,t,peer,stop):
    s.settimeout(.1)
    while not stop.is_set():
        try: data,src=s.recvfrom(65535)
        except socket.timeout:continue
        except OSError:break
        got=decode_udp(data); stamp=ns()
        if got is None:
            if data==b"register" and role=="target":
                peer[0]=src
            else:t.add("corrupt")
            continue
        kind,seq,length,sent=got
        if kind==3:
            if role=="target": peer[0]=src
            try:s.sendto(udp_packet(4,seq,96,0x515151,sent),src)
            except OSError:t.add("probe_send_errors")
            continue
        if kind in (5,6,7,8,9) and role in ("biz","target"):
            size={5:8972,6:8973,7:65507,8:8936,9:8937}[kind]
            with t.lock:
                key=str(size)
                d=t.d.setdefault("udp_big_rtt_ns",{})
                values=d.setdefault(key,[])
                if len(values)<10000:values.append(max(0,stamp-sent))
            continue
        if kind==4:
            with t.lock:
                if seq in t.probes:t.d["duplicates"]+=1
                else:
                    t.probes.add(seq);t.d["probe_received"]+=1
                    t.d["probe_rtt_ns"].append(stamp-sent)
                    if len(t.d["probe_events"])<3000:
                        t.d["probe_events"].append([seq,sent,stamp])
            continue
        if kind!=1:continue
        key=(seq,length)
        with t.lock:
            if key in t.seen:
                t.d["duplicates"]+=1;continue
            t.seen.add(key)
        t.record("udp_rx",length,1,length)
        if length in (8972,8973,65507,8936,8937):
            receipt_kind={8972:5,8973:6,65507:7,8936:8,8937:9}[length]
            try:s.sendto(udp_packet(receipt_kind,seq,96,0x929292,sent),src)
            except OSError:t.add("probe_send_errors")
        t.add("received_bytes",length)
        t.bucket("receive_bucket_10ms",stamp,length)

def udp_send(s,role,peer,start,stop,rate,seed,scenario,size_profile,t):
    if rate<=0:return
    values=BOUNDARY if size_profile=="boundary" else ((A_JUMBO if scenario=="udp" else C_JUMBO) if size_profile=="jumbo" else (A_ORDINARY if scenario=="udp" else C_ORDINARY))
    # Fixed 100-slot count weights. Accounting and pacing are in total payload BYTES.
    budget=max(1.0,rate*125000-PROBE_BYTES_PER_SECOND)
    total=0; seq=0; end=start+300_000_000_000
    while not stop.is_set():
        length=values[seq%100]
        # Never emit an entire last UDP datagram beyond the logical byte cap.
        # Skipping the unscheduled final datagram is not packet truncation.
        if total+length>int(budget*300):break
        target=start+int(total*1e9/budget)
        if target>=end:break
        now=ns()
        t.add("offered_bytes",length)
        if now-target>20_000_000:
            t.add("skipped_bytes",length)
        else:
            wait(target)
            if ns()-target>20_000_000:
                t.add("skipped_bytes",length)
            elif peer[0] is None:
                t.add("send_errors")
            else:
                try:
                    s.sendto(udp_packet(1,seq,length,seed,ns()),peer[0])
                    t.record("udp_tx",length,1,length)
                    t.add("sent_bytes",length)
                    t.bucket("send_bucket_10ms",ns(),length)
                except OSError:t.add("send_errors")
        total+=length;seq+=1

def probe(s,remote,start,stop,t,offset_s=0):
    for seq in range(1500):
        target=start+int(offset_s*1e9)+seq*200_000_000
        if target>=start+300_000_000_000 or stop.is_set():break
        wait(target)
        destination=remote() if callable(remote) else remote
        t.add("offered_bytes",96)
        if destination is None:
            t.add("probe_send_errors")
            continue
        try:
            s.sendto(udp_packet(3,seq,96,0x515151,ns()),destination)
            t.add("probe_sent");t.add("sent_bytes",96)
            t.bucket("send_bucket_10ms",ns(),96)
        except OSError:t.add("probe_send_errors")

def recv_exact(s,n):
    b=bytearray()
    while len(b)<n:
        part=s.recv(n-len(b))
        if not part:raise EOFError
        b.extend(part)
    return bytes(b)

def tcp_reader(s,flow,seed,t):
    seq=0; h=hashlib.sha256();received=0;bad=False
    try:
        while True:
            try: hdr=recv_exact(s,T.size)
            except EOFError:break
            magic,i,length,crc,typ=T.unpack(hdr)
            if magic!=b"WTCP" or i!=seq or length>1048576 or length==0:
                t.add("tcp_rx_seq_errors");bad=True;break
            data=recv_exact(s,length)
            if zlib.crc32(data)!=crc or data!=body(i,length,seed+flow):
                t.add("tcp_rx_corrupt");bad=True;break
            received+=length;h.update(data);seq+=1
            t.add("received_bytes",length)
            t.bucket("receive_bucket_10ms",ns(),length)
    except (OSError, EOFError):
        pass
    finally:
        with t.lock:t.d["tcp_rx"][str(flow)]={"bytes":received,"frames":seq,"sha256":h.hexdigest(),"bad":bad}

def tcp_writer(s,flow,seed,start,stop,mbps,short,t):
    h=hashlib.sha256(); total=0;seq=0;end=start+300_000_000_000
    sizes=(4096,16384,4096) if short else TCP_WRITES
    if short:
        rate=1e20
    else:
        rate=max(1.0,mbps*125000)
    try:
        wait(start)
        while not stop.is_set():
            length=sizes[seq%len(sizes)]
            if not short:
                remaining=int(rate*300)-total
                if remaining<=0:break
                # TCP is a stream: the last application write may be smaller
                # than 1MiB; its CRC/hash covers exactly these bytes.
                length=min(length,remaining)
            goal=start+int(total*1e9/rate)
            if not short and goal>=end:break
            if short and seq>=3:break
            if ns()>=end and not short:break
            t.add("offered_bytes",length)
            payload=body(seq,length,seed+flow)
            frame=T.pack(b"WTCP",seq,length,zlib.crc32(payload),1 if short else 0)+payload
            wait(goal)
            before=ns()
            try:s.sendall(frame)
            except OSError:
                t.add("send_errors");break
            elapsed=ns()-before
            with t.lock:
                t.d["tcp_write_block_ns"]+=elapsed
                t.d["tcp_write_max_ns"]=max(t.d["tcp_write_max_ns"],elapsed)
                if elapsed>10_000_000:t.d["tcp_backpressure_events"]+=1
            h.update(payload);seq+=1;total+=length
            t.add("sent_bytes",length)
            t.bucket("send_bucket_10ms",ns(),length)
    finally:
        try:s.shutdown(socket.SHUT_WR)
        except OSError:pass
        with t.lock:t.d["tcp_tx"][str(flow)]={"bytes":total,"frames":seq,"sha256":h.hexdigest()}

def mss(s,t):
    try:
        val=s.getsockopt(socket.IPPROTO_TCP,socket.TCP_MAXSEG)
        with t.lock:t.d["tcp_mss"].append(val)
    except OSError:pass

def tcp_pair(s,flow,role,seed,start,stop,rate,short,t):
    mss(s,t)
    sndseed=seed+(1 if role=="biz" else 2)*100000
    rcvseed=seed+(2 if role=="biz" else 1)*100000
    send=threading.Thread(target=tcp_writer,args=(s,flow,sndseed,start,stop,rate,short,t),daemon=True)
    recv=threading.Thread(target=tcp_reader,args=(s,flow,rcvseed,t),daemon=True)
    recv.start();send.start()
    send.join(timeout=max(1,(start+315_000_000_000-ns())/1e9))
    recv.join(timeout=max(1,(start+320_000_000_000-ns())/1e9))
    try:s.close()
    except OSError:pass

def target_tcp(listener,t,start,stop,seed,rate):
    listener.settimeout(.2)
    jobs=[]
    while not stop.is_set():
        try:s,_=listener.accept()
        except socket.timeout:continue
        except OSError:break
        try:
            s.settimeout(6)
            flow=struct.unpack("!I",recv_exact(s,4))[0]
            s.settimeout(None)
        except (OSError,EOFError):
            s.close();continue
        short=flow>=1000
        th=threading.Thread(target=tcp_pair,args=(s,flow,"target",seed,start if not short else ns(),stop,rate,short,t),daemon=True)
        th.start();jobs.append(th)
    for th in jobs: th.join(timeout=1)

def dial(ip,port,flow,timeout,t):
    until=ns()+int(timeout*1e9)
    while ns()<until:
        t.add("tcp_connect_attempts")
        s=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        s.settimeout(2)
        try:
            s.connect((ip,port));s.sendall(struct.pack("!I",flow));s.settimeout(None)
            return s
        except OSError:
            s.close();time.sleep(.15)
    t.add("tcp_connections_failed")
    return None

def business_tcp(remote,t,start,stop,seed,rate):
    jobs=[]
    for flow in range(4):
        s=dial(*remote,flow,8,t)
        if s is not None:
            th=threading.Thread(target=tcp_pair,args=(s,flow,"biz",seed,start,stop,rate,False,t),daemon=True)
            th.start();jobs.append(th)
    for seq in range(300):
        when=start+seq*1_000_000_000
        wait(when)
        if stop.is_set():break
        s=dial(*remote,1000+seq,1,t)
        if s:
            th=threading.Thread(target=tcp_pair,args=(s,1000+seq,"biz",seed,ns(),stop,0,True,t),daemon=True)
            th.start();jobs.append(th)
    for th in jobs:th.join(timeout=2)

def run(args):
    start=args.start_ns;end=start+300_000_000_000
    stop=threading.Event();t=Totals(start);jobs=[]
    own=addr(args.bind);remote=addr(args.peer)
    # HTTP+verified HTTPS sidecars reserve 20kbps total budget in TCP cases;
    # their bounded 20 x 2KiB replies use < 2kbps. No extra bulk traffic.
    rate=args.rate_mbps-(0.02 if args.workload!="udp" else 0)
    udp_rate=rate if args.workload=="udp" else (rate/2 if args.workload=="mixed" else 0)
    tcp_rate=rate if args.workload=="tcp" else (rate/2 if args.workload=="mixed" else 0)
    if udp_rate:
        s=setup_udp(*own)
        own_rcvbuf=s.getsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF)
        other=[remote if args.role=="biz" else None]
        jobs.append(threading.Thread(target=udp_recv,args=(s,args.role,t,other,stop),daemon=True))
        if args.role=="biz":
            def register():
                while ns()<start and not stop.is_set():
                    try:s.sendto(b"register",remote)
                    except OSError:pass
                    time.sleep(.2)
            jobs.append(threading.Thread(target=register,daemon=True))
        jobs.append(threading.Thread(target=udp_send,args=(s,args.role,other,start,stop,udp_rate,args.seed+(1 if args.role=="biz" else 2),args.workload,args.size_profile,t),daemon=True))
    if udp_rate or tcp_rate:
        if args.role=="biz":
            probe_socket=setup_udp(own[0],own[1]+2)
            jobs.append(threading.Thread(target=udp_recv,args=(probe_socket,"biz",t,[remote],stop),daemon=True))
            jobs.append(threading.Thread(target=probe,args=(probe_socket,(remote[0],remote[1]+2),start,stop,t),daemon=True))
        else:
            probe_socket=setup_udp(own[0],own[1]+2)
            reverse_peer=[None]
            jobs.append(threading.Thread(target=udp_recv,args=(probe_socket,"target",t,reverse_peer,stop),daemon=True))
            jobs.append(threading.Thread(target=probe,args=(probe_socket,lambda:reverse_peer[0],start,stop,t,1.0),daemon=True))
    if tcp_rate:
        if args.role=="target":
            ls=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
            ls.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
            ls.bind((own[0],own[1]+1));ls.listen(32)
            # Three long connections take the TCP budget after fixed short/probe quotas.
            jobs.append(threading.Thread(target=target_tcp,args=(ls,t,start,stop,args.seed,(tcp_rate-SHORT_BYTES_PER_SECOND/125000-PROBE_BYTES_PER_SECOND/125000)/4),daemon=True))
        else:
            jobs.append(threading.Thread(target=business_tcp,args=((remote[0],remote[1]+1),t,start,stop,args.seed,(tcp_rate-SHORT_BYTES_PER_SECOND/125000-PROBE_BYTES_PER_SECOND/125000)/4),daemon=True))
    for th in jobs:th.start()
    wait(end+DRAIN_S*1_000_000_000)
    stop.set()
    for th in jobs:th.join(timeout=2)
    if udp_rate:
        s.close()
    if udp_rate or tcp_rate: probe_socket.close()
    if tcp_rate and args.role=="target": ls.close()
    output={"schema":"wbd-large-mixed/v1","role":args.role,"workload":args.workload,
            "source_sha":args.source,"helper_sha":args.helper,
            "start_monotonic_ns":start,"duration_seconds":300,"drain_seconds":DRAIN_S,
            "configured_per_direction_mbps":args.rate_mbps,"size_profile":args.size_profile,"udp_budget_mbps":udp_rate,
            "tcp_budget_mbps":tcp_rate,"seed":args.seed,
            "ip_mtu_discover":"IP_PMTUDISC_DONT for UDP (DF off)",
            "udp_size_cycle":collections.Counter(BOUNDARY if args.size_profile=="boundary" else ((A_JUMBO if args.workload=="udp" else C_JUMBO) if args.size_profile=="jumbo" else (A_ORDINARY if args.workload=="udp" else C_ORDINARY))),
            "udp_socket_buffers": {"requested": 4<<20, "recv_effective": own_rcvbuf if udp_rate else None},
            "counters":t.snap()}
    Path(args.output).write_text(json.dumps(output,indent=2,sort_keys=True))
    print("WBD_MIXED_DONE role=%s workload=%s sent=%d rx=%d" %
          (args.role,args.workload,output["counters"]["sent_bytes"],output["counters"]["received_bytes"]),flush=True)

if __name__=="__main__":
    p=argparse.ArgumentParser()
    p.add_argument("--role",choices=["biz","target"],required=True)
    p.add_argument("--workload",choices=["udp","tcp","mixed"],required=True)
    p.add_argument("--bind",required=True);p.add_argument("--peer",required=True)
    p.add_argument("--seed",type=int,required=True);p.add_argument("--start-ns",type=int,required=True)
    p.add_argument("--source",required=True);p.add_argument("--helper",required=True)
    p.add_argument("--output",required=True)
    p.add_argument("--size-profile",choices=["ordinary","jumbo","boundary"],required=True)
    p.add_argument("--rate-mbps",type=float,choices=[3.0,10.0],required=True)
    run(p.parse_args())
