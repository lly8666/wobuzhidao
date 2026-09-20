#!/usr/bin/env python3
import argparse
import json
import os
import struct

SCHEMA = "wbd-p5-https-measurement/v1"


def fail(msg: str) -> None:
    raise SystemExit(f"P5 measurement validation failed: {msg}")


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest", required=True)
    ap.add_argument("--events", required=True)
    ap.add_argument("--pcap", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    with open(args.manifest, "r", encoding="utf-8") as f:
        manifest = json.load(f)
    if manifest.get("schema") != SCHEMA:
        fail("manifest schema")
    if manifest.get("source_sha") != args.source_sha:
        fail("manifest SOURCE_SHA mismatch")
    if manifest.get("harness_sha") != args.source_sha:
        fail("harness SHA must equal committed harness SOURCE_SHA for this atom")

    scenario = manifest.get("scenario", {})
    capture = manifest.get("capture", {})
    transport = manifest.get("transport", {})
    if scenario.get("https_flows") != 2:
        fail("expected exactly two controlled HTTPS flows")
    if scenario.get("outer_connection_count") != 1:
        fail("expected one reused outer connection")
    if scenario.get("fec_parity_shards") != 0:
        fail("first atom must keep FEC off")
    if scenario.get("padding") != "off":
        fail("first atom must keep padding off")
    if scenario.get("network_injection") != "none":
        fail("first atom must not run weak-network injection")
    if capture.get("capture_loss_packets") != 0:
        fail("capture loss must be explicit and zero for in-process capture")
    if capture.get("network_drop_injected") != 0:
        fail("network drop must remain separate and zero in this atom")
    if capture.get("client_initial_syns") != 1:
        fail("subsequent HTTPS flow created another outer connection")
    if transport.get("fec_recovery") != 0:
        fail("FEC recovery inventory must be zero when FEC is off")
    if transport.get("padding_bytes") != 0:
        fail("padding bytes must be zero when production padding is off")

    events = []
    with open(args.events, "r", encoding="utf-8") as f:
        for line_no, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            event = json.loads(line)
            if event.get("schema") != SCHEMA:
                fail(f"event schema at line {line_no}")
            events.append(event)
    outer = [e for e in events if e.get("event") == "outer_packet"]
    records = [e for e in events if e.get("event") == "tlslike_record"]
    flows = [e for e in events if e.get("event") == "https_flow"]
    if not outer or {e.get("direction") for e in outer} != {"c2s", "s2c"}:
        fail("outer packet events must cover both directions")
    if not records or {e.get("direction") for e in records} != {"c2s", "s2c"}:
        fail("steady TLS-like record events must cover both directions")
    if len(flows) != 2:
        fail("expected exactly two HTTPS flow events")
    flows.sort(key=lambda e: e.get("flow_ordinal", 0))
    expected = [
        "first_https_flow_on_initial_outer_connection",
        "subsequent_https_flow_existing_lane",
    ]
    if [e.get("scenario") for e in flows] != expected:
        fail("HTTPS flow scenario labels")
    for i, e in enumerate(flows, 1):
        if e.get("flow_ordinal") != i or e.get("outer_connection_id") != 1:
            fail("flow ordinal/outer connection identity")
        if e.get("http_status") != 200 or e.get("response_bytes", 0) <= 0:
            fail("real HTTPS response evidence")
        if e.get("business_latency_ns", 0) <= 0 or e.get("handshake_ns", 0) <= 0:
            fail("business/handshake timing evidence")
        if e.get("wire_bytes_c2s", 0) <= 0 or e.get("wire_bytes_s2c", 0) <= 0:
            fail("per-flow wire byte evidence")

    with open(args.pcap, "rb") as f:
        header = f.read(24)
    if len(header) != 24:
        fail("pcap header missing")
    magic = struct.unpack("<I", header[:4])[0]
    if magic != 0xA1B2C3D4:
        fail("pcap magic")
    if os.path.getsize(args.pcap) <= 24:
        fail("pcap has no packets")

    summary = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "outer_packet_events": len(outer),
        "tlslike_record_events": len(records),
        "https_flows": len(flows),
        "outer_connections": 1,
        "capture_loss_packets": capture["capture_loss_packets"],
        "network_drop_injected": capture["network_drop_injected"],
        "fec_recovery": transport["fec_recovery"],
        "padding_bytes": transport["padding_bytes"],
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")
    print(
        f"WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha={args.source_sha} "
        "flows=2 outer_connections=1 fec=off padding=off"
    )


if __name__ == "__main__":
    main()
