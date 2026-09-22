#!/usr/bin/env python3
import argparse
import json
import os
import subprocess
from pathlib import Path


def run(cmd):
    return subprocess.run(cmd, text=True, capture_output=True, check=True)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--tc-bin", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()
    tc = str(Path(args.tc_bin).resolve())
    if not Path(tc).is_file():
        raise SystemExit(f"missing tc binary: {tc}")

    suffix = str(os.getpid())
    left = ("wsp" + suffix)[-15:]
    right = ("wsq" + suffix)[-15:]
    result = {"schema": 1, "result": "FAIL", "tc_bin": tc}
    try:
        run(["ip", "link", "add", left, "type", "veth", "peer", "name", right])
        run(["ip", "link", "set", left, "up"])
        run(["ip", "link", "set", right, "up"])
        receipts = {}
        for seed in (123456, 654321):
            run([
                tc, "qdisc", "replace", "dev", left, "root", "netem",
                "limit", "200000", "delay", "300ms",
                "loss", "random", "20%", "seed", str(seed),
            ])
            shown = run([tc, "-s", "-j", "qdisc", "show", "dev", left])
            receipts[str(seed)] = json.loads(shown.stdout)
        result = {
            "schema": 1, "result": "PASS", "tc_bin": tc,
            "tc_version": run([tc, "-V"]).stdout.strip(),
            "kernel": run(["uname", "-a"]).stdout.strip(),
            "seeded_netem": receipts,
        }
    except subprocess.CalledProcessError as exc:
        result.update({
            "command": exc.cmd, "returncode": exc.returncode,
            "stdout": exc.stdout, "stderr": exc.stderr,
        })
    finally:
        subprocess.run(["ip", "link", "del", left], check=False,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_STRICT_NETEM_SEED_PREFLIGHT " + json.dumps(result, sort_keys=True))
    raise SystemExit(0 if result["result"] == "PASS" else 1)


if __name__ == "__main__":
    main()
