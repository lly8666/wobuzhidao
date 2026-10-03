import argparse,json
from pathlib import Path

def collect(root,sha):
    rows=[json.loads(p.read_text()) for p in Path(root).rglob('actions-receipt.json')]
    targets=[(r.get('target',{}).get('goos'),r.get('target',{}).get('goarch')) for r in rows]
    if sorted(targets)!=[('linux','amd64'),('linux','arm64'),('windows','amd64')]:raise ValueError('missing/duplicate/unexpected package target')
    for r in rows:
        if r.get('source_sha')!=sha or r.get('schema')!='wbd-p6-actions-receipt/v1' or r.get('status',{}).get('actions')!='ACTIONS_PASS':raise ValueError('package receipt source/status mismatch')
        if any(r.get('status',{}).get(k)!='NOT_RUN' for k in ('physical','release_qualified')):raise ValueError('package falsely claims physical/release qualification')
    return dict(source_sha=sha,result='PASS',scope='HOSTED_PACKAGE_ONLY',physical='NOT_RUN',release_qualified='NOT_RUN',receipts=rows)

if __name__=='__main__':
    a=argparse.ArgumentParser();a.add_argument('--root',required=True);a.add_argument('--source-sha',required=True);a.add_argument('--output',required=True);x=a.parse_args();r=collect(x.root,x.source_sha);Path(x.output).write_text(json.dumps(r,indent=2)+'\n');print('Exact-source three-target packages PASS; physical NOT_RUN')
