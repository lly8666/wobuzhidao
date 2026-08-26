#!/usr/bin/env python3
"""Black-box timing test for wbd-game-lane-client -inner-rate-mbps."""
from __future__ import annotations

import os
import select
import signal
import socket
import subprocess
import sys
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
    deadline = time.monotonic() + 5
    lines = []
    assert proc.stdout is not None
    while time.monotonic() < deadline:
        if proc.poll() is not None:
            err = proc.stderr.read() if proc.stderr else ""
            raise AssertionError(f"client exited before ready rc={proc.returncode} lines={lines} stderr={err}")
        ready, _, _ = select.select([proc.stdout], [], [], 0.1)
        if not ready:
            continue
        line = proc.stdout.readline()
        if not line:
            continue
        lines.append(line.rstrip())
        if "WBD_GAME_LANE_CLIENT_READY" in line:
            return line
    raise AssertionError(f"client ready timeout lines={lines}")


def measure(binary: Path, rate_mbps: float) -> tuple[float, str]:
    sink = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sink.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, 4 << 20)
    sink.bind(("127.0.0.1", 0))
    sink.settimeout(3)
    sink_port = sink.getsockname()[1]
    app_port = free_port()
    proc = subprocess.Popen(
        [str(binary), "-listen", f"127.0.0.1:{app_port}", "-lanes", f"127.0.0.1:{sink_port}",
         "-session-id", SESSION, "-inner-rate-mbps", str(rate_mbps)],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    try:
        ready = wait_ready(proc)
        expected = f"inner_ceiling_mbps={rate_mbps:.6f}"
        if expected not in ready:
            raise AssertionError(f"rate setting not reflected in READY: want {expected}, got {ready}")

        app = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        app.bind(("127.0.0.1", 0))
        payload = b"g" * PAYLOAD
        for _ in range(PACKETS):
            app.sendto(payload, ("127.0.0.1", app_port))
        app.close()

        stamps = []
        for _ in range(PACKETS):
            wire, _ = sink.recvfrom(65535)
            if len(wire) <= PAYLOAD:
                raise AssertionError(f"lane envelope missing: wire={len(wire)} payload={PAYLOAD}")
            stamps.append(time.monotonic())
        span = stamps[-1] - stamps[0]
        return span, ready
    finally:
        sink.close()
        if proc.poll() is None:
            proc.send_signal(signal.SIGTERM)
            try:
                proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait(timeout=2)


def main() -> None:
    if len(sys.argv) != 2:
        raise SystemExit("usage: test_game_pacing.py WBD_GAME_LANE_CLIENT_BINARY")
    binary = Path(sys.argv[1])

    unlimited, _ = measure(binary, 0)
    four, _ = measure(binary, 4)
    one, _ = measure(binary, 1)

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

    # Negative rates must fail before any traffic is accepted.
    p = subprocess.run(
        [str(binary), "-listen", f"127.0.0.1:{free_port()}", "-lanes", f"127.0.0.1:{free_port()}",
         "-inner-rate-mbps", "-1"],
        stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
    )
    if p.returncode == 0:
        raise AssertionError("negative inner rate unexpectedly accepted")

    print(f"WBD_GAME_PACING_PASS unlimited_span_s={unlimited:.6f} four_mbps_span_s={four:.6f} one_mbps_span_s={one:.6f} logical_before_copy=1")


if __name__ == "__main__":
    main()
