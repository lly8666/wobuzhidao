"""Serialized control-only dispatch and collection; never execute a workload."""
import hashlib,io,json,os,re,subprocess,time,zipfile
from pathlib import Path
req=json.loads(Path('.github/predelivery-request.json').read_text());repo=os.environ['GITHUB_REPOSITORY'];root=Path('predelivery');root.mkdir(exist_ok=True)
(root/'request.json').write_text(json.dumps(req,indent=2)+'\n')
sha=req['source_sha'];ref=req['ref']
if not re.fullmatch(r'perf-fixed/'+sha+r'(?:-r\d+)?',ref) or not re.fullmatch('[a-f0-9]{40}',sha):raise SystemExit('immutable source/ref invalid')
def api(path):return json.loads(subprocess.check_output(['gh','api',path]))
def artifact(run_id,name):
    arts=api(f'repos/{repo}/actions/runs/{run_id}/artifacts')['artifacts']
    selected=[a for a in arts if a['name']==name and not a['expired']]
    if len(selected)!=1:raise ValueError('missing/ambiguous artifact '+name)
    a=selected[0];blob=subprocess.check_output(['gh','api',f'repos/{repo}/actions/artifacts/{a["id"]}/zip'])
    if a.get('digest')!='sha256:'+hashlib.sha256(blob).hexdigest():raise ValueError('artifact digest mismatch')
    return a,blob
authorized_cancelled=set()
deadline=time.monotonic()+20*60
if req.get('retirement_run'):
    id=req['retirement_run']
    while True:
        r=api(f'repos/{repo}/actions/runs/{id}')
        if r['path']!='.github/workflows/next-predelivery-retire.yml':raise SystemExit('unexpected control gate')
        if r['status']=='completed':
            if r['conclusion']!='success':raise SystemExit('retirement gate failed')
            break
        if time.monotonic()>deadline:raise SystemExit('retirement gate timeout')
        time.sleep(10)
    a,blob=artifact(id,f'control-retirement-{id}')
    with zipfile.ZipFile(io.BytesIO(blob)) as z:retirement=json.loads(z.read('receipt.json'))
    if retirement['request']['source_sha']!=sha or retirement['result']!='PASS':raise SystemExit('retirement receipt mismatch')
    authorized_cancelled={x['id'] for x in retirement['cancellations'] if x.get('cancel_requested') and x['reason'].startswith('extra duplicate dispatch')}
    (root/'control-retirement.json').write_text(json.dumps(dict(run=r,artifact=a,receipt=retirement),indent=2)+'\n')
for id in req['required_runs']:
    while True:
        r=api(f'repos/{repo}/actions/runs/{id}')
        if r['head_sha']!=sha:raise SystemExit('required gate source mismatch')
        if r['status']=='completed':
            if r['conclusion']!='success':raise SystemExit('required gate failed; no dispatch')
            (root/f'gate-{id}.json').write_text(json.dumps(r,indent=2)+'\n');break
        if time.monotonic()>deadline:raise SystemExit('required gate timeout')
        time.sleep(20)
titles={p['title']:p for p in req['samples']}
if len(titles)!=len(req['samples']):raise SystemExit('duplicate plan identity')
refs=sorted({p.get('ref',ref) for p in titles.values()})
for sample_ref in refs:
    if not re.fullmatch(r'perf-fixed/'+sha+r'(?:-r\d+)?',sample_ref):raise SystemExit('sample frozen ref invalid')
    if subprocess.check_output(['git','ls-remote','origin','refs/heads/'+sample_ref]).decode().split()[0]!=sha:raise SystemExit('sample source mismatch')
def inventory():
    rs=[]
    for sample_ref in refs:
        for page in range(1,21):
            batch=api(f'repos/{repo}/actions/runs?event=workflow_dispatch&branch={sample_ref}&per_page=100&page={page}')['workflow_runs'];rs+=batch
            if len(batch)<100:break
        else:raise SystemExit('run inventory exceeded bound')
    groups={}
    for title,p in titles.items():
        group=sorted([r for r in rs if r['head_sha']==sha and r['head_branch']==p.get('ref',ref) and r['display_title']==p.get('run_title',title)],key=lambda r:r['id'])
        if group:groups[title]=group
    return groups
