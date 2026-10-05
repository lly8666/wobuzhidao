"""Bounded metadata for native uplink traffic outside the lifecycle fixture."""
import argparse
import ipaddress
import json
import re
import signal
import subprocess
import time
from pathlib import Path

MAX_FRAMES = 1000


def parse_event(line):
    match = re.match(r"^(\d+\.\d+) IP ([0-9.]+)\.(\d+) > ([0-9.]+)\.(\d+):", line)
    if not match:
        return None
    return {"unix_s": float(match[1]), "src": match[2], "src_port": int(match[3]),
            "dst": match[4], "dst_port": int(match[5]),
            "transport": "UDP" if "UDP" in line else "TCP" if "tcp" in line.lower() else "UNKNOWN"}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--lease", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--seconds", type=int, default=335)
    args = ap.parse_args()
    ipaddress.IPv4Address(args.lease)
    if not 1 <= args.seconds <= 400:
        raise ValueError("Bounded observer duration required")
    started = time.time()
    process = subprocess.Popen([
        "tcpdump", "-i", "wbdg0", "-n", "-l", "-q", "-tt", "-s", "96", "-B", "1024",
        "-c", str(MAX_FRAMES), "src host %s and not (udp port 18445)" % args.lease,
    ], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)

    def stop(_signum, _frame):
        if process.poll() is None:
            process.send_signal(signal.SIGINT)

    signal.signal(signal.SIGINT, stop)
    signal.signal(signal.SIGTERM, stop)
    timed_out = False
    try:
        try:
            text, error = process.communicate(timeout=args.seconds)
        except subprocess.TimeoutExpired:
            timed_out = True
            stop(None, None)
            text, error = process.communicate(timeout=5)
        # Never store tcpdump text or payload/question bytes, only exact numeric
        # tuples. Reaching the cap or losing capture invalidates silence proof.
        lines = text.splitlines()
        events = [event for line in lines if (event := parse_event(line)) is not None]
        drops = re.search(r"(\d+) packets dropped by kernel", error)
        result = dict(lease=args.lease, start_unix_s=started, end_unix_s=time.time(),
                      duration_bound_s=args.seconds, events=events, filtered_frames=len(lines),
                      unparsed_frames=len(lines)-len(events), cap_reached=len(lines)>=MAX_FRAMES,
                      observer_drops=int(drops[1]) if drops else None, tcpdump_exit=process.returncode,
                      ended_by_timeout=timed_out, payload_stored=False, pcap_written=False,
                      excluded_fixture_udp_port=18445)
        Path(args.output).write_text(json.dumps(result, indent=2)+"\n")
    finally:
        if process.poll() is None:
            stop(None, None)
            process.wait(timeout=5)


if __name__ == "__main__":
    main()
