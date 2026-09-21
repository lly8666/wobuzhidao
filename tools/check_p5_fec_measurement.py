#!/usr/bin/env python3
import argparse
import json
import math
import os
import struct

SCHEMA = "wbd-p5-https-measurement/v1"
PROFILE = "20:10"
PARITY = 10
RESPONSE_BYTES = 256 * 1024


def fail(msg: str) -> None:
    raise SystemExit(f"P5 fixed-FEC measurement validation failed: {msg}")


def positive_int(value, name: str) -> int:
    if not isinstance(value, int) or value <= 0:
        fail(name)
    return value


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
        fail("harness SHA must equal committed fixed-FEC harness SOURCE_SHA")

    scenario = manifest.get("scenario", {})
    certificate = manifest.get("certificate_scenario", {})
    application = manifest.get("application", {})
    capture = manifest.get("capture", {})
    fec = manifest.get("fec", {})
    transport = manifest.get("transport", {})
    cost = manifest.get("cost", {})

    if scenario.get("name") != "controlled-real-https-fixed-fec-20x10-lossless":
        fail("scenario name")
    if scenario.get("https_connections") != 1 or scenario.get("https_requests") != 1:
        fail("expected one controlled HTTPS connection/request")
    if scenario.get("business_flow_count") != 1 or scenario.get("outer_connection_count") != 1:
        fail("business/outer connection count")
    if scenario.get("desired_lanes") != 1:
        fail("fixed-FEC atom must stay on one Normal lane")
    if scenario.get("fec_profile") != PROFILE or scenario.get("fec_data_shards") != 20 or scenario.get("fec_parity_shards") != PARITY:
        fail("fixed FEC profile")
    if scenario.get("fec_flush_after_ns") != 8_000_000 or scenario.get("fec_max_blocks") != 8:
        fail("fixed FEC runtime bounds")
    if scenario.get("fec_profile_role") != "explicit-controlled-capability-cost-scenario-not-production-default":
        fail("FEC profile role")
    if scenario.get("production_default_fec") != "off":
        fail("production FEC default must remain off")
    if scenario.get("padding") != "off":
        fail("padding must remain off")
    if scenario.get("network_injection") != "none":
        fail("fixed-FEC atom must remain lossless/no-injection")
    if scenario.get("tls_version") != "TLS1.3" or scenario.get("handshake_mode") != "full":
        fail("TLS handshake mode")
    if scenario.get("certificate_chain_id") != "fec-chain":
        fail("certificate chain identity")
    if scenario.get("response_body_bytes") != RESPONSE_BYTES:
        fail("controlled response size")

    if certificate.get("chain_id") != "fec-chain":
        fail("certificate scenario chain id")
    if certificate.get("server_name") != "target.test" or certificate.get("verified_chain_length") != 3:
        fail("certificate verification provenance")
    for field in ("root_sha256", "intermediate_sha256", "leaf_sha256"):
        value = certificate.get(field, "")
        if not isinstance(value, str) or len(value) != 64:
            fail(f"certificate {field}")

    if application.get("requests") != 1 or application.get("responses") != 1:
        fail("application request/response inventory")
    if application.get("loss") != 0 or application.get("duplicates") != 0:
        fail("application loss/duplicate accounting")
    if application.get("response_bytes") != RESPONSE_BYTES:
        fail("application response bytes")
    if application.get("handshake_ns", 0) <= 0 or application.get("business_latency_ns", 0) <= 0:
        fail("application timing")
    inner_c2s = positive_int(application.get("inner_tls_wire_bytes_c2s"), "inner TLS c2s bytes")
    inner_s2c = positive_int(application.get("inner_tls_wire_bytes_s2c"), "inner TLS s2c bytes")

    if capture.get("capture_loss_packets") != 0:
        fail("capture loss must be separate and zero")
    if capture.get("network_drop_injected") != 0:
        fail("network drop must be separate and zero")
    if capture.get("client_initial_syns") != 1:
        fail("inner HTTPS flow created another outer connection")
    outer_c2s = positive_int(capture.get("steady_outer_wire_bytes_c2s"), "steady outer c2s bytes")
    outer_s2c = positive_int(capture.get("steady_outer_wire_bytes_s2c"), "steady outer s2c bytes")

    if fec.get("profile") != PROFILE:
        fail("FEC inventory profile")
    client_source = positive_int(fec.get("client_source_shards"), "client source shard inventory")
    client_parity = positive_int(fec.get("client_parity_shards"), "client parity shard inventory")
    server_source = positive_int(fec.get("server_source_shards"), "server source shard inventory")
    server_parity = positive_int(fec.get("server_parity_shards"), "server parity shard inventory")
    if fec.get("server_full_blocks", 0) <= 0:
        fail("large response did not form any full FEC block")
    if fec.get("client_pending_sources") != 0 or fec.get("server_pending_sources") != 0:
        fail("FEC partial block did not flush before snapshot")
    if fec.get("client_reconstruction_events") != 0 or fec.get("server_reconstruction_events") != 0:
        fail("lossless scenario unexpectedly reconstructed a block")
    if fec.get("client_recovered_sources") != 0 or fec.get("server_recovered_sources") != 0:
        fail("lossless scenario unexpectedly recovered missing sources")
    if fec.get("recovery_sources_total") != 0:
        fail("FEC recovery total must remain zero without injected loss")

    if transport.get("repair_retransmits") != 0:
        fail("lossless fixed-FEC scenario unexpectedly used repair")
    if transport.get("padding_bytes") != 0:
        fail("padding bytes must remain zero")

    numerator = positive_int(cost.get("wire_amplification_numerator_bytes"), "wire amplification numerator")
    denominator = positive_int(cost.get("wire_amplification_denominator_bytes"), "wire amplification denominator")
    if numerator != outer_c2s + outer_s2c:
        fail("wire amplification numerator does not equal steady outer wire bytes")
    if denominator != inner_c2s + inner_s2c:
        fail("wire amplification denominator does not equal inner TLS ciphertext bytes")
    ratio = cost.get("wire_amplification")
    if not isinstance(ratio, (int, float)) or not math.isfinite(ratio) or ratio <= 0:
        fail("wire amplification ratio")
    expected_ratio = numerator / denominator
    if not math.isclose(ratio, expected_ratio, rel_tol=1e-9, abs_tol=1e-12):
        fail("wire amplification ratio does not match raw byte inventory")
    if cost.get("wire_amplification_scope") != "steady-outer-ipv4-tcp-bytes-over-inner-tls-ciphertext-bytes":
        fail("wire amplification scope")

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
    flows = [e for e in events if e.get("event") == "https_fec_flow"]
    inventories = [e for e in events if e.get("event") == "fec_inventory"]

    if not outer or {e.get("direction") for e in outer} != {"c2s", "s2c"}:
        fail("outer packet events must cover both directions")
    if not records or {e.get("direction") for e in records} != {"c2s", "s2c"}:
        fail("TLS-like record events must cover both directions")
    if len(flows) != 1:
        fail("expected one fixed-FEC HTTPS flow event")
    if len(inventories) != 1:
        fail("expected one raw FEC inventory event")

    flow = flows[0]
    flow_id = flow.get("business_flow_id", 0)
    if not isinstance(flow_id, int) or flow_id <= 0:
        fail("business flow id")
    if flow.get("outer_connection_id") != 1:
        fail("flow outer connection identity")
    start = flow.get("request_start_ns", 0)
    complete = flow.get("request_complete_ns", 0)
    if start <= 0 or complete <= start or flow.get("business_latency_ns") != complete - start:
        fail("request timing/latency evidence")
    if flow.get("handshake_ns", 0) <= 0:
        fail("flow handshake timing")
    if flow.get("http_status") != 200 or flow.get("response_bytes") != RESPONSE_BYTES:
        fail("flow response evidence")
    if flow.get("inner_tls_wire_bytes_c2s") != inner_c2s or flow.get("inner_tls_wire_bytes_s2c") != inner_s2c:
        fail("flow inner TLS wire inventory")
    if flow.get("outer_wire_bytes_c2s") != outer_c2s or flow.get("outer_wire_bytes_s2c") != outer_s2c:
        fail("flow outer wire inventory")
    if flow.get("tls_version") != 0x0304 or flow.get("handshake_mode") != "full" or flow.get("tls_resumed") is not False:
        fail("flow TLS full-handshake evidence")
    if flow.get("certificate_chain_id") != certificate.get("chain_id") or flow.get("certificate_scenario") != certificate.get("name"):
        fail("flow certificate scenario identity")
    if flow.get("certificate_verified") is not True or flow.get("verified_chain_length") != 3:
        fail("flow certificate verification")
    if flow.get("verified_root_sha256") != certificate.get("root_sha256"):
        fail("flow verified root")
    if flow.get("verified_intermediate_sha256") != certificate.get("intermediate_sha256"):
        fail("flow verified intermediate")
    if flow.get("verified_leaf_sha256") != certificate.get("leaf_sha256"):
        fail("flow verified leaf")
    if flow.get("fec_parity_shards") != PARITY or flow.get("fec_flush_after_ns") != 8_000_000 or flow.get("fec_max_blocks") != 8:
        fail("flow fixed-FEC runtime provenance")
    if flow.get("repair_retransmits", 0) != 0 or flow.get("padding_bytes", 0) != 0:
        fail("flow repair/padding separation")
    if flow.get("wire_amplification_numerator_bytes") != numerator or flow.get("wire_amplification_denominator_bytes") != denominator:
        fail("flow wire amplification byte inventory")

    inventory = inventories[0]
    expected_inventory = {
        "fec_parity_shards": PARITY,
        "fec_flush_after_ns": 8_000_000,
        "fec_max_blocks": 8,
        "client_source_shards": client_source,
        "client_parity_shards": client_parity,
        "client_pending_sources": 0,
        "client_reconstruction_events": 0,
        "client_recovered_sources": 0,
        "server_source_shards": server_source,
        "server_parity_shards": server_parity,
        "server_pending_sources": 0,
        "server_reconstruction_events": 0,
        "server_recovered_sources": 0,
        "repair_retransmits": 0,
        "padding_bytes": 0,
        "wire_amplification_numerator_bytes": numerator,
        "wire_amplification_denominator_bytes": denominator,
    }
    for key, value in expected_inventory.items():
        if inventory.get(key) != value:
            fail(f"raw FEC inventory mismatch: {key}")
    if inventory.get("server_full_blocks", 0) <= 0:
        fail("raw FEC inventory lacks full server block")
    inv_ratio = inventory.get("wire_amplification")
    if not isinstance(inv_ratio, (int, float)) or not math.isclose(inv_ratio, expected_ratio, rel_tol=1e-9, abs_tol=1e-12):
        fail("raw inventory wire amplification ratio")

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
        "scenario": "fixed-fec-20x10-lossless",
        "profile": PROFILE,
        "https_connections": 1,
        "business_flows": 1,
        "outer_connections": 1,
        "client_source_shards": client_source,
        "client_parity_shards": client_parity,
        "server_source_shards": server_source,
        "server_parity_shards": server_parity,
        "fec_recovery_sources": 0,
        "repair_retransmits": 0,
        "padding_bytes": 0,
        "capture_loss_packets": capture["capture_loss_packets"],
        "network_drop_injected": capture["network_drop_injected"],
        "wire_amplification_numerator_bytes": numerator,
        "wire_amplification_denominator_bytes": denominator,
        "wire_amplification": expected_ratio,
        "outer_packet_events": len(outer),
        "tlslike_record_events": len(records),
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_FIXED_FEC_PASS source_sha={args.source_sha} profile=20:10 "
        f"flows=1 outer_connections=1 client_source={client_source} client_parity={client_parity} "
        f"server_source={server_source} server_parity={server_parity} "
        f"fec_recovery=0 repair=0 padding=off wire_amp={expected_ratio:.6f}"
    )


if __name__ == "__main__":
    main()
