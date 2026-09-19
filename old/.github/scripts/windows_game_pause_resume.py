import argparse,ipaddress,json,pathlib,select,shutil,socket,struct,subprocess,time

p=argparse.ArgumentParser()
p.add_argument('--target',required=True)
p.add_argument('--bind',required=True)
p.add_argument('--first-duration',type=float,default=20.0)
p.add_argument('--pause-duration',type=float,default=220.0)
p.add_argument('--second-duration',type=float,default=60.0)
p.add_argument('--rate-bps',type=int,default=2_000_000)
p.add_argument('--payload-bytes',type=int,default=1000)
p.add_argument('--out',required=True)
a=p.parse_args()
if min(a.first_duration,a.second_duration)<=0 or a.pause_duration<=0 or a.rate_bps<=0 or a.payload_bytes<32:
    raise SystemExit('invalid pause/resume load parameters')
host,port=a.target.rsplit(':',1); port=int(port)
host=str(ipaddress.IPv4Address(host)); bind_ip=str(ipaddress.IPv4Address(a.bind))
pps=a.rate_bps/(a.payload_bytes*8.0); interval=1.0/pps

def powershell(script):
    return subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',script],check=True,text=True,capture_output=True).stdout.strip()

def provision_rebind_script():
    source=pathlib.Path(__file__).resolve().parents[2]/'scripts'/'windows_tun_rebind.ps1'
    if not source.is_file():
        raise RuntimeError(f'current-HEAD route rebind script is missing: {source}')
    qualifier_path=powershell("(Get-Process -Name 'wbd-windows-qualify' -ErrorAction Stop | Select-Object -First 1 -ExpandProperty Path)")
    destination=pathlib.Path(qualifier_path).resolve().parent/'windows_tun_rebind.ps1'
    shutil.copy2(source,destination)
    print(f'WBD_KEEPALIVE_REBIND_ASSET_READY path={destination}',flush=True)

def isolate_wintun_target():
    ifindex=int(powershell(f"(Get-NetIPAddress -IPAddress '{bind_ip}' -AddressFamily IPv4 -ErrorAction Stop | Select-Object -First 1 -ExpandProperty InterfaceIndex)"))
    for prefix in ('0.0.0.0/1','128.0.0.0/1'):
        powershell(f"Get-NetRoute -DestinationPrefix '{prefix}' -InterfaceIndex {ifindex} -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Where-Object {{ $_.NextHop -eq '0.0.0.0' }} | Remove-NetRoute -Confirm:$false -ErrorAction Stop")
    target=f'{host}/32'
    existing=powershell(f"@(Get-NetRoute -DestinationPrefix '{target}' -InterfaceIndex {ifindex} -NextHop '0.0.0.0' -PolicyStore ActiveStore -ErrorAction SilentlyContinue).Count")
    added=int(existing)==0
    if added:
        powershell(f"New-NetRoute -DestinationPrefix '{target}' -InterfaceIndex {ifindex} -NextHop '0.0.0.0' -RouteMetric 5 -PolicyStore ActiveStore | Out-Null")
    remaining=0
    for prefix in ('0.0.0.0/1','128.0.0.0/1'):
        remaining+=int(powershell(f"@(Get-NetRoute -DestinationPrefix '{prefix}' -InterfaceIndex {ifindex} -PolicyStore ActiveStore -ErrorAction SilentlyContinue | Where-Object {{ $_.NextHop -eq '0.0.0.0' }}).Count"))
    target_count=int(powershell(f"@(Get-NetRoute -DestinationPrefix '{target}' -InterfaceIndex {ifindex} -NextHop '0.0.0.0' -PolicyStore ActiveStore -ErrorAction SilentlyContinue).Count"))
    if remaining or target_count==0:
        raise RuntimeError(f'keepalive route isolation failed remaining_full={remaining} target_count={target_count}')
    print(f'WBD_KEEPALIVE_ROUTE_ISOLATION_PASS prefix={target} ifindex={ifindex}',flush=True)
    return ifindex,target,added

def cleanup_isolation(ifindex,target,added):
    if added:
        subprocess.run(['powershell.exe','-NoProfile','-NonInteractive','-Command',f"Remove-NetRoute -DestinationPrefix '{target}' -InterfaceIndex {ifindex} -NextHop '0.0.0.0' -PolicyStore ActiveStore -Confirm:$false -ErrorAction SilentlyContinue"],check=False)

