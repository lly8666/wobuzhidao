#!/usr/bin/env python3
"""One constant two-direction seeded netem impairment over exactly 300s."""
import argparse
import json
import os
import time
from pathlib import Path
from strict_weaknet_stage import change, emit, wait_until

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--namespace", required=True)
    ap.add_argument("--tc-bin", required=True)
    ap.add_argument("--c2s-dev", required=True)
    ap.add_argument("--s2c-dev", required=True)
    ap.add_argument("--start-ns", type=int, required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--output", required=True)
    # The fixed-profile wrapper replaces the legacy three-stage arguments.
    ap.add_argument("--loss-percent", type=int, required=True)
    args = ap.parse_args()
    if args.loss_percent not in (0, 5, 20, 30):
        ap.error("unsupported fixed loss")
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    with open(args.output, "w", encoding="utf-8", buffering=1) as stream:
        try:
            wait_until(args.start_ns)
            change(args.namespace, args.tc_bin, args.c2s_dev,
                   args.loss_percent, args.seed * 100 + 1)
            change(args.namespace, args.tc_bin, args.s2c_dev,
                   args.loss_percent, args.seed * 100 + 2)
            emit(stream, "stress_start", args.namespace, args.tc_bin,
                 args.c2s_dev, args.s2c_dev, args.loss_percent,
                 args.loss_percent)
            wait_until(args.start_ns + 300_000_000_000)
            emit(stream, "stress_end", args.namespace, args.tc_bin,
                 args.c2s_dev, args.s2c_dev, args.loss_percent,
                 args.loss_percent)
            change(args.namespace, args.tc_bin, args.c2s_dev, 0,
                   args.seed * 100 + 91)
            change(args.namespace, args.tc_bin, args.s2c_dev, 0,
                   args.seed * 100 + 92)
            emit(stream, "drain_start", args.namespace, args.tc_bin,
                 args.c2s_dev, args.s2c_dev, 0, 0)
        except BaseException as exc:
            stream.write(json.dumps({"event":"stage_error",
                        "monotonic_ns":time.monotonic_ns(), "error":str(exc)})+"\n")
            raise

if __name__ == "__main__":
    main()
