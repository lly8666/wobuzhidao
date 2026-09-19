#!/usr/bin/env python3
"""Black-box timing and lane-setting test for wbd-game-lane-client."""
from __future__ import annotations

import queue
import re
import signal
import socket
import subprocess
import sys
import threading
import time
from pathlib import Path

PACKETS = 60
PAYLOAD = 1200
SESSION = "11223344556677889900aabbccddeeff"


def free_port() -> int:
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.bind(("127.0.0.1", 0))
    port = s.getsockname()[1]
    s.close()
    return port


def wait_ready(proc: subprocess.Popen[str]) -> str:
    """Read child stdout until the real READY line without TextIO/select races.

    TextIOWrapper.readline() may pull multiple lines from the OS fd into Python's
    own buffer. select() on the underlying fd can then report no readability even
    though READY is already buffered in user space. A dedicated blocking reader
    thread keeps the qualification strict (STATE is not treated as READY) while
    making line delivery independent of that buffering detail.
    """
    deadline = time.monotonic() + 5
    lines: list[str] = []
    assert proc.stdout is not None
    lineq: queue.Queue[str] = queue.Queue()

    def read_lines() -> None:
        assert proc.stdout is not None
        for line in proc.stdout:
            lineq.put(line)

    threading.Thread(target=read_lines, name="wbd-game-ready-reader", daemon=True).start()
    while time.monotonic() < deadline:
        try:
            line = lineq.get(timeout=min(0.1, max(0.0, deadline - time.monotonic())))
        except queue.Empty:
            if proc.poll() is not None:
                err = proc.stderr.read() if proc.stderr else ""
                raise AssertionError(
                    f"client exited before ready rc={proc.returncode} lines={lines} stderr={err}"
                )
            continue
        lines.append(line.rstrip())
        if "WBD_GAME_LANE_CLIENT_READY" in line:
            return line
    raise AssertionError(f"client ready timeout lines={lines}")


def stop(proc: subprocess.Popen[str]) -> None:
    if proc.poll() is None:
        proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait(timeout=2)


