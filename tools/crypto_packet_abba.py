#!/usr/bin/env python3
"""Exact two-SOURCE serial fullstack. Every case is a fresh 5-netns test."""
import argparse, fcntl, json, os, subprocess, sys, time
from pathlib import Path
import fec_policy_batch as old

A="7fb98fab79834a351a1dbe04eebb207f66bea28b"
B="37e18653b0d08f4a1d932b6fd67fe081e84bda78"
CONFIG=Path(".github/crypto-packet-abba.json")
PARAMS={
 "Q1":("normal",1,10,20,300,0,2261),
 "OFF0":("normal",1,10,0,15,0,2267),
 "OFF1":("normal",1,10,0,15,1,2268),
 "G2":("game",2,3,20,300,0,2269),
 "G4":("game",4,3,20,300,0,2270),
 "G4LOSS":("game",4,3,20,300,5205,2271),
 "Q2":("normal",1,10,20,300,5205,2262),
 "Q3":("game",2,3,20,300,5205,2263),
 "L1":("normal",1,10,4,15,1,2264),
 "L2":("normal",1,10,10,15,1,2265),
 "L3":("normal",1,10,0,15,0,2266),
}
def plan(phase):
 groups={"preflight":("Q1","Q2","Q3","L1","L2","L3"),"pilot":("Q1",),"q1screen120":("Q1",),
         "q120":("Q1","Q2","Q3"),"l120":("L1","L2","L3"),
         "off120":("OFF0","OFF1"),"game120":("G2","G4","G4LOSS"),
         "confirm300":("Q1","Q2","Q3"),"q2historic300":("Q2",),"q2observe300":("Q2",)}[phase]
 # Pilot Q1 must exercise all 20 serial HTTP/HTTPS requests over 300ms RTT;
 # 15s + fixed 3s drain can close targets before their final TCP opens.
 # Use the same 120s real workload as Q1 without relaxing any validity gate.
 n=300 if phase in ("confirm300","q2historic300","q2observe300") else 120
 result=[]
 for context in groups:
  mode,lanes,rate,parity,delay,loss,seed=PARAMS[context]
  if phase in ("q2historic300","q2observe300"):seed=1844 # Same as historical 300s old A PASS run37924613499.
  labels=("A","B") if phase=="pilot" else ("A",) if phase=="preflight" else ("A","B","B","A")
  for label in labels:
   result.append(dict(id=("pilot-%d"%(len(result)+1) if phase=="pilot" else "s%02d"%(len(result)+1)),
      context=context,label=label,source=A if label=="A" else B,
      workload="mixed",mode=mode,lanes=lanes,rate=rate,parity=parity,
      delay=delay,loss=loss,seed=seed,duration=n))
 return result

def conf():
 d=json.loads(CONFIG.read_text())
 if d != {"schema":"wbd-crypto-packet-abba/v1","phase":d.get("phase"),
          "baseline_source_sha":A,"candidate_source_sha":B,"nonce":d.get("nonce")}:
  raise ValueError("unapproved source or config keys")
 if d["phase"] not in ("preflight","pilot","q1screen120","q120","l120","off120","game120","confirm300","q2historic300","q2observe300") or type(d["nonce"])!=int or d["nonce"]<1:
  raise ValueError("unapproved phase")
 return d

def prepared(c,root):
 d=root/c["id"];d.mkdir(parents=True,exist_ok=False)
 for role in ("client","server"):
  p=root/"binaries"/c["label"]/("wbd-"+role)
  if p.is_file(): (d/("wbd-"+role)).symlink_to(p)
 cmd=[sys.executable,"tools/prepare_large_mtu_harness.py","--fec-experiment","--simd-ab",
      "--output",str(d/"generated.sh"),"--workload","mixed","--loss",str(c["loss"]),
      "--duration-s",str(c["duration"]),"--delay-ms",str(c["delay"])]
 subprocess.run(cmd,check=True,env={**os.environ,"WBD_EFF_DIAGNOSTIC":"0"})
 subprocess.run(["bash","-n",str(d/"generated.sh")],check=True)
 return d

