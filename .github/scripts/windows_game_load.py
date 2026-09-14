import argparse,json,select,socket,struct,time

p=argparse.ArgumentParser()
p.add_argument('--target',default='127.0.0.1:47500')
p.add_argument('--duration',type=float,default=300)
p.add_argument('--rate-bps',type=int,default=2_000_000)
p.add_argument('--payload-bytes',type=int,default=1000)
p.add_argument('--out',required=True)
a=p.parse_args()
host,port=a.target.rsplit(':',1); port=int(port)
pps=a.rate_bps/(a.payload_bytes*8.0); interval=1.0/pps
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind(('127.0.0.1',0)); s.setblocking(False)
start=time.monotonic(); deadline=start+a.duration; next_tx=start
sent=0; seen=set(); dup=bad=0; rx_times=[]; last_rx=start
pad=b'Z'*max(0,a.payload_bytes-20)
while True:
    now=time.monotonic()
    while next_tx < deadline and now >= next_tx:
        payload=b'WBD1'+struct.pack('!Qd',sent,next_tx-start)+pad
        payload=payload[:a.payload_bytes]
        s.sendto(payload,(host,port)); sent+=1; next_tx+=interval; now=time.monotonic()
    r,_,_=select.select([s],[],[],0.002)
    if r:
        try: data,_=s.recvfrom(65535)
        except BlockingIOError: data=b''
        if len(data)!=a.payload_bytes or not data.startswith(b'WBD1'):
            bad+=1
        else:
            seq=struct.unpack('!Q',data[4:12])[0]
            if seq in seen: dup+=1
            else:
                seen.add(seq); t=time.monotonic()-start; rx_times.append(t); last_rx=time.monotonic()
    now=time.monotonic()
    if now>=deadline and (len(seen)>=sent or now-last_rx>=5 or now>=deadline+65): break
max_gap=0.0
if len(rx_times)>1: max_gap=max(b-a for a,b in zip(rx_times,rx_times[1:]))
result={
 'requested_duration_sec':a.duration,'requested_bps':a.rate_bps,'payload_bytes':a.payload_bytes,
 'sent':sent,'unique_rx':len(seen),'duplicate':dup,'bad_payload':bad,
 'inner_loss_pct':(100.0*(sent-len(seen))/sent if sent else 100.0),
 'max_rx_gap_ms':max_gap*1000.0,'elapsed_sec':time.monotonic()-start,
 'rx_times_sec':rx_times,
}
with open(a.out,'w') as f: json.dump(result,f,sort_keys=True)
print('WBD_WINDOWS_GAME_LOAD_RESULT '+json.dumps({k:v for k,v in result.items() if k!='rx_times_sec'},sort_keys=True))
