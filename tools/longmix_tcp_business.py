#!/usr/bin/env python3
"""Actual full-duplex TCP socket workload for one Actions B/C sample.

Three independent long TCP connections carry both directions concurrently.
A separately paced short-connection request/reply stream runs at 1 Hz.
TCP application write sizes never pretend to be IP frame sizes. Only hashes,
byte counts, TCP_INFO scalars and bounded timings are written to artifacts.
"""
import argparse
import collections
import hashlib
import json
import os
import socket
import struct
import threading
import time
from pathlib import Path
from longmix_profile import TCP_WRITE_SIZES, TCP_LONG_CONNECTIONS, TCP_SHORT_CONNECTIONS

MAGIC = b"WBLT"
HEADER = struct.Struct("!4sI")
PORT = 18100
SHORT_BYTES = 96
SHORT_HZ = 1
MAX_SHORT_EVENTS = 512
TCP_INFO_OPT = getattr(socket, "TCP_INFO", 11)
TCP_MAXSEG_OPT = getattr(socket, "TCP_MAXSEG", 2)
LATENCY_CAP = 8192


def now_ns():
    return time.monotonic_ns()


def wait_until_ns(deadline):
    while True:
        left = deadline - now_ns()
        if left <= 0:
            return
        time.sleep(min(left / 1e9, 0.1))


def tcp_info(sock):
    result = {}
    try:
        result["tcp_maxseg_socket"] = sock.getsockopt(socket.IPPROTO_TCP, TCP_MAXSEG_OPT)
        raw = sock.getsockopt(socket.IPPROTO_TCP, TCP_INFO_OPT, 128)
        for key, offset in (("tcpi_rto_us",8),("tcpi_snd_mss",16),
                            ("tcpi_rcv_mss",20),("tcpi_unacked",24),
                            ("tcpi_lost",32),("tcpi_retrans",36),
                            ("tcpi_pmtu",60),("tcpi_rtt_us",68),
                            ("tcpi_total_retrans",100)):
            if len(raw) >= offset+4:
                result[key]=struct.unpack_from("=I",raw,offset)[0]
    except OSError as ex:
        result["tcp_info_error"] = str(ex)
    return result


def read_exact(sock, n, deadline_ns):
    acc=bytearray()
    while len(acc) < n and now_ns() < deadline_ns:
        try:
            b=sock.recv(n-len(acc))
        except socket.timeout:
            continue
        if not b:
            raise ConnectionError("EOF before connection hello")
        acc.extend(b)
    if len(acc)!=n:
        raise TimeoutError("partial TCP hello")
    return bytes(acc)


