#!/usr/bin/env python3
"""Strict scoped N0 three-concurrent-native-client functional receipt.
No performance inference from opt-in JSONL. Raw bounded captures hashed/deleted.
"""
import argparse
import hashlib
import json
from pathlib import Path
from check_realpath_calibration import parse_pcap, pair_delay, outer_ledger, capture_drop, pct


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def read_jsonl(path):
    if not path.exists():
        raise ValueError("missing diagnostic file: "+str(path))
    return [json.loads(line)["product"] for line in path.read_text().splitlines() if line.strip()]


def check(art, source, output):
    problems=[]
    def issue(msg):
        problems.append(msg)
    manifest=json.loads((art/"manifest.json").read_text())
    prep=json.loads((art/"harness-prep.json").read_text())
    if manifest.get("source_sha")!=source or manifest.get("harness_sha")!=source:
        issue("manifest SOURCE/harness mismatch")
    if not prep.get("template_sha256") or not prep.get("derived_script_sha256"):
        issue("missing exact source-derived script hashes")
    cfg=manifest.get("config",{})
    for k,w in {"lanes":1,"fec_parity":0,"server_config_fec_parity":20,
                "lease_mode":"automatic","one_way_delay_ms_each_direction":300,
                "loss_percent":0,"duration_s":8,
                "application_mbps_each_direction":0.15}.items():
        if cfg.get(k)!=w: issue(f"config {k}: got {cfg.get(k)} want {w}")
    expected=[{"client":1,"lanes":1,"fec_parity":0},
              {"client":2,"lanes":1,"fec_parity":4},
              {"client":3,"lanes":2,"fec_parity":20}]
    if cfg.get("client_profiles")!=expected:
        issue("manifest mismatch for 3 independent profiles")
    state=(art/"process-state-after-drain.txt").read_text()
    for name in ("server_alive","client_alive","client2_alive","client3_alive"):
        if f"{name}=1" not in state: issue(f"early exit: {name}")
    payload={}
    observed={}
    leases=set()
    for i,parity,lanes,transport in [(1,0,1,1),(2,4,1,1),(3,20,2,2)]:
        bp="biz.json" if i==1 else f"biz{i}.json"
        tp="target.json" if i==1 else f"target{i}.json"
        b=json.loads((art/bp).read_text())["stats"]
        t=json.loads((art/tp).read_text())["stats"]
        directions={}
        for key,tx,rx in (("c2s",b,t),("s2c",t,b)):
            actual={"sent":tx["sent_packets"],"received_first_unique":rx["recv_unique_packets"],
                    "sent_bytes":tx["sent_bytes"],"received_first_unique_bytes":rx["recv_unique_bytes"],
                    "send_failures":tx["send_failures"],"corrupt":rx["corrupt"],
                    "duplicates":rx["recv_duplicates"],"send_lag_p99_ns":tx["send_lag_p99_ns"]}
            directions[key]=actual
            if not actual["sent"] or actual["sent"]!=actual["received_first_unique"] or actual["sent_bytes"]!=actual["received_first_unique_bytes"]:
                issue(f"client{i} {key}: missing first unique bytes")
            if actual["send_failures"] or actual["corrupt"] or actual["duplicates"] or actual["send_lag_p99_ns"] is None:
                issue(f"client{i} {key}: injection/integrity failure")
        if b["probe_sent"] < 1 or b["probe_recv"]!=b["probe_sent"]:
            issue(f"client{i}: zero-loss probe path failed")
        payload[f"client{i}"]={"directions":directions,
                                 "probes":{k:b[k] for k in ("probe_sent","probe_recv","probe_rtt_p99_ns")}}
        samples=read_jsonl(art/f"client{i}-diag.jsonl")
        candidates=[p for p in samples if p.get("record_version")==3 and p.get("admission_policy")
                    and len(p.get("lanes",[]))==lanes]
        if not candidates:
            issue(f"client{i}: no observed authenticated V3 lanes")
            continue
        o=candidates[-1]
        policy=o["admission_policy"]
        expect={"Schema":1,"Transport":transport,"DesiredLanes":lanes,
                "FECMode":0 if parity==0 else 1,"FixedParity":parity,"Cipher":1,"Quality":0}
        for field,want in expect.items():
            if policy.get(field)!=want: issue(f"client{i}: observed policy {field}={policy.get(field)} wanted {want}")
        if any(v.get("parity_shards")!=parity for v in o["lanes"]):
            issue(f"client{i}: actual client lane FEC parity mismatch")
        lease=o.get("lease4")
        if not lease or lease=="0.0.0.0/32" or lease in leases:
            issue(f"client{i}: missing/reused independent automatic lease")
        leases.add(lease)
        if not o.get("tls_cipher_suite"):
            issue(f"client{i}: actual TLS negotiated suite missing")
        observed[i]={"lease4":lease,"tunnel_id":o.get("tunnel_id"),
                     "record_version":o["record_version"],"admission_policy":policy,
                     "tls_cipher_suite":o.get("tls_cipher_suite"),"active_lanes":len(o["lanes"]),
                     "lane_parities":[x["parity_shards"] for x in o["lanes"]]}
    server_samples=read_jsonl(art/"server-diag.jsonl")
    server_multi=[p["tunnels"] for p in server_samples if isinstance(p.get("tunnels"),list) and len(p["tunnels"])>=3]
    if not server_multi: issue("server: no three simultaneous authenticated tunnels on single port")
    else:
        snapshots=server_multi[-1]
        for i,o in observed.items():
            matches=[v for v in snapshots if v.get("lease4")==o["lease4"]]
            if len(matches)!=1:
                issue(f"client{i}: shared server missing/duplicate matching lease")
                continue
            remote=matches[0]
            for k in ("record_version","admission_policy","tls_cipher_suite","tunnel_id"):
                if remote.get(k)!=o[k]: issue(f"client{i}: actual server/client {k} mismatch")
            if len(remote.get("lanes",[]))!=o["active_lanes"]:
                issue(f"client{i}: server active lanes mismatch")
            if [v["parity_shards"] for v in remote.get("lanes",[])]!=o["lane_parities"]:
                issue(f"client{i}: server effective parity mismatch")
    capture={}
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post","c2-underlay","c3-underlay"):
        path=art/(stem+".pcap")
        d=capture_drop(art/(stem+".tcpdump.log"))
        capture[stem]={"sha256":sha(path),"bytes":path.stat().st_size,"kernel_drops":d}
        if d is None or d!=0: issue(f"{stem}: capture drop/unknown")
    for direction in ("c2s","s2c"):
        pre=parse_pcap(art/(direction+"-pre.pcap"))
        post=parse_pcap(art/(direction+"-post.pcap"))
        delays,missing,extra=pair_delay(pre,post)
        median=pct(delays,0.5)
        capture[direction]={"outer_before":outer_ledger(pre),"unpaired_before":missing,
                            "unpaired_after":extra,"qdisc_median_ns":median}
        if missing or extra or median is None or not 260_000_000<=median<=500_000_000:
            issue(f"first client {direction}: missing/delay qdisc invalid")
    for i in (2,3):
        ledger=outer_ledger(parse_pcap(art/f"c{i}-underlay.pcap"))
        capture[f"c{i}-underlay"]["ledger"]=ledger
        if ledger.get("outer_syn_flows",0)<1:
            issue(f"client{i}: no independent real FakeTCP SYN")
        q=json.loads((art/f"qdisc-rcli{i}-after.json").read_text())
        if not any(x.get("kind")=="netem" for x in q):
            issue(f"client{i}: missing real netem qdisc")
    for stem in ("c2s-pre","c2s-post","s2c-pre","s2c-post","c2-underlay","c3-underlay"):
        (art/(stem+".pcap")).unlink()
    result={"schema":1,"source_sha":source,"status":"PASS_N0_3_NATIVE_CLIENTS_SCOPED" if not problems else "FAIL",
            "problems":problems,"manifest":manifest,"prep":prep,"payload":payload,
            "observed_effective_v3":observed,"capture_hashed_deleted":capture,
            "limitations":["no exact first S2C before any business in native case; covered by separate in-memory test",
                           "no deep same-Seq same-wire full ciphertext pcap qualification",
                           "not performance, AES, auto, Windows Wintun, P6 or physical"]}
    output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("WBD_ADAPTIVE_N0_MULTI_"+result["status"])
    if problems: raise SystemExit(1)


def main():
    a=argparse.ArgumentParser()
    a.add_argument("--artifact-dir",type=Path,required=True)
    a.add_argument("--source-sha",required=True)
    a.add_argument("--output",type=Path,required=True)
    args=a.parse_args()
    check(args.artifact_dir,args.source_sha,args.output)


if __name__=="__main__":
    main()
