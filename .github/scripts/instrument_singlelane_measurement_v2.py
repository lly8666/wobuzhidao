#!/usr/bin/env python3
from pathlib import Path
import re
import sys


def patch_generated(path: Path, load_src: str) -> None:
    s = path.read_text()

    # This function receives the actual fullstack script produced by the
    # rotation wrapper (PATCHED), not either of the two wrapper scripts above it.
    start = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD\'\n'
    measured_start = 'cat >"$LOG_DIR/load.py" <<\'PY_LOAD_V2\'\n'
    if measured_start not in s:
        i = s.find(start)
        if i < 0:
            raise SystemExit("generated load heredoc start marker missing")
        j = s.find('\nPY_LOAD\n', i + len(start))
        if j < 0:
            raise SystemExit("generated load heredoc end marker missing")
        replacement = measured_start + load_src.rstrip() + '\nPY_LOAD_V2\n'
        s = s[:i] + replacement + s[j + len('\nPY_LOAD\n'):]

    old_launch = 'sudo ip netns exec "$C" python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'
    new_launch = '''date +%s%N >"$LOG_DIR/load-start-epoch-ns.txt"
sudo ip netns exec "$C" env \\
  WBD_LOAD_MAX_PAYLOAD="$INNER_MTU" \\
  WBD_LOAD_PHASE_SPEC="${WBD_LOAD_PHASE_SPEC:-}" \\
  WBD_LOAD_DRAIN_SEC="${WBD_LOAD_DRAIN_SEC:-15}" \\
  WBD_LOAD_MAX_SEND_BURST="${WBD_LOAD_MAX_SEND_BURST:-64}" \\
  WBD_LOAD_MAX_RECV_BURST="${WBD_LOAD_MAX_RECV_BURST:-256}" \\
  python3 "$LOG_DIR/load.py" "$LOG_DIR/load-result.json" "$DURATION_SEC" "$RATE_BPS" "$PAYLOAD_BYTES" >"$LOG_DIR/load.log" 2>&1 &'''
    if new_launch not in s:
        if s.count(old_launch) != 1:
            raise SystemExit(f"generated load launch marker drift: {s.count(old_launch)}")
        s = s.replace(old_launch, new_launch, 1)

    flag = '-diag-rcvbuf-effective-bytes "${WBD_SERVER_LINK_RCVBUF_EFFECTIVE_BYTES:-0}"'
    if flag not in s:
        # Match the unique server LINK command block rather than relying on one
        # exact redirection layout.  The regex intentionally lets the redirect
        # capture own the newline/indentation, so prefix ends at the existing
        # shell continuation backslash.
        block_re = re.compile(
            r'(?ms)(sudo ip netns exec "\$S" "\$ASSET_DIR/wbd-link-server-mux" \\\n'
            r'.*?)(\s*>"\$LOG_DIR/link-server\.log" 2>&1 &)'
        )
        hits = list(block_re.finditer(s))
        if len(hits) != 1:
            raise SystemExit(f"generated server LINK command marker drift: {len(hits)}")
        m = hits[0]
        prefix = m.group(1)
        redirect = m.group(2)
        if not prefix.endswith('\\'):
            raise SystemExit("generated server LINK command missing continuation before redirect")
        replacement = prefix + '\n  ' + flag + ' \\\n  ' + redirect.lstrip()
        s = s[:m.start()] + replacement + s[m.end():]

    path.write_text(s)
    print("WBD_SINGLELANE_MEASUREMENT_V2_GENERATED actual_tx_timestamp=1 bounded_catchup=1 isolated_link_rcvbuf=1 final_fullstack=1")


