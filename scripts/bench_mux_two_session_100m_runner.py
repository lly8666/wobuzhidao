#!/usr/bin/env python3
"""Run the two-session capacity characterization with loss isolated to data measurement.

The core harness still contains the historical 100 Mbit/s defaults. This wrapper
keeps its command contract stable while allowing qualification to override the
netem rate with --link-mbps. It also strips only the initial qdisc-add loss terms;
the core harness's qdisc-replace immediately before the offered interval still
applies the requested random loss and resets qdisc counters.
"""

import json
import pathlib
import sys

import bench_mux_two_session_100m as core


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
    # Parameterize the historical harness without changing the frozen transport
    # startup sequence or its existing 100 Mbit/s workflow callers.
    argv = [RATE_TOKEN if x == "100mbit" else x for x in argv]
    if "tc" in argv and "qdisc" in argv and "add" in argv and "loss" in argv:
        i = argv.index("loss")
        # netem syntax emitted by the core harness is:
        #   ... loss random <pct>% rate <link>mbit
        if i + 2 < len(argv) and argv[i + 1] == "random":
            del argv[i : i + 3]
    return _original_run(argv, check=check, capture=capture, timeout=timeout)


core.run = setup_safe_run
rc = core.main()

# Make the qualification boundary explicit in the durable artifact. The
# requested loss still applies to the measured offered interval; setup is
# intentionally loss-free so handshake survivability is not confused with
# sustained data-path delivery/FEC behavior.
if rc == 0 and len(sys.argv) > 1 and sys.argv[1] == "run":
    try:
        out_dir = pathlib.Path(sys.argv[sys.argv.index("--out-dir") + 1])
        path = out_dir / "result.json"
        result = json.loads(path.read_text())
        result["link_mbps"] = LINK_MBPS
        result["setup_loss_pct"] = 0.0
        result["measurement_loss_pct"] = result.get("loss_pct")
        result["loss_activation"] = "after_two_link_sessions_ready_before_offered_interval"
        result["capacity_override"] = "wrapper_rewrites_netem_rate_only"
        path.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
    except (ValueError, IndexError, OSError, json.JSONDecodeError) as exc:
        print(f"benchmark runner result annotation failed: {exc}", file=sys.stderr)
        rc = 2

raise SystemExit(rc)
