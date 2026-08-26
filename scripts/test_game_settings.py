#!/usr/bin/env python3
"""Black-box qualification for game controller CLI settings."""
from __future__ import annotations

import json
import math
import subprocess
import sys
from pathlib import Path


def run(binary: Path, *args: str, ok: bool = True):
    p = subprocess.run([str(binary), *args], text=True, capture_output=True)
    if ok and p.returncode != 0:
        raise AssertionError(f"command failed rc={p.returncode} args={args}\nstdout={p.stdout}\nstderr={p.stderr}")
    if not ok:
        if p.returncode == 0:
            raise AssertionError(f"command unexpectedly succeeded args={args}\n{p.stdout}")
        return None
    dec = json.JSONDecoder()
    obj, end = dec.raw_decode(p.stdout.lstrip())
    tail = p.stdout.lstrip()[end:]
    if "WBD_GAME_PLAN_PASS" not in tail:
        raise AssertionError(f"missing pass marker args={args}\n{p.stdout}")
    return obj


def close(a: float, b: float, rel: float = 1e-8) -> bool:
    return math.isclose(a, b, rel_tol=rel, abs_tol=1e-10)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: test_game_settings.py WBD_GAME_PLAN_BINARY")
    binary = Path(sys.argv[1])

    # Manual mode is a forceful authority and must work before auto estimator has a sample.
    manual = run(binary,
        "-link-speed-mode", "manual", "-manual-link-mbps", "80", "-auto-link-mbps", "0",
        "-lanes", "2", "-max-lanes", "2", "-fec", "off")
    assert manual["link_speed_mode"] == "manual", manual
    assert manual["effective_link_mbps"] == 80, manual
    assert manual["active_lanes"] == 2, manual

    # Auto mode owns the ceiling even when a conflicting manual value is supplied.
    auto = run(binary,
        "-link-speed-mode", "auto", "-auto-link-mbps", "123", "-manual-link-mbps", "999",
        "-lanes", "2", "-max-lanes", "2", "-fec", "off")
    assert auto["link_speed_mode"] == "auto" and auto["effective_link_mbps"] == 123, auto

    # Fixed lane count must stay exactly 1..4 even at severe measured loss when auto-add is disabled.
    fixed = {}
    for lanes in range(1, 5):
        p = run(binary,
            "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0",
            "-loss-pct", "30", "-mean-burst", "4", "-lanes", str(lanes),
            "-max-lanes", "4", "-auto-add-lane=false", "-fec", "off")
        assert p["requested_lanes"] == lanes and p["active_lanes"] == lanes, p
        assert not p["auto_lane_added"], p
        fixed[lanes] = p
    assert close(fixed[4]["inner_ceiling_mbps"], fixed[1]["inner_ceiling_mbps"] / 4), fixed

    # Auto-add may raise the floor but never exceed max-lanes. Under this risk it should hit each cap.
    for cap in (2, 3, 4):
        p = run(binary,
            "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0",
            "-loss-pct", "20", "-mean-burst", "2", "-lanes", "1",
            "-max-lanes", str(cap), "-auto-add-lane=true", "-fec", "off")
        assert p["active_lanes"] == cap and p["auto_lane_added"], p

    # Zero loss must never auto-downshift a requested lane floor.
    floor = run(binary,
        "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0",
        "-loss-pct", "0", "-lanes", "3", "-max-lanes", "4", "-auto-add-lane=true", "-fec", "off")
    assert floor["active_lanes"] == 3 and not floor["auto_lane_added"], floor

    # Live 20:20 is exactly 2x shard expansion, so at identical settings its inner ceiling is half off.
    off = run(binary,
        "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0",
        "-lanes", "2", "-max-lanes", "2", "-fec", "off")
    fec = run(binary,
        "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0",
        "-lanes", "2", "-max-lanes", "2", "-fec", "20:20")
    assert fec["fec"] == "20:20", fec
    assert close(fec["inner_ceiling_mbps"], off["inner_ceiling_mbps"] / 2), (off, fec)

    # Manual link speed is linear and carrier expansion/utilization are charged exactly once.
    fifty = run(binary, "-link-speed-mode", "manual", "-manual-link-mbps", "50", "-auto-link-mbps", "0", "-lanes", "1", "-max-lanes", "1")
    hundred = run(binary, "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0", "-lanes", "1", "-max-lanes", "1")
    assert close(hundred["inner_ceiling_mbps"], fifty["inner_ceiling_mbps"] * 2), (fifty, hundred)
    expanded = run(binary, "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0", "-lanes", "1", "-max-lanes", "1", "-carrier-expansion", "1.25")
    assert close(expanded["inner_ceiling_mbps"], hundred["inner_ceiling_mbps"] / 1.25), (hundred, expanded)
    half_util = run(binary, "-link-speed-mode", "manual", "-manual-link-mbps", "100", "-auto-link-mbps", "0", "-lanes", "1", "-max-lanes", "1", "-max-wire-util", "0.46")
    assert close(half_util["inner_ceiling_mbps"], hundred["inner_ceiling_mbps"] / 2), (hundred, half_util)

    # Invalid user settings must fail closed rather than silently clamp or substitute defaults.
    bad = [
        ("-link-speed-mode", "auto", "-auto-link-mbps", "0"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "0", "-auto-link-mbps", "0"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "10", "-auto-link-mbps", "0", "-lanes", "0"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "10", "-auto-link-mbps", "0", "-lanes", "5"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "10", "-auto-link-mbps", "0", "-lanes", "3", "-max-lanes", "2"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "10", "-auto-link-mbps", "0", "-fec", "20:8"),
        ("-link-speed-mode", "manual", "-manual-link-mbps", "10", "-auto-link-mbps", "0", "-race-target", "1"),
    ]
    for args in bad:
        run(binary, *args, ok=False)

    print("WBD_GAME_SETTINGS_MATRIX_PASS auto=1 manual=1 fixed_lanes=1..4 auto_add=1 max_lanes=1..4 fec=off,20:20 ceiling=1 invalid_fail_closed=1")


if __name__ == "__main__":
    main()
