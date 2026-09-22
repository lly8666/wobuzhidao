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
def near(rows,t): return min(rows,key=lambda r:abs(int(r.get("unix_ns",0))-t)) if rows else None
def gens(r,side="client"):
 return {int((x.get("ref")or{}).get("ID",0)):int((x.get("ref")or{}).get("Generation",0)) for x in ((prod(r,side)or{}).get("lanes")or[])}
def health(r):
 return sum(int((x.get("transport")or{}).get("HealthSent",0)or 0) for x in ((prod(r,"client")or{}).get("lanes")or[]))
def load(p): return json.loads(Path(p).read_text())
def ratio(s,r,n):
 a=(s.get("stats")or{}).get("sent_bytes_by_second")or[]; b=(r.get("stats")or{}).get("recv_bytes_by_second")or[]; n=min(n,len(a),len(b)); x=sum(a[-n:]) if n else 0
 return None if not x else sum(b[-n:])/x
def pairs(root):
 out={}
 for b in root.glob("*-biz.json"):
  label=b.name[:-9]; t=root/f"{label}-target.json"
  if t.exists():
   bj,tj=load(b),load(t); out[label]={"c2s30":ratio(bj,tj,30),"s2c30":ratio(tj,bj,30),"c2s120":ratio(bj,tj,120),"s2c120":ratio(tj,bj,120)}
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
def main():
 a=argparse.ArgumentParser(); a.add_argument("--artifact-dir",required=True); a.add_argument("--source-sha",required=True); a.add_argument("--scenario",required=True); a.add_argument("--lanes",type=int,required=True); a.add_argument("--output",required=True); x=a.parse_args()
 root=Path(x.artifact_dir); m=load(root/"manifest.json"); c=jl(root/"client-diag.jsonl"); s=jl(root/"server-diag.jsonl"); ev=jl(root/"events.jsonl"); ae=jl(root/"client-acceptance-events.jsonl"); rs=jl(root/"resources.jsonl"); err=[]
 if m.get("source_sha")!=x.source_sha:err.append("SOURCE_SHA mismatch")
 if not c or not s:err.append("missing diagnostics")
 first=next((r for r in c if int(own(r,"client").get("ActiveLogicalLanes",0)or 0)>0),None); lc=next((r for r in reversed(c) if prod(r,"client")),None); ls=next((r for r in reversed(s) if prod(r,"server")),None)
 if not lc or not ls:err.append("missing final snapshots")
 else:
  for side,r in (("client",lc),("server",ls)):
   o=own(r,side); p=prod(r,side)or{}
   if int(o.get("ActiveLogicalLanes",0)or 0)!=x.lanes:err.append(f"{side} final lanes {o.get('ActiveLogicalLanes')}")
   if o.get("Dormant"):err.append(f"{side} final dormant")
   if p.get("lease4")!="10.66.0.2/32":err.append(f"{side} lease drift {p.get('lease4')}")
 maxp=max([int(own(r,"client").get("PhysicalLanes",0)or 0) for r in c]+[0]); maxcand=max([int(own(r,"client").get("Candidates",0)or 0) for r in c]+[0]); maxret=max([int(own(r,"client").get("Retiring",0)or 0) for r in c]+[0])
 if maxp>x.lanes+2 or maxcand>1 or maxret>1:err.append(f"lane bound physical={maxp} candidate={maxcand} retiring={maxret}")
 maxg=max([int((r.get("runtime")or{}).get("goroutines",0)or 0) for r in c]+[0]); maxh=max([int((r.get("runtime")or{}).get("heap_inuse",0)or 0) for r in c]+[0])
 if maxg>512 or maxh>268435456:err.append(f"runtime bound goroutines={maxg} heap={maxh}")
 lf=life(lc) if lc else {}
 if int(lf.get("RecoveryAttempts",0)or 0)>64:err.append(f"retry storm {lf}")
 pr=pairs(root); em={z.get("event"):z for z in ev}; sc=x.scenario
 if sc=="l0_config":
  if health(lc)<4:err.append(f"CLI keepalive override ineffective {health(lc)}")
  if (own(lc,"client").get("Padding")or{}).get("Enabled"):err.append("CLI false did not override JSON true")
 if sc=="l1_idle_downlink":
  z=em.get("idle_observed"); q=near(c,int(z["unix_ns"])) if z else None
  if not q or not own(q,"client").get("Dormant"):err.append("30s idle did not dormancy")
  if (pr.get("downlink")or{}).get("s2c30") is None or pr["downlink"]["s2c30"]<.95:err.append(f"pure downlink failed {pr.get('downlink')}")
 if sc in ("l2_c2s","l2_s2c"):
  z=em.get("fault_start"); side="client" if sc=="l2_c2s" else "server"; rows=c if side=="client" else s; q=near(rows,int(z["unix_ns"])+10_000_000_000) if z else None
  if not q or own(q,side).get("Dormant"):err.append(f"{side} false dormant under lost business")
  key="c2s30" if sc=="l2_c2s" else "s2c30"; v=(pr.get("main")or{}).get(key)
  if v is None or v<.95:err.append(f"post-loss delivery {key}={v}")
 if sc=="l3_health":
  if sum(1 for z in ae if z.get("kind")=="health")!=6:err.append(f"health receipts={ae}")
  if any(own(r,"client").get("Dormant") for r in c):err.append("health loss caused dormant")
 repl=sc.startswith("l4_") or sc.startswith("l5_") or sc=="l6_partial"
 if repl and first and lc and not any(gens(lc).get(k,0)>v for k,v in gens(first).items()):err.append(f"generation did not advance {gens(first)}->{gens(lc)}")
 if sc.startswith("l4_") or sc.startswith("l5_") or sc=="l6_partial":
  if int(lf.get("RecoverySucceeded",0)or 0)<1:err.append(f"no recovery success {lf}")
  for k in ("c2s30","s2c30"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.90:err.append(f"30s recovery {k}={v}")
 if sc in ("l5_tls","l5_admission","l5_detach"):
  k=sc[3:]
  if sum(1 for z in ae if z.get("kind")==k)!=1:err.append(f"{k} receipt missing")
  if int(lf.get("RecoveryFailed",0)or 0)<1:err.append(f"{k} failure uncounted")
 sy=syns(root/"underlay.pcap")
 if sc=="l5_syn":
  if int(sy.get("40001",0))<2:err.append(f"SYN evidence {sy}")
  if int(lf.get("RecoveryFailed",0)or 0)<1:err.append("SYN failure uncounted")
 if sc in ("l6_race1","l6_race4"):
  if int(lf.get("RecoveryAttempts",0)or 0)<1:err.append("race did not exercise wake")
  for k in ("c2s30","s2c30"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.80:err.append(f"race delivery {k}={v}")
 if sc=="l6_partial" and sum(1 for z in ae if z.get("kind")=="admission" and int(z.get("lane",0))==3)!=1:err.append("partial wake lane3 receipt missing")
 if sc=="l7_defaults":
  for k in ("c2s120","s2c120"):
   v=(pr.get("main")or{}).get(k)
   if v is None or v<.90:err.append(f"default 120s recovery {k}={v}")
  if health(lc)<2:err.append("default 15s keepalive not observed")
  if int(lf.get("RecoveryAttempts",0)or 0)<1:err.append("default 90s dead not exercised")
 result={"schema":1,"source_sha":x.source_sha,"scenario":sc,"result":"PASS" if not err else "FAIL","errors":err,"lifecycle_final":lf,"first_generation":gens(first) if first else{},"final_generation":gens(lc) if lc else{},"resource":{"max_physical":maxp,"max_candidates":maxcand,"max_retiring":maxret,"max_goroutines":maxg,"max_heap":maxh,"samples":len(rs)},"traffic":pr,"acceptance_events":ae,"syn_by_source_port":sy}
 Path(x.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n"); print("WBD_LIFECYCLE_FULLSTACK "+json.dumps({"scenario":sc,"result":result["result"],"errors":err,"lifecycle":lf,"traffic":pr},sort_keys=True)); raise SystemExit(0 if not err else 1)
if __name__=="__main__":main()