class OneFlow:
    def __init__(self, role, index, sock, start_ns, duration_s, drain_s,
                 per_flow_bps):
        self.role=role
        self.index=index
        self.sock=sock
        self.start_ns=start_ns
        self.end_ns=start_ns+int(duration_s*1e9)
        self.stop_ns=self.end_ns+int(drain_s*1e9)
        self.per_flow_bps=per_flow_bps
        self.byte_value=(17+index*29+(0 if role=="biz" else 101))%256
        self.sent_hash=hashlib.sha256()
        self.recv_hash=hashlib.sha256()
        self.sent=0
        self.received=0
        self.write_calls=collections.Counter()
        self.sent_by_write_size=collections.Counter()
        self.timeout_count=0
        self.partial_write_count=0
        self.socket_errors=[]
        self.write_max_block_ns=0
        self.write_over_10ms=0
        self.recv_by_10ms={}
        self.send_by_second=[0]*int(duration_s)
        self.recv_by_second=[0]*int(duration_s+drain_s+1)
        self.tcp_initial=tcp_info(sock)
        self.tcp_final={}
        self.recv_eof=False
        self.completed=False
        self.lock=threading.Lock()

    def recv_loop(self):
        try:
            wait_until_ns(self.start_ns)
            self.sock.settimeout(.2)
            while now_ns()<self.stop_ns:
                try:
                    part=self.sock.recv(65536)
                except socket.timeout:
                    continue
                except OSError as ex:
                    self.socket_errors.append("recv:"+str(ex))
                    break
                if not part:
                    self.recv_eof=True
                    break
                ts=now_ns()
                self.recv_hash.update(part)
                self.received+=len(part)
                sec=int((ts-self.start_ns)//1_000_000_000)
                if 0<=sec<len(self.recv_by_second):
                    self.recv_by_second[sec]+=len(part)
                bucket=(ts-self.start_ns)//10_000_000
                if 0<=bucket<=(self.stop_ns-self.start_ns)//10_000_000:
                    self.recv_by_10ms[int(bucket)]=self.recv_by_10ms.get(int(bucket),0)+len(part)
        finally:
            self.tcp_final=tcp_info(self.sock)

    def send_loop(self):
        wait_until_ns(self.start_ns)
        i=0
        limit=int(self.per_flow_bps*300)
        sizes=TCP_WRITE_SIZES
        try:
            while self.sent<limit:
                want=min(sizes[i%len(sizes)],limit-self.sent)
                # Byte-rate pacing accounts for the whole application write.
                # A 1MiB application write is a bounded burst, not a 1MiB IP packet.
                due=self.start_ns+int((self.sent+want)*1e9/self.per_flow_bps)
                if due>=self.end_ns:
                    break
                wait_until_ns(due)
                if now_ns()>=self.end_ns:
                    break
                payload=bytes([self.byte_value])*want
                view=memoryview(payload)
                count=0
                self.write_calls[want]+=1
                while count<want:
                    if now_ns()>=self.stop_ns:
                        self.socket_errors.append("send:drain_deadline")
                        break
                    before=now_ns()
                    try:
                        n=self.sock.send(view[count:])
                        if n<=0:
                            raise ConnectionError("zero-byte write")
                    except socket.timeout:
                        self.timeout_count+=1
                        continue
                    except OSError as ex:
                        self.socket_errors.append("send:"+str(ex))
                        break
                    blocked=now_ns()-before
                    self.write_max_block_ns=max(self.write_max_block_ns,blocked)
                    self.write_over_10ms+=int(blocked>10_000_000)
                    if n<want-count:
                        self.partial_write_count+=1
                    self.sent_hash.update(view[count:count+n])
                    count+=n
                    self.sent+=n
                    sec=int((now_ns()-self.start_ns)//1_000_000_000)
                    if 0<=sec<len(self.send_by_second):
                        self.send_by_second[sec]+=n
                self.sent_by_write_size[want]+=count
                if count<want:
                    break
                i+=1
        finally:
            try: self.sock.shutdown(socket.SHUT_WR)
            except OSError: pass
            self.completed=self.sent>=limit

    def snapshot(self):
        zeros=0
        longest=0
        for b in range(30000):
            if self.recv_by_10ms.get(b,0)==0:
                zeros+=1
                longest=max(longest,zeros)
            else:
                zeros=0
        return {
            "index":self.index,"role":self.role,
            "app_write_sizes":list(TCP_WRITE_SIZES),
            "send_bytes":self.sent,
            "recv_bytes":self.received,
            "send_sha256":self.sent_hash.hexdigest(),
            "recv_sha256":self.recv_hash.hexdigest(),
            "write_call_sizes":dict(self.write_calls),
            "written_bytes_by_call_size":dict(self.sent_by_write_size),
            "socket_timeout_backpressure":self.timeout_count,
            "partial_socket_writes":self.partial_write_count,
            "socket_errors":self.socket_errors[:32],
            "write_max_block_ns":self.write_max_block_ns,
            "write_over_10ms":self.write_over_10ms,
            "recv_eof":self.recv_eof,
            "completed_quota":self.completed,
            "send_by_second":self.send_by_second,
            "recv_by_second":self.recv_by_second,
            "recv_10ms_bucket_nonzero":len(self.recv_by_10ms),
            "recv_longest_zero_delivery_ms":longest*10,
            "tcp_info_initial":self.tcp_initial,
            "tcp_info_final":self.tcp_final,
        }


def short_client(peer, start_ns, duration_s, seed, samples):
    for sec in range(int(duration_s)):
        if len(samples)>=MAX_SHORT_EVENTS:
            break
        wait_until_ns(start_ns+sec*1_000_000_000)
        now=now_ns()
        if now>start_ns+int((sec+1)*1e9):
            samples.append({"sec":sec,"classification":"MISSED_SCHEDULE"})
            continue
        sock=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        sock.settimeout(2.0)
        try:
            sock.connect(peer)
            sock.sendall(HEADER.pack(MAGIC,1000+sec))
            payload=bytes([(seed+sec)&255])*SHORT_BYTES
            sock.sendall(payload)
            got=read_exact(sock,SHORT_BYTES,now_ns()+3_000_000_000)
            if got!=bytes([(seed+sec+101)&255])*SHORT_BYTES:
                raise ValueError("short reply integrity mismatch")
            samples.append({"sec":sec,"classification":"RETURNED",
                            "rtt_ns":now_ns()-now,"tx_bytes":SHORT_BYTES,
                            "rx_bytes":SHORT_BYTES,
                            "tcp_maxseg":sock.getsockopt(socket.IPPROTO_TCP,TCP_MAXSEG_OPT)})
        except (OSError,ValueError,TimeoutError,ConnectionError) as ex:
            samples.append({"sec":sec,"classification":"FAIL","error":str(ex)[:130]})
        finally:
            sock.close()


def short_server(sock, index, seed, samples, stop_ns):
    sec=index-1000
    sock.settimeout(.3)
    started=now_ns()
    try:
        buf=read_exact(sock,SHORT_BYTES,min(now_ns()+3_000_000_000,stop_ns))
        if buf!=bytes([(seed+sec)&255])*SHORT_BYTES:
            raise ValueError("short request integrity mismatch")
        sock.sendall(bytes([(seed+sec+101)&255])*SHORT_BYTES)
        samples.append({"sec":sec,"classification":"REPLIED",
                        "rx_bytes":SHORT_BYTES,"tx_bytes":SHORT_BYTES,
                        "service_ns":now_ns()-started})
    except (OSError,ValueError,TimeoutError,ConnectionError) as ex:
        samples.append({"sec":sec,"classification":"FAIL","error":str(ex)[:130]})
    finally:
        sock.close()


def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--role",choices=("biz","target"),required=True)
    ap.add_argument("--bind",required=True)
    ap.add_argument("--peer")
    ap.add_argument("--workload",choices=("B","C"),required=True)
    ap.add_argument("--seed",type=int,required=True)
    ap.add_argument("--start-ns",type=int,required=True)
    ap.add_argument("--duration",type=int,required=True)
    ap.add_argument("--drain",type=int,required=True)
    ap.add_argument("--output",required=True)
    args=ap.parse_args()
    if args.duration!=300 or args.drain!=10:
        ap.error("formal 300s+10s only")
    if args.seed!=(2608102 if args.workload=="B" else 2608103):
        ap.error("wrong paired workload seed")
    if args.role=="biz" and not args.peer:
        ap.error("biz must specify tunneled private target")
    if args.role=="target" and args.peer:
        ap.error("target cannot directly send to app route")
    rate=10 if args.workload=="B" else 5
    # A separate short TCP request/reply is explicitly reserved inside
    # each direction's TCP 10 or 5Mbps budget, not added on top of it.
    rate_bps=rate*1_000_000/8
    per_flow_bps=(rate_bps - SHORT_BYTES*SHORT_HZ)/TCP_LONG_CONNECTIONS
    start=args.start_ns
    stop=start+(args.duration+args.drain)*1_000_000_000
    host,port=args.bind.rsplit(":",1)
    port=int(port)
    flows=[]
    short_samples=[]
    long_threads=[]
    passive_threads=[]
    early_errors=[]
    if args.role=="target":
        listen=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
        listen.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
        listen.bind((host,port))
        listen.listen(128)
        listen.settimeout(.2)
        accepting=True
        seen=set()
        while now_ns()<stop and (len(seen)<TCP_LONG_CONNECTIONS or now_ns()<start+args.duration*1_000_000_000):
            try:
                conn,_=listen.accept()
            except socket.timeout:
                continue
            except OSError as ex:
                early_errors.append("accept:"+str(ex))
                break
            conn.settimeout(.4)
            try:
                h=read_exact(conn,HEADER.size,now_ns()+2_000_000_000)
                magic,index=HEADER.unpack(h)
                if magic!=MAGIC:raise ValueError("invalid TCP flow header")
            except (OSError,ConnectionError,ValueError,TimeoutError) as ex:
                early_errors.append("header:"+str(ex))
                conn.close()
                continue
            if index>=1000:
                thr=threading.Thread(target=short_server,
                        args=(conn,index,args.seed,short_samples,stop),daemon=True)
                thr.start()
                passive_threads.append(thr)
                continue
            if index in seen or index>=TCP_LONG_CONNECTIONS:
                early_errors.append("duplicate or invalid long flow")
                conn.close()
                continue
            seen.add(index)
            flow=OneFlow(args.role,index,conn,start,args.duration,args.drain,per_flow_bps)
            flows.append(flow)
            for target in (flow.recv_loop,flow.send_loop):
                thr=threading.Thread(target=target,daemon=True)
                thr.start()
                long_threads.append(thr)
        listen.close()
        # Remain alive during the fixed drain; the last TCP bytes may arrive
        # after the 300s active send deadline, especially with 600ms RTT.
        wait_until_ns(stop)
        if len(flows)!=TCP_LONG_CONNECTIONS:
            early_errors.append("incomplete long connection admission")
    else:
        peer_host,peer_port=args.peer.rsplit(":",1)
        peer=(peer_host,int(peer_port))
        for index in range(TCP_LONG_CONNECTIONS):
            conn=socket.socket(socket.AF_INET,socket.SOCK_STREAM)
            conn.settimeout(5)
            try:
                conn.connect(peer)
                conn.sendall(HEADER.pack(MAGIC,index))
                conn.settimeout(.3)
            except OSError as ex:
                early_errors.append("connect:"+str(ex))
                conn.close()
                continue
            flow=OneFlow(args.role,index,conn,start,args.duration,args.drain,per_flow_bps)
            flows.append(flow)
            for target in (flow.recv_loop,flow.send_loop):
                thr=threading.Thread(target=target,daemon=True)
                thr.start()
                long_threads.append(thr)
        short_thread=threading.Thread(target=short_client,
                    args=(peer,start,args.duration,args.seed,short_samples),daemon=True)
        short_thread.start()
        passive_threads.append(short_thread)
        wait_until_ns(stop)
    for thr in long_threads+passive_threads:
        thr.join(timeout=1.5)
    for flow in flows:
        try:flow.sock.close()
        except OSError:pass
    result={
       "schema":"wbd-longmix-tcp-sockets/v1","workload":args.workload,
       "role":args.role,"seed":args.seed,"start_monotonic_ns":start,
       "duration_s":args.duration,"drain_s":args.drain,
       "tcp_target_mbps_each_direction":rate,
       "tcp_short_reserved_bytes_per_s":SHORT_BYTES*SHORT_HZ,
       "tcp_long_connections_target":TCP_LONG_CONNECTIONS,
       "tcp_short_connections_target":args.duration,
       "app_write_sizes":list(TCP_WRITE_SIZES),
       "flows":[x.snapshot() for x in sorted(flows,key=lambda x:x.index)],
       "short_samples":short_samples[:MAX_SHORT_EVENTS],
       "short_cap":MAX_SHORT_EVENTS,
       "errors":early_errors[:32],
    }
    Path(args.output).write_text(json.dumps(result,separators=(",",":"))+"\n")
    print("WBD_LONGMIX_TCP_RECORDED",args.role,"flows",len(flows),
          "sent",sum(x.sent for x in flows),"recv",sum(x.received for x in flows),
          "short",len(short_samples))
    if len(flows)!=TCP_LONG_CONNECTIONS:
        raise SystemExit("invalid TCP connection setup")

if __name__=="__main__":main()
