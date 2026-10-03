"""Feature-path receipt only. Does not replace any performance/quality gate."""
import argparse
import json
from pathlib import Path

p = argparse.ArgumentParser()
p.add_argument('--artifact-dir', required=True)
args = p.parse_args()
root = Path(args.artifact_dir)
receipt = {'scope': 'feature-path counters, not resource attribution', 'endpoints': {}}
for endpoint in ('client', 'server'):
    rows = [json.loads(line) for line in (root / f'{endpoint}-diag.jsonl').read_text().splitlines() if line.strip()]
    counters = [row['raw_io'] for row in rows if row.get('raw_io', {}).get('enabled')]
    if not counters:
        raise SystemExit(f'{endpoint}: missing enabled raw IO diagnostics')
    last = counters[-1]
    for direction in ('receive', 'send'):
        calls, messages, multi = (last[direction + '_' + field] for field in ('calls', 'messages', 'multi'))
        if messages <= 0 or calls <= 0 or multi <= 0 or last[direction + '_fallbacks'] != 0:
            raise SystemExit(f'{endpoint}/{direction}: native batch not exercised or fallback occurred: {last}')
    receipt['endpoints'][endpoint] = last
(root / 'raw-io-receipt.json').write_text(json.dumps(receipt, indent=2) + '\n')
print('WBD_RAW_IO_RECEIPT ' + json.dumps(receipt, sort_keys=True))
