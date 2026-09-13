"""Audit one artifact. Do not interpret a dead lane as steady-state FEC loss."""
import json
from pathlib import Path
import re
import sys

root = Path(sys.argv[1])
failures = []
observations = {}
for name in ('link-1.log', 'link-server.log', 'game-client.log', 'game-server.log'):
    path = root/name
    if not path.exists():
        failures.append('missing '+name)
        continue
    text = path.read_text(errors='replace')
    fatal = [line for line in text.splitlines() if re.search(r'WBD_\w*FAIL\b', line)]
    failures.extend(name+': '+line for line in fatal)
    for key in ('lane_fail', 'dormant_drop'):
        if any(int(x) for x in re.findall(r'\b'+key+r'=(\d+)', text)):
            failures.append(name+': '+key+' is nonzero')
    rows = [json.loads(line.split('WBD_LINK_FEC_DIAG ', 1)[1])
            for line in text.splitlines() if 'WBD_LINK_FEC_DIAG ' in line]
    if rows:
        d = rows[-1]
        settled = d.get('rx_horizon_settled_blocks', 0)
        under = d.get('rx_horizon_under_required', 0)
        observations[name] = dict(last_snapshot=d,
            under_required_ratio=under/settled if settled else None,
            mean_missing_among_under=d.get('rx_horizon_missing_required',0)/under if under else None,
            caveat='Last periodic snapshot, not a synchronized TX/RX or final total')
for name in ('faketcp-1.log', 'faketcp-mux.log'):
    path = root/name
    rows = [json.loads(line.split('WBD_CARRIER_FRAGMENT_STATS ',1)[1])
            for line in path.read_text(errors='replace').splitlines()
            if 'WBD_CARRIER_FRAGMENT_STATS ' in line] if path.exists() else []
    observations[name] = rows
    if not rows:
        failures.append(name+': missing carrier fragmentation stats')
    for d in rows:
        d['fragmented_ratio'] = d['fragmented_datagrams']/d['datagrams'] if d['datagrams'] else None
result = dict(steady_state_sample_valid=not failures, failures=failures, observations=observations)
(root/'loss-unit-audit.json').write_text(json.dumps(result,indent=2)+'\n')
print(json.dumps(dict(steady_state_sample_valid=not failures, failures=failures)))
raise SystemExit(0 if not failures else 1)
