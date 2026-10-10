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

# Opt-in single-forensic retransmission measurement; no second workload, dispatch or matrix.
retrans = Path(".github/workflows/next-fec-retrans-300ms.yml").read_text()
if 'branches: ["experiment/fec-policy-sequential-20261009"]' not in retrans:
    errors.append("FEC retrans: only exact experiment branch")
if retrans.count('      - ".github/fec-retrans-probe.json"') != 1 or "workflow_dispatch:" in retrans:
    errors.append("FEC retrans: config-only push, no workflow dispatch")
if re.search(r"(?m)^\s+matrix:|^\s+pull_request:",retrans):
    errors.append("FEC retrans: no parallel matrix or pull requests")
if retrans.count("  one-fec-off-wire-retrans-forensic:\n")!=1 or retrans.count("tools/fec_retrans_probe.py --mode run")!=1:
    errors.append("FEC retrans: exactly one real sample in one job")
if "cancel-in-progress: false" not in retrans or 'test "$SAMPLE" = success' not in retrans:
    errors.append("FEC retrans: conservative no-parallel fail-closed gate")
# Explicit user-authorized 36 tuples, not the older one-sample formal workflows.
# One job, serial subprocess, branch+config-only push, no dispatcher or matrix.
sweep = Path(".github/workflows/next-fec-retrans-sweep.yml").read_text()
if 'branches: ["experiment/fec-policy-sequential-20261009"]' not in sweep:
    errors.append("FEC sweep: exact isolated branch required")
if sweep.count('      - ".github/fec-retrans-sweep.json"')!=1 or "workflow_dispatch:" in sweep:
    errors.append("FEC sweep: config-only trigger required")
if re.search(r"(?m)^\s+matrix:|^\s+pull_request:",sweep):
    errors.append("FEC sweep: no matrix or pull request")
if sweep.count("  one-job-thirty-six-serial-realpath:\n")!=1 or sweep.count("tools/fec_retrans_sweep.py --mode run")!=1:
    errors.append("FEC sweep: precisely one serial execution job")
if "cancel-in-progress: false" not in sweep or "timeout-minutes: 240" not in sweep:
    errors.append("FEC sweep: bounded one-run scope")
if 'test "$SAMPLE" = success' not in sweep or 'd["valid_cases"]==36' not in sweep:
    errors.append("FEC sweep: fail-closed all 36 cases required")
if "tools/perf_sample_guard.py claim" in sweep:
    errors.append("FEC sweep: must not replace formal sample guard")

# Only this exact FEC SIMD experiment has user-authorized same-job serial ABBA.
simd = Path(".github/workflows/next-fec-simd-ab.yml").read_text()
if 'branches: ["next/fec-simd-20261010"]' not in simd or simd.count('      - ".github/fec-simd-ab.json"')!=1:
    errors.append("FEC SIMD: exact branch/config push missing")
if "workflow_dispatch:" in simd or re.search(r"(?m)^\s+matrix:|^\s+pull_request:",simd):
    errors.append("FEC SIMD: no dispatch, matrix or PR")
if simd.count("  one-fec-simd-serial-abba:\n")!=1 or "cancel-in-progress: false" not in simd:
    errors.append("FEC SIMD: single serial job and concurrency gate required")
if simd.count("tools/fec_simd_ab.py --mode execute --root")==2:
    pass  # preflight branch OR one actual run, never simultaneous
else:
    errors.append("FEC SIMD: expect preflight and one actual execute only")
if "a2db258b436a41fdee98c6c53abec9bab6ce600f" not in simd or "7fb98fab79834a351a1dbe04eebb207f66bea28b" not in simd:
    errors.append("FEC SIMD: two independent immutable SOURCE identities required")

micro = Path(".github/workflows/next-fec-simd-micro.yml").read_text()
if micro.count("  one-micro-cpu-sample:\n")!=1 or micro.count("      - uses: actions/upload-artifact@v4")!=1:
    errors.append("FEC SIMD micro: invalid YAML layout/fanout")
if re.search(r"(?m)^ -bench|^\s*go test .* -run '\^\s*$",micro):
    errors.append("FEC SIMD micro: suspicious concatenated benchmark command")
if "wbd_fec_span" not in micro or "wbd_fec_scalar" not in micro:
    errors.append("FEC SIMD micro: independent fallback attribution missing")

if errors:
    raise SystemExit("\n".join(errors))
print("WBD_PERF_WORKFLOW_POLICY_PASS")

# Exact 2026-10-10 crypto/packet ABBA exception. Never alter legacy guards.
packet = Path(".github/workflows/next-crypto-packet-abba.yml").read_text()
if 'branches: ["next/crypto-packet-efficiency-20261010"]' not in packet:
    errors.append("Crypto packet ABBA: wrong exact working branch")
if packet.count('      - ".github/crypto-packet-abba.json"') != 1 or "workflow_dispatch:" in packet:
    errors.append("Crypto packet ABBA: exact config-only trigger mandatory")
if re.search(r"(?m)^\\s+matrix:|^\\s+pull_request:", packet):
    errors.append("Crypto packet ABBA: no fanout/PR")
if packet.count("  one-crypto-packet-serial-abba:\\n") != 1 or packet.count("tools/crypto_packet_abba.py --mode execute") != 1:
    errors.append("Crypto packet ABBA: one job and exact serial runner")
if "cancel-in-progress: false" not in packet or "tools/perf_sample_guard.py claim" in packet:
    errors.append("Crypto packet ABBA: concurrency and existing guard must remain isolated")
if 'test "$BATCH" = success' not in packet:
    errors.append("Crypto packet ABBA: failed qualification must propagate")
