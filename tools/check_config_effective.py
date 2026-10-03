"""Fail closed on live configuration, actual business and observed wire limits."""
import argparse,json,struct
from pathlib import Path
from config_effective_args import cases
from check_lifecycle_acceptance import jl,prod,own,cfg

def check(root,sha,case):
    root=Path(root);want=cases()[case];errors=[];m=json.loads((root/'manifest.json').read_text());events=jl(root/'events.jsonl');c=jl(root/'client-diag.jsonl');s=jl(root/'server-diag.jsonl')
    if m.get('source_sha')!=sha or m.get('schema')!='wbd-config-effective/v1' or m.get('configuration')!=dict(case=case,**want):errors.append('manifest source/configuration mismatch')
    udp_end=next(int(r['unix_ns']) for r in events if r['event']=='udp_complete');start=next(int(r['unix_ns']) for r in events if r['event']=='udp_start');effective={}
    for side,rows in (('client',c),('server',s)):
        rows=[r for r in rows if prod(r,side)]
        if not rows:errors.append(side+' no actual tunnel');continue
        final=rows[-1];p=prod(final,side);o=own(final,side);effective[side]=dict(config=cfg(final,side),owner=o,lanes=p.get('lanes'))
        if o.get('ActiveLogicalLanes')!=want['lanes'] or o.get('PhysicalLanes')!=want['lanes'] or o.get('Dormant'):errors.append(side+' lane state mismatch')
        if p.get('lease4')!='10.66.0.2/32':errors.append(side+' lease mismatch')
        lanes=p.get('lanes')or[]
        if len(lanes)!=want['lanes']:errors.append(side+' missing actual lane diagnostics')
        for lane in lanes:
            st=lane.get('lane')or{};tx=st.get('TxPath')or{};rx=st.get('RxPath')or{}
            if lane.get('parity_shards')!=want['fec'] or bool(tx.get('FECEnabled'))!=bool(want['fec']) or bool(rx.get('FECEnabled'))!=bool(want['fec']):errors.append(side+' FEC runtime profile mismatch')
            if st.get('RecordErrors',0) or st.get('PathErrors',0):errors.append(side+' record/path integrity error')
            if want['fec'] and (not (tx.get('Encoder')or{}).get('SourceShards') or not (tx.get('Encoder')or{}).get('ParityShards')):errors.append(side+' requested FEC never emitted source/parity')
        pad=o.get('Padding')or{}
        if bool(pad.get('Enabled'))!=want['padding'] or bool(pad.get('TLSStartupOnly'))!=want['padding']:errors.append(side+' padding effective mismatch')
        before=[r for r in rows if start<=int(r.get('unix_ns',0))<=udp_end]
        if not before:errors.append(side+' no UDP-only checkpoint')
        elif any((own(r,side).get('Padding')or{}).get('PaddingBytes',0) for r in before):errors.append(side+' non-TLS UDP was padded')
        if not want['padding'] and pad.get('PaddingBytes',0):errors.append(side+' padding off emitted padding')
        if want['mode']!='defaults':
            expected={'keepalive_interval':'1s','dormant_after':'0s'}
            if side=='client':expected.update(dead_after='6s',reconnect_min='1s',reconnect_max='4s')
            for k,v in expected.items():
                if cfg(final,side).get(k)!=v:errors.append(side+' effective '+k+' mismatch')
    pads=[(x.get('owner')or{}).get('Padding')or{} for x in effective.values()]
    if want['padding']:
        if not sum(p.get('StartupDetected',0) for p in pads):errors.append('inner TLS startup never detected')
        if not sum(p.get('PaddingBytes',0)+p.get('HeadroomSkips',0)+p.get('StartupBudgetSkips',0) for p in pads):errors.append('startup padding never applied or explicitly budget skipped')
        if want['mtu']==1400 and want['client_limit']==1300 and not sum(p.get('PaddingBytes',0) for p in pads):errors.append('ordinary-size HTTPS emitted no requested padding')
    for name in ('business-biz','business-target'):
        if json.loads((root/(name+'.json')).read_text()).get('result')!='PASS':errors.append(name+' failed')
    business=json.loads((root/'business-biz.json').read_text())
    if not all(business.get(k) for k in ('dns_exact','tcp_exact','https_exact')):errors.append('actual DNS/TCP/HTTPS not proven')
    biz=json.loads((root/'main-biz.json').read_text())['stats'];tgt=json.loads((root/'main-target.json').read_text())['stats']
    for side,tx,rx in (('c2s',biz,tgt),('s2c',tgt,biz)):
        sent=sum(tx.get('sent_packets_by_second')or[]);received=sum(rx.get('recv_packets_by_second')or[])
        if sent<50 or sent!=received:errors.append(side+f' lossless UDP not exact {sent}/{received}')
        if any(rx.get(k,0) for k in ('corrupt','recv_duplicates','unexpected')) or tx.get('send_failures',0):errors.append(side+' UDP integrity/send error')
    wire={'c2s':dict(records=0,max_ip=0,max_record=0),'s2c':dict(records=0,max_ip=0,max_record=0)}
    with (root/'underlay.pcap').open('rb') as f:
        header=f.read(24)
        if header[:4]!=b'\xd4\xc3\xb2\xa1' or struct.unpack('<I',header[20:24])[0]!=1:raise ValueError('capture format differs')
        while True:
            h=f.read(16)
            if not h:break
            if len(h)!=16:raise ValueError('truncated capture header')
            sec,usec,n,original=struct.unpack('<IIII',h);frame=f.read(n)
            if len(frame)!=n:raise ValueError('truncated capture data')
            if sec*10**9+usec*1000<start or len(frame)<54 or frame[12:14]!=b'\x08\x00':continue
            ip=frame[14:];ih=(ip[0]&15)*4
            if ip[9]!=6:continue
            side='c2s' if ip[12:16]==bytes((198,18,0,2)) else 's2c';r=wire[side];total=struct.unpack('!H',ip[2:4])[0];r['max_ip']=max(r['max_ip'],total)
            if total>want['mtu'] or struct.unpack('!H',ip[6:8])[0]&0x3fff:errors.append(side+' MTU/fragment violation')
            tcp=ip[ih:];th=(tcp[12]>>4)*4;payload=tcp[th:];length=total-ih-th
            if length and payload[:3]==b'\x17\x03\x03' and len(payload)>=5:
                record=5+struct.unpack('!H',payload[3:5])[0];r['records']+=1;r['max_record']=max(r['max_record'],record);limit=want['server_limit'] if side=='c2s' else want['client_limit']
                if record!=length or record>limit:errors.append(side+' negotiated record limit/wire framing violation')
    if any(r['records']<10 for r in wire.values()):errors.append('insufficient observed steady TLS-like records')
    if '0 packets dropped by kernel' not in (root/'tcpdump.log').read_text():errors.append('capture dropped packets or final counter missing')
    return dict(source_sha=sha,case=case,scope='FUNCTIONAL_ONLY_NOT_PERFORMANCE',result='FAIL' if errors else 'PASS',errors=sorted(set(errors)),configuration=want,effective=effective,wire=wire,business=business)

if __name__=='__main__':
    a=argparse.ArgumentParser();a.add_argument('--artifact-dir',required=True);a.add_argument('--source-sha',required=True);a.add_argument('--case',choices=cases(),required=True);a.add_argument('--output',required=True);x=a.parse_args()
    try:r=check(x.artifact_dir,x.source_sha,x.case)
    except Exception as e:r=dict(source_sha=x.source_sha,case=x.case,result='FAIL',errors=[type(e).__name__+': '+str(e)])
    Path(x.output).write_text(json.dumps(r,indent=2)+'\n');print(json.dumps(dict(case=x.case,result=r['result'],errors=r['errors'])));raise SystemExit(0 if r['result']=='PASS' else 1)
