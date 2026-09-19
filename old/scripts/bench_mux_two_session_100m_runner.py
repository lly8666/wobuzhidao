#!/usr/bin/env python3
"""Run the ADR-0014 single-public-flow capacity characterization.

The wrapper keeps the historical CLI and result annotations while the core uses
one public FakeTCP association: Reality-like TLS bootstrap on that association,
then DTLS/LINK steady state carrying two independent inner streams. It strips
loss only from initial setup; the measured offered interval still applies the
requested random loss.

The logical-tunnel product contract no longer accepts arbitrary bare
application datagrams at LINK. Capacity probes therefore set a test-only mode
that wraps each inner UDP datagram in the real WBDP v1 platformproxy envelope;
the probe receiver unwraps the same envelope after LINK. This preserves the
existing transport benchmark while exercising the released application-frame
boundary instead of a retired bare-UDP shortcut.
"""

import json
import os
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
_original_wait_text = core.wait_text


def setup_safe_run(cmd, *, check=True, capture=False, timeout=None):
    argv = list(cmd)
    argv = [RATE_TOKEN if x == "100mbit" else x for x in argv]
    if "tc" in argv and "qdisc" in argv and "add" in argv and "loss" in argv:
        i = argv.index("loss")
        if i + 2 < len(argv) and argv[i + 1] == "random":
            del argv[i : i + 3]
    return _original_run(argv, check=check, capture=capture, timeout=timeout)


def single_flow_wait_text(path, needle, timeout=20.0, count=1):
    # ADR-0014 Logical Tunnel sessions are identified by tunnel_id, not the
    # retired account field. Keep the mature benchmark orchestration while
    # translating only its obsolete READY marker expectation.
    if needle == "WBD_LINK_MUX_SESSION_READY account=solo":
        needle = "WBD_LINK_MUX_SESSION_READY "
    return _original_wait_text(path, needle, timeout, count)


core.run = setup_safe_run
core.wait_text = single_flow_wait_text
os.environ["WBD_BENCH_PLATFORM_ENVELOPE"] = "1"
rc = core.main()

if rc == 0 and len(sys.argv) > 1 and sys.argv[1] == "run":
    try:
        out_dir = pathlib.Path(sys.argv[sys.argv.index("--out-dir") + 1])
        path = out_dir / "result.json"
        result = json.loads(path.read_text())
        result["link_mbps"] = LINK_MBPS
        result["setup_loss_pct"] = 0.0
        result["measurement_loss_pct"] = result.get("loss_pct")
        result["loss_activation"] = "after_single_public_flow_link_ready_before_offered_interval"
        result["capacity_override"] = "wrapper_rewrites_netem_rate_only"
        result["qualification_setup"] = "one_public_faketcp_flow_reality_like_tls_then_dtls_with_two_inner_streams"
        result["application_envelope"] = "platformproxy/WBDP-v1"
        result["application_envelope_header_bytes"] = 44
        result["bare_application_datagrams"] = False
        path.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
    except (ValueError, IndexError, OSError, json.JSONDecodeError) as exc:
        print(f"benchmark runner result annotation failed: {exc}", file=sys.stderr)
        rc = 2

raise SystemExit(rc)
