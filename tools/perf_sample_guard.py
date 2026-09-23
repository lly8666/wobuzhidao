#!/usr/bin/env python3
import argparse
import json
import os
from pathlib import Path

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("command", choices=("claim",))
    parser.add_argument("--identity", required=True)
    parser.add_argument("--receipt", required=True)
    args = parser.parse_args()

    root = Path(os.environ.get("RUNNER_TEMP") or os.environ.get("GITHUB_WORKSPACE") or ".")
    claim = root / "wbd-performance-sample.claim"
    payload = {
        "schema": 1,
        "github_run_id": os.environ.get("GITHUB_RUN_ID", ""),
        "github_run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT", ""),
        "source_sha": os.environ.get("GITHUB_SHA", ""),
        "identity": args.identity,
    }
    try:
        fd = os.open(claim, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    except FileExistsError:
        raise SystemExit(
            "WBD_PERF_SECOND_SAMPLE_REJECTED existing="
            + claim.read_text(encoding="utf-8", errors="replace").strip()
            + " attempted="
            + json.dumps(payload, sort_keys=True)
        )
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(json.dumps(payload, sort_keys=True) + "\n")
    receipt = Path(args.receipt)
    receipt.parent.mkdir(parents=True, exist_ok=True)
    receipt.write_text(json.dumps(payload, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_PERF_SINGLE_SAMPLE_CLAIM " + json.dumps(payload, sort_keys=True))

if __name__ == "__main__":
    main()
