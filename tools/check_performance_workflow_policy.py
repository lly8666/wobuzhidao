#!/usr/bin/env python3
"""Static contract for performance-measurement GitHub Actions workflows."""
from pathlib import Path
import re

MEASUREMENT_MARKERS = (
    "tools/prepare_soak_harness.py",
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
    ".github/workflows/next-target-soak.yml",
    ".github/workflows/next-strict-weaknet.yml",
    ".github/workflows/next-shared-blackhole.yml",
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

# continue-on-error exists only to preserve artifacts, never to waive a failed
# qualification analyzer. The final always step must propagate both outcomes.
for name in (".github/workflows/next-strict-weaknet.yml", ".github/workflows/next-shared-blackhole.yml"):
    text = Path(name).read_text(encoding="utf-8")
    final = text.rsplit("      - name: Preserve collection", 1)[-1]
    if ('ANALYZER_OUTCOME: ${{ steps.validate.outcome }}' not in final
            or 'test "$ANALYZER_OUTCOME" = success' not in final
            or 'test "$SAMPLE_OUTCOME" = success' not in final
            or 'if: always()' not in final):
        errors.append(f"{name}: collection and analyzer outcomes must gate final success")

foundation = Path(".github/workflows/next-foundation.yml").read_text(encoding="utf-8")
for job in FOUNDATION_DISABLED:
    m = re.search(
        rf"(?ms)^  {re.escape(job)}:\n(?P<body>.*?)(?=^  [A-Za-z0-9_-]+:\n|\Z)",
        foundation,
    )
    if not m or "if: ${{ false }}" not in m.group("body"):
        errors.append(f"next-foundation: legacy measurement job {job} is not disabled")


# Separate synthetic observer overhead workflow: explicitly case-config PUSH only.
# A config-only commit is one standalone 12s AF_UNIX case, not product traffic,
# and OFF/ON comparisons cannot be performed in the same Action.
cal = Path(".github/workflows/next-e4-recvmmsg-calibration.yml").read_text(encoding="utf-8")
# GitHub only exposes a newly created workflow_dispatch workflow after
# its file exists on the repository default branch; main is prohibited here.
# Exactly one explicit CASE FILE push on this branch is the only trigger.
if "workflow_dispatch:" in cal:
    errors.append("E4 calibration: feature-branch-only dispatch is not initially usable")
if 'branches: ["next/performance-efficiency-20261008"]' not in cal:
    errors.append("E4 calibration: only dedicated working branch allowed")
if 'paths:\n      - ".github/e4-recvmmsg-calibration-case.json"' not in cal:
    errors.append("E4 calibration: only explicit single case config may trigger")
if cal.count("      - \".github/e4-recvmmsg-calibration-case.json\"")!=1:
    errors.append("E4 calibration: config-trigger must be unique")
if re.search(r"(?m)^\s+pull_request:",cal):
    errors.append("E4 calibration: PR measurement forbidden")
if ('"git","diff","--name-status","HEAD^","HEAD"' not in cal or
        'assert len(changed)==3 and len(cases)==1 and len(stat)==1 and len(log)==1' not in cal or
        'stat[0][0]=="M" and log[0][0]=="A"' not in cal or
        "unapproved_helper_change" not in cal):
    errors.append("E4 calibration: exactly one case config, STATUS change, new devlog and frozen helpers required")
if re.search(r"(?m)^\s+matrix:",cal):
    errors.append("E4 calibration: matrix/fanout forbidden")
if cal.count("  one-nonproduct-calibration-case:\n")!=1:
    errors.append("E4 calibration: exactly one measurement job required")
if 'assert d["mode"] in ("off","on")' not in cal or 'assert re.fullmatch(r"e4-"+d["mode"]' not in cal:
    errors.append("E4 calibration: config mode must be exactly one of off/on with scoped case ID")
if cal.count("python3 -m py_compile tools/e4_recvmmsg_calibration.py")!=1 or cal.count("sudo python3 tools/e4_recvmmsg_calibration.py")!=1:
    errors.append("E4 calibration: exactly one py_compile and one actual execution required")
if "continue-on-error: true" not in cal or 'test "$RESULT" = success' not in cal:
    errors.append("E4 calibration: failed measurement must propagate even after artifact upload")
if any(x in cal for x in ("scripts/strict_weaknet_sample.sh","tools/prepare_large_mtu_harness.py","tools/prepare_soak_harness.py")):
    errors.append("E4 calibration: product sample scripts forbidden")


# E1 sparse 15ms fullstack: one pinned 32ms product, 120s functional
# business case and one Actions measurement job, never an A/B/matrix.
e1 = Path(".github/workflows/next-e1-lowrtt-fullstack.yml").read_text(encoding="utf-8")
if 'branches: ["next/performance-efficiency-20261008"]' not in e1:
    errors.append("E1 lowrtt: exact working branch required")
if "workflow_dispatch:" in e1 or re.search(r"(?m)^\s+matrix:",e1):
    errors.append("E1 lowrtt: matrix or manual variants forbidden")
if e1.count("  one-15ms-real-fullstack-protector:\n")!=1:
    errors.append("E1 lowrtt: one physical measurement job required")
if e1.count("tools/perf_sample_guard.py claim")!=1:
    errors.append("E1 lowrtt: exactly one source/seed/workload claim required")
if 'a2db258b436a41fdee98c6c53abec9bab6ce600f' not in e1:
    errors.append("E1 lowrtt: fullstack must pin uniform32 SOURCE")
if 'test "$SAMPLE" = success' not in e1 or 'test "$AUDIT" = success' not in e1:
    errors.append("E1 lowrtt: real business and analyzer failures must propagate")
if "tools/prepare_e1_lowrtt_fullstack.py --output" not in e1:
    errors.append("E1 lowrtt: audited strict topology derivation required")
if 'WBD_STRICT_RATE_MBPS: "0.328"' not in e1:
    errors.append("E1 lowrtt: exact sparse payload rate model required")

# Exactly one named FEC policy sequential exception. Original ACTIVE
# single-sample checks above remain mandatory and unmodified.
fec = Path(".github/workflows/next-fec-policy-sequential.yml").read_text()
if 'branches: ["experiment/fec-policy-sequential-20261009"]' not in fec:
    errors.append("FEC serial: incorrect branch")
if fec.count('      - ".github/fec-policy-batch.json"') != 1 or "workflow_dispatch:" in fec:
    errors.append("FEC serial: config-only push required; no dispatch")
if re.search(r"(?m)^\s+matrix:|^\s+pull_request:",fec):
    errors.append("FEC serial: matrix/PR forbidden")
if fec.count("  one-serial-fullstack-fec-policy-batch:\n") != 1 or fec.count("tools/fec_policy_batch.py --mode run") != 1:
    errors.append("FEC serial: one job, one sequential runner only")
if "tools/perf_sample_guard.py claim" in fec:
    errors.append("FEC serial: cannot rewrite old formal sample guard")
if "test \"$BATCH\" = success" not in fec or "cancel-in-progress: false" not in fec:
    errors.append("FEC serial: must gate failures and prevent parallel Actions")

if errors:
    raise SystemExit("\n".join(errors))
print("WBD_PERF_WORKFLOW_POLICY_PASS")
