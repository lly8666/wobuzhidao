"""Owned TCP egress drop fixture; never changes root qdisc or OUTPUT policy.

Unlike netfilter OUTPUT DROP, a tc egress drop is acknowledged by raw sendto.
Use only for authorized native qualification, not as product network policy.
"""
import argparse
import ipaddress
import json
import subprocess
import time
from pathlib import Path


def tc(*args):
    return subprocess.check_output(['tc'] + list(args), text=True)


def filters(interface, direction):
    return json.loads(tc('-j', '-s', 'filter', 'show', 'dev', interface, direction))


def owned_rows(state):
    return [r for r in filters(state['interface'], 'egress')
            if r.get('pref') == state['pref'] and r.get('options')]


def verify_owned(state, rows):
    if len(rows) != 1:
        raise RuntimeError('Missing or ambiguous owned egress filter')
    row = rows[0]
    keys = row.get('options', {}).get('keys', {})
    actions = row.get('options', {}).get('actions', [])
    if (row.get('kind') != 'flower' or keys.get('ip_proto') != 'tcp'
            or int(keys.get('src_port', 0)) != state['port']
            or ipaddress.ip_network(keys.get('dst_ip', '0.0.0.0/0'), strict=False)
            != ipaddress.ip_network(state['peer'] + '/32')
            or len(actions) != 1 or actions[0].get('kind') != 'gact'
            or actions[0].get('control_action', {}).get('type') != 'drop'
            or int(str(row['options']['handle']), 0) != state['handle']):
        raise RuntimeError('Owned egress selector/action changed; preserve it')


def enable(interface, peer, port, seed, path):
    peer = str(ipaddress.IPv4Address(peer))
    if not interface or not 1 <= port <= 65535 or not 1 <= seed <= 65535:
        raise ValueError('Invalid scoped fault config')
    path = Path(path)
    if path.exists():
        raise RuntimeError('Fault state already exists; refuse overwrite')
    before = json.loads(tc('-j', 'qdisc', 'show', 'dev', interface))
    state = dict(interface=interface, peer=peer, port=port, seed=seed,
                 pref=40000 + seed % 20000, handle=seed, created_clsact=False,
                 root_before=[q for q in before if q.get('root')])
    if any(r.get('pref') == state['pref'] for r in filters(interface, 'egress')):
        raise RuntimeError('Reserved preference occupied; preserve foreign filter')
    if any(q.get('kind') == 'ingress' for q in before):
        raise RuntimeError('Foreign ingress qdisc present; refuse replacing it')
    # Write ownership before mutation. A failed add leaves a receipt for cleanup.
    state['created_clsact'] = not any(q.get('kind') == 'clsact' for q in before)
    with path.open('x') as out:
        json.dump(state, out)
    if state['created_clsact']:
        tc('qdisc', 'add', 'dev', interface, 'clsact')
    tc('filter', 'add', 'dev', interface, 'egress', 'protocol', 'ip',
       'pref', str(state['pref']), 'handle', hex(seed), 'flower', 'skip_hw',
       'ip_proto', 'tcp', 'src_port', str(port), 'dst_ip', peer + '/32',
       'action', 'drop')
    rows = owned_rows(state)
    verify_owned(state, rows)
    return dict(enabled=True, unix_ns=time.time_ns(), direction='server-to-client',
                mechanism='tc-egress-drop', state=state, filter=rows)


def disable(path):
    path = Path(path)
    state = json.loads(path.read_text())
    rows = owned_rows(state)
    if rows:
        verify_owned(state, rows)
        tc('filter', 'del', 'dev', state['interface'], 'egress', 'protocol', 'ip',
           'pref', str(state['pref']), 'handle', hex(state['handle']), 'flower')
    if any(r.get('pref') == state['pref'] for r in filters(state['interface'], 'egress')):
        raise RuntimeError('Fault preference still present after exact delete')
    clsact_retained_for_foreign = False
    if state['created_clsact']:
        foreign = filters(state['interface'], 'egress') + filters(state['interface'], 'ingress')
        if foreign:
            clsact_retained_for_foreign = True
        elif any(q.get('kind') == 'clsact' for q in json.loads(tc('-j', 'qdisc', 'show', 'dev', state['interface']))):
            tc('qdisc', 'del', 'dev', state['interface'], 'clsact')
    after = json.loads(tc('-j', 'qdisc', 'show', 'dev', state['interface']))
    if [q for q in after if q.get('root')] != state['root_before']:
        raise RuntimeError('Root qdisc changed during fixture; preserve evidence')
    path.unlink()
    return dict(enabled=False, unix_ns=time.time_ns(), mechanism='tc-egress-drop',
                direction='server-to-client', filter=rows, owned_filter_absent=True,
                root_qdisc_unchanged=True, clsact_retained_for_foreign=clsact_retained_for_foreign)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument('operation', choices=('enable', 'disable'))
    ap.add_argument('--state', required=True)
    ap.add_argument('--interface')
    ap.add_argument('--peer')
    ap.add_argument('--port', type=int, default=443)
    ap.add_argument('--seed', type=int)
    args = ap.parse_args()
    result = (enable(args.interface, args.peer, args.port, args.seed, args.state)
              if args.operation == 'enable' else disable(args.state))
    print(json.dumps(result))


if __name__ == '__main__':
    main()
