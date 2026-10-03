"""Control-only dispatch/collection. Never execute a workload here."""
import hashlib,io,json,os,re,subprocess,time,zipfile
from pathlib import Path
req=json.loads(Path('.github/predelivery-request.json').read_text());repo=os.environ['GITHUB_REPOSITORY'];root=Path('predelivery');root.mkdir(exist_ok=True)
(root/'request.json').write_text(json.dumps(req,indent=2)+'\n')
sha=req['source_sha'];ref=req['ref']
if not re.fullmatch(r'perf-fixed/'+sha+r'(?:-r\d+)?',ref) or not re.fullmatch('[a-f0-9]{40}',sha):raise SystemExit('immutable source/ref invalid')
if subprocess.check_output(['git','ls-remote','origin','refs/heads/'+ref]).decode().split()[0]!=sha:raise SystemExit('frozen ref/source mismatch')
def api(path):return json.loads(subprocess.check_output(['gh','api',path]))
deadline=time.monotonic()+20*60
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
for title,p in titles.items():
    if p['workflow'] not in ['next-target-soak.yml','next-config-effective.yml','next-p6-package.yml']:raise SystemExit('unsupported workflow')
    cmd=['gh','workflow','run',p['workflow'],'--ref',ref]
    for k,v in p['inputs'].items():cmd+=['-f',f'{k}={v}']
    subprocess.check_call(cmd)
    print('DISPATCH '+title,flush=True)
deadline=time.monotonic()+80*60
while True:
    rs=api(f'repos/{repo}/actions/runs?event=workflow_dispatch&branch={ref}&per_page=100')['workflow_runs']
    matches={}
    for r in rs:
        if r['head_sha']==sha and r['display_title'] in titles:
            if r['display_title'] in matches:raise SystemExit('duplicate run identity')
            matches[r['display_title']]=r
    print(f'PROGRESS {len(matches)}/{len(titles)} completed={sum(r["status"]=="completed" for r in matches.values())}',flush=True)
    if len(matches)==len(titles) and all(r['status']=='completed' for r in matches.values()):break
    if time.monotonic()>deadline:raise SystemExit('campaign incomplete')
    time.sleep(30)
errors=[];receipts=[]
for title,p in titles.items():
    r=matches[title];folder=root/title;folder.mkdir()
    (folder/'run-receipt.json').write_text(json.dumps(r,indent=2)+'\n')
    if r['conclusion']!='success' or r['run_attempt']!=1:errors.append(f'{title}: original run failed')
    arts=api(f'repos/{repo}/actions/runs/{r["id"]}/artifacts')['artifacts'];matches_art=[a for a in arts if a['name']==p['artifact']]
    if len(matches_art)!=1:errors.append(f'{title}: missing summary artifact');continue
    a=matches_art[0];blob=subprocess.check_output(['gh','api',f'repos/{repo}/actions/artifacts/{a["id"]}/zip'])
    if a.get('digest')!='sha256:'+hashlib.sha256(blob).hexdigest():raise SystemExit('artifact digest mismatch')
    with zipfile.ZipFile(io.BytesIO(blob)) as z:
        names=[n for n in z.namelist() if n.endswith(p['summary'])]
        if len(names)!=1:errors.append(title+': ambiguous summary');continue
        summary=json.loads(z.read(names[0]))
    if summary.get('source_sha')!=sha or summary.get('result')!='PASS':errors.append(title+': invalid summary/source')
    (folder/'summary.json').write_text(json.dumps(summary,indent=2)+'\n')
    receipts.append(dict(title=title,run=r['id'],url=r['html_url'],artifact=a['id'],digest=a['digest'],summary=summary))
result=dict(campaign=req['campaign'],source_sha=sha,result='FAIL' if errors else 'PASS',errors=errors,receipts=receipts)
(root/'result.json').write_text(json.dumps(result,indent=2)+'\n');print('RESULT '+json.dumps(dict(result=result['result'],errors=errors)),flush=True)
if errors:raise SystemExit(1)
