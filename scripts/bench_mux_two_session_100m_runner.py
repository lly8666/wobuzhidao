#!/usr/bin/env python3
"""Run the V2.3 single-flow two-session capacity characterization.

The wrapper keeps the historical command contract and result annotations while
using the single-flow setup core: one public FakeTCP association per client,
Reality-like TLS bootstrap on that association, then DTLS/LINK steady state.
It strips loss only from initial setup; the measured offered interval still
applies the requested random loss.
"""

import json
import pathlib
import sys

import bench_mux_two_session_single_flow_100m as core


def pop_wrapper_float(name, default):
    if name not in sys.argv:
        return float(default)
    i = sys.argv.index(name)
    if i + 1 >= len(sys.argv):
        raise SystemExit(f"{name} requires a value")
    try:
        value = float(sys.argv[i + 1])
    except ValueError as exc:
        raise SystemExit(f"invalid {name}: {sys.argv[i + 1]}") from exc
    del sys.argv[i : i + 2]
    return value


LINK_MBPS = pop_wrapper_float("--link-mbps", 100.0)
if LINK_MBPS <= 0:
    raise SystemExit("--link-mbps must be positive")
RATE_TOKEN = f"{LINK_MBPS:g}mbit"
_original_run = core.run


def setup_safe_run(cmd, *, check=True, capture=False, timeout=None):
    argv = list(cmd)
    argv = [RATE_TOKEN if x == "100mbit" else x for x in argv]
    if "tc" in argv and "qdisc" in argv and "add" in argv and "loss" in argv:
        i = argv.index("loss")
        if i + 2 < len(argv) and argv[i + 1] == "random":
            del argv[i : i + 3]
    return _original_run(argv, check=check, capture=capture, timeout=timeout)


core.run = setup_safe_run
rc = core.main()

if rc == 0 and len(sys.argv) > 1 and sys.argv[1] == "run":
    try:
        out_dir = pathlib.Path(sys.argv[sys.argv.index("--out-dir") + 1])
        path = out_dir / "result.json"
        result = json.loads(path.read_text())
        result["link_mbps"] = LINK_MBPS
        result["setup_loss_pct"] = 0.0
        result["measurement_loss_pct"] = result.get("loss_pct")
        result["loss_activation"] = "after_two_single_flow_link_sessions_ready_before_offered_interval"
        result["capacity_override"] = "wrapper_rewrites_netem_rate_only"
        result["qualification_setup"] = "single_public_faketcp_flow_with_in_association_reality_like_tls"
        path.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
    except (ValueError, IndexError, OSError, json.JSONDecodeError) as exc:
        print(f"benchmark runner result annotation failed: {exc}", file=sys.stderr)
        rc = 2

raise SystemExit(rc)
