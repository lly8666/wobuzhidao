"""Control cleanup only. Preserve all original results and explain cancellations."""
import json, os, subprocess, time
from pathlib import Path
req=json.loads(Path('.github/predelivery-retire-request.json').read_text());repo=os.environ['GITHUB_REPOSITORY']
out=Path('retirement');out.mkdir();rows=[]
def api(path):return json.loads(subprocess.check_output(['gh','api',path]))
def cancel(run,reason):
    row=dict(id=run['id'],head_sha=run['head_sha'],ref=run['head_branch'],title=run['display_title'],status_before=run['status'],conclusion_before=run['conclusion'],reason=reason)
    if run['status']!='completed':
        p=subprocess.run(['gh','api','--method','POST',f'repos/{repo}/actions/runs/{run["id"]}/cancel'],capture_output=True,text=True)
        row['cancel_requested']=p.returncode==0
        if p.returncode:
            fresh=api(f'repos/{repo}/actions/runs/{run["id"]}')
            if fresh['status']!='completed':raise SystemExit('cancel failed: '+str(run['id']))
    rows.append(row)
for id in req['controllers']:
    r=api(f'repos/{repo}/actions/runs/{id}')
    if r['path']!='.github/workflows/next-predelivery-coordinator.yml' or r['head_branch']!='main':raise SystemExit('unexpected controller')
    cancel(r,'superseded concurrent controller')
# Let remote cancellation settle before deciding the final dispatch inventory.
time.sleep(10)
rs=[]
for sha in [req['source_sha'],req['superseded_sha']]:
    for page in range(1,21):
        batch=api(f'repos/{repo}/actions/runs?head_sha={sha}&per_page=100&page={page}')['workflow_runs'];rs+=batch
        if len(batch)<100:break
allowed={'next-config-effective.yml','next-target-soak.yml','next-lifecycle-fullstack.yml','next-p6-package.yml','next-shared-blackhole.yml'}
groups={}
for r in rs:
    if r['event']!='workflow_dispatch' or not r['head_branch'].startswith('perf-fixed/') or r['path'].split('/')[-1] not in allowed:continue
    if r['head_sha']==req['superseded_sha']:
        if r['status']!='completed':cancel(r,'superseded product source; not qualification')
    elif r['head_sha']==req['source_sha']:
        groups.setdefault((r['head_branch'],r['display_title']),[]).append(r)
for key,group in groups.items():
    group.sort(key=lambda r:r['id'])
    for r in group[1:]:
        if r['status']!='completed':cancel(r,'extra duplicate dispatch; canonical original run '+str(group[0]['id']))
        else:rows.append(dict(id=r['id'],reason='completed extra duplicate retained unchanged',conclusion_before=r['conclusion'],canonical_run=group[0]['id']))
(out/'receipt.json').write_text(json.dumps(dict(request=req,result='PASS',cancellations=rows,canonical={str(k):v[0]['id'] for k,v in groups.items()}),indent=2)+'\n')
