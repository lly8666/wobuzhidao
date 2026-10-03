"""Control plane only. Every actual workload is a separate workflow run."""
import hashlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import time
import zipfile

repo = os.environ['GITHUB_REPOSITORY']
request = json.loads(Path('.github/perf-optimization-5205-request.json').read_text())
root = Path('optimization-5205')
root.mkdir(exist_ok=True)
(root / 'request.json').write_text(json.dumps(request, indent=2) + '\n')
sha, branch = request['source_sha'], request['dispatch_ref']
if not re.fullmatch(r'[0-9a-f]{40}', sha) or branch != 'perf-fixed/' + sha:
    raise SystemExit('immutable full source SHA/ref required')
if not re.fullmatch(r'[0-9a-f]{40}', request['baseline_source_sha']):
    raise SystemExit('baseline source SHA required')
remote = subprocess.check_output(['git', 'ls-remote', 'origin', 'refs/heads/' + branch]).decode().split()
if not remote or remote[0] != sha:
    raise SystemExit('frozen dispatch ref differs from source')


def api(path):
    return json.loads(subprocess.check_output(['gh', 'api', path]))


def summary_artifact(artifact_id, expected_sha, mode):
    receipt = api(f'repos/{repo}/actions/artifacts/{artifact_id}')
    expected_name = f'strict-summary-{mode}-5205-seed101-{expected_sha}'
    if receipt['name'] != expected_name or receipt['expired']:
        raise SystemExit('summary artifact identity invalid')
    blob = subprocess.check_output(['gh', 'api', f'repos/{repo}/actions/artifacts/{artifact_id}/zip'])
    if receipt.get('digest') != 'sha256:' + hashlib.sha256(blob).hexdigest():
        raise SystemExit('summary artifact digest mismatch')
    with zipfile.ZipFile(io.BytesIO(blob)) as z:
        names = [n for n in z.namelist() if n.endswith('summary-loss-tolerant-v1.json')]
        if len(names) != 1:
            raise SystemExit('ambiguous summary')
        s = json.loads(z.read(names[0]))
    identity = (s['source_sha'], s['mode'], s['scenario'], s['seed'], s['lanes'], s['target_mbps_each_direction'])
    expected = (expected_sha, mode, '5205', 101, 1 if mode == 'normal' else 4, 10 if mode == 'normal' else 3)
    if identity != expected or any(v != 'PASS' for v in s['classifications'].values()):
        raise SystemExit(f'wrong or failed summary: {identity}')
    return s, receipt


# Do not dispatch a performance sample until the exact source passed full
# foundation unit/build/race. No green inheritance from a different HEAD.
deadline = time.monotonic() + 10 * 60
while True:
    foundation = api(f'repos/{repo}/actions/runs/{request["foundation_run"]}')
    if foundation['head_sha'] != sha or foundation['name'] != 'next-foundation':
        raise SystemExit('foundation source/workflow mismatch')
    if foundation['status'] == 'completed':
        if foundation['conclusion'] != 'success':
            raise SystemExit('foundation did not pass; no performance dispatch')
        break
    if time.monotonic() > deadline:
        raise SystemExit('foundation gate timeout')
    time.sleep(20)
(root / 'foundation-receipt.json').write_text(json.dumps(foundation, indent=2) + '\n')

plan = {}
baselines = {}
for mode, rate, lanes in [('normal', 10, 1), ('game', 3, 4)]:
    baselines[mode], baseline_receipt = summary_artifact(request['baseline_artifacts'][mode], request['baseline_source_sha'], mode)
    (root / f'baseline-{mode}.json').write_text(json.dumps(baselines[mode], indent=2) + '\n')
    (root / f'baseline-artifact-{mode}.json').write_text(json.dumps(baseline_receipt, indent=2) + '\n')
    title = f'strict-{mode}-5205-seed101-rate{rate}-lanes{lanes}'
    plan[title] = mode
    existing = api(f'repos/{repo}/actions/runs?event=workflow_dispatch&branch={branch}&per_page=100')['workflow_runs']
    matching = [r for r in existing if r['display_title'] == title and r['head_sha'] == sha]
    if len(matching) > 1:
        raise SystemExit('duplicate sample identity')
    if not matching:
        subprocess.check_call(['gh', 'workflow', 'run', 'next-strict-weaknet.yml', '--ref', branch,
                               '-f', f'mode={mode}', '-f', 'scenario=5205', '-f', 'seed=101',
                               '-f', f'rate_mbps={rate}', '-f', f'lanes={lanes}'])
    print(f'WBD_OPTIMIZATION_DISPATCH stage={request["stage"]} title={title} source={sha}', flush=True)

