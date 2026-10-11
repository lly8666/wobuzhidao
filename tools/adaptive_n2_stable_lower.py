#!/usr/bin/env python3
"""N2 Normal auto max20 production path single 10Mbps low-RTT load sample.

Derive the five-netns script from the pinned in-repo realpath calibration
template, changing ONLY the protected client auto policy, removing the historical
explicit asymmetric record caps in favor of real default auto MTU, and
correcting the manifest. Opt-in bounded 1s diagnostic snapshots record the
actual new-block TX profile and source-correlated controller reasoning.
The 100s low-RTT lossless condition is NOT the 0/1/5/20/5/0 waveform
or an unprofiled CPU optimization claim.
"""
import argparse
import hashlib
import json
from collections import Counter
from pathlib import Path
from check_realpath_calibration import parse_pcap, pair_delay, outer_ledger, capture_drop, pct


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def prepare(root, output, receipt):
    template = root / "scripts/realpath_calibration.sh"
    raw = template.read_text(encoding="utf-8")
    old = "--client-record-limit 1300 --mtu 1400 --fec-parity 20 --lanes 1"
    new = '--client-record-limit 0 --mtu 1400 --fec-mode auto --fec-min-parity 4 --lanes 1 --diagnostic-jsonl "$ART/client-diag.jsonl" --diagnostic-interval 1s'
    if raw.count(old) != 1:
        raise ValueError("N0 template client FEC anchor changed: fail closed")
    server_old = "--server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 1"
    server_new = '--server-record-limit 0 --mtu 1400 --fec-parity 20 --lanes 1 --diagnostic-jsonl "$ART/server-diag.jsonl" --diagnostic-interval 1s'
    if raw.count(server_old) != 1:
        raise ValueError("N2 template server record cap anchor changed")
    manifest_old = '"fec_parity": 20,'
    if raw.count(manifest_old) != 1:
        raise ValueError("N0 template manifest parity anchor changed")
    if raw.count("netem limit 200000 delay 300ms") != 2:
        raise ValueError("N2 stability low-RTT expected exactly two independent netem directions")
    if raw.count("--duration 8 --drain 10 --rate-mbps 0.5") != 2:
        raise ValueError("N2 stability low-RTT expected exactly two business roles")
    transformed = raw.replace(old, new).replace(server_old, server_new).replace(
        "netem limit 200000 delay 300ms","netem limit 200000 delay 15ms",
    ).replace(
        "--duration 8 --drain 10 --rate-mbps 0.5",
        "--duration 100 --drain 10 --rate-mbps 10 --bounded-stats",
    ).replace(
        manifest_old,
        '"fec_parity": 20,\n        "server_config_fec_parity": 20,\n'
        '        "client_record_cap_config": 0,\n'
        '        "server_record_cap_config": 0,\n'
        '        "v3_client_fec_mode": "auto",\n'
        '        "v3_client_fec_min_parity": 4,\n'
        '        "v3_client_record_cipher": "chacha20-poly1305",',
    )
    if transformed.count('"one_way_delay_ms_each_direction": 300') != 1 or transformed.count('"application_mbps_each_direction": 0.5') != 1 or transformed.count('"duration_s": 8') != 1:
        raise ValueError("N2 stability low-RTT exact workload manifest anchor changed")
    transformed=transformed.replace('"one_way_delay_ms_each_direction": 300','"one_way_delay_ms_each_direction": 15').replace('"application_mbps_each_direction": 0.5','"application_mbps_each_direction": 10.0').replace('"duration_s": 8','"duration_s": 100')
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(transformed, encoding="utf-8")
    receipt.write_text(json.dumps({
        "schema": 1,
        "source_template": str(template.relative_to(root)),
        "template_sha256": digest(template),
        "derived_script_sha256": digest(output),
        "derivation": "one explicit Normal auto (min20:4, initial20:20) / server configured fixed20:20, both record caps default auto",
        "source_sample": "one Normal auto single-client bidirectional lossless 15ms one-way UDP 10Mbps each way 100s (diagnostics ON), NOT adaptive waveform",
    }, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def controller_trace(path,server,errors):
    if not path.is_file():
        errors.append(f"missing bounded N2 diagnostic {path.name}")
        return {"samples":0,"active":0}
    try:
        snapshots=[json.loads(row) for row in path.read_text(encoding="utf-8").splitlines() if row.strip()]
    except (OSError,ValueError) as exc:
        errors.append(f"invalid N2 diagnostic {path.name}: {exc}")
        return {"samples":0,"active":0}
    active=[]
    for sample in snapshots:
        product=sample.get("product") or {}
        if server:
            if product.get("present"):
                product=product.get("tunnel") or {}
            elif product.get("tunnels"):
                product=product["tunnels"][0]
        for lane in product.get("lanes",[]):
            control=lane.get("control") or {}
            if not control.get("n2_controller_enabled"):
                continue
            parity=lane.get("actual_tx_parity")
            if parity not in (0,4,8,10,12,16,20):
                errors.append(f"{path.name} invalid actual TX parity {parity}")
            active.append({
                "unix_ns":sample.get("unix_ns"),
                "actual_tx_parity":parity,
                "decision_version":control.get("n2_decision_version",0),
                "reason":control.get("n2_decision_reason",""),
                "source_seq":control.get("n2_source_report_seq",0),
                "source_generation":control.get("n2_source_generation",0),
                "ordinary":control.get("n2_ordinary"),
                "advised_parity":control.get("n2_advised_parity"),
                "bound_width":control.get("n2_first_loss_bound_width"),
                "confidence_upper":control.get("n2_confidence_upper"),
                "accepted_records":control.get("n2_accepted_records",0),
                "committed_switches":control.get("n2_committed_switches",0),
                "busy_holds":control.get("n2_busy_holds",0),
                "pending":control.get("n2_proposal_pending",False),
                "raw_source_evidence":control.get("n2_source_evidence"),
            })
    if len(active)<3:
        errors.append(f"{path.name}: fewer than 3 actual enabled N2 controller snapshots")
    if not any(row["decision_version"]>0 for row in active):
        errors.append(f"{path.name}: no authenticated source-return control decision in 100s")
    return {"samples":len(snapshots),"active":len(active),
        "actual_profiles":sorted({x["actual_tx_parity"] for x in active if x["actual_tx_parity"] is not None}),
        "reason_counts":dict(Counter(x["reason"] for x in active)),
        "last_source_seq":max((x["source_seq"] for x in active),default=0),
        "raw_source_qualifier_counts":{
            "matched":sum(bool((x["raw_source_evidence"] or {}).get("source_matched")) for x in active),
            "repair_covered":sum(bool((x["raw_source_evidence"] or {}).get("repair_coverage")) for x in active),
            "complete_first_bound":sum(bool((x["raw_source_evidence"] or {}).get("first_attempt_bound_complete")) for x in active),
            "invalid_shape_samples":sum((x["raw_source_evidence"] or {}).get("shape_invalid",0)>0 for x in active),
            "edge_cropped_samples":sum((x["raw_source_evidence"] or {}).get("shape_cropped",0)>0 for x in active),
            "capacity_limited_samples":sum(bool((x["raw_source_evidence"] or {}).get("window_capacity_limited")) for x in active),
            "source_not_matched":sum(x["decision_version"]>0 and not (x["raw_source_evidence"] or {}).get("source_matched") for x in active),
        },
        "committed_switches":max((x["committed_switches"] for x in active),default=0),
        "busy_holds":max((x["busy_holds"] for x in active),default=0),
        "bounded_epoch_trace":active[:100],
    }


def check(art, source, output):
    errors = []
    prep = json.loads((art / "harness-prep.json").read_text())
    manifest = json.loads((art / "manifest.json").read_text())
    if manifest["source_sha"] != source or manifest["harness_sha"] != source:
        errors.append("SOURCE does not match product and harness")
    c = manifest["config"]
    for key, want in {
        "lanes": 1, "fec_parity": 20, "server_config_fec_parity": 20,
        "client_record_cap_config": 0, "server_record_cap_config": 0,
        "v3_client_fec_mode": "auto", "v3_client_fec_min_parity": 4,
        "v3_client_record_cipher": "chacha20-poly1305",
        "mtu": 1400, "one_way_delay_ms_each_direction": 15, "loss_percent": 0,
        "duration_s": 100, "application_mbps_each_direction": 10.0,
    }.items():
        if c.get(key) != want:
            errors.append(f"config {key} observed {c.get(key)} expected {want}")
    if not prep["derived_script_sha256"] or not prep["template_sha256"]:
        errors.append("missing exact derived harness hash")
    proc = (art / "process-state-after-drain.txt").read_text()
    if "client_alive=1 " not in proc or "server_alive=1 " not in proc:
        errors.append("formal binary exited before drain")
    biz = json.loads((art / "biz.json").read_text())["stats"]
    tgt = json.loads((art / "target.json").read_text())["stats"]
    payload = {}
    for name, src, dst in (("c2s", biz, tgt), ("s2c", tgt, biz)):
        sent = src["sent_packets"]
        recvd = dst["recv_unique_packets"]
        sent_bytes = src["sent_bytes"]
        recvd_bytes = dst["recv_unique_bytes"]
        payload[name] = {
            "sent": sent, "received_first_unique": recvd,
            "sent_bytes": sent_bytes, "received_first_unique_bytes": recvd_bytes,
            "send_failures": src["send_failures"],
            "recv_corrupt": dst["corrupt"],
            "duplicates": dst["recv_duplicates"],
            "send_lag_p99_ns": src["send_lag_p99_ns"],
        }
        if not sent or recvd != sent or recvd_bytes != sent_bytes:
            errors.append(f"{name}: missing/bad first unique payload")
        if src["send_failures"] != 0 or dst["corrupt"] != 0 or dst["recv_duplicates"] != 0:
            errors.append(f"{name}: failure/corrupt/duplicate")
        if src["send_lag_p99_ns"] is None:
            errors.append(f"{name}: injection health unknown")
    if not biz["probe_sent"] or biz["probe_recv"] != biz["probe_sent"]:
        errors.append("lossless RTT probes missing")
    captures = {}
    for stem in ("c2s-pre", "c2s-post", "s2c-pre", "s2c-post"):
        d = capture_drop(art / f"{stem}.tcpdump.log")
        captures[stem] = {"capture_kernel_drops": d}
        if d is None or d > 0:
            errors.append(f"{stem}: lost/unknown capture evidence")
    for direction in ("c2s", "s2c"):
        pre = parse_pcap(art / f"{direction}-pre.pcap")
        post = parse_pcap(art / f"{direction}-post.pcap")
        delays, missing, extra = pair_delay(pre, post)
        median = pct(delays, 0.50)
        captures[direction] = {
            "pre": outer_ledger(pre), "post_packets": len(post),
            "unpaired_pre": missing, "unpaired_post": extra,
            "qdisc_delay_median_ns": median,
        }
        if missing or extra:
            errors.append(f"{direction}: capture pairing mismatch")
        if median is None or not 9_000_000 <= median <= 25_000_000:
            errors.append(f"{direction}: netem not effective")
    if captures["c2s"]["pre"]["outer_syn_flows"] != 1:
        errors.append("expected one authenticated real underlay association")
    # A 300ms one-way / ~600ms RTT path can legitimately retransmit even
    # without configured packet loss if a local repair timer fires before its
    # ACK is observed. Retransmission is not by itself failed first delivery.
    # Preserve exact counts separately: these require later CPU/bandwidth
    # performance scrutiny and same-Seq same-wire directed qualification.
    repair = {}
    for direction in ("c2s", "s2c"):
        total = captures[direction]["pre"]["repair_tcp_payload_bytes"]
        fresh = captures[direction]["pre"]["fresh_tcp_payload_bytes"]
        repair[direction] = {
            "repair_payload_bytes": total,
            "fresh_payload_bytes": fresh,
            "repair_to_fresh_ratio": total / fresh if fresh else None,
        }
    # Hash and remove raw packet captures: never upload large pcaps.
    pcap_provenance = {}
    for stem in ("c2s-pre", "c2s-post", "s2c-pre", "s2c-post"):
        path = art / f"{stem}.pcap"
        pcap_provenance[stem] = {"sha256": digest(path), "bytes": path.stat().st_size}
        path.unlink()
    directional_controller={
        "c2s_client_tx":controller_trace(art/"client-diag.jsonl",False,errors),
        "s2c_server_tx":controller_trace(art/"server-diag.jsonl",True,errors),
    }
    for direction,trace in directional_controller.items():
        profiles=trace.get("actual_profiles",[])
        if trace.get("committed_switches",0)<1 or 20 not in profiles or not any(0<=int(p)<20 for p in profiles):
            errors.append(f"{direction}: NO ACTUAL NORMAL TX LOWER over qualified 100s window (not control-pass)")
        if trace.get("reason_counts",{}).get("LOWER_ONE",0)<1:
            errors.append(f"{direction}: no sampled LOWER_ONE source-feedback cause for actual profile")
    result = {
        "schema": 1, "source_sha": source,
        "result": "PASS_N2_BOTH_DIRECTIONS_REAL_TX_LOWER_LOSSLESS_ONLY" if not errors else "FAIL",
        "errors": errors,
        "scenario": "five_netns_formal_binaries_real_raw_tproxy_tun",
        "actual_admission": "V3 verified by production client/server codepath, not independent encrypted-policy packet introspection",
        "effective_fec": "only actual_tx_parity and committed counters from authenticated client/server product diagnostic are accepted as evidence, not initial/advised parity; see directional_controller",
        "limits": ["one Normal client, not simultaneous three live clients", "not Windows Wintun direct",
                   "not N2 0→1→5→20→5→0 waveform, diagnostic ON CPU not comparable to OFF, or AES/P6"],
        "config": c, "input_and_unique_delivery": payload,
        "probe": {"sent": biz["probe_sent"], "received": biz["probe_recv"],
                  "returned_only_rtt_p99_ns": biz["probe_rtt_p99_ns"]},
        "capture": captures, "outer_repair": repair,
        "repair_not_a_first_delivery_failure": "visible; no bandwidth or latency qualification from this sample",
        "pcap_redacted_hashes": pcap_provenance,
        "cpu_quota_psi_preflight": "see runner-cpu-scope.json and runner-preflight.txt; no capacity claim",
        "harness": prep,
        "diagnostic_scope": "explicit 1s diagnostic ON; source/role local TX trace, no per-packet logging or CPU comparison to OFF",
        "directional_controller": directional_controller,
        "stable_normal_actual_downshift_gate": "BOTH local-TX directions must have at least one committed lower and observable max20 then <20 on true authenticated source",

    }
    output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print("WBD_ADAPTIVE_N2_REALPATH_" + result["result"])
    if errors:
        raise SystemExit(1)


def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="cmd", required=True)
    a = sub.add_parser("prepare")
    a.add_argument("--root", type=Path, default=Path("."))
    a.add_argument("--script", required=True, type=Path)
    a.add_argument("--receipt", required=True, type=Path)
    b = sub.add_parser("check")
    b.add_argument("--artifact-dir", required=True, type=Path)
    b.add_argument("--source-sha", required=True)
    b.add_argument("--output", required=True, type=Path)
    args = p.parse_args()
    if args.cmd == "prepare":
        prepare(args.root, args.script, args.receipt)
    else:
        check(args.artifact_dir, args.source_sha, args.output)


if __name__ == "__main__":
    main()
