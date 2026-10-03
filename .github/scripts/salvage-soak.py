"""Offline salvage only. Keep original run FAIL, do not execute a workload."""
import gzip, hashlib, json, os, subprocess, sys, zipfile
from pathlib import Path
req=json.loads(Path('.github/predelivery-salvage-request.json').read_text())
repo=os.environ['GITHUB_REPOSITORY'];out=Path('recovered');out.mkdir()
def api(p):return json.loads(subprocess.check_output(['gh','api',p]))
run=api(f'repos/{repo}/actions/runs/{req["run"]}')
if run['head_sha']!=req['source_sha'] or run['conclusion']!='failure':raise SystemExit('original source/result mismatch')
arts=api(f'repos/{repo}/actions/runs/{req["run"]}/artifacts')['artifacts']
a=next(x for x in arts if x['id']==req['artifact'])
with open('original.zip','wb') as f:subprocess.check_call(['gh','api',f'repos/{repo}/actions/artifacts/{a["id"]}/zip'],stdout=f)
h=hashlib.sha256()
with open('original.zip','rb') as f:
    while b:=f.read(1024*1024):h.update(b)
if a['digest']!='sha256:'+h.hexdigest():raise SystemExit('archive digest mismatch')
root=Path('original').resolve();root.mkdir()
with zipfile.ZipFile('original.zip') as z:
    for n in z.namelist():
        if not (root/n).resolve().is_relative_to(root):raise SystemExit('unsafe artifact path')
    z.extractall(root)
# Restore any chunks already compressed before the first oversized chunk.
for p in (root/'capture').glob('*.pcap.gz'):
    with gzip.open(p,'rb') as src,p.with_suffix('').open('wb') as dst:
        while b:=src.read(1024*1024):dst.write(b)
    p.unlink()
script=(root/'generated-soak.sh').read_text()
marker='import hashlib, json, sys\nfrom pathlib import Path\n'
code=marker+script.split(marker)[-1].split('\nPY\n')[0]
sys.argv=['recovered-manifest',str(root),req['source_sha'],str(Path('source').resolve()),'game','5205','404','3','4','0','0']
exec(compile(code,'immutable-generated-manifest','exec'),{'__name__':'__main__'})
# A retrospective *file* bound correction only; retain all payload/latency gates.
capture=(Path('source')/'tools/soak_capture.py').read_text().replace('100*1024*1024','256*1024*1024')
Path('source/tools/recovered_capture.py').write_text(capture)
subprocess.check_call([sys.executable,'source/tools/recovered_capture.py','--artifact-dir',str(root),'--duration','180'])
rc=subprocess.call([sys.executable,'source/tools/check_target_soak.py','--artifact-dir',str(root),'--source-sha',req['source_sha'],'--output',str(out/'summary-soak.json')])
receipt=dict(original_run=run,artifact=a,reason=req['reason'],original_result='FAIL',scope='RECOVERED_DIAGNOSTIC_ONLY',offline_capture_chunk_bound_mib=256,analyzer_exit=rc)
(out/'receipt.json').write_text(json.dumps(receipt,indent=2)+'\n')
for name in ['biz.json','target.json','client-diag.jsonl','server-diag.jsonl','stage-events.jsonl','manifest.json','client.log','server.log','resources.jsonl','capture-receipt.json']:
    p=root/name
    if p.exists():(out/name).write_bytes(p.read_bytes())
