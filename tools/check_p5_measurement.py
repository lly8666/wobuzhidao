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
    tls_session = manifest.get("tls_session", {})
    certificate_scenarios = manifest.get("certificate_scenarios", [])
    transport = manifest.get("transport", {})
    if scenario.get("https_flows") != 3:
        fail("expected exactly three controlled HTTPS flows")
    if scenario.get("outer_connection_count") != 1:
        fail("expected one reused outer connection")
    if scenario.get("sequential_close_before_next") is not True:
        fail("expected first HTTPS flow to close before the subsequent flow")
    if scenario.get("fec_parity_shards") != 0:
        fail("first atom must keep FEC off")
    if scenario.get("padding") != "off":
        fail("first atom must keep padding off")
    if scenario.get("network_injection") != "none":
        fail("handshake atom must not run weak-network injection")
    if scenario.get("tls_version") != "TLS1.3":
        fail("handshake atom must pin TLS1.3")
    if scenario.get("handshake_modes") != ["full", "resumed", "full"]:
        fail("expected preserved full/resumed plus different-chain full handshake modes")
    if scenario.get("session_cache") != "shared-lru" or scenario.get("session_cache_capacity") != 8:
        fail("session cache provenance")
    if scenario.get("certificate_chains") != ["chain-a", "chain-b"]:
        fail("certificate chain scenario list")
    if scenario.get("certificate_comparison_flows") != [1, 3]:
        fail("certificate comparison flow provenance")
    if tls_session.get("cache_puts", 0) <= 0 or tls_session.get("cache_hits", 0) <= 0:
        fail("manifest session cache did not prove ticket storage and reuse")
    chain_cache = tls_session.get("chains", {})
    if chain_cache.get("chain-a", {}).get("cache_puts", 0) <= 0 or chain_cache.get("chain-a", {}).get("cache_hits", 0) <= 0:
        fail("chain-a cache must prove full ticket storage and resumed reuse")
    if chain_cache.get("chain-b", {}).get("cache_puts", 0) <= 0 or chain_cache.get("chain-b", {}).get("cache_hits", 0) != 0:
        fail("chain-b must be an independent full handshake without a resume hit")
    if len(certificate_scenarios) != 2:
        fail("expected exactly two controlled certificate scenarios")
    cert_by_id = {entry.get("chain_id"): entry for entry in certificate_scenarios}
    if set(cert_by_id) != {"chain-a", "chain-b"}:
        fail("certificate scenario identities")
    for chain_id, expected_flows in (("chain-a", [1, 2]), ("chain-b", [3])):
        entry = cert_by_id[chain_id]
        if entry.get("server_name") != "target.test" or entry.get("verified_chain_length") != 3:
            fail(f"{chain_id} certificate verification provenance")
        if entry.get("flow_ordinals") != expected_flows:
            fail(f"{chain_id} flow provenance")
        for field in ("root_sha256", "intermediate_sha256", "leaf_sha256"):
            value = entry.get(field, "")
            if not isinstance(value, str) or len(value) != 64:
                fail(f"{chain_id} {field}")
    for field in ("root_sha256", "intermediate_sha256", "leaf_sha256"):
        if cert_by_id["chain-a"][field] == cert_by_id["chain-b"][field]:
            fail(f"independent certificate chains share {field}")
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
    if len(flows) != 3:
        fail("expected exactly three HTTPS flow events")
    flows.sort(key=lambda e: e.get("flow_ordinal", 0))
    expected = [
        "first_https_flow_on_initial_outer_connection",
        "subsequent_https_flow_after_first_close_existing_lane",
        "different_certificate_chain_full_handshake_existing_lane",
    ]
    if [e.get("scenario") for e in flows] != expected:
        fail("HTTPS flow scenario labels")
    flow_ids = set()
    expected_modes = ["full", "resumed", "full"]
    expected_resumed = [False, True, False]
    expected_chain_ids = ["chain-a", "chain-a", "chain-b"]
    for i, e in enumerate(flows, 1):
        if e.get("flow_ordinal") != i or e.get("outer_connection_id") != 1:
            fail("flow ordinal/outer connection identity")
        if e.get("flow_closed") is not True:
            fail("HTTPS flow was not fully closed before measurement completion")
        if e.get("tls_version") != 0x0304:
            fail("HTTPS flow TLS version is not TLS1.3")
        if e.get("handshake_mode") != expected_modes[i - 1]:
            fail("HTTPS flow handshake mode")
        if e.get("tls_resumed") is not expected_resumed[i - 1]:
            fail("HTTPS flow resume state")
        if e.get("session_cache_gets", 0) <= 0:
            fail("HTTPS flow session cache lookup provenance")
        if i in (1, 3) and e.get("session_cache_puts", 0) <= 0:
            fail("full handshake did not store a TLS session ticket")
        if i == 2 and e.get("session_cache_hits", 0) <= 0:
            fail("resumed handshake did not use cached session state")
        if i == 3 and e.get("session_cache_hits", 0) != 0:
            fail("different-chain full handshake unexpectedly resumed")
        chain_id = e.get("certificate_chain_id")
        if chain_id != expected_chain_ids[i - 1]:
            fail("HTTPS flow certificate chain identity")
        if e.get("certificate_verified") is not True or e.get("verified_chain_length") != 3:
            fail("HTTPS flow certificate verification evidence")
        cert_entry = cert_by_id[chain_id]
        if e.get("certificate_scenario") != cert_entry.get("name"):
            fail("HTTPS flow certificate scenario identity")
        if e.get("verified_root_sha256") != cert_entry.get("root_sha256"):
            fail("HTTPS flow verified root identity")
        if e.get("verified_intermediate_sha256") != cert_entry.get("intermediate_sha256"):
            fail("HTTPS flow verified intermediate identity")
        if e.get("verified_leaf_sha256") != cert_entry.get("leaf_sha256"):
            fail("HTTPS flow verified leaf identity")
        flow_id = e.get("business_flow_id", 0)
        if not isinstance(flow_id, int) or flow_id <= 0:
            fail("business flow id evidence")
        flow_ids.add(flow_id)
        if e.get("http_status") != 200 or e.get("response_bytes", 0) <= 0:
            fail("real HTTPS response evidence")
        if e.get("business_latency_ns", 0) <= 0 or e.get("handshake_ns", 0) <= 0:
            fail("business/handshake timing evidence")
        if e.get("wire_bytes_c2s", 0) <= 0 or e.get("wire_bytes_s2c", 0) <= 0:
            fail("per-flow wire byte evidence")
    if len(flow_ids) != 3:
        fail("expected three distinct inner TCP flow ids")

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
        "sequential_close_before_next": True,
        "handshake_modes": ["full", "resumed", "full"],
        "certificate_chains": ["chain-a", "chain-b"],
        "tls_version": "TLS1.3",
        "session_cache_gets": tls_session["cache_gets"],
        "session_cache_hits": tls_session["cache_hits"],
        "session_cache_puts": tls_session["cache_puts"],
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
        "flows=3 outer_connections=1 sequential_close=pass handshakes=full,resumed,full "
        "certificate_chains=2 fec=off padding=off"
    )


if __name__ == "__main__":
    main()