def j(d,name):
 try:return json.loads((d/name).read_text())
 except (OSError,ValueError):return {}


def shell_failure_classes(raw):
 # Fixed status vocabulary, never return passwords, stderr lines or PIDs.
 import re
 markers=[]
 for match in re.finditer(r"FEC_EXPERIMENT_SHELL_FAIL code=(\d+) line=(\d+)",raw):
  markers.append({"class":"SHELL_FAIL","exit_code":int(match.group(1)),
                  "script_line":int(match.group(2))})
 for match in re.finditer(r"WBD_STRICT_(CLIENT|SERVER)_EARLY_EXIT pid=\d+",raw):
  markers.append({"class":"PRODUCT_"+match.group(1)+"_EARLY_EXIT"})
 for word,label in (("Operation not permitted","NETNS_PERMISSION"),
                    ("invalid strict mode tuple","INVALID_MODE_TUPLE"),
                    ("No such file or directory","FILE_NOT_FOUND"),
                    ("sudo:","SUDO_REJECTED")):
  if word in raw:markers.append({"class":label})
 return markers[-16:]



def sanitized_product_error_line(line):
 # Only print bounded textual error categories after replacing identifiers.
 # No raw log upload, command lines, payloads or credentials. Test fixture's
 # passwords are synthetic but must still never appear in public artifacts.
 import re
 if "WBD_RAW_RCVBUF" in line or "WBD_SERVER_STOPPED" in line:return ""
 if not re.match(r"^\d{4}/\d\d/\d\d ",line):return ""
 line=re.sub(r"^\d{4}/\d\d/\d\d \d\d:\d\d:\d\d(?:\.\d+)?\s*","",line)
 line=re.sub(r"(?i)(--(?:password|username|route-key-hex|tls-key|token)\s+)\S+",r"\1<redacted>",line)
 line=re.sub(r"(?i)\b(?:password|passwd|secret|token|credential|route[-_]?key|tls[-_]?key|username|account)\s*[:=]\s*\S+","<redacted_field>",line)
 line=re.sub(r'"[^"]*"',"<quoted>",line)
 line=re.sub(r"'[^']*'","<quoted>",line)
 line=re.sub(r"\b[0-9a-fA-F]{16,}\b","<hex>",line)
 line=re.sub(r"\b\d{1,3}(?:\.\d{1,3}){3}(?::\d+)?\b","<ip>",line)
 line=re.sub(r"/(?:[\w.-]+/)*[\w.-]+","<path>",line)
 line="".join(c if c.isascii() and (c.isalnum() or c in " .:,;_-()<>[]=+") else " " for c in line).strip()
 if not any(word in line.lower() for word in ("error","no such file","connection","fail","closed","panic","invalid","cannot","denied","unreachable","route","timeout")):return ""
 return line[:240]

def bounded_product_exit_evidence(path):
 # No raw product logs in artifacts. Preserve only fixed error vocabulary,
 # per-file sizes and SHA; unknown/private words cannot pass the vocabulary.
 import re
 known=set("""fatal error panic runtime server stopped cleanup network
   read write send receive packet route routing tun raw socket syscalls
   netns netlink file descriptor permission denied operation permitted
   timeout deadline context canceled cancelled close closed closing
   connection reset broken pipe EOF eof network unreachable unavailable
   device exists invalid argument no such missing cannot failed fail
   bad unexpected short length mtu malformed overflow buffer generation
   unknown tunnel lease authentication unauthenticated transport address
   already in use not qualified systemd notify ready recvfrom sendmmsg
   recvmmsg queue full exhausted shutdown checksum resource too many
   threads memory signal killed segmentation fault index out of range
   concurrent map writes WBD_SERVER_STOPPED WBD_RAW_RCVBUF
   cleanup=owned-only""".lower().split())
 try:raw=path.read_bytes()
 except OSError:return {"present":False}
 import hashlib
 lines=raw.decode("utf-8","replace").splitlines()
 groups=[]
 for line in lines[-12:]:
  words=re.findall(r"[A-Za-z][A-Za-z_]*",line.lower())
  kept=[w for w in words if w in known]
  if kept:groups.append(" ".join(kept[:32]))
 return {"present":True,"byte_size":len(raw),"sha256":hashlib.sha256(raw).hexdigest(),
         "line_count":len(lines),"last_error_vocabulary":groups[-6:],
         "panic_present":any("panic:" in s or "fatal error:" in s for s in lines),
         "server_stopped_present":any("WBD_SERVER_STOPPED" in s for s in lines),
         "sanitized_error_tail":[x for x in (sanitized_product_error_line(line)
             for line in lines[-8:]) if x][-3:]}

