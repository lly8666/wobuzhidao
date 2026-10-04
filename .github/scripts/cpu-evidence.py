"""Read existing immutable artifacts. Never builds or runs a performance workload."""
import hashlib,json,os,subprocess,zipfile
from pathlib import Path
repo=os.environ['GITHUB_REPOSITORY'];root=Path('cpu-evidence');root.mkdir(exist_ok=True)
plan=[('baseline',37140881784,11280192571,'2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8'),
      ('split',37154677946,11285311356,'a67e10fa2875162eeac926b970a0c486a239748d')]
allowed={'runner-host.txt','resources.jsonl','raw-io-receipt.json','manifest.json','summary-loss-tolerant-v1.json'}
for label,run,aid,sha in plan:
    folder=root/label;folder.mkdir()
    meta=json.loads(subprocess.check_output(['gh','api',f'repos/{repo}/actions/artifacts/{aid}']))
    rr=json.loads(subprocess.check_output(['gh','api',f'repos/{repo}/actions/runs/{run}']))
    if rr['head_sha']!=sha or rr['conclusion']!='success' or rr['run_attempt']!=1 or meta['workflow_run']['id']!=run:raise SystemExit('original identity mismatch')
    blob=Path(label+'.zip')
    with blob.open('wb') as f:subprocess.check_call(['gh','api',f'repos/{repo}/actions/artifacts/{aid}/zip'],stdout=f)
    h=hashlib.sha256()
    with blob.open('rb') as f:
        for chunk in iter(lambda:f.read(1<<20),b''):h.update(chunk)
    if meta['digest']!='sha256:'+h.hexdigest():raise SystemExit('original ZIP digest mismatch')
    found=set()
    with zipfile.ZipFile(blob) as z:
        for name in z.namelist():
            leaf=Path(name).name
            if leaf in allowed:
                if leaf in found:raise SystemExit('ambiguous allowed file')
                data=z.read(name)
                if len(data)>64<<20:raise SystemExit('bounded evidence file exceeded')
                (folder/leaf).write_bytes(data);found.add(leaf)
    if not {'runner-host.txt','resources.jsonl','raw-io-receipt.json','manifest.json'}<=found:raise SystemExit('missing original CPU evidence')
    (folder/'receipt.json').write_text(json.dumps(dict(run=rr,artifact=meta),indent=2)+'\n')
    print('ORIGINAL_CPU_EVIDENCE '+label+' source='+sha+' digest='+meta['digest'],flush=True)
    print((folder/'runner-host.txt').read_text(),flush=True)
    blob.unlink()