provision_rebind_script()
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM); s.bind((bind_ip,0)); s.setblocking(False)
isolation=isolate_wintun_target()
try:
    start=time.monotonic()
    phase1_end=start+a.first_duration
    phase2_start=phase1_end+a.pause_duration
    phase2_end=phase2_start+a.second_duration
    seq=0; sent={1:0,2:0}; seen={1:set(),2:set()}; dup=bad=0; rx_times={1:[],2:[]}
    pad=b'Z'*max(0,a.payload_bytes-21)
    next_tx={1:start,2:phase2_start}
    last_rx=start

    def send_due(phase,deadline):
        global seq
        now=time.monotonic()
        while next_tx[phase] < deadline and now >= next_tx[phase]:
            payload=b'WBD2'+struct.pack('!QBd',seq,phase,next_tx[phase]-start)+pad
            payload=payload[:a.payload_bytes]
            s.sendto(payload,(host,port))
            sent[phase]+=1; seq+=1; next_tx[phase]+=interval; now=time.monotonic()

    def recv_once(timeout):
        global dup,bad,last_rx
        r,_,_=select.select([s],[],[],timeout)
        if not r: return
        try: data,_=s.recvfrom(65535)
        except BlockingIOError: return
        now=time.monotonic(); last_rx=now
        if len(data)!=a.payload_bytes or not data.startswith(b'WBD2'):
            bad+=1; return
        rseq,phase=struct.unpack('!QB',data[4:13])
        if phase not in seen:
            bad+=1; return
        if rseq in seen[phase]: dup+=1; return
        seen[phase].add(rseq); rx_times[phase].append(now-start)

    while True:
        now=time.monotonic()
        if now < phase1_end:
            send_due(1,phase1_end)
        elif now < phase2_start:
            pass
        elif now < phase2_end:
            send_due(2,phase2_end)
        recv_once(0.002)
        now=time.monotonic()
        if now>=phase2_end and (len(seen[1])+len(seen[2])>=sent[1]+sent[2] or now-last_rx>=5 or now>=phase2_end+65):
            break

    result={
        'target':a.target,'bind':a.bind,'payload_bytes':a.payload_bytes,'requested_bps':a.rate_bps,
        'first_duration_sec':a.first_duration,'pause_duration_sec':a.pause_duration,'second_duration_sec':a.second_duration,
        'phase1':{'sent':sent[1],'unique_rx':len(seen[1]),'inner_loss_pct':(100.0*(sent[1]-len(seen[1]))/sent[1] if sent[1] else 100.0),'rx_times_sec':rx_times[1]},
        'phase2':{'sent':sent[2],'unique_rx':len(seen[2]),'inner_loss_pct':(100.0*(sent[2]-len(seen[2]))/sent[2] if sent[2] else 100.0),'rx_times_sec':rx_times[2]},
        'duplicate':dup,'bad_payload':bad,'elapsed_sec':time.monotonic()-start,
        'phase1_end_sec':a.first_duration,'phase2_start_sec':a.first_duration+a.pause_duration,'phase2_end_sec':a.first_duration+a.pause_duration+a.second_duration,
        'route_isolation_prefix':isolation[1],
    }
    if rx_times[1]: result['phase1_last_rx_sec']=max(rx_times[1])
    if rx_times[2]: result['phase2_first_rx_sec']=min(rx_times[2]); result['phase2_recovery_delay_sec']=max(0.0,min(rx_times[2])-(a.first_duration+a.pause_duration))
    with open(a.out,'w') as f: json.dump(result,f,sort_keys=True)
    summary={k:v for k,v in result.items() if k not in ('phase1','phase2')}
    summary['phase1']={k:v for k,v in result['phase1'].items() if k!='rx_times_sec'}
    summary['phase2']={k:v for k,v in result['phase2'].items() if k!='rx_times_sec'}
    print('WBD_WINDOWS_GAME_PAUSE_RESUME_RESULT '+json.dumps(summary,sort_keys=True))
finally:
    cleanup_isolation(*isolation)
    s.close()