def run_case(c,root,helper):
 d=prepared(c,root)
 env={**os.environ,"GITHUB_WORKSPACE":str(Path.cwd()),
      "WBD_STRICT_MODE":c["mode"],"WBD_STRICT_SCENARIO":"5205" if c["loss"]==5205 else "lossless",
      "WBD_STRICT_SEED":str(c["seed"]),"WBD_STRICT_RATE_MBPS":str(c["rate"]),
      "WBD_STRICT_LANES":str(c["lanes"]),"WBD_STRICT_FEC_PARITY":str(c["parity"]),
      "WBD_STRICT_FEC_SCREEN":"1" if c["parity"]!=20 else "0",
      "WBD_STRICT_CPU_PROFILE":"0","WBD_STRICT_STATEFUL_GATE":"0",
      "WBD_STRICT_WAN_NEIGHBORS":"dynamic","WBD_LARGE_WORKLOAD":"mixed",
      "WBD_LARGE_LOSS":str(c["loss"]),"WBD_EFF_SIZE_PROFILE":"ordinary",
      "WBD_EFF_DIAGNOSTIC":"0","WBD_FEC_CASE_SFX":old.namespace_suffix(c["id"]),
      "WBD_FEC_DURATION_S":str(c["duration"]),"WBD_FEC_DELAY_MS":str(c["delay"])}
 before=old.snapshot()
 cmd=["sudo","--preserve-env="+",".join(old.ENV_NAMES),"env","GITHUB_SHA="+c["source"],
      "WBD_HARNESS_SHA="+helper,"WBD_STRICT_ARTIFACT_DIR="+str(d),
      "bash",str(d/"generated.sh")]
 rc=None;error=None
 try:
  with (d/"private-product.log").open("w") as log:
   rc=subprocess.run(cmd,env=env,stdout=log,stderr=subprocess.STDOUT,
                     timeout=c["duration"]+160).returncode
 except (OSError,subprocess.TimeoutExpired) as e:error=type(e).__name__
 # The audited fullstack subprocess is privileged and owns its receipts,
 # pcaps and summary input files. Only chown this case's private artifact
 # directory back to the GitHub runner after processes exit, so Python's
 # parser/hash/cleanup cannot fail reading tcpdump's root-only captures.
 subprocess.run(["sudo","chown","-R",str(os.getuid())+":"+str(os.getgid()),
                 str(d)],check=True,timeout=60)
 analyzer=[sys.executable,"tools/check_large_mtu_mixed.py","--fec-experiment","--simd-ab",
     "--artifact-dir",str(d),"--source",c["source"],"--helper",helper,
     "--workload","mixed","--loss",str(c["loss"]),"--seed",str(c["seed"]),
     "--target-mbps",str(c["rate"]),"--size-profile","ordinary",
     "--mode",c["mode"],"--lanes",str(c["lanes"]),"--diagnostic-mode","0",
     "--duration-s",str(c["duration"]),"--delay-ms",str(c["delay"]),
     "--fec-parity",str(c["parity"]),"--output",str(d/"summary.json")]
 with (d/"private-analyzer.log").open("w") as log:
  ac=subprocess.run(analyzer,stdout=log,stderr=subprocess.STDOUT).returncode
 with (d/"private-ledger.log").open("w") as log:
  lc=subprocess.run([sys.executable,"tools/efficiency_cost_ledger.py",
        "--artifact-dir",str(d),"--output",str(d/"efficiency-ledger.json")],
        stdout=log,stderr=subprocess.STDOUT).returncode
 captures=[]
 for p in d.glob("*.pcap*"):
  captures.append({"file":p.name,"sha256":old.filehash(p),"bytes":p.stat().st_size})
  p.unlink()
 (d/"capture-cleanup.json").write_text(json.dumps(captures,indent=2)+"\n")
 p=d/"private-product.log"
 markers=shell_failure_classes(p.read_text(errors="replace") if p.is_file() else "")
 (d/"sanitized-startup.json").write_text(json.dumps({
    "private_log_sha256":old.filehash(p) if p.is_file() else None,
    "private_log_bytes":p.stat().st_size if p.is_file() else 0,
    "sample_exit":rc,"sample_error":error,"markers":markers[-16:],
    "receipt_files_present":{name:(d/name).is_file() for name in
        ("manifest.json","biz.json","target.json","stage-events.jsonl","runtime-flags.json")},
    "product_log_bytes":{role:(d/(role+".log")).stat().st_size
       if (d/(role+".log")).is_file() else None for role in ("client","server")},
    "product_exit_evidence":{role:bounded_product_exit_evidence(d/(role+".log"))
        for role in ("client","server")}
    },indent=2)+"\n")
 owned=simd_owned(d,old.namespace_suffix(c["id"]))
 s=j(d,"summary.json");l=j(d,"efficiency-ledger.json");flags=j(d,"runtime-flags.json")
 state=s.get("classification","INFRA_INVALID")
 if any("CORRUPT" in str(issue) or "HASH_MISMATCH" in str(issue) for issue in s.get("issues",[])):
  state="INTEGRITY_STOP"
 elif rc!=0 or error or ac!=0 or lc!=0 or not owned["clean"] or not s or not l:
  if state!="FAIL":state="INFRA_INVALID"
 elif any(flags.get(role,{}).get("--fec-parity")!=str(c["parity"]) for role in ("client","server")):
  state="INFRA_INVALID"
 binaries={role:old.filehash(root/"binaries"/c["label"]/("wbd-"+role))
           for role in ("client","server")}
 evidence={name:old.filehash(d/name) for name in
    ("summary.json","manifest.json","efficiency-ledger.json","runtime-flags.json","generated.sh.receipt.json")
    if (d/name).is_file()}
 row={"case":c,"source_sha":c["source"],"helper_sha":helper,
      "binary_sha256":binaries,"before":before,"after":old.snapshot(),
      "sample_exit":rc,"sample_error":error,"analyzer_exit":ac,
      "ledger_exit":lc,"classification":state,"issues":s.get("issues",[]),
      "owned_cleanup":owned,"evidence_sha256":evidence}
 row["sanitized_shell_markers"]=markers
 (d/"case-receipt.json").write_text(json.dumps(row,indent=2)+"\n")
 return row

