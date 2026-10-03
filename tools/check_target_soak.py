"""Predeclared full-run soak gates. Never treat a shorter canary as30min PASS."""
import argparse
import json
import math
import re
import statistics
from pathlib import Path
from check_strict_weaknet_loss_tolerant_v1 import resource_summary, pct, parse_skmem_stats
from check_lifecycle_acceptance import prod, own, gens


def rows(path):
    with open(path) as f:
        for line in f:
            yield json.loads(line)


def main():
    a=argparse.ArgumentParser();a.add_argument('--artifact-dir',required=True);a.add_argument('--source-sha',required=True);a.add_argument('--output',required=True)
    x=a.parse_args();root=Path(x.artifact_dir);m=json.loads((root/'manifest.json').read_text())
    duration=m['config']['duration_s'];rate=m['config']['application_mbps_each_direction'];errors=[]
    if m['schema']!='wbd-target-soak/v1' or m['source_sha']!=x.source_sha or duration not in [180,1800]:errors.append('source/schema/duration mismatch')
    if m['qualification']!=('FORMAL' if duration==1800 else 'DIAGNOSTIC_ONLY'):errors.append('incorrect qualification scope')
    if (m['mode'],m['config']['lanes'],rate) not in [('normal',1,10),('game',4,3)]:errors.append('wrong workload')
    if m['config']['fec']!='20:20' or m['config']['padding']!='off' or m['config']['one_way_delay_ms']!=300:errors.append('changed product/network configuration')
    start=int((root/'start-unix-ns.txt').read_text());end=start+duration*1_000_000_000
    b=json.loads((root/'biz.json').read_text());t=json.loads((root/'target.json').read_text());phases=[]
    for g in [b,t]:
        s=g['stats'];bounded=s.get('bounded_stats',{})
        if not bounded.get('exact_dedupe') or bounded.get('capacity_errors') or not bounded.get('sequence_capacity'):errors.append('invalid bounded statistics')
        if any(z['overflow'] for z in bounded.get('histograms',{}).values()):errors.append('latency histogram overflow')
        if s['corrupt'] or s['unexpected'] or s['recv_duplicates']:errors.append('application integrity/duplicate failure')
        if s['send_failures'] or s['skipped_slots']>max(1,s['sent_packets']*.0001) or s['send_lag_p99_ns'] is None or s['send_lag_p99_ns']>10_000_000:errors.append('input validity failed')
    capture=json.loads((root/'capture-receipt.json').read_text())
    for point,p in capture['points'].items():
        if any(z['bad_headers'] for z in p['chunks']):errors.append(point+' invalid captured headers/MTU/fragment')
        log=(root/(point+'.tcpdump.log')).read_text()
        dropped=re.findall(r'(\d+) packets dropped by kernel',log)
        if not dropped or max(map(int,dropped))!=0:errors.append(point+' capture drops/missing final receipt')
    for phase in m['stage_plan']:
        lo,hi=phase['start_s'],phase['end_s'];loss=phase['loss_percent'];p=dict(phase=phase['name'],loss_limit_percent=loss,directions={})
        for d,sender,receiver in [('c2s',b,t),('s2c',t,b)]:
            ss,rs=sender['stats'],receiver['stats'];sent=sum(ss['sent_packets_by_second'][lo:hi]);received=sum(rs['recv_packets_by_second'][lo:hi]);sentbytes=sum(ss['sent_bytes_by_second'][lo:hi])
            fraction=100*(sent-received)/sent if sent else None
            offered=sentbytes*8/(hi-lo)/1e6
            # Same300ms path alignment as strict120s: 10ms wall buckets.
            wall=sum(rs['recv_wall_bytes_by_bucket'][lo*100+30:hi*100+30])*8/(hi-lo)/1e6
            cp=capture['points'][d+'-pre']['packets_by_second'];cr=capture['points'][d+'-post']['packets_by_second']
            pre=sum(cp[lo+2:hi-2]);post=sum(cr[lo+2:hi-2]);actual=100*(pre-post)/pre if pre else None
            p['directions'][d]=dict(packet_loss_percent=fraction,wall_goodput_mbps=wall,offered_mbps=offered,netem_actual_loss_percent=actual)
            if not rate*.99<=offered<=rate*1.01:errors.append(phase['name']+'/'+d+' input rate')
            if fraction is None or fraction>loss or wall<rate*(1-loss/100)*.99:errors.append(phase['name']+'/'+d+' loss/goodput')
            if actual is None or abs(actual-loss)>2:errors.append(phase['name']+'/'+d+' netem injection')
        probe=b['stats']['probe_rtt_ns_by_second'][lo:hi];p['probe']=dict(p95_ns=pct(probe,.95),p99_ns=pct(probe,.99),received=sum(v is not None for v in probe))
        if p['probe']['received']<(hi-lo)*(1-loss/100)*.9:errors.append(phase['name']+' probe coverage')
        if p['probe']['p95_ns'] is None or p['probe']['p95_ns']>850_000_000 or p['probe']['p99_ns']>1_100_000_000:errors.append(phase['name']+' probe tail latency')
        phases.append(p)
    # Read bounded per-second resource rows only; no per-packet PCAP expansion.
    resource=resource_summary(list(rows(root/'resources.jsonl')),start,end)
    errors+=resource['errors']
    diagnostics={}
    for side in ['client','server']:
        diag=list(rows(root/(side+'-diag.jsonl')));active=[r for r in diag if prod(r,side) and own(r,side).get('ActiveLogicalLanes')==m['config']['lanes']]
        if not active:errors.append(side+' missing active diagnostics');continue
        first,last=gens(active[0],side),gens(active[-1],side)
        if not any(last.get(i,0)>g for i,g in first.items()):errors.append(side+' automatic rotation not observed')
        heaps=[(r['unix_ns'],r['runtime']['heap_inuse']) for r in diag if 'runtime' in r]
        early=[v for ts,v in heaps if start+duration*.2*1e9<=ts<start+duration*.4*1e9]
        late=[v for ts,v in heaps if start+duration*.8*1e9<=ts<end]
        if not early or not late or statistics.median(late)>statistics.median(early)*1.5+32*1024**2:errors.append(side+' heap plateau')
        if max([v for _,v in heaps]+[0])>256*1024**2:errors.append(side+' heap hard bound')
        if any(own(r,side).get('PhysicalLanes',0)>10 for r in diag):errors.append(side+' physical lane bound')
        tail=[r for r in active if r['unix_ns']>end+30*1e9]
        if not tail:errors.append(side+' missing drain diagnostics')
        for r in tail:
            for lane in prod(r,side).get('lanes',[]):
                paths=lane.get('lane',{})
                for key in ['TxPath','RxPath']:
                    state=paths.get(key,{})
                    if state.get('Recovery',{}).get('PendingDeadlines',0) or state.get('Reassembly',{}).get('Assemblies',0):errors.append(side+' drain FEC/LINK state retained')
        diagnostics[side]=dict(first_generation=first,last_generation=last,heap_peak=max([v for _,v in heaps]+[0]),early_heap_median=statistics.median(early) if early else None,late_heap_median=statistics.median(late) if late else None)
    result=dict(schema='wbd-target-soak-result/v1',source_sha=x.source_sha,qualification=m['qualification'],duration_s=duration,mode=m['mode'],result='FAIL' if errors else 'PASS',errors=errors,phases=phases,resource=resource,diagnostics=diagnostics,statistics={k:v['stats']['bounded_stats'] for k,v in [('biz',b),('target',t)]})
    Path(x.output).write_text(json.dumps(result,indent=2)+'\n');print(json.dumps({k:result[k] for k in ['source_sha','qualification','mode','result','errors']}))
    if errors:raise SystemExit(1)


if __name__=='__main__':main()
