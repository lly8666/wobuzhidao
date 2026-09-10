from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{label} old block count={count}")
    return text.replace(old, new)


p = Path("scripts/bench_v2_transport_20x20.py")
s = p.read_text()
s = replace_once(
    s,
    '''def install_netem(ns: str, dev: str, rtt_ms: int, loss_pct: float, seed: int) -> None:
    half = rtt_ms / 2.0
    cmd = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "10000",
           "delay", f"{half:g}ms"]
    if loss_pct > 0:
        cmd += ["loss", "random", f"{loss_pct:g}%", "seed", str(seed)]
    run_ns(ns, cmd)
''',
    '''def install_netem(ns: str, dev: str, rtt_ms: int, loss_pct: float, seed: int) -> None:
    half = rtt_ms / 2.0
    base = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "10000",
            "delay", f"{half:g}ms"]
    if loss_pct <= 0:
        run_ns(ns, base)
        return

    seeded = [*base, "loss", "random", f"{loss_pct:g}%", "seed", str(seed)]
    cp = run_ns(ns, seeded, check=False, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if cp.returncode == 0:
        return

    # Some hosted runner iproute2 builds reject netem's optional RNG seed.
    # Keep the requested random loss/delay. If the portable unseeded form also
    # fails, surface the original and fallback errors instead of hiding them.
    fallback = [*base, "loss", "random", f"{loss_pct:g}%"]
    fb = run_ns(ns, fallback, check=False, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    if fb.returncode != 0:
        raise RuntimeError(
            f"netem install failed seeded_rc={cp.returncode} seeded={cp.stdout!r} "
            f"fallback_rc={fb.returncode} fallback={fb.stdout!r}"
        )
    detail = (cp.stdout or "").strip().replace("\\n", " ")[:400]
    print(
        f"WBD_NETEM_SEED_UNSUPPORTED ns={ns} dev={dev} seed={seed} "
        f"loss_pct={loss_pct:g} detail={detail!r}",
        file=sys.stderr,
        flush=True,
    )
''',
    "install_netem",
)
s = replace_once(
    s,
    '''            write_results(rows, a.out, a.seed)
    return 0
''',
    '''            write_results(rows, a.out, a.seed)
    harness_errors = [r for r in rows if r.get("case_status") == "harness_error"]
    baseline_failures = [
        r for r in rows
        if float(r.get("loss_pct_per_direction", -1)) == 0.0
        and (r.get("case_status") != "pass" or not (r.get("delivery_ratio") or 0) > 0)
    ]
    if harness_errors or baseline_failures:
        print(
            f"MATRIX_SEED_INVALID seed={a.seed} harness_errors={len(harness_errors)} "
            f"baseline_failures={len(baseline_failures)}",
            file=sys.stderr,
        )
        return 2
    return 0
''',
    "benchmark tail",
)
s = replace_once(
    s,
    '''    if not rows:
        raise SystemExit("no seed result CSVs found")
    keys: list[str] = []
''',
    '''    if not rows:
        raise SystemExit("no seed result CSVs found")
    status_counts = Counter(str(r.get("case_status") or "missing") for r in rows)
    harness_errors = status_counts.get("harness_error", 0)
    baseline_failures = sum(
        float(r.get("loss_pct_per_direction", -1)) == 0.0
        and (r.get("case_status") != "pass" or not (r.get("delivery_ratio") or 0) > 0)
        for r in rows
    )
    keys: list[str] = []
''',
    "aggregate header",
)
s = replace_once(
    s,
    '''    receipt = {
        "schema": "wbd-v2-transport-20x20-matrix/v1",
        "result": "completed",
        "environment": "GitHub hosted Ubuntu runner; root network namespaces; veth; symmetric tc netem installed before connection establishment",
        "topology": "UDP echo -> UDPspeeder mode0 20:20 timeout8 -> rebuilt exact-source DTLS1.3 shim -> pinned udp2raw FakeTCP; reverse path identical",
        "rtt_ms": sorted({int(r["rtt_ms"]) for r in rows}),
        "loss_pct_per_direction": sorted({float(r["loss_pct_per_direction"]) for r in rows}),
        "seeds": sorted({int(r["seed"]) for r in rows}),
        "cases": len(rows),
        "handshake_failures": sum(r.get("case_status") == "handshake_fail" for r in rows),
''',
    '''    receipt = {
        "schema": "wbd-v2-transport-20x20-matrix/v1",
        "result": "completed",
        "environment": "GitHub hosted Ubuntu runner; root network namespaces; veth; symmetric tc netem installed before connection establishment",
        "topology": "UDP echo -> UDPspeeder mode0 20:20 timeout8 -> rebuilt exact-source DTLS1.3 shim -> pinned udp2raw FakeTCP; reverse path identical",
        "rtt_ms": sorted({int(r["rtt_ms"]) for r in rows}),
        "loss_pct_per_direction": sorted({float(r["loss_pct_per_direction"]) for r in rows}),
        "seeds": sorted({int(r["seed"]) for r in rows}),
        "cases": len(rows),
        "case_status_counts": dict(sorted(status_counts.items())),
        "harness_errors": harness_errors,
        "zero_loss_baseline_failures": baseline_failures,
        "handshake_failures": sum(r.get("case_status") == "handshake_fail" for r in rows),
''',
    "aggregate receipt",
)
s = replace_once(
    s,
    '''    (a.out / "report.md").write_text("\\n".join(report) + "\\n")
    return 0
''',
    '''    (a.out / "report.md").write_text("\\n".join(report) + "\\n")
    if harness_errors or baseline_failures:
        print(
            f"MATRIX_INVALID harness_errors={harness_errors} "
            f"zero_loss_baseline_failures={baseline_failures} status_counts={dict(status_counts)}",
            file=sys.stderr,
        )
        return 2
    return 0
''',
    "aggregate tail",
)
p.write_text(s)

y = Path(".github/workflows/transport-20x20-matrix.yml")
t = y.read_text()
t = replace_once(
    t,
    '''      - uses: actions/download-artifact@v4
        with:
          name: transport-bench-assets
          path: /tmp/assets
      - name: Preflight namespace/raw networking
''',
    '''      - uses: actions/download-artifact@v4
        with:
          name: transport-bench-assets
          path: /tmp/assets
      - name: Restore executable modes lost through artifact transport
        run: chmod +x /tmp/assets/udp2raw_amd64 /tmp/assets/speederv2_amd64 /tmp/assets/wbd_dtls_shim
      - name: Preflight namespace/raw networking
''',
    "chmod insertion",
)
t = replace_once(
    t,
    '''          if receipt.get('cases') != 126: problems.append(f"cases={receipt.get('cases')} want=126")
          if receipt.get('rtt_ms') != want_rtt: problems.append(f"rtt={receipt.get('rtt_ms')}")
''',
    '''          if receipt.get('cases') != 126: problems.append(f"cases={receipt.get('cases')} want=126")
          if receipt.get('harness_errors') != 0: problems.append(f"harness_errors={receipt.get('harness_errors')}")
          if receipt.get('zero_loss_baseline_failures') != 0: problems.append(f"zero_loss_baseline_failures={receipt.get('zero_loss_baseline_failures')}")
          if receipt.get('rtt_ms') != want_rtt: problems.append(f"rtt={receipt.get('rtt_ms')}")
''',
    "completeness checks",
)
t = replace_once(
    t,
    '''      - uses: actions/github-script@v7
        with:
''',
    '''      - uses: actions/github-script@v7
        continue-on-error: true
        with:
''',
    "notify best effort",
)
y.write_text(t)
