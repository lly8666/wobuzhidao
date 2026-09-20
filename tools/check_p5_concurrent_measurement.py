#!/usr/bin/env python3
import argparse
import json
import os
import struct

SCHEMA = "wbd-p5-https-measurement/v1"
FLOW_COUNT = 3
RESPONSE_BYTES = 128 * 1024


def fail(msg: str) -> None:
    raise SystemExit(f"P5 natural concurrency validation failed: {msg}")


def max_overlap(flows) -> int:
    best = 0
    for pivot in flows:
        point = pivot["request_start_ns"]
        active = sum(
            1
            for candidate in flows
            if candidate["request_start_ns"] <= point < candidate["request_complete_ns"]
        )
        best = max(best, active)
    return best


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
        fail("harness SHA must equal committed concurrency harness SOURCE_SHA")

    scenario = manifest.get("scenario", {})
    certificate = manifest.get("certificate_scenario", {})
    application = manifest.get("application", {})
    capture = manifest.get("capture", {})
    transport = manifest.get("transport", {})

    if scenario.get("name") != "controlled-real-https-natural-concurrency":
        fail("scenario name")
    if scenario.get("https_connections") != FLOW_COUNT or scenario.get("https_requests") != FLOW_COUNT:
        fail("expected three independent HTTPS connections/requests")
    if scenario.get("business_flow_count") != FLOW_COUNT:
        fail("expected three business flows")
    if scenario.get("outer_connection_count") != 1:
        fail("expected one reused outer connection")
    if scenario.get("application_start_model") != "barrier-release-after-all-handshakes-ready":
        fail("application start model")
    if scenario.get("fixed_sleep") is not False:
        fail("natural concurrency must not use fixed sleep")
    if scenario.get("transport_batch_wait") != "none":
        fail("transport must not wait for peers/batching")
    if scenario.get("response_body_bytes") != RESPONSE_BYTES:
        fail("controlled response size")
    if scenario.get("max_concurrent_business_flows_client") != FLOW_COUNT:
        fail("client did not actually hold three concurrent business flows")
    if scenario.get("max_concurrent_business_flows_server") != FLOW_COUNT:
        fail("server did not actually hold three concurrent business flows")
    if scenario.get("per_flow_wire_scope") != "inner-tls-ciphertext-counted-at-application-socket":
        fail("per-flow wire-byte scope")
    if scenario.get("tls_version") != "TLS1.3" or scenario.get("handshake_mode") != "full":
        fail("concurrent flows must use TLS1.3 full handshakes")
    if scenario.get("certificate_chain_id") != "concurrent-chain":
        fail("certificate chain identity")
    if scenario.get("fec_parity_shards") != 0:
        fail("concurrency atom must keep FEC off")
    if scenario.get("padding") != "off":
        fail("concurrency atom must keep padding off")
    if scenario.get("network_injection") != "none":
        fail("concurrency atom must not inject weak-network conditions")

    if certificate.get("chain_id") != "concurrent-chain":
        fail("certificate scenario chain id")
    if certificate.get("server_name") != "target.test" or certificate.get("verified_chain_length") != 3:
        fail("certificate verification provenance")
    for field in ("root_sha256", "intermediate_sha256", "leaf_sha256"):
        value = certificate.get(field, "")
        if not isinstance(value, str) or len(value) != 64:
            fail(f"certificate {field}")

    if application.get("requests") != FLOW_COUNT or application.get("responses") != FLOW_COUNT:
        fail("application request/response inventory")
    if application.get("loss") != 0 or application.get("duplicates") != 0:
        fail("application loss/duplicate inventory")
    if application.get("response_bytes_per_flow") != RESPONSE_BYTES:
        fail("application response size")

    if capture.get("capture_loss_packets") != 0:
        fail("capture loss must remain separate and zero for in-process capture")
    if capture.get("network_drop_injected") != 0:
        fail("network drop must remain separate and zero")
    if capture.get("client_initial_syns") != 1:
        fail("concurrent inner flows created another outer connection")
    if capture.get("outer_wire_bytes_c2s", 0) <= 0 or capture.get("outer_wire_bytes_s2c", 0) <= 0:
        fail("aggregate outer wire-byte evidence")

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
    flows = [e for e in events if e.get("event") == "https_concurrent_flow"]
    summaries = [e for e in events if e.get("event") == "https_concurrency_summary"]

    if not outer or {e.get("direction") for e in outer} != {"c2s", "s2c"}:
        fail("outer packet events must cover both directions")
    if not records or {e.get("direction") for e in records} != {"c2s", "s2c"}:
        fail("TLS-like record events must cover both directions")
    if len(flows) != FLOW_COUNT:
        fail("expected three concurrent flow events")
    if len(summaries) != 1:
        fail("expected one concurrency summary event")

    flows.sort(key=lambda e: e.get("flow_ordinal", 0))
    flow_ids = set()
    for i, event in enumerate(flows, 1):
        if event.get("flow_ordinal") != i:
            fail("flow ordinal")
        flow_id = event.get("business_flow_id", 0)
        if not isinstance(flow_id, int) or flow_id <= 0:
            fail("business flow id")
        flow_ids.add(flow_id)
        if event.get("outer_connection_id") != 1:
            fail("flow outer connection identity")
        start = event.get("request_start_ns", 0)
        complete = event.get("request_complete_ns", 0)
        latency = event.get("business_latency_ns", 0)
        if start <= 0 or complete <= start or latency != complete - start:
            fail("flow request interval/latency evidence")
        if event.get("http_status") != 200 or event.get("response_bytes") != RESPONSE_BYTES:
            fail("flow HTTP response evidence")
        if event.get("inner_tls_wire_bytes_c2s", 0) <= 0 or event.get("inner_tls_wire_bytes_s2c", 0) <= 0:
            fail("per-flow inner TLS wire bytes")
        if event.get("handshake_ns", 0) <= 0:
            fail("flow handshake timing")
        if event.get("tls_version") != 0x0304 or event.get("handshake_mode") != "full" or event.get("tls_resumed") is not False:
            fail("flow TLS full-handshake evidence")
        if event.get("flow_closed") is not True:
            fail("flow close evidence")
        if event.get("certificate_chain_id") != certificate.get("chain_id"):
            fail("flow certificate chain id")
        if event.get("certificate_scenario") != certificate.get("name"):
            fail("flow certificate scenario")
        if event.get("certificate_verified") is not True or event.get("verified_chain_length") != 3:
            fail("flow certificate verification")
        if event.get("verified_root_sha256") != certificate.get("root_sha256"):
            fail("flow verified root")
        if event.get("verified_intermediate_sha256") != certificate.get("intermediate_sha256"):
            fail("flow verified intermediate")
        if event.get("verified_leaf_sha256") != certificate.get("leaf_sha256"):
            fail("flow verified leaf")

    if len(flow_ids) != FLOW_COUNT:
        fail("flows are not distinct BusinessFlowIDs")

    summary_event = summaries[0]
    if summary_event.get("request_count") != FLOW_COUNT or summary_event.get("outer_connection_id") != 1:
        fail("concurrency summary request/outer identity")
    if summary_event.get("application_start_model") != "barrier-release-after-all-handshakes-ready":
        fail("concurrency summary start model")
    barrier = summary_event.get("barrier_release_ns", 0)
    if barrier <= 0:
        fail("barrier release timestamp")
    if any(event["request_start_ns"] < barrier for event in flows):
        fail("request started before application barrier release")
    if summary_event.get("max_concurrent_business_flows_client") != FLOW_COUNT:
        fail("summary client business-flow concurrency")
    if summary_event.get("max_concurrent_business_flows_server") != FLOW_COUNT:
        fail("summary server business-flow concurrency")

    overlap = max_overlap(flows)
    if overlap < 2:
        fail("request intervals did not naturally overlap")
    if summary_event.get("max_overlapping_request_intervals") != overlap:
        fail("summary overlap does not match raw request intervals")
    if scenario.get("max_overlapping_request_intervals") != overlap:
        fail("manifest overlap does not match raw request intervals")
    if application.get("max_overlapping_request_intervals") != overlap:
        fail("application overlap does not match raw request intervals")

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
        "scenario": "natural-concurrency",
        "https_connections": FLOW_COUNT,
        "business_flows": FLOW_COUNT,
        "outer_connections": 1,
        "max_concurrent_business_flows": FLOW_COUNT,
        "max_overlapping_request_intervals": overlap,
        "outer_packet_events": len(outer),
        "tlslike_record_events": len(records),
        "capture_loss_packets": capture["capture_loss_packets"],
        "network_drop_injected": capture["network_drop_injected"],
        "fec_recovery": transport["fec_recovery"],
        "padding_bytes": transport["padding_bytes"],
        "application_loss": application["loss"],
        "application_duplicates": application["duplicates"],
        "per_flow_wire_scope": scenario["per_flow_wire_scope"],
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_NATURAL_CONCURRENCY_PASS source_sha={args.source_sha} "
        f"flows=3 outer_connections=1 max_business_flows=3 max_overlapping_requests={overlap} "
        "tls=full fec=off padding=off"
    )


if __name__ == "__main__":
    main()
