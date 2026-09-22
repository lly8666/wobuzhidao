#!/usr/bin/env python3
import argparse
import json
import subprocess
import time
from pathlib import Path


def wait_until(target_ns):
    while True:
        now = time.monotonic_ns()
        remaining = target_ns - now
        if remaining <= 0:
            return
        if remaining > 2_000_000:
            time.sleep((remaining - 1_000_000) / 1e9)


def tc_args(ns, tc_bin, dev, loss, seed):
    cmd = ["ip", "netns", "exec", ns, tc_bin, "qdisc", "change", "dev", dev,
           "root", "netem", "limit", "200000", "delay", "300ms"]
    if loss > 0:
        cmd += ["loss", "random", f"{loss:g}%", "seed", str(seed)]
    return cmd


def qdisc(ns, tc_bin, dev):
    p = subprocess.run(
        ["ip", "netns", "exec", ns, tc_bin, "-s", "-j", "qdisc", "show", "dev", dev],
        check=True, text=True, capture_output=True,
    )
    return json.loads(p.stdout)


def change(ns, tc_bin, dev, loss, seed):
    cmd = tc_args(ns, tc_bin, dev, loss, seed)
    p = subprocess.run(cmd, check=False, text=True, capture_output=True)
    if p.returncode != 0:
        raise RuntimeError(
            f"netem change failed rc={p.returncode} cmd={cmd!r} "
            f"stdout={p.stdout!r} stderr={p.stderr!r}"
        )


def emit(f, event, ns, tc_bin, cdev, sdev, c_loss, s_loss):
    row = {
        "schema": 1,
        "event": event,
        "monotonic_ns": time.monotonic_ns(),
        "unix_ns": time.time_ns(),
        "loss_percent": {"c2s": c_loss, "s2c": s_loss},
        "tc_bin": tc_bin,
        "qdisc": {"c2s": qdisc(ns, tc_bin, cdev), "s2c": qdisc(ns, tc_bin, sdev)},
    }
    f.write(json.dumps(row, sort_keys=True) + "\n")
    f.flush()


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--namespace", required=True)
    ap.add_argument("--c2s-dev", required=True)
    ap.add_argument("--tc-bin", default="tc")
    ap.add_argument("--s2c-dev", required=True)
    ap.add_argument("--start-ns", type=int, required=True)
    ap.add_argument("--pre-loss", type=float, required=True)
    ap.add_argument("--stress-loss", type=float, required=True)
    ap.add_argument("--post-loss", type=float, required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    phases = [
        ("pre", 0, args.pre_loss),
        ("stress", 30, args.stress_loss),
        ("post", 90, args.post_loss),
    ]
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    with open(args.output, "w", encoding="utf-8", buffering=1) as f:
        try:
            for idx, (name, offset_s, loss) in enumerate(phases):
                target = args.start_ns + offset_s * 1_000_000_000
                wait_until(target)
                if idx > 0:
                    prev = phases[idx - 1][0]
                    emit(f, prev + "_end", args.namespace, args.tc_bin, args.c2s_dev, args.s2c_dev,
                         phases[idx - 1][2], phases[idx - 1][2])
                change(args.namespace, args.tc_bin, args.c2s_dev, loss, args.seed * 100 + idx * 10 + 1)
                change(args.namespace, args.tc_bin, args.s2c_dev, loss, args.seed * 100 + idx * 10 + 2)
                emit(f, name + "_start", args.namespace, args.tc_bin, args.c2s_dev, args.s2c_dev, loss, loss)

            wait_until(args.start_ns + 120 * 1_000_000_000)
            emit(f, "post_end", args.namespace, args.tc_bin, args.c2s_dev, args.s2c_dev,
                 args.post_loss, args.post_loss)
            change(args.namespace, args.tc_bin, args.c2s_dev, 0.0, args.seed * 100 + 91)
            change(args.namespace, args.tc_bin, args.s2c_dev, 0.0, args.seed * 100 + 92)
            emit(f, "drain_start", args.namespace, args.tc_bin, args.c2s_dev, args.s2c_dev, 0.0, 0.0)
        except Exception as exc:
            f.write(json.dumps({
                "schema": 1, "event": "harness_error",
                "monotonic_ns": time.monotonic_ns(), "unix_ns": time.time_ns(),
                "tc_bin": args.tc_bin, "error": str(exc),
            }, sort_keys=True) + "\n")
            f.flush()
            raise


if __name__ == "__main__":
    main()
