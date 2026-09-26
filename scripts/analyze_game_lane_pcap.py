#!/usr/bin/env python3
"""Verify that game-mode lanes are independent TCP-shaped outer flows."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

from analyze_faketcp_pcap import ACK, SYN, parse_tcp, read_pcap


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("pcap", type=Path)
    ap.add_argument("--server-port", type=int, required=True)
    ap.add_argument("--client-ports", required=True, help="comma-separated expected FakeTCP source ports")
    ap.add_argument("--out", type=Path)
    args = ap.parse_args()

    expected = {int(x.strip()) for x in args.client_ports.split(",") if x.strip()}
    if not expected or len(expected) > 4:
        raise SystemExit("expected 1..4 client ports")

    packets = []
    for ts, frame in read_pcap(args.pcap):
        p = parse_tcp(ts, frame)
        if p and (p["srcp"] in expected or p["dstp"] in expected) and (p["srcp"] == args.server_port or p["dstp"] == args.server_port):
            packets.append(p)
    if not packets:
        raise AssertionError("no game-lane FakeTCP packets in pcap")

    flows = {}
    for port in sorted(expected):
        c2s = [p for p in packets if p["srcp"] == port and p["dstp"] == args.server_port]
        s2c = [p for p in packets if p["srcp"] == args.server_port and p["dstp"] == port]
        syn = next((p for p in c2s if p["flags"] & SYN and not (p["flags"] & ACK)), None)
        synack = next((p for p in s2c if p["flags"] & SYN and p["flags"] & ACK), None)
        data = [p for p in c2s if p["payload_len"] > 0]
        if syn is None or synack is None or not data:
            raise AssertionError(f"flow {port} incomplete: syn={bool(syn)} synack={bool(synack)} data={len(data)}")
        flows[port] = {
            "client_isn": syn["seq"],
            "server_isn": synack["seq"],
            "data_packets": len(data),
            "first_data_seq": data[0]["seq"],
        }

    client_isns = {v["client_isn"] for v in flows.values()}
    server_isns = {v["server_isn"] for v in flows.values()}
    first_data_seq = {v["first_data_seq"] for v in flows.values()}
    if len(client_isns) != len(expected):
        raise AssertionError(f"client ISNs not independent: {flows}")
    if len(server_isns) != len(expected):
        raise AssertionError(f"server ISNs not independent: {flows}")
    if len(first_data_seq) != len(expected):
        raise AssertionError(f"data sequence spaces not independent: {flows}")

    out = {
        "server_port": args.server_port,
        "expected_client_ports": sorted(expected),
        "flow_count": len(flows),
        "flows": {str(k): v for k, v in flows.items()},
        "outer_distinct": True,
    }
    text = json.dumps(out, indent=2, sort_keys=True)
    print(text)
    if args.out:
        args.out.write_text(text + "\n")
    print(f"WBD_GAME_LANE_PCAP_PASS flows={len(flows)} distinct_5tuple=1 distinct_seq_space=1")


if __name__ == "__main__":
    main()
