#!/usr/bin/env python3
import pathlib
import sys

product = pathlib.Path(sys.argv[1])
rotation = product / 'scripts/game_lane_rotation_soak.sh'
s = rotation.read_text()

# Test-only observation. This adds host/resource snapshots around the existing
# offered-load window and changes no product packet flow or wire semantics.
marker = 'outer_metrics_start\n'
if s.count(marker) != 1:
    raise SystemExit(f'host pressure: outer_metrics_start call drift: {s.count(marker)}')

funcs = r'''host_pressure_snapshot() {
  local tag=$1
  awk '/^cpu / {print; exit}' /proc/stat >"$LOG_DIR/host-${tag}-proc-stat.txt"
  cat /proc/net/softnet_stat >"$LOG_DIR/host-${tag}-softnet.txt"
  getconf _NPROCESSORS_ONLN >"$LOG_DIR/host-${tag}-cpus.txt"
  sudo ip netns exec "$C" cat /proc/net/snmp >"$LOG_DIR/host-${tag}-client-snmp.txt"
  sudo ip netns exec "$S" cat /proc/net/snmp >"$LOG_DIR/host-${tag}-server-snmp.txt"
  sudo ip netns exec "$C" tc -s -j qdisc show dev gc0 >"$LOG_DIR/host-${tag}-client-qdisc.json"
  sudo ip netns exec "$S" tc -s -j qdisc show dev gs0 >"$LOG_DIR/host-${tag}-server-qdisc.json"
}

host_pressure_finish() {
  python3 - "$LOG_DIR" <<'PY_HOST'
import json, os, sys
root=sys.argv[1]
def text(name):
    with open(os.path.join(root,name),errors='replace') as f: return f.read()
def cpu(line): return [int(x) for x in line.split()[1:]]
a=cpu(text('host-start-proc-stat.txt').splitlines()[0]); b=cpu(text('host-end-proc-stat.txt').splitlines()[0])
d=[max(0,y-x) for x,y in zip(a,b)]; total=sum(d)
idle=(d[3] if len(d)>3 else 0)+(d[4] if len(d)>4 else 0)
cpus=int(text('host-start-cpus.txt').strip() or '1')
busy=((total-idle)/total if total else 0.0)
def soft(t):
    p=dr=sq=0
    for line in t.splitlines():
        f=line.split()
        if len(f)>=3: p+=int(f[0],16); dr+=int(f[1],16); sq+=int(f[2],16)
    return p,dr,sq
s0=soft(text('host-start-softnet.txt')); s1=soft(text('host-end-softnet.txt'))
def udp(t):
    ls=t.splitlines()
    for i,line in enumerate(ls[:-1]):
        if line.startswith('Udp:') and 'InDatagrams' in line:
            return dict(zip(line.split()[1:],[int(x) for x in ls[i+1].split()[1:]]))
    return {}
def ud(ns):
    x=udp(text(f'host-start-{ns}-snmp.txt')); y=udp(text(f'host-end-{ns}-snmp.txt'))
    keys=['InDatagrams','NoPorts','InErrors','OutDatagrams','RcvbufErrors','SndbufErrors','InCsumErrors','IgnoredMulti','MemErrors']
    return {k:int(y.get(k,0))-int(x.get(k,0)) for k in keys}
def netem(tag,ns):
    for q in json.load(open(os.path.join(root,f'host-{tag}-{ns}-qdisc.json'))):
        if q.get('kind')=='netem': return q
    return {}
def qd(ns):
    x=netem('start',ns); y=netem('end',ns); out={}
    for k in ('bytes','packets','drops','overlimits','requeues'):
        out[k]=int(y.get(k,0))-int(x.get(k,0))
    denom=out['packets']+out['drops']
    out['loss_ratio_from_qdisc']=(out['drops']/denom if denom>0 else None)
    out['final_backlog']=int(y.get('backlog',0) or 0); out['final_qlen']=int(y.get('qlen',0) or 0)
    return out
obj={'host_cpu_busy_fraction':busy,'host_cpu_busy_cores':busy*cpus,'host_cpu_count':cpus,
     'softnet_processed_delta':s1[0]-s0[0],'softnet_dropped_delta':s1[1]-s0[1],
     'softnet_time_squeeze_delta':s1[2]-s0[2],
     'client_udp_delta':ud('client'),'server_udp_delta':ud('server'),
     'client_netem_delta':qd('client'),'server_netem_delta':qd('server')}
open(os.path.join(root,'host-pressure.json'),'w').write(json.dumps(obj,sort_keys=True,indent=2)+'\n')
print('WBD_HOST_PRESSURE_RESULT '+json.dumps(obj,sort_keys=True))
PY_HOST
}

'''
s=s.replace(marker, funcs + 'host_pressure_snapshot start\n' + marker, 1)

marker = 'cpu_snapshot "$LOG_DIR/cpu-end.json"\n'
if s.count(marker) != 1:
    raise SystemExit(f'host pressure: cpu-end call drift: {s.count(marker)}')
s=s.replace(marker, marker+'  host_pressure_snapshot end\n', 1)

marker = 'resource_metrics_finish\n'
if s.count(marker) != 1:
    raise SystemExit(f'host pressure: resource finish call drift: {s.count(marker)}')
s=s.replace(marker, marker+'  host_pressure_finish\n', 1)

marker='cat "$LOG_DIR/resource-metrics.json"\n'
if s.count(marker) != 1:
    raise SystemExit(f'host pressure: resource result call drift: {s.count(marker)}')
s=s.replace(marker, marker+'cat "$LOG_DIR/host-pressure.json"\n', 1)

rotation.write_text(s)
print('WBD_SINGLELANE_HOST_PRESSURE_PATCHED '+str(rotation))
