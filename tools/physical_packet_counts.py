"""In-memory IPv4/TCP header counters only. Never writes packet bytes."""
import argparse, json, socket, struct, time
from pathlib import Path

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument('--interface',required=True)
    ap.add_argument('--local-ip',required=True)
    ap.add_argument('--port',type=int,default=443)
    ap.add_argument('--seconds',type=int,default=8)
    ap.add_argument('--output',required=True)
    args=ap.parse_args()
    if not 1<=args.seconds<=15:raise ValueError('bounded observation:1..15 seconds')
    local=socket.inet_aton(args.local_ip);flows={};fragmented=0
    with socket.socket(socket.AF_PACKET,socket.SOCK_RAW,socket.htons(0x0800)) as s:
        s.bind((args.interface,0));s.settimeout(.2)
        deadline=time.monotonic()+args.seconds
        while time.monotonic()<deadline:
            try:frame,address=s.recvfrom(96)
            except socket.timeout:continue
            if len(frame)<54 or frame[12:14]!=b'\x08\x00':continue
            ip=frame[14:]
            ihl=(ip[0]&15)*4
            if ip[0]>>4!=4 or ip[9]!=6 or len(ip)<ihl+20:continue
            sport,dport=struct.unpack('!HH',ip[ihl:ihl+4])
            if ip[12:16]==local and sport==args.port:direction='S2C';peer_port=dport
            elif ip[16:20]==local and dport==args.port:direction='C2S';peer_port=sport
            else:continue
            total=struct.unpack('!H',ip[2:4])[0];tcp=(ip[ihl+12]>>4)*4;flags=ip[ihl+13]
            fragment=struct.unpack('!H',ip[6:8])[0]&0x3fff
            if fragment:fragmented+=1
            key='%s:%d'%(direction,peer_port)
            f=flows.setdefault(key,dict(Direction=direction,PeerPort=peer_port,Packets=0,IPv4Bytes=0,DataPackets=0,SYN=0,FIN=0,RST=0,MaxIPv4Length=0))
            f['Packets']+=1;f['IPv4Bytes']+=total
            if total>ihl+tcp:f['DataPackets']+=1
            for flag,label in ((2,'SYN'),(1,'FIN'),(4,'RST')):
                if flags&flag:f[label]+=1
            f['MaxIPv4Length']=max(f['MaxIPv4Length'],total)
        observed,drops=struct.unpack('II',s.getsockopt(263,6,8))
    result=dict(Seconds=args.seconds,Flows=list(flows.values()),ObserverKernelPackets=observed,
                ObserverSocketDrops=drops,FragmentedTargetPackets=fragmented,
                NoPcap=True,NoPayloadStored=True)
    Path(args.output).write_text(json.dumps(result,indent=2))
    print(json.dumps(result))

if __name__=='__main__':main()
