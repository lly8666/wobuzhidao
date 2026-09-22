#!/usr/bin/env python3
import argparse,json,struct
from pathlib import Path

def jl(p):
 o=[]; p=Path(p)
 if not p.exists(): return o
 for x in p.read_text(errors="replace").splitlines():
  try:o.append(json.loads(x))
  except:pass
 return o

def prod(r,side):
 p=(r or {}).get("product") or {}
 return (p.get("tunnel") if p.get("present") else None) if side=="server" else p

def own(r,side): return (prod(r,side) or {}).get("owner") or {}
def life(r): return (prod(r,"client") or {}).get("lifecycle") or {}
def cfg(r,side): return (prod(r,side) or {}).get("config") or {}
def near(rows,t): return min(rows,key=lambda r:abs(int(r.get("unix_ns",0))-t)) if rows else None

def gens(r,side="client"):
 return {int((x.get("ref")or{}).get("ID",0)):int((x.get("ref")or{}).get("Generation",0)) for x in ((prod(r,side)or{}).get("lanes")or[])}

def health(r,side="client",field="HealthSent"):
 return sum(int((x.get("transport")or{}).get(field,0)or 0) for x in ((prod(r,side)or{}).get("lanes")or[]))

def parity(r,side):
 return sorted({int(x.get("parity_shards",0)or 0) for x in ((prod(r,side)or{}).get("lanes")or[])})

def load(p): return json.loads(Path(p).read_text())

def ratio_range(s,r,lo,hi):
 a=(s.get("stats")or{}).get("sent_bytes_by_second")or[]
 b=(r.get("stats")or{}).get("recv_bytes_by_second")or[]
 hi=min(hi,len(a),len(b)); lo=max(0,min(lo,hi)); x=sum(a[lo:hi])
 return None if not x else sum(b[lo:hi])/x

def ratio(s,r,n):
 a=(s.get("stats")or{}).get("sent_bytes_by_second")or[]
 b=(r.get("stats")or{}).get("recv_bytes_by_second")or[]
 n=min(n,len(a),len(b)); x=sum(a[-n:]) if n else 0
 return None if not x else sum(b[-n:])/x

def pairs(root):
 out={}
 for b in root.glob("*-biz.json"):
  label=b.name[:-9]; t=root/f"{label}-target.json"
  if t.exists():
   bj,tj=load(b),load(t)
   out[label]={
    "c2s30":ratio(bj,tj,30),"s2c30":ratio(tj,bj,30),
    "c2s120":ratio(bj,tj,120),"s2c120":ratio(tj,bj,120),
    "c2s_first90":ratio_range(bj,tj,0,90),"s2c_first90":ratio_range(tj,bj,0,90),
    "c2s_sent_packets":sum((bj.get("stats")or{}).get("sent_packets_by_second")or[]),
    "s2c_sent_packets":sum((tj.get("stats")or{}).get("sent_packets_by_second")or[]),
   }
 return out

def syns(path):
 d=Path(path).read_bytes(); out={}
 if len(d)<24:return out
 e="<" if d[:4]==b"\xd4\xc3\xb2\xa1" else ">"
 off=24
 while off+16<=len(d):
  _,_,n,_=struct.unpack(e+"IIII",d[off:off+16]); off+=16; f=d[off:off+n]; off+=n
  if len(f)<54 or f[12:14]!=b"\x08\x00":continue
  ip=f[14:]; ih=(ip[0]&15)*4
  if ip[9]!=6 or len(ip)<ih+20:continue
  tcp=ip[ih:]; sp=struct.unpack("!H",tcp[:2])[0]; fl=tcp[13]
  if fl&2 and not fl&16: out[str(sp)]=out.get(str(sp),0)+1
 return out

def counter_transitions(rows,key):
 out=[]; prev=0
 for r in rows:
  v=int(life(r).get(key,0)or 0)
  if v>prev:
   out.append({"unix_ns":int(r.get("unix_ns",0)),"value":v})
   prev=v
 return out

def event_map(events):
 out={}
 for e in events: out.setdefault(e.get("event"),[]).append(e)
 return out

def delta_s(a,b):
 return (int(b)-int(a))/1e9

