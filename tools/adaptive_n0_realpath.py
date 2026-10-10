#!/usr/bin/env python3
"""N0 fixed V3 production path single functional sample.

Derive the five-netns script from the pinned in-repo realpath calibration
template, changing ONLY the client fixed FEC request and correcting the
manifest. This is a functional test, not a multi-client, AES, Windows, or
performance qualification; do not reuse the historical 20:20 analyzer.
"""
import argparse
import hashlib
import json
from pathlib import Path
from check_realpath_calibration import parse_pcap, pair_delay, outer_ledger, capture_drop, pct


def digest(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()


def prepare(root, output, receipt):
    template = root / "scripts/realpath_calibration.sh"
    raw = template.read_text(encoding="utf-8")
    old = "--client-record-limit 1300 --mtu 1400 --fec-parity 20 --lanes 1"
    new = "--client-record-limit 1300 --mtu 1400 --fec-parity 4 --lanes 1"
    if raw.count(old) != 1:
        raise ValueError("N0 template client FEC anchor changed: fail closed")
    if raw.count("--server-record-limit 1250 --mtu 1400 --fec-parity 20 --lanes 1") != 1:
        raise ValueError("N0 template server fixed parity anchor changed")
    manifest_old = '"fec_parity": 20,'
    if raw.count(manifest_old) != 1:
        raise ValueError("N0 template manifest parity anchor changed")
    transformed = raw.replace(old, new).replace(
        manifest_old,
        '"fec_parity": 4,\n        "server_config_fec_parity": 20,\n'
        '        "v3_client_fec_mode": "fixed",\n'
        '        "v3_client_record_cipher": "chacha20-poly1305",',
    )
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(transformed, encoding="utf-8")
    receipt.write_text(json.dumps({
        "schema": 1,
        "source_template": str(template.relative_to(root)),
        "template_sha256": digest(template),
        "derived_script_sha256": digest(output),
        "derivation": "one 20:4 client / server configured 20:20, V3 production default fixed",
        "source_sample": "single lossless 300ms each way 0.5Mbps UDP 8s",
    }, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def check(art, source, output):
    errors = []
    prep = json.loads((art / "harness-prep.json").read_text())
    manifest = json.loads((art / "manifest.json").read_text())
    if manifest["source_sha"] != source or manifest["harness_sha"] != source:
        errors.append("SOURCE does not match product and harness")
    c = manifest["config"]
    for key, want in {
        "lanes": 1, "fec_parity": 4, "server_config_fec_parity": 20,
        "v3_client_fec_mode": "fixed", "v3_client_record_cipher": "chacha20-poly1305",
        "mtu": 1400, "one_way_delay_ms_each_direction": 300, "loss_percent": 0,
        "duration_s": 8,
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
        if median is None or not 280_000_000 <= median <= 340_000_000:
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
    result = {
        "schema": 1, "source_sha": source,
        "result": "PASS_N0_SINGLE_REALPATH_FIXED_ONLY" if not errors else "FAIL",
        "errors": errors,
        "scenario": "five_netns_formal_binaries_real_raw_tproxy_tun",
        "actual_admission": "V3 verified by production client/server codepath, not independent encrypted-policy packet introspection",
        "effective_fec": "inferred 20:4 at both lane endpoints from first-bidirectional authenticated payload with server configured 20:20; independent runtime snapshot not collected",
        "limits": ["one client, not simultaneous three live clients", "not Windows Wintun direct",
                   "not performance, stress, AES, auto, or P6"],
        "config": c, "input_and_unique_delivery": payload,
        "probe": {"sent": biz["probe_sent"], "received": biz["probe_recv"],
                  "returned_only_rtt_p99_ns": biz["probe_rtt_p99_ns"]},
        "capture": captures, "outer_repair": repair,
        "repair_not_a_first_delivery_failure": "visible; no bandwidth or latency qualification from this sample",
        "pcap_redacted_hashes": pcap_provenance,
        "cpu_quota_psi_preflight": "see runner-cpu-scope.json and runner-preflight.txt; no capacity claim",
        "harness": prep,
    }
    output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print("WBD_ADAPTIVE_N0_REALPATH_" + result["result"])
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
