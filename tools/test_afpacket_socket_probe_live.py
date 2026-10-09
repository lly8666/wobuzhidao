#!/usr/bin/env python3
"""Privileged Linux-only contract: parse genuine ss output for one AF_PACKET.

Called by Foundation in a newly created network namespace. No workload,
packets, packet captures, secrets, host networking changes or performance data.
"""
import socket
import subprocess
import time

from afpacket_socket_probe import numeric_row


def main():
    if not hasattr(socket, "AF_PACKET"):
        raise RuntimeError("Linux AF_PACKET required")
    # Use the exact transport family / protocol type as production RawIPv4Endpoint.
    # The enclosing workflow owns a fresh netns, so exactly one socket is expected.
    with socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x0800)) as recv:
        recv.bind(("lo", 0))
        began = time.monotonic_ns()
        ss = subprocess.run(["ss", "-0", "-a", "-m", "-n"],
                            capture_output=True, text=True, timeout=2, check=False)
        finished = time.monotonic_ns()
        row = numeric_row("server", began, finished, ss.stdout, ss.returncode, "")
        if not row["ss_ok"]:
            raise AssertionError("real Linux packet socket was not numerically parsed")
        skmem = row["packet_socket"]
        if skmem["rb"] <= 0 or skmem["r"] < 0 or skmem["d"] != 0:
            raise AssertionError("unexpected initial packet socket skmem counters")
        if row["ss_duration_ns"] < 0:
            raise AssertionError("non-monotonic ss timing")
    print("WBD_LIVE_AF_PACKET_NUMERIC_PARSE_PASS clean_netns=1 sockets=1 no_business=1")


if __name__ == "__main__":
    main()