def simd_owned(d,suffix):
 # Preserve the old fixture's strict pre-cleanup leak accounting, while
 # allowing this new SIMD pilot to terminate scoped root-owned netns children.
 # Never signal an unrelated PID: recheck the precise case path immediately
 # before attempting a signal, and never treat a cleaned leak as a PASS.
 import re, signal
 ps=subprocess.run(["ps","-eo","pid=,args="],check=True,text=True,capture_output=True).stdout
 pids=[]
 for line in ps.splitlines():
  m=re.match(r"\s*(\d+)\s+(.*)",line)
  if m and str(d)+"/" in m[2] and "crypto_packet_abba.py" not in m[2] and "fec_policy_batch.py" not in m[2]:
   pids.append(int(m[1]))
 fallbacks=0;failures=[]
 for pid in pids:
  p=subprocess.run(["ps","-p",str(pid),"-o","args="],text=True,capture_output=True)
  if p.returncode!=0:continue
  if str(d)+"/" not in p.stdout:
   failures.append(pid);continue
  try:os.kill(pid,signal.SIGTERM)
  except ProcessLookupError:continue
  except PermissionError:
   # GitHub Actions runner owns the test, but product/netns processes run as root.
   # Use noninteractive sudo only for the already-scoped PID, never a process group.
   sent=subprocess.run(["sudo","-n","kill","-TERM","--",str(pid)],
                       text=True,capture_output=True)
   fallbacks+=1
   if sent.returncode!=0:
    again=subprocess.run(["ps","-p",str(pid),"-o","args="],text=True,capture_output=True)
    if again.returncode==0 and str(d)+"/" in again.stdout:failures.append(pid)
 if pids:time.sleep(2)
 ns=subprocess.run(["sudo","ip","netns","list"],check=True,text=True,capture_output=True).stdout
 leaked_ns=[n+"-"+suffix for n in ("wbiz","wcli","wrtr","wsrv","wtgt")
            if re.search(r"(?m)^"+re.escape(n+"-"+suffix)+r"(?:\s|$)",ns)]
 return {"original_leaked_pids":pids,"sudo_fallback_count":fallbacks,
         "cleanup_signal_failures":failures,"leftover_namespaces":leaked_ns,
         "clean":not pids and not leaked_ns and not failures}