def main():
 a=argparse.ArgumentParser()
 a.add_argument("--artifact-dir",required=True); a.add_argument("--source-sha",required=True)
 a.add_argument("--scenario",required=True); a.add_argument("--lanes",type=int,required=True)
 a.add_argument("--fec",type=int,required=True); a.add_argument("--padding",type=int,choices=(0,1),required=True)
 a.add_argument("--output",required=True); x=a.parse_args()
 root=Path(x.artifact_dir); m=load(root/"manifest.json")
 c=jl(root/"client-diag.jsonl"); s=jl(root/"server-diag.jsonl"); ev=jl(root/"events.jsonl")
 cae=jl(root/"client-acceptance-events.jsonl"); sae=jl(root/"server-acceptance-events.jsonl")
 rs=jl(root/"resources.jsonl"); err=[]
 if m.get("source_sha")!=x.source_sha:err.append("SOURCE_SHA mismatch")
 if int(m.get("fec_parity",-1))!=x.fec or bool(m.get("padding_enabled"))!=bool(x.padding):err.append("manifest functional cross mismatch")
 if not c or not s:err.append("missing diagnostics")
 first=next((r for r in c if int(own(r,"client").get("ActiveLogicalLanes",0)or 0)>0),None)
 lc=next((r for r in reversed(c) if prod(r,"client")),None)
 ls=next((r for r in reversed(s) if prod(r,"server")),None)
 if not lc or not ls:err.append("missing final snapshots")
 else:
  for side,r in (("client",lc),("server",ls)):
   o=own(r,side); p=prod(r,side)or{}
   if int(o.get("ActiveLogicalLanes",0)or 0)!=x.lanes:err.append(f"{side} final lanes {o.get('ActiveLogicalLanes')}")
   if o.get("Dormant"):err.append(f"{side} final dormant")
   if p.get("lease4")!="10.66.0.2/32":err.append(f"{side} lease drift {p.get('lease4')}")
   if parity(r,side)!=[x.fec]:err.append(f"{side} fec effective {parity(r,side)} want {[x.fec]}")
 maxp=max([int(own(r,"client").get("PhysicalLanes",0)or 0) for r in c]+[0])
 maxcand=max([int(own(r,"client").get("Candidates",0)or 0) for r in c]+[0])
 maxret=max([int(own(r,"client").get("Retiring",0)or 0) for r in c]+[0])
 if maxp>x.lanes+2 or maxcand>1 or maxret>1:err.append(f"lane bound physical={maxp} candidate={maxcand} retiring={maxret}")
 maxg=max([int((r.get("runtime")or{}).get("goroutines",0)or 0) for r in c]+[0])
 maxh=max([int((r.get("runtime")or{}).get("heap_inuse",0)or 0) for r in c]+[0])
 if maxg>512 or maxh>268435456:err.append(f"runtime bound goroutines={maxg} heap={maxh}")
 lf=life(lc) if lc else {}
 if int(lf.get("RecoveryAttempts",0)or 0)>64:err.append(f"retry storm {lf}")
 pr=pairs(root); em=event_map(ev); sc=x.scenario
 attempts=counter_transitions(c,"RecoveryAttempts"); failed=counter_transitions(c,"RecoveryFailed"); succeeded=counter_transitions(c,"RecoverySucceeded")

 if sc=="l0_config":
  want_client={"keepalive_interval":"1s","dead_after":"6s","reconnect_min":"1s","reconnect_max":"4s","dormant_after":"0s"}
  want_server={"keepalive_interval":"1s","dormant_after":"0s"}
  for k,v in want_client.items():
   if cfg(lc,"client").get(k)!=v:err.append(f"client config {k}={cfg(lc,'client').get(k)} want={v}")
  for k,v in want_server.items():
   if cfg(ls,"server").get(k)!=v:err.append(f"server config {k}={cfg(ls,'server').get(k)} want={v}")
  pad=(own(lc,"client").get("Padding")or{})
  if bool(pad.get("Enabled"))!=bool(x.padding):err.append(f"padding effective {pad.get('Enabled')} want {bool(x.padding)}")
  if x.padding and int(pad.get("PaddingBytes",0)or 0)!=0:err.append("startup-only padding touched non-TLS UDP")
  if health(lc)<4:err.append(f"CLI keepalive override ineffective {health(lc)}")

 if sc=="l1_idle_downlink":
  z=(em.get("idle_observed")or[None])[0]; q=near(c,int(z["unix_ns"])) if z else None
  if not q or not own(q,"client").get("Dormant"):err.append("30s no-business did not dormancy")
  z2=(em.get("sparse_idle_observed")or[None])[0]; q2=near(c,int(z2["unix_ns"])) if z2 else None
  if not q2 or not own(q2,"client").get("Dormant"):err.append("second sparse precondition did not dormancy")
  if (pr.get("sparse")or{}).get("c2s30") is None or pr["sparse"]["c2s30"]<.95:err.append(f"sparse c2s failed {pr.get('sparse')}")
  if (pr.get("downlink")or{}).get("s2c30") is None or pr["downlink"]["s2c30"]<.95:err.append(f"pure downlink failed {pr.get('downlink')}")
  done=(em.get("scenario_complete")or[None])[-1]; q3=near(c,int(done["unix_ns"])-2_000_000_000) if done else None
  if not q3 or own(q3,"client").get("Dormant"):err.append("pure downlink allowed false dormancy")

 if sc in ("l2_c2s","l2_s2c"):
  z=(em.get("fault_start")or[None])[0]; side="client" if sc=="l2_c2s" else "server"; rows=c if side=="client" else s
  q=near(rows,int(z["unix_ns"])+10_000_000_000) if z else None
  if not q or own(q,side).get("Dormant"):err.append(f"{side} false dormant under lost offered business")
  key="c2s30" if sc=="l2_c2s" else "s2c30"; v=(pr.get("main")or{}).get(key)
  if v is None or v<.95:err.append(f"post-loss delivery {key}={v}")

 if sc=="l3_health":
  if sum(1 for z in cae if z.get("kind")=="health")!=6:err.append(f"client health receipts={cae}")
  if sum(1 for z in sae if z.get("kind")=="health")!=6:err.append(f"server health receipts={sae}")
  if any(own(r,"client").get("Dormant") for r in c) or any(own(r,"server").get("Dormant") for r in s):err.append("health loss caused dormant")
  for k in ("c2s30","s2c30"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.95:err.append(f"health-loss business delivery {k}={v}")

 repl=sc.startswith("l4_") or sc.startswith("l5_") or sc=="l6_partial"
 if repl and first and lc and not any(gens(lc).get(k,0)>v for k,v in gens(first).items()):err.append(f"generation did not advance {gens(first)}->{gens(lc)}")

 if sc.startswith("l4_") or sc.startswith("l5_") or sc=="l6_partial":
  if int(lf.get("RecoverySucceeded",0)or 0)<1:err.append(f"no recovery success {lf}")
  for k in ("c2s30","s2c30"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.90:err.append(f"30s recovery {k}={v}")

 if sc.startswith("l4_"):
  z=(em.get("fault_end")or em.get("fault_start")or[None])[-1]
  if z and succeeded:
   ds=delta_s(z["unix_ns"],succeeded[-1]["unix_ns"])
   if ds<0 or ds>25:err.append(f"L4 recovery budget {ds:.3f}s")
  elif not succeeded:err.append("L4 missing recovery timeline")

 if sc.startswith("l5_"):
  if cfg(lc,"client").get("rotate_min")!="1m10s" or cfg(lc,"client").get("rotate_max")!="1m10s":err.append(f"L5 rotation config {cfg(lc,'client')}")
  if int(lf.get("RecoveryFailed",0)or 0)<1:err.append(f"candidate failure uncounted {lf}")
  if int(lf.get("RecoveryAttempts",0)or 0)>3:err.append(f"candidate retry count too high {lf}")
  for k in ("c2s_first90","s2c_first90"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.90:err.append(f"old lane did not continue service {k}={v}")
  if failed and len(attempts)>=2:
   later=next((z for z in attempts if z["unix_ns"]>failed[0]["unix_ns"]),None)
   if later:
    ds=delta_s(failed[0]["unix_ns"],later["unix_ns"])
    if ds<.75 or ds>5.5:err.append(f"candidate backoff out of bounds {ds:.3f}s")
   else:err.append("missing retry after candidate failure")
  else:err.append(f"missing failure/backoff timeline attempts={attempts} failed={failed}")
  if sc in ("l5_tls","l5_admission","l5_detach"):
   k=sc[3:]
   if sum(1 for z in cae if z.get("kind")==k)!=1:err.append(f"{k} receipt missing")
  if sc=="l5_syn":
   sy=syns(root/"underlay.pcap")
   if int(sy.get("40001",0))<2:err.append(f"SYN evidence {sy}")
   if attempts and failed:
    ds=delta_s(attempts[0]["unix_ns"],failed[0]["unix_ns"])
    if ds<12 or ds>19:err.append(f"SYN candidate timeout {ds:.3f}s")

 sy=syns(root/"underlay.pcap")

 if sc in ("l6_race1","l6_race4"):
  if int(lf.get("RecoveryAttempts",0)or 0)<1:err.append("race did not exercise wake")
  main=pr.get("main")or{}
  if int(main.get("c2s_sent_packets",0))<100 or int(main.get("s2c_sent_packets",0))<100:err.append(f"race did not reach 100 alternating demand events {main}")
  for k in ("c2s30","s2c30"):
   v=main.get(k)
   if v is None or v<.80:err.append(f"race delivery {k}={v}")

 if sc=="l6_partial":
  if sum(1 for z in cae if z.get("kind")=="admission" and int(z.get("lane",0))==3)!=1:err.append("partial wake lane3 receipt missing")

 if sc=="l7_defaults":
  cc=cfg(lc,"client"); ss=cfg(ls,"server")
  if cc.get("keepalive_interval")!="15s" or cc.get("dead_after")!="1m30s" or ss.get("keepalive_interval")!="15s":err.append(f"default timing config client={cc} server={ss}")
  for k in ("c2s30","s2c30"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.90:err.append(f"default sustained recovery {k}={v}")
  if health(lc)<2:err.append("default 15s keepalive not observed")
  start=(em.get("fault_start")or[None])[0]; end=(em.get("fault_end")or[None])[0]
  if not start or not attempts:err.append("default 90s dead not exercised")
  else:
   ds=delta_s(start["unix_ns"],attempts[0]["unix_ns"])
   if ds<82 or ds>103:err.append(f"default first suspicion {ds:.3f}s outside 90s budget")
  if end and succeeded:
   after=[z for z in succeeded if z["unix_ns"]>=int(end["unix_ns"])]
   if not after:err.append("default no recovery success after blackhole clear")
   elif delta_s(end["unix_ns"],after[0]["unix_ns"])>50:err.append(f"default recovery after clear {delta_s(end['unix_ns'],after[0]['unix_ns']):.3f}s")

 result={
  "schema":1,"source_sha":x.source_sha,"scenario":sc,"lanes":x.lanes,"seed":int(m.get("seed",0)),
  "fec_parity":x.fec,"padding_enabled":bool(x.padding),"result":"PASS" if not err else "FAIL","errors":err,
  "effective_config":{"client":cfg(lc,"client") if lc else{},"server":cfg(ls,"server") if ls else{}},
  "lifecycle_final":lf,"recovery_timeline":{"attempts":attempts,"failed":failed,"succeeded":succeeded},
  "first_generation":gens(first) if first else{},"final_generation":gens(lc) if lc else{},
  "resource":{"max_physical":maxp,"max_candidates":maxcand,"max_retiring":maxret,"max_goroutines":maxg,"max_heap":maxh,"samples":len(rs)},
  "traffic":pr,"acceptance_events":{"client":cae,"server":sae},"syn_by_source_port":sy
 }
 Path(x.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
 print("WBD_LIFECYCLE_FULLSTACK "+json.dumps({"scenario":sc,"seed":result["seed"],"fec":x.fec,"padding":bool(x.padding),"result":result["result"],"errors":err,"lifecycle":lf,"traffic":pr},sort_keys=True))
 raise SystemExit(0 if not err else 1)

if __name__=="__main__":main()
