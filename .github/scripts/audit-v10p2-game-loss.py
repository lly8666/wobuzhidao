"""Read raw queue and application timelines without repeating measurements."""
import json
import os
from pathlib import Path
import subprocess
import zipfile

SHA = '56eb5413c3cf2e559b82026e8a5783508764e2f4'
RUNS = ((37065865089, '5205'), (37065869715, '5305'))
repo = os.environ['GITHUB_REPOSITORY']
root = Path('game-loss-audit')
root.mkdir(exist_ok=True)


def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path]))


def find(z, suffix):
    matches = [n for n in z.namelist() if n.endswith(suffix)]
    if len(matches) != 1:
        raise RuntimeError(f'ambiguous/missing {suffix}')
    return matches[0]


def queue_snapshots(obj):
    if isinstance(obj, dict):
        for k, v in obj.items():
            if k in ('client_udp', 'server_udp') and isinstance(v, dict):
                yield k, v
            else:
                yield from queue_snapshots(v)
    elif isinstance(obj, list):
        for v in obj:
            yield from queue_snapshots(v)


for run_id, scenario in RUNS:
    run = api(f'repos/{repo}/actions/runs/{run_id}')
    if run['head_sha'] != SHA or run['run_attempt'] != 1:
        raise SystemExit('unexpected source/attempt')
    expected = f'strict-single-game-{scenario}-seed303-{SHA}'
    artifacts = api(f'repos/{repo}/actions/runs/{run_id}/artifacts')['artifacts']
    candidates = [a for a in artifacts if a['name'] == expected and not a['expired']]
    if len(candidates) != 1:
        raise SystemExit('missing immutable evidence')
    artifact = candidates[0]
    archive = Path(os.environ['RUNNER_TEMP']) / f'evidence-{run_id}.zip'
    with archive.open('wb') as f:
        subprocess.run(['gh', 'api', f'repos/{repo}/actions/artifacts/{artifact["id"]}/zip'], stdout=f, check=True)
    out = {'source_sha': SHA, 'run': run_id, 'scenario': scenario,
           'artifact_id': artifact['id'], 'queues': {}, 'loss_seconds': {}}
    with zipfile.ZipFile(archive) as z:
        for endpoint in ('client', 'server'):
            snapshots = []
            with z.open(find(z, endpoint + '-diag.jsonl')) as f:
                for line in f:
                    row = json.loads(line)
                    for name, values in queue_snapshots(row):
                        snapshots.append({'unix_ns': row.get('unix_ns'), 'queue': name, **values})
            if not snapshots:
                raise SystemExit('missing queue diagnostics')
            keys = ('queue_current', 'queue_bytes', 'inflight_current', 'inflight_bytes',
                    'total_peak', 'total_bytes_peak', 'overflow_drops', 'overflow_bytes',
                    'gate_errors', 'forward_errors', 'send_errors', 'stale_drops',
                    'close_drops', 'eviction_max_scan')
            out['queues'][endpoint] = {'samples': len(snapshots), 'final': snapshots[-1],
                'max': {key: max(s.get(key, 0) for s in snapshots) for key in keys}}
        biz = json.loads(z.read(find(z, 'biz.json')))['stats']
        target = json.loads(z.read(find(z, 'target.json')))['stats']
        for direction, tx, rx in (('c2s', biz, target), ('s2c', target, biz)):
            out['loss_seconds'][direction] = [
                {'second': i, 'sent': sent, 'received': rx['recv_packets_by_second'][i],
                 'lost': sent - rx['recv_packets_by_second'][i]}
                for i, sent in enumerate(tx['sent_packets_by_second'])
                if sent != rx['recv_packets_by_second'][i]]
    (root / f'{run_id}.json').write_text(json.dumps(out, indent=2) + '\n')
    print('WBD_GAME_LOSS_AUDIT ' + json.dumps(out, sort_keys=True), flush=True)
    archive.unlink()
