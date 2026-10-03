"""Prepare explicit configuration probes; no workload or local verification."""
import argparse,json
from pathlib import Path

def cases():
    out={}
    for fec in (0,4,8,10,12,16,20):
        for lanes in (1,2,3,4):
            for pad in (False,True):
                out[f'f{fec}-l{lanes}-p{int(pad)}']=dict(fec=fec,lanes=lanes,padding=pad,mode='priority',mtu=1400,client_limit=1300,server_limit=1250)
        out[f'json-f{fec}']=dict(fec=fec,lanes=1+(fec%4),padding=True,mode='json',mtu=1400,client_limit=1300,server_limit=1250)
    out['defaults']=dict(fec=0,lanes=1,padding=False,mode='defaults',mtu=1400,client_limit=1300,server_limit=1250)
    for mtu in (1280,1400,1500):
        for reverse in (False,True):
            out[f'mtu{mtu}-reverse{int(reverse)}']=dict(fec=20 if reverse else 0,lanes=4 if reverse else 1,padding=True,mode='json',mtu=mtu,client_limit=768 if reverse else 512,server_limit=512 if reverse else 768)
    return out

def prepare(root,case,side,args):
    root=Path(root); p=cases()[case]; values={'fec-parity':p['fec'],'lanes':p['lanes'],'tls-startup-padding':p['padding'],'mtu':p['mtu'],side+'-record-limit':p[side+'_limit']}
    if p['mode']!='defaults':
        values.update({'keepalive-interval':'1s','idle-dormant':'0s'})
        if side=='client':values.update({'dead-after':'6s','reconnect-min':'1s','reconnect-max':'4s'})
    clean=[];i=0
    while i<len(args):
        key=args[i].removeprefix('--')
        if key in values or key in ('fec-parity','lanes','tls-startup-padding'):
            i+=2;continue
        clean.extend(args[i:i+2]);i+=2
    conf={}
    if p['mode']=='json':conf=values
    elif p['mode']=='priority':
        conf=dict(values);conf.update({'fec-parity':0 if p['fec'] else 20,'lanes':1 if p['lanes']!=1 else 4,'tls-startup-padding':not p['padding'],'keepalive-interval':'4s','idle-dormant':'30s'})
        if side=='client':conf.update({'dead-after':'12s','reconnect-min':'2s','reconnect-max':'8s'})
    if conf:
        f=root/f'{side}-config.json';f.write_text(json.dumps(conf,indent=2)+'\n');clean+=['--config',str(f)]
    if p['mode'] in ('priority','defaults'):
        for k,v in values.items():
            if p['mode']=='defaults' and k in ('fec-parity','lanes','tls-startup-padding'):continue
            clean.extend(['--'+k,str(v).lower() if isinstance(v,bool) else str(v)])
    (root/f'{side}-args.bin').write_bytes(b'\0'.join(x.encode() for x in clean)+b'\0')
    (root/'configuration.json').write_text(json.dumps(dict(case=case,**p),indent=2)+'\n')

if __name__=='__main__':
    a=argparse.ArgumentParser();a.add_argument('--root',required=True);a.add_argument('--case',choices=cases(),required=True);a.add_argument('--side',choices=('client','server'),required=True);a.add_argument('args',nargs=argparse.REMAINDER);x=a.parse_args();prepare(x.root,x.case,x.side,x.args[1:] if x.args[:1]==['--'] else x.args)
