"""One predeclared periodic weak-network workload; no additional measurement."""
import argparse
import json
import time
from pathlib import Path
from strict_weaknet_stage import change, emit, wait_until


def plan(duration):
    cycle = 300 if duration >= 1800 else 60
    stress = 60 if duration >= 1800 else 15
    return [dict(name=f'cycle{n//cycle}-{kind}', start_s=n+offset, end_s=n+end, loss_percent=loss)
            for n in range(0,duration,cycle)
            for kind,offset,end,loss in [('base',0,cycle-2*stress,5),
                                        ('stress',cycle-2*stress,cycle-stress,20),
                                        ('recovery',cycle-stress,cycle,5)]]


def main():
    a=argparse.ArgumentParser()
    for key in ['namespace','tc-bin','c2s-dev','s2c-dev','output']: a.add_argument('--'+key,required=True)
    for key in ['start-ns','seed','duration']: a.add_argument('--'+key,type=int,required=True)
    for key in ['pre-loss','stress-loss','post-loss']: a.add_argument('--'+key,type=float,required=True)
    x=a.parse_args()
    if x.duration not in [180,1800] or [x.pre_loss,x.stress_loss,x.post_loss]!=[5,20,5]:
        raise SystemExit('undeclared soak schedule')
    stages=plan(x.duration)
    Path(x.output).parent.mkdir(parents=True,exist_ok=True)
    with open(x.output,'w',buffering=1) as f:
        for i,p in enumerate(stages):
            wait_until(x.start_ns+p['start_s']*1_000_000_000)
            if i: emit(f,stages[i-1]['name']+'_end',x.namespace,x.tc_bin,x.c2s_dev,x.s2c_dev,stages[i-1]['loss_percent'],stages[i-1]['loss_percent'])
            for d,dev in enumerate([x.c2s_dev,x.s2c_dev]): change(x.namespace,x.tc_bin,dev,p['loss_percent'],x.seed*1000+i*10+d)
            emit(f,p['name']+'_start',x.namespace,x.tc_bin,x.c2s_dev,x.s2c_dev,p['loss_percent'],p['loss_percent'])
        wait_until(x.start_ns+x.duration*1_000_000_000)
        emit(f,stages[-1]['name']+'_end',x.namespace,x.tc_bin,x.c2s_dev,x.s2c_dev,5,5)
        for d,dev in enumerate([x.c2s_dev,x.s2c_dev]): change(x.namespace,x.tc_bin,dev,0,x.seed*1000+991+d)
        emit(f,'drain_start',x.namespace,x.tc_bin,x.c2s_dev,x.s2c_dev,0,0)


if __name__=='__main__': main()
