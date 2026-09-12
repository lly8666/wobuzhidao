#!/usr/bin/env python3
import pathlib
import sys

product = pathlib.Path(sys.argv[1])
rotation = product / 'scripts/game_lane_rotation_soak.sh'
s = rotation.read_text()

# This patch runs after instrument_singlelane_observability.py. It adds host-level
# pressure snapshots without changing product packet flow or wire semantics.
marker = 'outer_metrics_start\n'
if s.count(marker) != 1:
    raise SystemExit(f'host pressure: outer_metrics_start marker drift: {s.count(marker)}')

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

def read_text(name):
    with open(os.path.join(root,name),errors='replace') as f: return f.read()

def cpu(line):
    f=line.split(); return [int(x) for x in f[1:]]
a=cpu(read_text('host-start-proc-stat.txt').splitlines()[0])
b=cpu(read_text('host-end-proc-stat.txt').splitlines()[0])
d=[max(0,y-x) for x,y in zip(a,b)]
total=sum(d); idle=(d[3] if len(d)>3 else 0)+(d[4] if len(d)>4 else 0)
cpus=int(read_text('host-start-cpus.txt').strip() or '1')
busy_frac=((total-idle)/total if total else 0.0)

def softnet(text):
    p=dr=sq=0
    for line in text.splitlines():
        f=line.split()
        if len(f)>=3:
            p+=int(f[0],16); dr+=int(f[1],16); sq+=int(f[2],16)
    return p,dr,sq
s0=softnet(read_text('host-start-softnet.txt')); s1=softnet(read_text('host-end-softnet.txt'))

def udp(text):
    ls=text.splitlines()
    for i,line in enumerate(ls[:-1]):
        if line.startswith('Udp:') and 'InDatagrams' in line:
            h=line.split()[1:]; v=[int(x) for x in ls[i+1].split()[1:]]
            return dict(zip(h,v))
    return {}
def udp_delta(ns):
    x=udp(read_text(f'host-start-{ns}-snmp.txt')); y=udp(read_text(f'host-end-{ns}-snmp.txt'))
    keys=['InDatagrams','NoPorts','InErrors','OutDatagrams','RcvbufErrors','SndbufErrors','InCsumErrors','IgnoredMulti','MemErrors']
    return {k:int(y.get(k,0))-int(x.get(k,0)) for k in keys}

def netem(tag,ns):
    arr=json.load(open(os.path.join(root,f'host-{tag}-{ns}-qdisc.json')))
    for q in arr:
        if q.get('kind')=='netem': return q
    return {}
def qdelta(ns):
    x=netem('start',ns); y=netem('end',ns)
    out={}
    for k in ('bytes','packets','drops','overlimits','requeues'):
        out[k]=int(y.get(k,0))-int(x.get(k,0))
    denom=out['packets']+out['drops']
    out['loss_ratio_from_qdisc']=(out['drops']/denom if denom>0 else None)
    out['final_backlog']=int(y.get('backlog',0) or 0)
    out['final_qlen']=int(y.get('qlen',0) or 0)
    return out
obj={
 'host_cpu_busy_fraction':busy_frac,
 'host_cpu_busy_cores':busy_frac*cpus,
 'host_cpu_count':cpus,
 'softnet_processed_delta':s1[0]-s0[0],
 'softnet_dropped_delta':s1[1]-s0[1],
 'softnet_time_squeeze_delta':s1[2]-s0[2],
 'client_udp_delta':udp_delta('client'),
 'server_udp_delta':udp_delta('server'),
 'client_netem_delta':qdelta('client'),
 'server_netem_delta':qdelta('server'),
}
open(os.path.join(root,'host-pressure.json'),'w').write(json.dumps(obj,sort_keys=True,indent=2)+'\n')
print('WBD_HOST_PRESSURE_RESULT '+json.dumps(obj,sort_keys=True))
PY_HOST
}

'''
s=s.replace(marker, funcs+marker, 1)

old='''cpu_snapshot "$LOG_DIR/cpu-start.json"\nouter_metrics_start\n'''
new='''cpu_snapshot "$LOG_DIR/cpu-start.json"\nhost_pressure_snapshot start\nouter_metrics_start\n'''
if s.count(old)!=1:
    raise SystemExit(f'host pressure: load-start marker drift: {s.count(old)}')
s=s.replace(old,new,1)

old='''  cpu_snapshot "$LOG_DIR/cpu-end.json"\n  outer_metrics_finish\n  resource_metrics_finish\n'''
new='''  cpu_snapshot "$LOG_DIR/cpu-end.json"\n  host_pressure_snapshot end\n  outer_metrics_finish\n  resource_metrics_finish\n  host_pressure_finish\n'''
if s.count(old)!=1:
    raise SystemExit(f'host pressure: load-end marker drift: {s.count(old)}')
s=s.replace(old,new,1)

old='''cat "$LOG_DIR/resource-metrics.log"\ncat "$LOG_DIR/resource-metrics.json"\n'''
new='''cat "$LOG_DIR/resource-metrics.log"\ncat "$LOG_DIR/resource-metrics.json"\ncat "$LOG_DIR/host-pressure.json"\n'''
if s.count(old)!=1:
    raise SystemExit(f'host pressure: result marker drift: {s.count(old)}')
s=s.replace(old,new,1)

rotation.write_text(s)
print('WBD_SINGLELANE_HOST_PRESSURE_PATCHED '+str(rotation))
