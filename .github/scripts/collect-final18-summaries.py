#!/usr/bin/env python3
"""Control-plane collection only: each measurement is a separate Actions run."""
import io
import json
import os
from pathlib import Path
import subprocess
import time
import zipfile

request = json.loads(Path('.github/perf-final18-fixed-ref-request.json').read_text())
repo = os.environ['GITHUB_REPOSITORY']
sha, branch = request['expected_sha'], request['dispatch_ref']
root = Path('final18')
root.mkdir(exist_ok=True)
plan = {}
for mode, rate, lanes in (('normal', 10, 1), ('game', 3, 4)):
    for scenario in ('lossless', '5205', '5305'):
        for seed in (101, 202, 303):
            title = f'strict-{mode}-{scenario}-seed{seed}-rate{rate}-lanes{lanes}'
            plan[title] = (mode, scenario, seed, rate, lanes)


def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path]))


deadline = time.monotonic() + 70 * 60
while True:
    runs = api(f'repos/{repo}/actions/workflows/{request["workflow"]}/runs?event=workflow_dispatch&branch={branch}&per_page=100')['workflow_runs']
    matches = {}
    for run in runs:
        title = run['display_title']
        if run['head_sha'] == sha and run['head_branch'] == branch and title in plan:
            if title in matches:
                raise SystemExit(f'duplicate run identity on frozen campaign ref: {title}')
            matches[title] = run
    completed = sum(r['status'] == 'completed' for r in matches.values())
    print(f'WBD_FINAL18_PROGRESS found={len(matches)}/18 completed={completed}/18', flush=True)
    if len(matches) == 18 and completed == 18:
        break
    if time.monotonic() >= deadline:
        (root / 'incomplete-campaign.json').write_text(json.dumps(matches, indent=2))
        raise SystemExit('campaign incomplete; no qualification')
    time.sleep(20)

receipts, failures = [], []
for title, identity in plan.items():
    run = api(f'repos/{repo}/actions/runs/{matches[title]["id"]}')
    folder = root / title
    folder.mkdir()
    receipt = {k: run[k] for k in ('id', 'event', 'head_sha', 'head_branch', 'run_attempt', 'status', 'conclusion', 'html_url', 'display_title')}
    (folder / 'run-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
    if run['run_attempt'] != 1 or run['conclusion'] != 'success':
        failures.append(f'run did not pass original attempt: {run["id"]}')
    mode, scenario, seed, rate, lanes = identity
    expected_name = f'strict-summary-{mode}-{scenario}-seed{seed}-{sha}'
    artifacts = api(f'repos/{repo}/actions/runs/{run["id"]}/artifacts?per_page=100')['artifacts']
    summaries = [a for a in artifacts if a['name'] == expected_name and not a['expired']]
    if len(summaries) != 1:
        failures.append(f'missing/duplicate compact summary: {run["id"]}')
        continue
    artifact = summaries[0]
    (folder / 'artifact-receipt.json').write_text(json.dumps(artifact, indent=2) + '\n')
    blob = subprocess.check_output(['gh', 'api', f'repos/{repo}/actions/artifacts/{artifact["id"]}/zip'])
    with zipfile.ZipFile(io.BytesIO(blob)) as z:
        names = [n for n in z.namelist() if n.endswith('summary-loss-tolerant-v1.json')]
        if len(names) != 1:
            raise SystemExit('ambiguous summary archive')
        raw = z.read(names[0])
    row = json.loads(raw)
    got = (row.get('mode'), row.get('scenario'), int(row.get('seed', 0)), row.get('target_mbps_each_direction'), row.get('lanes'))
    if got != identity or row.get('source_sha') != sha:
        raise SystemExit(f'summary identity mismatch: {run["id"]}')
    (folder / 'summary-loss-tolerant-v1.json').write_bytes(raw)
    info = {'run': run['id'], 'identity': identity, 'source_sha': sha,
            'classifications': row['classifications'],
            'socket_drop_max': row['resource']['socket_drop_max'],
            'cpu_seconds': row['resource']['process_cpu_seconds'],
            'stress_goodput_mbps': {d: row['generator'][d]['stress']['wall_delivered_mbps_path_delay_aligned'] for d in ('c2s', 's2c')},
            'stress_probe': row['latency']['probe_stage']['stress'],
            'outer_ip_per_app': {d: row['cost'][d]['outer_ip_per_app_raw_input'] for d in ('c2s', 's2c')}}
    receipts.append(info)
    print('WBD_FINAL18_RECEIPT ' + json.dumps(info, sort_keys=True), flush=True)

(root / 'campaign.json').write_text(json.dumps({'request': request, 'samples': receipts, 'collection_errors': failures}, indent=2) + '\n')
if failures:
    raise SystemExit('\n'.join(failures))
