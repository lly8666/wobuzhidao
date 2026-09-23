#!/usr/bin/env python3
"""Static contract for performance-measurement GitHub Actions workflows."""
from pathlib import Path
import re

MEASUREMENT_MARKERS = (
    "scripts/strict_weaknet_sample.sh",
    "scripts/strict_capacity_bypass.sh",
    "scripts/realpath_calibration.sh",
    "WBD_P5_WEAKNET_MEASURE",
    "WBD_P5_FEC2020_WEAKNET_MEASURE",
    "WBD_P5_SOAK_MEASURE",
    "TestP5LoadMeasurementHarness",
    "TestP5ControlledHTTPSMeasurementHarness",
)
ACTIVE = (
    ".github/workflows/next-strict-weaknet.yml",
    ".github/workflows/next-performance-capacity-diagnostic.yml",
    ".github/workflows/next-realpath-calibration.yml",
    ".github/workflows/next-strict-capacity-diagnostics.yml",
    ".github/workflows/next-strict-packet-socket-diagnostic.yml",
)
RETIRED = (
    ".github/workflows/next-performance-ab-game.yml",
    ".github/workflows/next-performance-recovery.yml",
)
FOUNDATION_DISABLED = (
    "p5-https-measurement-base",
    "p5-weaknet-5-20-5",
    "p5-weaknet-5-30-5",
    "p5-load",
    "p5-load-40",
    "p5-fec2020-weaknet-5-20-5",
    "p5-fec2020-weaknet-5-30-5",
    "p5-soak",
)

errors = []
for name in ACTIVE:
    text = Path(name).read_text(encoding="utf-8")
    if "workflow_dispatch:" not in text:
        errors.append(f"{name}: workflow_dispatch required")
    if r"\${{" in text:
        errors.append(f"{name}: escaped GitHub expression forbidden")
    if re.search(r"(?m)^\s+push:", text):
        errors.append(f"{name}: automatic push measurement forbidden")
    if re.search(r"(?m)^\s+matrix:", text):
        errors.append(f"{name}: matrix fanout forbidden")
    if text.count("tools/perf_sample_guard.py claim") != 1:
        errors.append(f"{name}: exactly one sample claim required")
    jobs = re.findall(r"(?m)^  ([A-Za-z0-9_-]+):\n", text.split("jobs:", 1)[1] if "jobs:" in text else "")
    marker_jobs = 0
    for job in jobs:
        m = re.search(
            rf"(?ms)^  {re.escape(job)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)",
            text,
        )
        body = m.group("body") if m else ""
        if any(marker in body for marker in MEASUREMENT_MARKERS):
            marker_jobs += 1
    if marker_jobs != 1:
        errors.append(f"{name}: expected exactly one measurement job, got {marker_jobs}")

for name in RETIRED:
    text = Path(name).read_text(encoding="utf-8")
    if any(marker in text for marker in MEASUREMENT_MARKERS):
        errors.append(f"{name}: retired workflow still contains a measurement entry")

foundation = Path(".github/workflows/next-foundation.yml").read_text(encoding="utf-8")
for job in FOUNDATION_DISABLED:
    m = re.search(
        rf"(?ms)^  {re.escape(job)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)",
        foundation,
    )
    if not m or "if: ${{ false }}" not in m.group("body"):
        errors.append(f"next-foundation: legacy measurement job {job} is not disabled")

if errors:
    raise SystemExit("\n".join(errors))
print("WBD_PERF_WORKFLOW_POLICY_PASS")