# The original quality FAIL remains a FAIL. The diagnostic observation
# alone may move to the next leg after a measured UDP/probe rate FAIL.
ALLOW_QUALITY=(
 "5205_STAGE_PROBE_LOSS_OVER_1PCT_",
 "5205_STAGE_UDP_LOSS_OVER_0P1PCT_",
 "5205_UDP_EVENTUAL_LOSS_OVER_0P1PCT_",
 "5205_PROBE_TIMEOUT_OVER_1PCT_",
)
def may_observe_next(phase,r):
 if phase!="q2observe300" or r.get("classification")!="FAIL":return False
 issues=r.get("issues",[])
 if not isinstance(issues,list) or not issues:return False
 if not all(isinstance(x,str) and x.startswith(ALLOW_QUALITY) for x in issues):return False
 if (r.get("sample_exit")!=0 or r.get("sample_error") is not None or
     r.get("analyzer_exit")!=1 or r.get("ledger_exit")!=0 or
     not r.get("owned_cleanup",{}).get("clean")):return False
 return all(r.get("evidence_sha256",{}).get(x) for x in
       ("summary.json","manifest.json","efficiency-ledger.json","runtime-flags.json"))

def loss_overview(root,cases,receipts):
 byid={r["case"]["id"]:r for r in receipts}
 out={"schema":"wbd-crypto-packet-all-legs-loss/v1","quality_gates_unchanged":True,
      "missing_is_never_zero":True,"leg_details":[],"aggregate_by_label_direction_medium_phase":{},
      "not_measurable":[]}
 total=out["aggregate_by_label_direction_medium_phase"]
 for c in cases:
  r=byid.get(c["id"],{})
  d=j(root/c["id"],"summary.json")
  clean=bool(d.get("direction") and
    (r.get("classification")=="VALID_OBSERVATION" or may_observe_next("q2observe300",r)))
  leg={"id":c["id"],"label":c["label"],"classification":r.get("classification","NOT_RUN"),
       "issues":r.get("issues",[]),"phase_counts":{}}
  vals=[]
  if clean:
   for direction in ("c2s","s2c"):
    phases=d["direction"].get(direction,{}).get("phase_delivery_5205",{})
    for med in ("udp","probe"):
     for stage in ("pre","stress","post"):
      z=phases.get(med,{}).get(stage,{})
      if (not all(isinstance(z.get(k),int) and z[k]>=0 for k in ("sent","delivered","missing","over_3s"))
          or z["sent"]!=z["delivered"]+z["missing"]):
       clean=False;break
      vals.append((direction,med,stage,z))
     if not clean:break
    if not clean:break
  if not clean:
   leg["status"]="NOT_MEASURABLE"
   out["not_measurable"].append(c["id"])
  else:
   leg["status"]="COMPLETE_ORIGINAL_ANALYZER_COUNTS"
   for direction,med,stage,z in vals:
    key="/".join((c["label"],direction,med,stage))
    slot=total.setdefault(key,{"label":c["label"],"direction":direction,"medium":med,
        "phase":stage,"sent":0,"delivered":0,"missing":0,"over_3s":0,
        "missing_by_size":{},"legs":[]})
    for k in ("sent","delivered","missing","over_3s"):slot[k]+=z[k]
    slot["legs"].append(c["id"])
    for size,part in z.get("by_size",{}).items():
     if isinstance(part.get("missing"),int):
      slot["missing_by_size"][size]=slot["missing_by_size"].get(size,0)+part["missing"]
    leg["phase_counts"][direction+"/"+med+"/"+stage]={
      "sent":z["sent"],"missing":z["missing"],
      "loss_pct":100*z["missing"]/z["sent"] if z["sent"] else None}
  out["leg_details"].append(leg)
 full={}
 for item in total.values():
  item["loss_pct"]=100*item["missing"]/item["sent"] if item["sent"] else None
  key="/".join((item["label"],item["direction"],item["medium"]))
  slot=full.setdefault(key,{"sent":0,"delivered":0,"missing":0,"over_3s":0})
  for k in ("sent","delivered","missing","over_3s"):slot[k]+=item[k]
 for z in full.values():z["loss_pct"]=100*z["missing"]/z["sent"] if z["sent"] else None
 out["full_window_loss_by_label_direction_medium"]=full
 out["all_four_measurable"]=len(out["leg_details"])==4 and not out["not_measurable"]
 (root/"loss-overview.json").write_text(json.dumps(out,indent=2)+"\n")
 return out

