#!/usr/bin/env python3
"""Read-only pre-netem outer FakeTCP replay counter; no packet or secret persistence."""
import argparse, hashlib, json, selectors, socket, struct, time
from pathlib import Path

SOL_PACKET=263
PACKET_STATISTICS=6
BOTH={"rcli":(b"\xc6\x12\x00\x02",b"\xc6\x12\x00\x06","c2s"),
      "rsrv":(b"\xc6\x12\x00\x06",b"\xc6\x12\x00\x02","s2c")}
def parse(frame, source, destination):
    if len(frame)<54 or frame[12:14]!=b"\x08\x00":return None
    ip=frame[14:]
    if (ip[0]>>4)!=4:return None
    ihl=(ip[0]&15)*4
    if ihl<20 or len(ip)<ihl+20 or ip[9]!=6 or ip[12:16]!=source or ip[16:20]!=destination:return None
    if (struct.unpack("!H",ip[6:8])[0]&0x3fff)!=0:return ("fragment",)
    iplen=struct.unpack("!H",ip[2:4])[0]
    thl=(ip[ihl+12]>>4)*4
    if thl<20 or iplen<ihl+thl:return ("malformed",)
    plen=iplen-ihl-thl
    seq=struct.unpack("!I",ip[ihl+4:ihl+8])[0]
    ports=ip[ihl:ihl+4]
    payload=ip[ihl+thl:ihl+thl+plen]
    if len(payload)!=plen:return ("malformed",)
    return ("tcp",iplen,plen,seq,ports,payload)

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--start-ns",type=int,required=True)
    p.add_argument("--duration-s",type=int,choices=(120,),required=True)
    p.add_argument("--output",required=True)
    x=p.parse_args()
    sel=selectors.DefaultSelector()
    rows={}
    for iface,(src,dst,side) in BOTH.items():
        sock=socket.socket(socket.AF_PACKET,socket.SOCK_RAW,socket.htons(0x0800))
        sock.setsockopt(socket.SOL_SOCKET,socket.SO_RCVBUF,4*1024*1024)
        sock.bind((iface,0))
        sock.setblocking(False)
        sel.register(sock,selectors.EVENT_READ,(side,src,dst))
        rows[side]={"interface":iface,"tcp_outer_packets":0,"tcp_outer_ip_bytes":0,
                    "fresh_data_packets":0,"fresh_data_payload_bytes":0,"fresh_data_ip_bytes":0,
                    "repeat_data_packets":0,"repeat_data_payload_bytes":0,"repeat_data_ip_bytes":0,
                    "control_packets":0,"control_ip_bytes":0,"malformed":0,
                    "fragmented":0,"same_seq_different_ciphertext":0,"seen":{}}
    deadline=x.start_ns+x.duration_s*1000000000
    while time.monotonic_ns()<deadline:
        remain=max(0,(deadline-time.monotonic_ns())/1e9)
        for key,_ in sel.select(min(.2,remain)):
            sock=key.fileobj
            side,src,dst=key.data
            for _ in range(96):
                try:buf,addr=sock.recvfrom(2048)
                except BlockingIOError:break
                if addr[2]==4 or time.monotonic_ns()<x.start_ns:continue
                packet=parse(buf,src,dst)
                if packet is None:continue
                row=rows[side]
                if packet[0]!="tcp":
                    row["fragmented" if packet[0]=="fragment" else "malformed"]+=1
                    continue
                _,iplen,plen,seq,ports,payload=packet
                row["tcp_outer_packets"]+=1
                row["tcp_outer_ip_bytes"]+=iplen
                if plen==0:
                    row["control_packets"]+=1
                    row["control_ip_bytes"]+=iplen
                    continue
                k=(seq,ports,plen)
                digest=hashlib.blake2b(payload,digest_size=8).digest()
                old=row["seen"].get(k)
                if old is None:
                    row["seen"][k]=digest
                    row["fresh_data_packets"]+=1
                    row["fresh_data_payload_bytes"]+=plen
                    row["fresh_data_ip_bytes"]+=iplen
                else:
                    if old!=digest:row["same_seq_different_ciphertext"]+=1
                    row["repeat_data_packets"]+=1
                    row["repeat_data_payload_bytes"]+=plen
                    row["repeat_data_ip_bytes"]+=iplen
    for key in list(sel.get_map().values()):
        sock=key.fileobj
        side=key.data[0]
        raw=sock.getsockopt(SOL_PACKET,PACKET_STATISTICS,8)
        pkts,drops=struct.unpack("II",raw)
        rows[side]["kernel_packet_socket_packets"]=pkts
        rows[side]["kernel_packet_socket_drops"]=drops
        sock.close()
    for row in rows.values():
        row.pop("seen")
        base=row["fresh_data_payload_bytes"]
        repair=row["repeat_data_payload_bytes"]
        all_ip=row["fresh_data_ip_bytes"]+row["repeat_data_ip_bytes"]
        row["repeat_over_fresh_payload_percent"]=(100*repair/base if base else None)
        row["repeat_fraction_data_payload_percent"]=(100*repair/(base+repair) if base+repair else None)
        row["repeat_fraction_data_ip_bytes_percent"]=(100*row["repeat_data_ip_bytes"]/all_ip if all_ip else None)
        row["capture_ok"]=bool(row["tcp_outer_packets"]>1000 and row["kernel_packet_socket_drops"]==0
            and not row["malformed"] and not row["fragmented"] and not row["same_seq_different_ciphertext"])
    result={"schema":"wbd-fec-retrans-wire-observation/v1","scope":"one_router_ingress_pre_netem_FakeTCP_TCP_IP_packets",
            "start_monotonic_ns":x.start_ns,"duration_s":x.duration_s,
            "theory_p":0.01,"theory_repeats_over_fresh_percent":100*(.01/.99),
            "theory_repeat_fraction_all_data_percent":1.0,
            "capture_complete":all(row["capture_ok"] for row in rows.values()),
            "note":"Only data segments with identical flow/sequence/length and ciphertext are counted as retransmissions; no raw frames retained. ACK/control separately. NOT a product retransmission-counter proof; capture drops invalidate estimate.",
            "direction":rows}
    Path(x.output).write_text(json.dumps(result,sort_keys=True,indent=2)+"\n")
    print("FEC_RETRANS_WIRE",json.dumps({side:{k:r[k] for k in ("tcp_outer_packets","repeat_data_packets","repeat_over_fresh_payload_percent","kernel_packet_socket_drops")} for side,r in rows.items()}),flush=True)
    if not result["capture_complete"]:raise SystemExit("UNUSABLE_RETRANS_CAPTURE")
if __name__=="__main__":main()
