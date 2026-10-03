import argparse,json,re
from pathlib import Path

def jl(p):return [json.loads(x) for x in p.read_text().splitlines() if x.strip()]
def owner(r,side):return (r['product'] if side=='client' else r['product'].get('tunnel',{})).get('owner',{})

def check(root,mode,sha):
    errors=[];manifest=json.loads((root/'manifest.json').read_text())
    if manifest['source_sha']!=sha:errors.append('wrong source')
    if 'WBD_ROUTE_POLICY' not in (root/'client.log').read_text():errors.append('no runtime route receipt')
    rules=(root/'tproxy-final.txt').read_text()
    if mode!='all' and not re.search(r'@direct4 counter packets [1-9][0-9]*',rules):errors.append('no kernel direct hits')
    expected={'lan':mode!='all','cn':mode=='embedded','foreign':mode=='manual'}
    for label,direct in expected.items():
        target=json.loads((root/f'{label}-target.json').read_text());biz=json.loads((root/f'{label}-biz.json').read_text())
        want='10.40.0.2' if direct else '10.50.0.1'
        if target.get('peers')!={'dns':want,'tcp':want,'https':want}:errors.append(f'{label} actual source differs: {target.get("peers")} want={want}')
        if biz.get('result')!='PASS' or not all(biz.get(k) for k in ('dns_exact','tcp_exact','https_exact')):errors.append(f'{label} content failed')
    if mode!='all':
        events=jl(root/'events.jsonl');rows={s:jl(root/f'{s}-diag.jsonl') for s in ('client','server')}
        for event in ('split_dormant_before','split_dormant_after'):
            at=next(x['unix_ns'] for x in events if x['event']==event)
            for side,rr in rows.items():
                row=min(rr,key=lambda x:abs(x['unix_ns']-at));o=owner(row,side)
                if not o.get('Dormant') or o.get('PhysicalLanes')!=0:errors.append(f'{side} {event} not physically dormant')
        # Direct probes while dormant must stay on the original path and not
        # generate a tunnel wake; the foreign/CN wake probe must use server.
        for label,direct in expected.items():
            if direct:
                t=json.loads((root/f'dormant-{label}-target.json').read_text())
                if any(ip!='10.40.0.2' for ip in t['peers'].values()):errors.append('dormant direct went through tunnel')
        wake=json.loads((root/'wake-target.json').read_text())
        if any(ip!='10.50.0.1' for ip in wake['peers'].values()):errors.append('wake did not tunnel')
    after=(root/'rules-final.txt').read_text()
    if '1066:' in after:errors.append('policy rule leak')
    if 'wbd_tproxy' in (root/'nft-after.txt').read_text():errors.append('owned nft table leak')
    result={'result':'FAIL' if errors else 'PASS','source_sha':sha,'mode':mode,'errors':errors,'routing':expected,'scope':'real-process TCP/UDP-DNS/102400B TLS1.3 HTTPS, direct during dormancy, proxy wake; IPv4 only'}
    (root/'summary-splitroute.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result));return result

if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('--artifact-dir',required=True);p.add_argument('--mode',required=True);p.add_argument('--source-sha',required=True);a=p.parse_args();r=check(Path(a.artifact_dir),a.mode,a.source_sha)
    if r['errors']:raise SystemExit(1)
