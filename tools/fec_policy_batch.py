#!/usr/bin/env python3
"""FEC OFF/20:20 exact single-job sequential experiment; no parallel load."""
import argparse, datetime, fcntl, hashlib, json, os, re, signal, subprocess, sys, time
from pathlib import Path

SOURCE="a2db258b436a41fdee98c6c53abec9bab6ce600f"
BRANCH="experiment/fec-policy-sequential-20261009"
ENV_NAMES="GITHUB_WORKSPACE WBD_STRICT_MODE WBD_STRICT_SCENARIO WBD_STRICT_SEED WBD_STRICT_RATE_MBPS WBD_STRICT_LANES WBD_STRICT_TC WBD_STRICT_FEC_PARITY WBD_STRICT_FEC_SCREEN WBD_STRICT_CPU_PROFILE WBD_STRICT_STATEFUL_GATE WBD_STRICT_WAN_NEIGHBORS WBD_LARGE_WORKLOAD WBD_LARGE_LOSS WBD_EFF_SIZE_PROFILE WBD_EFF_DIAGNOSTIC WBD_FEC_CASE_SFX WBD_FEC_DURATION_S WBD_FEC_DELAY_MS".split()
def filehash(p):
    h=hashlib.sha256()
    with open(p,"rb") as f:
        for part in iter(lambda:f.read(1048576),b""):h.update(part)
    return h.hexdigest()
def cases():
    result=[]
    for w in ("udp","tcp","mixed"):
        for loss in (0,1):
            pair=len(result)//2
            for order,fec in enumerate(("off","on") if pair%2==0 else ("on","off")):
                result.append(dict(id="s%02d"%(len(result)+1),pair=pair,order=order,
                   workload=w,loss=loss,fec=fec,parity=0 if fec=="off" else 20,
                   seed=1909+pair,rate_mbps=10,lanes=1,mode="normal",
                   duration_s=120,drain_s=3,delay_ms=15))
    return result