def install_control_hook(root: Path) -> None:
    control = root / "scripts/game_lane_rotation_soak_control.sh"
    if not control.exists():
        raise SystemExit(f"control wrapper missing: {control}")
    s = control.read_text()

    # The control wrapper produces another wrapper (OUT).  That rotation wrapper
    # later produces the actual fullstack script (PATCHED).  Therefore edit OUT
    # so that it finalizes PATCHED immediately before PATCHED is executed.
    installer = '''python3 - "$OUT" <<'PY_WBD_ROTATION_FINALIZER_HOOK'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text()
marker = 'exec "$PATCHED" "$@"\\n'
hook = 'python3 "${WBD_MEASUREMENT_FINALIZER:?}" --generated "$PATCHED" "${WBD_PACED_LOAD_SCRIPT:?}"\\nbash -n "$PATCHED"\\n'
if hook not in s:
    if s.count(marker) != 1:
        raise SystemExit(f"rotation final exec marker drift: {s.count(marker)}")
    s = s.replace(marker, hook + marker, 1)
p.write_text(s)
PY_WBD_ROTATION_FINALIZER_HOOK
'''
    if 'PY_WBD_ROTATION_FINALIZER_HOOK' not in s:
        marker = 'exec "$OUT" "$@"\n'
        if s.count(marker) != 1:
            raise SystemExit(f"control final exec marker drift: {s.count(marker)}")
        s = s.replace(marker, installer + marker, 1)
        control.write_text(s)
    print("WBD_SINGLELANE_MEASUREMENT_V2_CONTROL_HOOK rotation_finalizer=1")


def install_helper_hook(root: Path) -> None:
    wrapper = root / ".github/scripts/run_singlelane_ab_observability.sh"
    if not wrapper.exists():
        raise SystemExit(f"A/B helper wrapper missing: {wrapper}")
    s = wrapper.read_text()

    install = '''export WBD_MEASUREMENT_FINALIZER="${GITHUB_WORKSPACE:?}/suite/.github/scripts/instrument_singlelane_measurement_v2.py"
export WBD_PACED_LOAD_SCRIPT="${GITHUB_WORKSPACE:?}/suite/.github/scripts/paced_udp_load_v2.py"
python3 - "$RUNNER" <<'PY_WBD_MEASUREMENT_HOOK'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text()
marker = 'bash -n scripts/game_lane_fullstack.sh\\n'
hook = 'python3 "${WBD_MEASUREMENT_FINALIZER:?}" --install-control "$PRODUCT_DIR" "${WBD_PACED_LOAD_SCRIPT:?}"\\n'
if hook not in s:
    if s.count(marker) != 1:
        raise SystemExit(f"generated validator measurement marker drift: {s.count(marker)}")
    s = s.replace(marker, hook + marker, 1)
p.write_text(s)
PY_WBD_MEASUREMENT_HOOK
'''
    if 'PY_WBD_MEASUREMENT_HOOK' not in s:
        marker = 'chmod +x "$RUNNER"\n'
        if s.count(marker) != 1:
            raise SystemExit(f"A/B helper generated-runner marker drift: {s.count(marker)}")
        s = s.replace(marker, marker + install, 1)
        wrapper.write_text(s)
    print("WBD_SINGLELANE_MEASUREMENT_V2_HELPER_HOOK generated_validator_finalizer=1")


def main() -> None:
    if len(sys.argv) == 4 and sys.argv[1] == "--generated":
        patch_generated(Path(sys.argv[2]), Path(sys.argv[3]).read_text())
        return
    if len(sys.argv) == 4 and sys.argv[1] == "--install-control":
        load = Path(sys.argv[3])
        if not load.is_file():
            raise SystemExit(f"paced load script missing: {load}")
        install_control_hook(Path(sys.argv[2]))
        return
    if len(sys.argv) == 3:
        root = Path(sys.argv[1])
        load = Path(sys.argv[2])
        if not load.is_file():
            raise SystemExit(f"paced load script missing: {load}")
        install_helper_hook(root)
        print("WBD_SINGLELANE_MEASUREMENT_V2_PATCHED helper_only=1 final_generated_patch=1")
        return
    raise SystemExit(
        "usage: instrument_singlelane_measurement_v2.py ROOT PACED_LOAD | "
        "--install-control PRODUCT_ROOT PACED_LOAD | --generated SCRIPT PACED_LOAD"
    )


if __name__ == "__main__":
    main()