def aggregate(root,phase,cases,receipts,helper):
 found={r["case"]["id"]:r for r in receipts}
 states=[{"case":c,"status":found[c["id"]]["classification"] if c["id"] in found else "NOT_RUN"} for c in cases]
 pairs=[]
 if phase in ("q1screen120","q120","l120","off120","game120","confirm300","q2historic300","q2observe300"):
  for i in range(0,len(cases),4):
   group=cases[i:i+4];v=[]
   for c in group:
    folder=root/c["id"];l=j(folder,"efficiency-ledger.json");s=j(folder,"summary.json")
    v.append({"label":c["label"],"state":found.get(c["id"],{}).get("classification","NOT_RUN"),
      "cpu_s_per_effective_GiB":l.get("cpu",{}).get("seconds_per_delivered_gib"),
      "delivered_bytes":l.get("delivery",{}).get("delivered_bytes_both_directions"),
      "delivery_and_p99":s.get("direction"),"resources":s.get("resources"),
      "netem":s.get("netem_realized")})
   good=(all(x["state"]=="VALID_OBSERVATION" for x in v)
     and all(isinstance(x["cpu_s_per_effective_GiB"],(float,int)) and x["cpu_s_per_effective_GiB"]>0 for x in v)
     and all(isinstance(x["delivered_bytes"],(float,int)) and x["delivered_bytes"]>0 for x in v))
   ac=sum(v[k]["cpu_s_per_effective_GiB"] for k in (0,3))/2 if good else None
   bc=sum(v[k]["cpu_s_per_effective_GiB"] for k in (1,2))/2 if good else None
   ab=sum(v[k]["delivered_bytes"] for k in (0,3))/2 if good else None
   bb=sum(v[k]["delivered_bytes"] for k in (1,2))/2 if good else None
   comparable=bool(good and .98<=ab/bb<=1.02)
   pairs.append({"context":group[0]["context"],"order":["A","B","B","A"],
      "comparable":comparable,"cpu_s_per_effective_GiB_A":ac,
      "cpu_s_per_effective_GiB_B":bc,"delivered_bytes_A":ab,"delivered_bytes_B":bb,
      "gain_B_vs_A":1-bc/ac if comparable else None,"legs":v})
 ok=all(x["status"]=="VALID_OBSERVATION" for x in states) and all(p["comparable"] for p in pairs)
 result={"schema":"wbd-crypto-packet-abba-result/v1","phase":phase,
    "old_source":A,"new_source":B,"helper_sha":helper,"one_job_serial":True,
    "states":states,"pairs":pairs,"not_run":sum(x["status"]=="NOT_RUN" for x in states),
    "classification":"SCOPED_OBSERVATION_NOT_PHYSICAL" if ok else "FAIL_OR_NOT_RUN",
    "physical":"NOT_RUN"}
 if phase=="q2observe300":
  observed=loss_overview(root,cases,receipts)
  result["diagnostic_loss_report"]="loss-overview.json"
  result["diagnostic_all_legs_measurable"]=observed["all_four_measurable"]
  result["original_quality_fail_legs"]=sum(x["status"]=="FAIL" for x in states)
  result["original_hard_gates_unchanged"]=True
 (root/"ab-manifest.json").write_text(json.dumps(result,indent=2)+"\n")
 return ok

