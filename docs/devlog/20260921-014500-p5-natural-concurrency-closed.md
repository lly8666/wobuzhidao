# P5 natural concurrency atom closed

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `24e6ff71d76b53d63719f83c5df8d2147a383977`
- GitHub Actions: `35529970723`
- Result: **8/8 PASS**
- Previous product qualification: `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e`
- This closure commit is docs-only and must not replace the product qualification SHA.

## Closed atom

The sixth P5 atom is closed: controlled natural concurrency for real inner HTTPS traffic.

Three independent BusinessFlows share one already-established outer FakeTCP + protected TLS/admission Normal lane. Each flow first completes a real TLS 1.3 full handshake against the controlled verified certificate chain and then waits only on an application-side start barrier. Once all three workers are ready and both client/server platformflow registries report three active flows, the barrier is released and each worker immediately sends its HTTPS request.

There is no fixed sleep in the concurrency generator and no transport-side wait-for-peer or batching rule.

## Raw concurrency evidence

Each flow records:

- distinct positive BusinessFlowID;
- TLS 1.3 full-handshake and controlled certificate verification;
- request start and completion timestamps;
- business latency;
- HTTP status and response bytes;
- inner TLS ciphertext bytes per direction;
- normal close state.

A summary event records the barrier release and simultaneous client/server flow counts.

The independent validator does not trust the manifest's overlap claim. It recomputes maximum overlap directly from raw request intervals:

`request_start_ns <= point < request_complete_ns`

and requires the manifest, summary event and application inventory to equal the recomputed value.

For the qualifying run the recomputed maximum overlap is **3**.

## Exact-SHA evidence

Actions `35529970723` completed with all eight jobs PASS.

Natural-concurrency harness marker:

`WBD_P5_NATURAL_CONCURRENCY_CAPTURED source_sha=24e6ff71d76b53d63719f83c5df8d2147a383977 flows=3 outer_connections=1 max_business_flows=3 max_overlapping_requests=3 tls=full fec=off padding=off records=458`

Independent validator marker:

`WBD_P5_NATURAL_CONCURRENCY_PASS source_sha=24e6ff71d76b53d63719f83c5df8d2147a383977 flows=3 outer_connections=1 max_business_flows=3 max_overlapping_requests=3 tls=full fec=off padding=off`

The prior certificate/full-resumed and sparse validators also passed on the same SHA.

Combined P5 artifact:

- ID `10610588812`
- size `530353` bytes
- digest `sha256:f4f99ee48fb3eed2aad15f97fdc547f7cd7138921f1bf8de6f39d5d0c8d6dc9f`.

The same SHA also passed repository-contract, Windows active Go tests, Ubuntu unit/build/race/directed fuzz/reference, P2 kernel fallback, OpenWrt privileged TPROXY/SocketTunnel, and Linux shared-TUN privileged iptables/nft.

## Wire-byte attribution boundary

When multiple inner flows overlap, the active tree has no truthful primitive that assigns every encrypted outer record to one BusinessFlow.

Therefore the qualified per-flow byte counters are explicitly **inner TLS ciphertext at the application socket**, while tunnel-wide outer packet/record bytes remain aggregate evidence in the raw PCAP/events. The atom does not mislabel overlapping global outer deltas as per-flow outer cost.

## Non-claims

- No FEC-on qualification yet.
- No weak-network matrix yet.
- No load/soak or long-run result.
- No performance conclusion from the concurrency sample.
- No classifier or indistinguishability claim.
- Production padding remains `0/off`.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No `old/` implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Cover only the actual fixed FEC profile used by the active product policy under controlled real HTTPS and no network loss.

First inspect the active runtime/config to identify whether an actual enabled fixed profile exists. Do not tune or compare profiles. If no product selection exists, record that fact and qualify one explicit fixed profile only as capability/cost evidence, without calling it a production default or preferred profile.

Keep padding `0/off`, preserve raw capture/inventory separation, and defer weak-network injection, load and soak to later atoms.