def measure(binary: Path, rate_mbps: float, lanes: int = 1) -> tuple[float, str]:
    sinks = []
    for _ in range(lanes):
        s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        s.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 << 20)
        s.bind(("127.0.0.1", 0))
        s.settimeout(3)
        sinks.append(s)
    lane_arg = ",".join(f"127.0.0.1:{s.getsockname()[1]}" for s in sinks)
    app_port = free_port()
    proc = subprocess.Popen(
        [str(binary), "-listen", f"127.0.0.1:{app_port}", "-lanes", lane_arg,
         "-session-id", SESSION, "-inner-rate-mbps", str(rate_mbps)],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    try:
        ready = wait_ready(proc)
        expected_rate = f"inner_ceiling_mbps={rate_mbps:.6f}"
        if expected_rate not in ready or f"lanes={lanes}" not in ready or f"session_id={SESSION}" not in ready:
            raise AssertionError(f"settings not reflected in READY: want rate={expected_rate} lanes={lanes} session={SESSION}, got {ready}")

        app = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        app.bind(("127.0.0.1", 0))
        payload = b"g" * PAYLOAD
        for _ in range(PACKETS):
            app.sendto(payload, ("127.0.0.1", app_port))
        app.close()

        # Time lane 1. Other lane sockets remain open and buffered, proving that
        # adding race copies does not multiply the logical pacing reservation.
        stamps = []
        for _ in range(PACKETS):
            wire, _ = sinks[0].recvfrom(65535)
            if len(wire) <= PAYLOAD:
                raise AssertionError(f"lane envelope missing: wire={len(wire)} payload={PAYLOAD}")
            stamps.append(time.monotonic())
        span = stamps[-1] - stamps[0]
        return span, ready
    finally:
        for sink in sinks:
            sink.close()
        stop(proc)


def start_and_read_settings(binary: Path, session: str, replay_window: int) -> str:
    sink = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sink.bind(("127.0.0.1", 0))
    proc = subprocess.Popen(
        [str(binary), "-listen", f"127.0.0.1:{free_port()}",
         "-lanes", f"127.0.0.1:{sink.getsockname()[1]}",
         "-session-id", session, "-replay-window", str(replay_window)],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    try:
        return wait_ready(proc)
    finally:
        sink.close()
        stop(proc)


def expect_fail(binary: Path, lane_arg: str, *extra: str) -> None:
    p = subprocess.run(
        [str(binary), "-listen", f"127.0.0.1:{free_port()}", "-lanes", lane_arg, *extra],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=3,
    )
    if p.returncode == 0:
        raise AssertionError(f"invalid client settings unexpectedly accepted: lanes={lane_arg} extra={extra}")


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: test_game_pacing.py WBD_GAME_LANE_CLIENT_BINARY")
    binary = Path(sys.argv[1])

    unlimited, _ = measure(binary, 0)
    four, _ = measure(binary, 4)
    one, _ = measure(binary, 1)
    four_lane_one, _ = measure(binary, 1, lanes=4)

    # Reserve() serializes logical inner bytes before lane copies. For 60*1200 B,
    # spans should be ~0.142s at 4 Mbps and ~0.566s at 1 Mbps. Keep broad bounds
    # for shared CI runners while still catching Mbps/MBps and misplaced pacing.
    if not (0.08 <= four <= 0.30):
        raise AssertionError(f"4 Mbps pacing span out of range: {four:.6f}s")
    if not (0.40 <= one <= 0.90):
        raise AssertionError(f"1 Mbps pacing span out of range: {one:.6f}s")
    if one / four < 2.5:
        raise AssertionError(f"pacing scale wrong: one={one:.6f}s four={four:.6f}s")
    if unlimited >= four * 0.6:
        raise AssertionError(f"0 should disable pacing: unlimited={unlimited:.6f}s four={four:.6f}s")
    if not (0.75 <= four_lane_one / one <= 1.35):
        raise AssertionError(f"lane copies changed logical pacing: one_lane={one:.6f}s four_lane={four_lane_one:.6f}s")

    # session-id and replay-window are public settings too. Minimum legal replay
    # window must start successfully; auto must produce a non-zero 128-bit ID.
    auto_ready = start_and_read_settings(binary, "auto", 64)
    m = re.search(r"session_id=([0-9a-f]{32})", auto_ready)
    if not m or int(m.group(1), 16) == 0:
        raise AssertionError(f"auto session ID invalid: {auto_ready}")
    fixed_ready = start_and_read_settings(binary, SESSION, 64)
    if f"session_id={SESSION}" not in fixed_ready:
        raise AssertionError(f"fixed session ID not preserved: {fixed_ready}")

    # Client-side settings must fail closed too, not only controller validation.
    good = f"127.0.0.1:{free_port()}"
    expect_fail(binary, good, "-inner-rate-mbps", "-1")
    expect_fail(binary, f"{good},{good}")
    five = ",".join(f"127.0.0.1:{free_port()}" for _ in range(5))
    expect_fail(binary, five)
    expect_fail(binary, good, "-session-id", "00" * 16)
    expect_fail(binary, good, "-session-id", "abcd")
    expect_fail(binary, good, "-session-id", "not-hex-at-all")
    expect_fail(binary, good, "-replay-window", "63")
    expect_fail(binary, good, "-replay-window", str((1 << 20) + 1))

    print(
        f"WBD_GAME_PACING_PASS unlimited_span_s={unlimited:.6f} four_mbps_span_s={four:.6f} "
        f"one_mbps_span_s={one:.6f} four_lane_one_mbps_span_s={four_lane_one:.6f} "
        "logical_before_copy=1 lane_validation=1 session_id=1 replay_window=1"
    )


if __name__ == "__main__":
    main()