def main():
 p=argparse.ArgumentParser()
 p.add_argument("--root",required=True)
 p.add_argument("--mode",choices=("verify","execute"),required=True)
 x=p.parse_args();phase=conf()["phase"]
 if os.environ.get("GITHUB_ACTIONS")=="true":
  if os.environ.get("GITHUB_REF_NAME")!="next/crypto-packet-efficiency-20261010" or os.environ.get("GITHUB_EVENT_NAME")!="push":
   raise ValueError("only exact config push in GitHub Actions allowed")
 if x.mode=="verify":
  print("CRYPTO_PACKET_ABBA_EXACT_PLAN",phase,A,B);return
 root=Path(x.root).resolve();root.mkdir(parents=True,exist_ok=True)
 helper=subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip()
 cases=plan(phase)
 if phase=="preflight":
  for c in (cases[0],cases[1],cases[3],cases[4]):
   d=prepared(c,root)
   content=(d/"generated.sh").read_text()
   for word in ("WBD_FEC_DURATION_S","WBD_FEC_DELAY_MS","next-crypto-packet-abba.yml","--simd-ab"):
    if word not in content:raise ValueError("missing generated shell marker "+word)
  # Validate the new 300s historical Q2 generator with bash -n, no product run.
  history=prepared(dict(plan("q2historic300")[0],id="history-preflight"),root)
  script=(history/"generated.sh").read_text()
  for word in ("--simd-ab","WBD_FEC_DURATION_S","WBD_FEC_DELAY_MS"):
   if word not in script:raise ValueError("historical static script missing "+word)
  observe=prepared(dict(plan("q2observe300")[0],id="observe-preflight"),root)
  if "--simd-ab" not in (observe/"generated.sh").read_text():raise ValueError("observe script missing")
  print("CRYPTO_PACKET_PREFLIGHT_STATIC_ONLY");return
 for label in ("A","B"):
  for role in ("client","server"):
   if not (root/"binaries"/label/("wbd-"+role)).is_file():raise ValueError("binary not frozen")
 with (root/"serial.lock").open("w") as lock:
  fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
  rows=[]
  try:
   for c in cases:
    print("ABBA_LEG_START",c["id"],c["context"],c["label"],flush=True)
    try:r=run_case(c,root,helper)
    except Exception as e:
     import traceback
     frames=traceback.extract_tb(e.__traceback__)[-7:]
     # Frame name/line + exception type only; never stdout, raw argv, secrets.
     loc=[{"file":Path(f.filename).name,"line":f.lineno,"function":f.name}
          for f in frames]
     r={"case":c,"classification":"INFRA_INVALID","issues":["LEG_EXCEPTION"],
        "exception_class":type(e).__name__,"exception_frames":loc}
     d=root/c["id"];d.mkdir(parents=True,exist_ok=True)
     (d/"case-receipt.json").write_text(json.dumps(r,indent=2)+"\n")
    rows.append(r)
    print("ABBA_LEG_END",c["id"],r["classification"],r.get("issues",[])[:6],flush=True)
    if r["classification"]!="VALID_OBSERVATION":
     if not may_observe_next(phase,r):break
     print("OBSERVE_NEXT_KEEPING_ORIGINAL_FAIL",c["id"],flush=True)
    time.sleep(3)
  finally:
   result=aggregate(root,phase,cases,rows,helper)
 if not result:raise SystemExit("CRYPTO_PACKET_ABBA_FAIL_OR_NOT_RUN")
 print("CRYPTO_PACKET_ABBA_SCOPED_VALID_NOT_PHYSICAL",flush=True)
if __name__=="__main__":main()