existing=inventory()
for title,p in titles.items():
    if p['workflow'] not in ['next-target-soak.yml','next-config-effective.yml','next-p6-package.yml','next-lifecycle-fullstack.yml','next-shared-blackhole.yml','next-splitroute.yml']:raise SystemExit('unsupported workflow')
    if title in existing:
        if any(r['path']!='.github/workflows/'+p['workflow'] for r in existing[title]):raise SystemExit('existing workflow identity mismatch')
        print('REUSE FIRST ORIGINAL '+title+' '+str(existing[title][0]['id']),flush=True);continue
    cmd=['gh','workflow','run',p['workflow'],'--ref',p.get('ref',ref)]
    for k,v in p['inputs'].items():cmd+=['-f',f'{k}={v}']
    subprocess.check_call(cmd);print('DISPATCH MISSING '+title,flush=True)
deadline=time.monotonic()+80*60
while True:
    groups=inventory()
    print(f'PROGRESS {len(groups)}/{len(titles)} canonical-completed={sum(v[0]["status"]=="completed" for v in groups.values())}',flush=True)
    if len(groups)==len(titles) and all(r['status']=='completed' for group in groups.values() for r in group):break
    if time.monotonic()>deadline:raise SystemExit('campaign incomplete')
    time.sleep(30)
errors=[];receipts=[];extra_receipts=[]
def collect(title,p,r):
    folder=root/title/str(r['id']);folder.mkdir(parents=True)
    (folder/'run-receipt.json').write_text(json.dumps(r,indent=2)+'\n')
    receipt=dict(title=title,run=r['id'],url=r['html_url'],original_conclusion=r['conclusion'],run_attempt=r['run_attempt'])
    if r['conclusion']=='cancelled' and r['id'] in authorized_cancelled:
        receipt['scope']='EXTRA_DUPLICATE_CANCELLED_BY_CONTROL_RECEIPT';return receipt
    if r['conclusion']!='success' or r['run_attempt']!=1:errors.append(f'{title}/{r["id"]}: original run failed')
    try:
        a,blob=artifact(r['id'],p['artifact'])
        with zipfile.ZipFile(io.BytesIO(blob)) as z:
            names=[n for n in z.namelist() if n.endswith(p['summary'])]
            if len(names)!=1:raise ValueError('ambiguous summary')
            summary=json.loads(z.read(names[0]))
        if p['workflow']=='next-shared-blackhole.yml':
            classes=summary.get('classifications',{})
            passed=all(classes.get(k)=='PASS' for k in ['CORRECTNESS','INPUT_VALIDITY','CAPTURE','ENVIRONMENT','PERFORMANCE'])
        else:passed=summary.get('result')=='PASS'
        if summary.get('source_sha')!=sha or not passed:errors.append(f'{title}/{r["id"]}: invalid summary/source')
        (folder/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
        receipt.update(artifact=a['id'],digest=a['digest'],summary=summary)
    except (ValueError,KeyError) as e:errors.append(f'{title}/{r["id"]}: '+str(e))
    return receipt
for title,p in titles.items():
    group=groups[title]
    receipts.append(collect(title,p,group[0]))
    for r in group[1:]:extra_receipts.append(collect(title,p,r))
result=dict(campaign=req['campaign'],source_sha=sha,result='FAIL' if errors else 'PASS',errors=errors,canonical_policy='first original run ID, never select best result',receipts=receipts,extra_dispatch_receipts=extra_receipts)
(root/'result.json').write_text(json.dumps(result,indent=2)+'\n');print('RESULT '+json.dumps(dict(result=result['result'],errors=errors)),flush=True)
if errors:raise SystemExit(1)
