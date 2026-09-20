#!/usr/bin/env python3
import argparse
import json
import os
import struct

SCHEMA = "wbd-p5-https-measurement/v1"
SCHEDULED_IDLE_NS = [0, 150_000_000, 300_000_000]


def fail(msg: str) -> None:
    raise SystemExit(f"P5 sparse measurement validation failed: {msg}")


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
        fail("harness SHA must equal committed sparse harness SOURCE_SHA")

    scenario = manifest.get("scenario", {})
    certificate = manifest.get("certificate_scenario", {})
    application = manifest.get("application", {})
    capture = manifest.get("capture", {})
    transport = manifest.get("transport", {})

    if scenario.get("name") != "controlled-real-https-sparse-single-connection":
        fail("scenario name")
    if scenario.get("https_connections") != 1:
        fail("expected one inner HTTPS connection")
    if scenario.get("https_requests") != 3:
        fail("expected three sparse requests")
    if scenario.get("business_flow_count") != 1:
        fail("expected one persistent business flow")
    if scenario.get("outer_connection_count") != 1:
        fail("expected one reused outer connection")
    if scenario.get("max_concurrent_business_flows") != 1:
        fail("sparse scenario must have no business concurrency")
    if scenario.get("application_arrival_model") != "explicit-deadline-timer-after-previous-response":
        fail("application arrival model")
    if scenario.get("scheduled_idle_ns") != SCHEDULED_IDLE_NS:
        fail("scheduled idle plan")
    if scenario.get("transport_batch_wait") != "none":
        fail("transport must not wait to batch sparse traffic")
    if scenario.get("tls_version") != "TLS1.3" or scenario.get("handshake_mode") != "full":
        fail("sparse connection must start with one TLS1.3 full handshake")
    if scenario.get("certificate_chain_id") != "sparse-chain":
        fail("certificate chain identity")
    if scenario.get("fec_parity_shards") != 0:
        fail("sparse atom must keep FEC off")
    if scenario.get("padding") != "off":
        fail("sparse atom must keep padding off")
    if scenario.get("network_injection") != "none":
        fail("sparse atom must not inject weak-network conditions")

    if certificate.get("chain_id") != "sparse-chain":
        fail("certificate scenario chain id")
    if certificate.get("server_name") != "target.test":
        fail("certificate server name")
    if certificate.get("verified_chain_length") != 3:
        fail("certificate verified chain length")
    for field in ("root_sha256", "intermediate_sha256", "leaf_sha256"):
        value = certificate.get(field, "")
        if not isinstance(value, str) or len(value) != 64:
            fail(f"certificate {field}")

    if application.get("requests") != 3 or application.get("responses") != 3:
        fail("application request/response count")
    if application.get("loss") != 0 or application.get("duplicates") != 0:
        fail("application loss/duplicate accounting")
    if application.get("response_bytes", 0) <= 0:
        fail("application response bytes")
    if application.get("request_wire_bytes_c2s", 0) <= 0 or application.get("request_wire_bytes_s2c", 0) <= 0:
        fail("application wire bytes")

    if capture.get("capture_loss_packets") != 0:
        fail("capture loss must be separate and zero for in-process capture")
    if capture.get("network_drop_injected") != 0:
        fail("network drop must remain separate and zero")
    if capture.get("client_initial_syns") != 1:
        fail("sparse workload created another outer connection")
    if capture.get("outer_wire_bytes_c2s", 0) <= 0 or capture.get("outer_wire_bytes_s2c", 0) <= 0:
        fail("outer wire byte inventory")
    if transport.get("fec_recovery") != 0:
        fail("FEC recovery must be zero when FEC is off")
    if transport.get("padding_bytes") != 0:
        fail("padding bytes must be zero when padding is off")

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
    requests = [e for e in events if e.get("event") == "https_sparse_request"]
    connections = [e for e in events if e.get("event") == "https_sparse_connection"]

    if not outer or {e.get("direction") for e in outer} != {"c2s", "s2c"}:
        fail("outer packet events must cover both directions")
    if not records or {e.get("direction") for e in records} != {"c2s", "s2c"}:
        fail("TLS-like record events must cover both directions")
    if len(connections) != 1:
        fail("expected one sparse connection event")
    if len(requests) != 3:
        fail("expected three sparse request events")

    connection = connections[0]
    flow_id = connection.get("business_flow_id", 0)
    if not isinstance(flow_id, int) or flow_id <= 0:
        fail("persistent business flow id")
    if connection.get("outer_connection_id") != 1:
        fail("connection outer identity")
    if connection.get("request_count") != 3 or connection.get("flow_closed") is not True:
        fail("connection request count/close evidence")
    if connection.get("tls_version") != 0x0304:
        fail("connection TLS version")
    if connection.get("handshake_mode") != "full" or connection.get("tls_resumed") is not False:
        fail("connection handshake mode")
    if connection.get("handshake_ns", 0) <= 0:
        fail("connection handshake timing")
    if connection.get("certificate_chain_id") != certificate.get("chain_id"):
        fail("connection certificate chain id")
    if connection.get("certificate_scenario") != certificate.get("name"):
        fail("connection certificate scenario")
    if connection.get("certificate_verified") is not True or connection.get("verified_chain_length") != 3:
        fail("connection certificate verification")
    if connection.get("verified_root_sha256") != certificate.get("root_sha256"):
        fail("connection verified root")
    if connection.get("verified_intermediate_sha256") != certificate.get("intermediate_sha256"):
        fail("connection verified intermediate")
    if connection.get("verified_leaf_sha256") != certificate.get("leaf_sha256"):
        fail("connection verified leaf")

    requests.sort(key=lambda e: e.get("request_ordinal", 0))
    last_start = -1
    last_complete = -1
    for i, event in enumerate(requests, 1):
        if event.get("request_ordinal") != i:
            fail("request ordinal")
        if event.get("business_flow_id") != flow_id:
            fail("requests did not stay on one business flow")
        if event.get("outer_connection_id") != 1:
            fail("request outer identity")
        if event.get("scheduled_gap_ns", 0) != SCHEDULED_IDLE_NS[i - 1]:
            fail("request scheduled gap")
        actual_idle = event.get("application_idle_ns", 0)
        if i == 1:
            if actual_idle != 0:
                fail("first request must not have prior application idle")
        elif actual_idle < SCHEDULED_IDLE_NS[i - 1]:
            fail("actual sparse application idle shorter than scheduled")
        start = event.get("request_start_ns", 0)
        complete = event.get("request_complete_ns", 0)
        if start <= last_start or complete <= start or complete <= last_complete:
            fail("request timing order")
        last_start = start
        last_complete = complete
        if event.get("business_latency_ns", 0) <= 0:
            fail("request business latency")
        if event.get("http_status") != 200 or event.get("response_bytes", 0) <= 0:
            fail("request response evidence")
        if event.get("wire_bytes_c2s", 0) <= 0 or event.get("wire_bytes_s2c", 0) <= 0:
            fail("request wire byte evidence")

    with open(args.pcap, "rb") as f:
        header = f.read(24)
    if len(header) != 24:
        fail("pcap header missing")
    if struct.unpack("<I", header[:4])[0] != 0xA1B2C3D4:
        fail("pcap magic")
    if os.path.getsize(args.pcap) <= 24:
        fail("pcap has no packets")

    summary = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "scenario": "sparse-single-connection",
        "https_connections": 1,
        "https_requests": 3,
        "business_flows": 1,
        "outer_connections": 1,
        "scheduled_idle_ns": SCHEDULED_IDLE_NS,
        "outer_packet_events": len(outer),
        "tlslike_record_events": len(records),
        "capture_loss_packets": capture["capture_loss_packets"],
        "network_drop_injected": capture["network_drop_injected"],
        "fec_recovery": transport["fec_recovery"],
        "padding_bytes": transport["padding_bytes"],
        "application_loss": application["loss"],
        "application_duplicates": application["duplicates"],
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_SPARSE_SINGLE_CONNECTION_PASS source_sha={args.source_sha} "
        "requests=3 business_flows=1 outer_connections=1 scheduled_idle_ms=0,150,300 "
        "fec=off padding=off"
    )


if __name__ == "__main__":
    main()