deadline = time.monotonic() + 22 * 60
while True:
    runs = api(f'repos/{repo}/actions/runs?event=workflow_dispatch&branch={branch}&per_page=100')['workflow_runs']
    matches = {}
    for r in runs:
        if r['head_sha'] == sha and r['display_title'] in plan:
            if r['display_title'] in matches:
                raise SystemExit('duplicate sample identity')
            matches[r['display_title']] = r
    print(f'WBD_OPTIMIZATION_PROGRESS found={len(matches)}/2 complete={sum(r["status"] == "completed" for r in matches.values())}/2', flush=True)
    if len(matches) == 2 and all(r['status'] == 'completed' for r in matches.values()):
        break
    if time.monotonic() > deadline:
        raise SystemExit('samples incomplete; no qualification')
    time.sleep(20)

errors, records = [], []
for title, mode in plan.items():
    run = matches[title]
    (root / f'run-{mode}.json').write_text(json.dumps(run, indent=2) + '\n')
    if run['run_attempt'] != 1 or run['conclusion'] != 'success':
        errors.append(f'{mode}: original run failed or rerun; id={run["id"]}')
        continue
    artifacts = api(f'repos/{repo}/actions/runs/{run["id"]}/artifacts')['artifacts']
    named = [a for a in artifacts if a['name'] == f'strict-summary-{mode}-5205-seed101-{sha}' and not a['expired']]
    if len(named) != 1:
        errors.append(f'{mode}: missing summary')
        continue
    s, ar = summary_artifact(named[0]['id'], sha, mode)
    (root / f'summary-{mode}.json').write_text(json.dumps(s, indent=2) + '\n')
    baseline = baselines[mode]
    differences = {}
    for phase in ('pre', 'stress', 'post'):
        p, b = s['latency']['probe_stage'][phase], baseline['latency']['probe_stage'][phase]
        delta = {q: p[q + '_ns'] - b[q + '_ns'] for q in ('p95', 'p99')}
        differences[phase] = delta
        if delta['p95'] > 200_000_000 or delta['p99'] > 500_000_000:
            errors.append(f'{mode}/{phase}: RTT delta exceeds original gate')
        for d in ('c2s', 's2c'):
            loss = s['generator'][d][phase]['byte_loss_percent']
            before = baseline['generator'][d][phase]['byte_loss_percent']
            if loss > before + 0.5:
                errors.append(f'{mode}/{phase}/{d}: loss grew >0.5 percentage points; diagnose before advancing')
    row = {'stage': request['stage'], 'mode': mode, 'source_sha': sha, 'run': run['id'], 'url': run['html_url'],
           'artifact': ar['id'], 'artifact_digest': ar['digest'], 'classifications': s['classifications'],
           'stress_goodput_mbps': {d: s['generator'][d]['stress']['wall_delivered_mbps_path_delay_aligned'] for d in ('c2s', 's2c')},
           'stress_byte_loss_percent': {d: s['generator'][d]['stress']['byte_loss_percent'] for d in ('c2s', 's2c')},
           'outer_ip_per_app': {d: s['cost'][d]['outer_ip_per_app_raw_input'] for d in ('c2s', 's2c')},
           'cpu_seconds': s['resource']['process_cpu_seconds'], 'baseline_cpu_seconds': baseline['resource']['process_cpu_seconds'],
           'socket_drop_max': s['resource']['socket_drop_max'], 'stress_probe': s['latency']['probe_stage']['stress'],
           'rtt_delta_ns': differences}
    records.append(row)
    print('WBD_OPTIMIZATION_RECEIPT ' + json.dumps(row, sort_keys=True), flush=True)
result = {'result': 'FAIL' if errors else 'PASS', 'source_sha': sha, 'stage': request['stage'], 'baseline_source_sha': request['baseline_source_sha'],
          'errors': errors, 'records': records, 'scope': 'targeted5205 only; independent runners; no full-matrix/soak/physical inheritance'}
(root / 'result.json').write_text(json.dumps(result, indent=2) + '\n')
print('WBD_OPTIMIZATION_RESULT ' + json.dumps(result, sort_keys=True), flush=True)
if errors:
    raise SystemExit('qualification failed; do not advance')
