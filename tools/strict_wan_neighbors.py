"""Explicit neighbor-cache diagnostic confined to the two owned WAN veth pairs."""
import argparse
import json
import re
import subprocess
from pathlib import Path


def prepare(client, router, server, mode, run=None):
    if mode not in ('dynamic', 'permanent'):
        raise ValueError('Explicit WAN neighbor mode required')
    # The existing strict harness uses the literal '$' suffix. Accept that
    # exact owned topology as well as numeric sample suffixes; never widen this
    # to arbitrary namespaces or interpolate these names through a shell.
    if any(not re.fullmatch(r'w(?:cli|rtr|srv)-(?:[0-9]+|\$)', ns) for ns in (client, router, server)):
        raise ValueError('Owned strict namespace names required')
    if not (client.startswith('wcli-') and router.startswith('wrtr-') and server.startswith('wsrv-')):
        raise ValueError('Namespace roles differ from strict topology')
    if len({ns.split('-')[1] for ns in (client, router, server)}) != 1:
        raise ValueError('Namespaces do not belong to the same sample')
    if run is None:
        def run(argv):
            raw = subprocess.check_output(argv)
            return json.loads(raw) if raw.strip() else None
    pairs = [(client, 'cwan', '198.18.0.1', router, 'rcli'),
             (router, 'rcli', '198.18.0.2', client, 'cwan'),
             (router, 'rsrv', '198.18.0.6', server, 'swan'),
             (server, 'swan', '198.18.0.5', router, 'rsrv')]
    evidence = []
    for ns, dev, peer, peer_ns, peer_dev in pairs:
        if mode == 'permanent':
            links = run(['ip', '-n', peer_ns, '-j', 'link', 'show', 'dev', peer_dev])
            if len(links) != 1 or not re.fullmatch(r'(?:[0-9a-f]{2}:){5}[0-9a-f]{2}', links[0].get('address', '')):
                raise ValueError('Peer MAC unavailable')
            mac = links[0]['address']
            run(['ip', '-n', ns, 'neigh', 'replace', peer, 'lladdr', mac, 'dev', dev, 'nud', 'permanent'])
            rows = run(['ip', '-n', ns, '-j', 'neigh', 'show', 'dev', dev])
            actual = [r for r in rows if r.get('dst') == peer]
            if len(actual) != 1 or actual[0].get('lladdr') != mac or 'PERMANENT' not in actual[0].get('state', []):
                raise ValueError('Permanent WAN neighbor did not take effect')
        else:
            rows = run(['ip', '-n', ns, '-j', 'neigh', 'show', 'dev', dev])
        evidence.append(dict(namespace=ns, interface=dev, peer=peer, neighbors=rows))
    return dict(mode=mode, scope='two owned WAN veth pairs only; no business/TUN/host neighbor changes',
                product_changed=False, diagnostic_only=True, evidence=evidence)


def main():
    ap = argparse.ArgumentParser()
    for name in ['client', 'router', 'server', 'output']:
        ap.add_argument('--'+name, required=True)
    ap.add_argument('--mode', choices=['dynamic', 'permanent'], default='dynamic')
    args = ap.parse_args()
    def run(argv):
        raw = subprocess.check_output(argv)
        return json.loads(raw) if raw.strip() else None
    Path(args.output).write_text(json.dumps(prepare(args.client, args.router, args.server, args.mode, run), indent=2)+'\n')


if __name__ == '__main__':
    main()
