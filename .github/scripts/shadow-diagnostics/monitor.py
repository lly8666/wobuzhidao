"""One-second raw observations; socket inode/PID and starttime preserve identity."""
import json
import os
from pathlib import Path
import subprocess
import sys
import time

root = Path(sys.argv[1])
namespaces = sys.argv[2:]
stop = root / 'monitor.stop'

def command(args):
    p = subprocess.run(args, capture_output=True, text=True, timeout=3)
    return dict(rc=p.returncode, stdout=p.stdout, stderr=p.stderr)

def snapshot():
    d = dict(monotonic=time.monotonic(), clock_ticks=os.sysconf('SC_CLK_TCK'),
             cpu=Path('/proc/stat').read_text(), softnet=Path('/proc/net/softnet_stat').read_text(),
             processes={}, namespaces={})
    d['host_network'] = {name: Path('/proc/net',name).read_text()
                         for name in ('udp','raw','snmp','netstat')}
    d['socket_limits'] = {name: Path('/proc/sys/net/core',name).read_text()
                          for name in ('rmem_default','rmem_max','wmem_default','wmem_max')
                          if Path('/proc/sys/net/core',name).exists()}
    for proc in Path('/proc').glob('[0-9]*'):
        try:
            comm = (proc/'comm').read_text().strip()
            if not (comm.startswith('wbd') or comm.startswith('python')):
                continue
            sockets = []
            for fd in (proc/'fd').iterdir():
                try:
                    link = os.readlink(fd)
                    if link.startswith('socket:'):
                        sockets.append(link)
                except OSError:
                    pass
            d['processes'][proc.name] = dict(comm=comm, stat=(proc/'stat').read_text(),
                status=(proc/'status').read_text(), sockets=sockets)
        except (OSError, ProcessLookupError):
            pass
    for ns in namespaces:
        d['namespaces'][ns] = {
            'network': command(['ip','netns','exec',ns,'sh','-c',
                'for f in udp udp6 raw raw6 snmp netstat; do echo "$f"; cat /proc/net/$f; done']),
            'qdisc': command(['ip','netns','exec',ns,'tc','-s','-j','qdisc','show'])}
    return d

with (root/'resource-samples.jsonl').open('w') as f:
    end = time.monotonic() + 110
    while True:
        final = stop.exists() or time.monotonic() >= end
        f.write(json.dumps(snapshot())+'\n')
        f.flush()
        (root/'monitor.ready').touch()
        if final:
            break
        time.sleep(1)
