"""Bounded native outer TCP header counters; no packet/payload files."""
import argparse,json,socket,struct,time
from pathlib import Path

def main():
    ap=argparse.ArgumentParser();ap.add_argument('--interface',required=True);ap.add_argument('--local-ip',required=True);ap.add_argument('--peer-ip',required=True);ap.add_argument('--seconds',type=int,default=360);ap.add_argument('--output',required=True);a=ap.parse_args()
    if not 1<=a.seconds<=660:raise ValueError('Duration out of bound')
    local=socket.inet_aton(a.local_ip);peer=socket.inet_aton(a.peer_ip);flows={};start=time.monotonic();next_report=1;received=0;dropped=0
    # ETH_P_ALL is needed for outgoing frames; ETH_P_IP sees ingress only here.
    with socket.socket(socket.AF_PACKET,socket.SOCK_RAW,socket.htons(0x0003)) as s,Path(a.output).open('w') as out:
        s.bind((a.interface,0));s.settimeout(.1)
        def report():
            nonlocal received,dropped
            packets,drop=struct.unpack('II',s.getsockopt(263,6,8));received+=packets;dropped+=drop
            out.write(json.dumps(dict(unix_ns=time.time_ns(),elapsed_seconds=time.monotonic()-start,flows=flows,observer_received=received,observer_dropped=dropped,observer_cpu_seconds=time.process_time(),payload_stored=False))+'\n');out.flush()
        while time.monotonic()-start<a.seconds:
            if time.monotonic()-start>=next_report:report();next_report=time.monotonic()-start+1
            try:b,_=s.recvfrom(160)
            except socket.timeout:continue
            if len(b)<54 or b[12:14]!=b'\x08\x00':continue
            ip=b[14:];ihl=(ip[0]&15)*4
            if ip[0]>>4!=4 or ip[9]!=6 or ihl<20 or len(ip)<ihl+20 or struct.unpack('!H',ip[6:8])[0]&0x3fff:continue
            if ip[12:16]==peer and ip[16:20]==local:direction='C2S'
            elif ip[12:16]==local and ip[16:20]==peer:direction='S2C'
            else:continue
            tcp=ip[ihl:];sport,dport,seq,ack=struct.unpack('!HHII',tcp[:12]);port=sport if direction=='C2S' else dport
            if (dport if direction=='C2S' else sport)!=443 or not 40000<=port<41024:continue
            hlen=(tcp[12]>>4)*4;length=struct.unpack('!H',ip[2:4])[0]-ihl-hlen
            if hlen<20 or length<0:continue
            key=direction+'/'+str(port);v=flows.setdefault(key,dict(packets=0,payload_packets=0,payload_bytes=0,syn=0,fin=0,rst=0,last_seq=0,last_ack=0))
            v['packets']+=1;v['payload_packets']+=int(length>0);v['payload_bytes']+=length;v['syn']+=int(bool(tcp[13]&2));v['fin']+=int(bool(tcp[13]&1));v['rst']+=int(bool(tcp[13]&4));v['last_seq']=seq;v['last_ack']=ack
        report()
if __name__=='__main__':main()
