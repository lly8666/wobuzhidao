#!/usr/bin/env python3
"""One explicit FEC profile sample, with the original gates retained as reference.

This is a screen, not final18 or an exact finite-policy reconstruction oracle.
No measurement workload is started here.
"""
import argparse
import json
import math
import subprocess
import sys
from pathlib import Path

import check_strict_weaknet_loss_tolerant_v1 as legacy

PROFILES = (0, 4, 8, 10, 12, 16, 20)


def source_loss_reference(k, parity, p):
    """IID timely source erasures, excluding finite policy, repair and Game."""
    if not 1 <= k <= 20 or parity not in PROFILES or not 0 <= p <= 1:
        raise ValueError("invalid reference geometry/probability")
    r = min(k, parity)
    n = k + r
    return p * sum(math.comb(n - 1, lost) * p**lost * (1 - p)**(n - 1 - lost)
                   for lost in range(r, n))


def validate_profile(manifest, diagnostics, parity, source):
    errors = []
    config = manifest.get("config") or {}
    name = "off" if parity == 0 else "20:" + str(parity)
    if (manifest.get("source_sha") != source or config.get("fec") != name
            or config.get("fec_parity") != parity or config.get("profile_screen") is not True):
        errors.append("manifest explicit source/profile/screen identity mismatch")
    observations = {}
    for side in ("client", "server"):
        count = 0
        for sample in diagnostics[side]:
            product = legacy.product(sample, side)
            for lane in (product or {}).get("lanes") or []:
                count += 1
                if lane.get("parity_shards") != parity:
                    errors.append(side + " negotiated parity mismatch")
                for path in ("TxPath", "RxPath"):
                    state = (lane.get("lane") or {}).get(path) or {}
                    if (state.get("FECEnabled") is not (parity != 0)
                            or state.get("ParityShards") != parity):
                        errors.append(side + "/" + path + " effective profile mismatch")
        observations[side] = count
        if not count:
            errors.append(side + " has no effective lane observations")
    return sorted(set(errors)), observations


def longest_zero_delivery(stats):
    wall = stats.get("recv_wall_bytes_by_second")
    if not isinstance(wall, list) or len(wall) < 120:
        return None
    peak = run = 0
    # Exclude propagation fill and final bucket; offered traffic is continuous.
    for value in wall[1:119]:
        run = run + 1 if value == 0 else 0
        peak = max(peak, run)
    return peak


def encoder_observations(diagnostics):
    out = {}
    for side, samples in diagnostics.items():
        refs = {}
        for sample in samples:
            for lane in (legacy.product(sample, side) or {}).get("lanes") or []:
                enc = ((lane.get("lane") or {}).get("TxPath") or {}).get("Encoder") or {}
                record = refs.setdefault(legacy.lane_key(lane), {})
                for key in ("source_shards", "parity_shards", "source_bytes", "parity_bytes",
                            "full_blocks", "partial_blocks"):
                    record[key] = max(record.get(key, 0), enc.get(key, 0))
        out[side] = {"refs": refs, "totals": {
            key: sum(row.get(key, 0) for row in refs.values())
            for key in ("source_shards", "parity_shards", "source_bytes", "parity_bytes",
                        "full_blocks", "partial_blocks")}}
    return out


def main():
    ap = argparse.ArgumentParser()
    for name in ("artifact-dir", "source-sha", "mode", "scenario", "output"):
        ap.add_argument("--" + name, required=True)
    ap.add_argument("--fec-parity", type=int, choices=PROFILES, required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--target-mbps", type=float, required=True)
    ap.add_argument("--lanes", type=int, required=True)
    args = ap.parse_args()
    root = Path(args.artifact_dir)
    reference = root / "profile-original-gate-reference.json"
    command = [sys.executable, str(Path(legacy.__file__)), "--artifact-dir", str(root),
               "--source-sha", args.source_sha, "--mode", args.mode,
               "--scenario", args.scenario, "--seed", str(args.seed),
               "--target-mbps", str(args.target_mbps), "--lanes", str(args.lanes),
               "--output", str(reference)]
    proc = subprocess.run(command, check=False)
    if proc.returncode not in (0, 1) or not reference.exists():
        raise SystemExit("original analyzer failed before producing a complete receipt")
    result = json.loads(reference.read_text())
    # This artifact can never be silently consumed by the formal20:20 final18.
    result["reference_analysis_version"] = result["analysis_version"]
    result["analysis_version"] = "fec-profile-original-gates-reference-only"
    reference.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    manifest = json.loads((root / "manifest.json").read_text())
    diagnostics = {side: legacy.read_jsonl(root / (side + "-diag.jsonl"))
                   for side in ("client", "server")}
    profile_errors, observations = validate_profile(manifest, diagnostics,
                                                    args.fec_parity, args.source_sha)
    continuity = {}
    for role in ("biz", "target"):
        stats = json.loads((root / (role + ".json")).read_text())["stats"]
        continuity[role] = longest_zero_delivery(stats)
        if continuity[role] is None:
            profile_errors.append(role + " missing full wall delivery coverage")
        elif continuity[role] >= 2:
            profile_errors.append(role + " at least2 complete seconds with no business delivery")
    probabilities = {"lossless": (0, 0, 0), "5205": (.05, .20, .05),
                     "5305": (.05, .30, .05)}[args.scenario]
    models = {}
    for stage, p in zip(("pre", "stress", "post"), probabilities):
        values = [source_loss_reference(k, args.fec_parity, p) for k in range(1, 21)]
        models[stage] = {"iid_shard_loss_percent": p * 100,
                         "full_block_missing_source_percent": values[-1] * 100,
                         "possible_k_source_loss_percent_min": min(values) * 100,
                         "possible_k_source_loss_percent_max": max(values) * 100}
    result.update(analysis_version="fec-profile-screen-v1", fec_parity=args.fec_parity,
                  profile_effective_observations=observations, profile_errors=profile_errors,
                  encoder_observations=encoder_observations(diagnostics),
                  longest_zero_delivery_seconds=continuity, theoretical_reference=models,
                  reference_limitations="IID timely source-shard model; not business-datagram loss, "
                  "finite3s/pressure retirement oracle, Game independent q^lanes guarantee or exact "
                  "weighted partial distribution. Returned-probe RTT is conditional. One sample "
                  "per identity; not final18/full70/soak or native capacity proof.",
                  screen_status="PASS_SCREEN" if not profile_errors and all(
                      v == "PASS" for v in result["classifications"].values()) else "REVIEW_OR_FAIL")
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    print("WBD_FEC_PROFILE_SCREEN " + json.dumps({
        "fec_parity": args.fec_parity, "status": result["screen_status"],
        "classifications": result["classifications"], "profile_errors": profile_errors}))
    # Preserve all original failures. Expert review can explain an exceeded
    # loss reference, but must not rewrite this run's original result as PASS.
    raise SystemExit(0 if result["screen_status"] == "PASS_SCREEN" else 1)


if __name__ == "__main__":
    main()
