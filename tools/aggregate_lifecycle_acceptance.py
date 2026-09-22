#!/usr/bin/env python3
import argparse,json
from pathlib import Path

def main():
 ap=argparse.ArgumentParser()
 ap.add_argument("--root",required=True); ap.add_argument("--source-sha",required=True); ap.add_argument("--output",required=True)
 x=ap.parse_args(); rows=[]; errors=[]
 for p in Path(x.root).rglob("summary.json"):
  try:r=json.loads(p.read_text())
  except Exception:continue
  if r.get("schema")==1 and r.get("scenario","").startswith("l"):
   r["_path"]=str(p); rows.append(r)
 idx={(r.get("scenario"),int(r.get("lanes",0)),int(r.get("fec_parity",-1)),bool(r.get("padding_enabled")),int(r.get("seed",0))):r for r in rows}
 expected=[]
 for seed in (101,202):
  for fec in (0,20):
   for pad in (False,True):expected.append(("l0_config",1,fec,pad,seed))
 for sc,lanes in (
  ("l1_idle_downlink",1),("l2_c2s",1),("l2_s2c",1),("l3_health",1),
  ("l4_old_tuple",1),("l4_all_tuple",1),
  ("l5_syn",1),("l5_tls",1),("l5_admission",1),("l5_detach",1),
  ("l6_race1",1),("l6_race4",4),("l6_partial",4),("l7_defaults",1),
 ):
  for seed in (101,202):expected.append((sc,lanes,20,False,seed))
 missing=[k for k in expected if k not in idx]
 if missing:errors.append(f"missing lifecycle samples: {missing}")
 unexpected=[k for k in idx if k not in expected]
 if unexpected:errors.append(f"unexpected lifecycle samples: {unexpected}")
 bad_sha=[k for k,r in idx.items() if r.get("source_sha")!=x.source_sha]
 if bad_sha:errors.append(f"SOURCE_SHA mismatch: {bad_sha}")
 failed=[{"key":k,"errors":idx[k].get("errors",[])} for k in expected if k in idx and idx[k].get("result")!="PASS"]
 if failed:errors.append(f"failed lifecycle samples: {failed}")
 groups={}
 for sc in sorted({k[0] for k in expected}):
  ks=[k for k in expected if k[0]==sc]
  groups[sc]={"sample_count":sum(k in idx for k in ks),"required":len(ks),"pass":all(k in idx and idx[k].get("result")=="PASS" for k in ks)}
 result={"schema":1,"source_sha":x.source_sha,"sample_count":len(idx),"required_count":len(expected),"result":"PASS" if not errors else "FAIL","groups":groups,"missing":missing,"failed":failed,"errors":errors}
 Path(x.output).write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
 print("WBD_LIFECYCLE_AGGREGATE "+json.dumps({"source_sha":x.source_sha,"result":result["result"],"sample_count":len(idx),"required_count":len(expected),"groups":groups},sort_keys=True))
 raise SystemExit(0 if not errors else 1)
if __name__=="__main__":main()