def validate(d):
    if set(d)!={"schema","phase","batch","source_sha","nonce"}:raise ValueError("batch schema keys")
    if d["schema"]!="wbd-fec-policy-batch/v1" or d["batch"]!="A" or d["phase"] not in ("preflight","batch_a") or d["source_sha"]!=SOURCE or type(d["nonce"]) is not int or d["nonce"]<1:raise ValueError("batch plan not authorized")
    out=cases()
    assert len(out)==12 and len({v["id"] for v in out})==12
    for i in range(0,12,2):
        a,b=out[i:i+2]
        assert all(a[k]==b[k] for k in ("workload","loss","seed","rate_mbps","duration_s","delay_ms"))
        assert {a["fec"],b["fec"]}=={"off","on"}
        assert [a["fec"],b["fec"]]==(["off","on"] if i//2%2==0 else ["on","off"])
    return out
def snapshot():
    def read(p):
        try:return Path(p).read_text()[:1500]
        except OSError:return "UNKNOWN"
    return {"utc":datetime.datetime.now(datetime.timezone.utc).isoformat(),
       "monotonic_ns":time.monotonic_ns(),"nproc":os.cpu_count(),
       "proc_cpu":read("/proc/stat").splitlines()[0],
       "cpu_max":read("/sys/fs/cgroup/cpu.max"),
       "cpuset":read("/sys/fs/cgroup/cpuset.cpus.effective"),
       "cpu_psi":read("/proc/pressure/cpu"),"memory_psi":read("/proc/pressure/memory")}
def make(case,root):
    d=root/case["id"];d.mkdir(parents=True,exist_ok=False)
    if (root/"binaries").exists():
        for role in ("client","server"):
            (d/("wbd-"+role)).symlink_to(root/"binaries"/("wbd-"+role))
    cmd=[sys.executable,"tools/prepare_large_mtu_harness.py","--fec-experiment",
         "--output",str(d/"generated.sh"),"--workload",case["workload"],
         "--loss",str(case["loss"]),"--duration-s",str(case["duration_s"]),
         "--delay-ms",str(case["delay_ms"])]
    subprocess.run(cmd,check=True,env={**os.environ,"WBD_EFF_DIAGNOSTIC":"0"})
    subprocess.run(["bash","-n",str(d/"generated.sh")],check=True)
    return d
def preflight(out,root):
    for case in (out[0],out[3]):
        d=make(case,root);script=(d/"generated.sh").read_text()
        for word in ("WBD_FEC_DURATION_S","WBD_FEC_DELAY_MS","WBD_FEC_CASE_SFX",
                     "runtime-flags.json","WBD_STRICT_FEC_PARITY","large_mtu_loss_stage.py"):
            if word not in script:raise ValueError("generated script missing "+word)
        (d/"preflight.json").write_text(json.dumps({"case":case,"generated_sha256":filehash(d/"generated.sh"),"static_only":True},indent=2))
    print("FEC_STATIC_PREFLIGHT_PASS; REALPATH_NOT_RUN",flush=True)
def owned(d,suffix):
    ps=subprocess.run(["ps","-eo","pid=,args="],check=True,text=True,capture_output=True).stdout
    pids=[]
    for line in ps.splitlines():
        match=re.match(r"\s*(\d+)\s+(.*)",line)
        if match and str(d)+"/" in match[2] and "fec_policy_batch.py" not in match[2]:
            pids.append(int(match[1]))
    for pid in pids:
        try:os.kill(pid,signal.SIGTERM)
        except ProcessLookupError:pass
    if pids:time.sleep(2)
    ns=subprocess.run(["sudo","ip","netns","list"],check=True,text=True,capture_output=True).stdout
    leaked_ns=[n+"-"+suffix for n in ("wbiz","wcli","wrtr","wsrv","wtgt") if re.search(r"(?m)^"+re.escape(n+"-"+suffix)+r"(?:\s|$)",ns)]
    return {"original_leaked_pids":pids,"leftover_namespaces":leaked_ns,"clean":not pids and not leaked_ns}
def one(case,root,helper):
    d=make(case,root)
    env=os.environ.copy()
    env.update({"GITHUB_WORKSPACE":str(Path.cwd()),"WBD_STRICT_MODE":"normal",
      "WBD_STRICT_SCENARIO":"lossless","WBD_STRICT_SEED":str(case["seed"]),
      "WBD_STRICT_RATE_MBPS":"10","WBD_STRICT_LANES":"1",
      "WBD_STRICT_FEC_PARITY":str(case["parity"]),
      "WBD_STRICT_FEC_SCREEN":"1" if case["parity"]==0 else "0",
      "WBD_STRICT_CPU_PROFILE":"0","WBD_STRICT_STATEFUL_GATE":"0",
      "WBD_STRICT_WAN_NEIGHBORS":"dynamic","WBD_LARGE_WORKLOAD":case["workload"],
      "WBD_LARGE_LOSS":str(case["loss"]),"WBD_EFF_SIZE_PROFILE":"ordinary",
      "WBD_EFF_DIAGNOSTIC":"0","WBD_FEC_CASE_SFX":case["id"],
      "WBD_FEC_DURATION_S":"120","WBD_FEC_DELAY_MS":"15"})
    before=snapshot()
    cmd=["sudo","--preserve-env="+",".join(ENV_NAMES),"env",
       "GITHUB_SHA="+SOURCE,"WBD_HARNESS_SHA="+helper,
       "WBD_STRICT_ARTIFACT_DIR="+str(d),"bash",str(d/"generated.sh")]
    rc=None;exc=None
    try:
        with (d/"run-private.log").open("w") as log:
            rc=subprocess.run(cmd,env=env,stdout=log,stderr=subprocess.STDOUT,timeout=280).returncode
    except (subprocess.TimeoutExpired,OSError) as err:exc=type(err).__name__
    cmd=[sys.executable,"tools/check_large_mtu_mixed.py","--fec-experiment",
       "--artifact-dir",str(d),"--source",SOURCE,"--helper",helper,
       "--workload",case["workload"],"--loss",str(case["loss"]),
       "--seed",str(case["seed"]),"--target-mbps","10","--size-profile","ordinary",
       "--mode","normal","--lanes","1","--diagnostic-mode","0",
       "--duration-s","120","--delay-ms","15","--fec-parity",str(case["parity"]),
       "--output",str(d/"summary.json")]
    with (d/"analyzer-private.log").open("w") as log:
        arc=subprocess.run(cmd,stdout=log,stderr=subprocess.STDOUT).returncode
    with (d/"ledger-private.log").open("w") as log:
        lrc=subprocess.run([sys.executable,"tools/efficiency_cost_ledger.py",
             "--artifact-dir",str(d),"--output",str(d/"efficiency-ledger.json")],
             stdout=log,stderr=subprocess.STDOUT).returncode
    raw=[]
    for f in d.glob("*.pcap*"):
        raw.append({"name":f.name,"bytes":f.stat().st_size,"sha256":filehash(f)})
        f.unlink()
    (d/"emergency-capture-cleanup.json").write_text(json.dumps(raw,indent=2))
    guard=owned(d,case["id"])
    try:s=json.loads((d/"summary.json").read_text())
    except (OSError,ValueError):s={}
    try:l=json.loads((d/"efficiency-ledger.json").read_text())
    except (OSError,ValueError):l={}
    classification=s.get("classification","INFRA_INVALID")
    if rc is None or exc or not guard["clean"] or not s:classification="INFRA_INVALID"
    if any("CORRUPT" in x or "HASH_MISMATCH" in x for x in s.get("issues",[])):
        classification="INTEGRITY_STOP"
    binary={k:filehash(root/"binaries"/("wbd-"+k)) for k in ("client","server")}
    hashes={f:filehash(d/f) for f in ("generated.sh","summary.json","manifest.json",
        "runtime-flags.json","efficiency-ledger.json") if (d/f).exists()}
    row={"case":case,"source_sha":SOURCE,"helper_sha":helper,"binary_sha256":binary,
      "before":before,"after":snapshot(),"sample_exit":rc,"sample_exception":exc,
      "analyzer_exit":arc,"ledger_exit":lrc,"classification":classification,
      "issues":s.get("issues",[]),"owned_cleanup":guard,"sha256":hashes}
    (d/"case-receipt.json").write_text(json.dumps(row,indent=2))
    return row
def aggregate(out,receipts,root,helper):
    rows=[];pairs=[]
    for case in out:
        r=next((x for x in receipts if x["case"]["id"]==case["id"]),None)
        rows.append({"case":case,"status":r["classification"] if r else "NOT_RUN"})
    for i in range(0,12,2):
        a,b=out[i:i+2]
        def load(case):
            try:
                p=root/case["id"]
                return json.loads((p/"summary.json").read_text()),json.loads((p/"efficiency-ledger.json").read_text())
            except (OSError,ValueError):return {},{}
        pair={a["fec"]:load(a),b["fec"]:load(b)}
        off,oc=pair["off"];on,nc=pair["on"]
        x=oc.get("cpu",{}).get("seconds_per_delivered_gib")
        y=nc.get("cpu",{}).get("seconds_per_delivered_gib")
        od=oc.get("delivery",{}).get("delivered_bytes_both_directions")
        nd=nc.get("delivery",{}).get("delivered_bytes_both_directions")
        comparable=bool(off.get("classification")=="VALID_OBSERVATION" and on.get("classification")=="VALID_OBSERVATION"
                    and x is not None and y and od and nd and .98<=od/nd<=1.02)
        pairs.append({"pair":i//2,"workload":a["workload"],"loss":a["loss"],
         "seed":a["seed"],"order":[a["fec"],b["fec"]],
         "off_cpu_s_per_delivered_gib":x,"on_cpu_s_per_delivered_gib":y,
         "off_delivered_bytes":od,"on_delivered_bytes":nd,
         "comparable":comparable,"off_cpu_gain_vs_on":(1-x/y) if comparable else None})
    (root/"batch-manifest.json").write_text(json.dumps({
      "schema":"wbd-fec-policy-batch-result/v1","source_sha":SOURCE,
      "helper_sha":helper,"one_job_sequential":True,"not_product_qualification":True,
      "rows":rows,"pairs":pairs},indent=2))
    return rows
def main():
    p=argparse.ArgumentParser()
    p.add_argument("--mode",choices=("verify","preflight","run"),required=True)
    p.add_argument("--root",required=True)
    a=p.parse_args()
    d=json.loads(Path(".github/fec-policy-batch.json").read_text())
    out=validate(d)
    if os.getenv("GITHUB_ACTIONS")=="true" and (os.getenv("GITHUB_REF_NAME")!=BRANCH or os.getenv("GITHUB_EVENT_NAME")!="push"):
        raise ValueError("only explicit branch push authorized")
    helper=subprocess.check_output(["git","rev-parse","HEAD"],text=True).strip()
    root=Path(a.root).resolve()
    if a.mode=="verify":
        print("FEC_BATCH_VERIFIED",d["phase"],len(out),SOURCE,helper);return
    root.mkdir(parents=True,exist_ok=True)
    if a.mode=="preflight":
        if d["phase"]!="preflight":raise ValueError("wrong phase")
        preflight(out,root);return
    if d["phase"]!="batch_a":raise ValueError("wrong batch phase")
    if not all((root/"binaries"/("wbd-"+k)).is_file() for k in ("client","server")):
        raise ValueError("missing frozen binary")
    lock=(root/"batch.lock").open("w")
    fcntl.flock(lock,fcntl.LOCK_EX|fcntl.LOCK_NB)
    receipts=[];abort=False
    try:
        for case in out:
            if abort:break
            print("FEC_CASE_START",case,flush=True)
            row=one(case,root,helper);receipts.append(row)
            print("FEC_CASE_END",case["id"],row["classification"],row["issues"][:8],flush=True)
            if row["classification"] in ("INTEGRITY_STOP","INFRA_INVALID"):abort=True
            if not abort:time.sleep(3)
    finally:rows=aggregate(out,receipts,root,helper)
    print("FEC_CASE_STATUSES",rows,flush=True)
    if len(receipts)!=12 or any(r["classification"]!="VALID_OBSERVATION" for r in receipts):
        raise SystemExit("batch not valid: keep FAIL/NOT_RUN, not a PASS")
if __name__=="__main__":main()
