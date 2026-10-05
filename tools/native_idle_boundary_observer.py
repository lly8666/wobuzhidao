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
    address = r"(?:\d{1,3}\.){3}\d{1,3}"
    match = re.match(r"^(\d+\.\d+) IP (" + address + r")\.(\d+) > (" + address + r")\.(\d+): (UDP\b|tcp\b)", line, re.IGNORECASE)
    if not match:
        return None
    try:
        ipaddress.IPv4Address(match[2])
        ipaddress.IPv4Address(match[4])
    except ipaddress.AddressValueError:
        return None
    if not (0 <= int(match[3]) <= 65535 and 0 <= int(match[5]) <= 65535):
        return None
    return {"unix_s": float(match[1]), "src": match[2], "src_port": int(match[3]),
            "dst": match[4], "dst_port": int(match[5]),
            "transport": match[6].upper()}


def capture_metadata(text, error):
    raw_lines = text.splitlines()
    # tcpdump can emit a final blank stdout line on SIGINT. It is not a
    # captured frame; compare against tcpdump's independent frame count.
    lines = [line for line in raw_lines if line.strip()]
    events = [event for line in lines if (event := parse_event(line)) is not None]
    captured = re.search(r"(\d+) packets captured", error)
    dropped = re.search(r"(\d+) packets dropped by kernel", error)
    count = int(captured[1]) if captured else None
    drops = int(dropped[1]) if dropped else None
    return dict(events=events, filtered_frames=len(lines), blank_lines=len(raw_lines)-len(lines),
                captured_frames=count, unparsed_frames=len(lines)-len(events),
                cap_reached=count is not None and count >= MAX_FRAMES,
                observer_drops=drops, capture_count_matches=count == len(lines),
                metadata_complete=count == len(events) and drops == 0 and count < MAX_FRAMES
                if count is not None else False)


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
        result = dict(lease=args.lease, start_unix_s=started, end_unix_s=time.time(),
                      duration_bound_s=args.seconds, **capture_metadata(text, error),
                      tcpdump_exit=process.returncode,
                      ended_by_timeout=timed_out, payload_stored=False, pcap_written=False,
                      excluded_fixture_udp_port=18445)
        result['metadata_complete'] = result['metadata_complete'] and process.returncode == 0
        Path(args.output).write_text(json.dumps(result, indent=2)+"\n")
    finally:
        if process.poll() is None:
            stop(None, None)
            process.wait(timeout=5)


if __name__ == "__main__":
    main()
