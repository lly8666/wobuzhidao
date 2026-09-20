# P5 sparse single-connection atom closed

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e`
- GitHub Actions: `35524525129`
- Result: **8/8 PASS**
- Previous product qualification: `e2c1d6685f678df8fa4157f97a29de54ef84b803`
- This closure commit is docs-only and must not replace the product qualification SHA.

## Closed atom

The fifth P5 atom is closed: controlled sparse real-HTTPS traffic on one persistent inner connection.

The qualified sparse scenario uses:

- one already-established outer FakeTCP + protected TLS/admission Normal lane;
- one inner BusinessFlow;
- one TLS 1.3 full handshake with normal controlled certificate-chain verification;
- one HTTP/1.1 keep-alive connection;
- three sequential real GET requests;
- zero business-flow concurrency.

The three application arrival targets are 0 ms, 150 ms and 300 ms, where requests 2/3 are scheduled relative to completion of the previous response.

## Arrival timing boundary

The request spacing is test-workload input only. The implementation computes a deadline and waits on a timer in the measurement generator.

It does not alter production transport pacing, batching, repair, FEC, or padding. The manifest records:

- `application_arrival_model=explicit-deadline-timer-after-previous-response`;
- `transport_batch_wait=none`;
- the exact scheduled idle vector.

The validator requires actual measured application idle to be at least the corresponding requested workload gap.

## Persistent BusinessFlow evidence

The inner BusinessFlow is opened exactly once before the TLS handshake.

After each of the three HTTP responses and before final TLS close, both:

- client `TCPFlows()==1`;
- server `TCPFlows()==1`.

All three request events use the same positive BusinessFlowID. After the third response the connection closes normally and both flow registries converge to zero.

The sparse validator independently rejects a changed BusinessFlowID, a second outer connection, concurrency, incorrect timing order, or missing request/response wire evidence.

## Raw evidence

The sparse artifact adds two event types to the existing raw capture schema:

- `https_sparse_request`: request ordinal, persistent BusinessFlowID, scheduled gap, actual application idle, request start/complete timestamps, business latency, HTTP status/response bytes and per-request c2s/s2c wire bytes;
- `https_sparse_connection`: request count, flow close state, TLS version/cipher, full-handshake status, handshake timing and verified controlled certificate-chain identities.

The manifest separately records:

- application requests/responses/loss/duplicates;
- capture loss;
- injected network drop;
- FEC recovery;
- repair retransmits;
- padding bytes;
- total outer wire bytes.

## Exact-SHA evidence

Actions `35524525129` completed with all eight jobs PASS.

The existing certificate/full-resumed P5 regression emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e flows=3 outer_connections=1 sequential_close=pass handshakes=full,resumed,full certificate_chains=2 fec=off padding=off records=61`

The sparse harness emitted:

`WBD_P5_SPARSE_SINGLE_CONNECTION_CAPTURED source_sha=d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e requests=3 business_flows=1 outer_connections=1 tls=full fec=off padding=off records=32`

The sparse validator emitted:

`WBD_P5_SPARSE_SINGLE_CONNECTION_PASS source_sha=d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e requests=3 business_flows=1 outer_connections=1 scheduled_idle_ms=0,150,300 fec=off padding=off`

Combined P5 artifact:

- ID `10609244483`
- size `47273` bytes
- digest `sha256:493548237ae84353bd01f2258206a5d74ba9d6bb86286b1dd33a774d16921b19`
- ten files across the existing certificate/full-resumed directory and the new sparse directory.

The same SHA passed repository-contract, Windows active Go tests, Ubuntu unit/build/race/directed fuzz/reference, P2 kernel fallback, OpenWrt privileged TPROXY/SocketTunnel and Linux shared-TUN privileged iptables/nft.

## Non-claims

- No natural-concurrency qualification yet.
- No FEC-on result.
- No weak-network, load or soak result.
- No classifier or indistinguishability claim.
- No production padding enablement; padding remains `0/off`.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No `old/` implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Cover only controlled **natural concurrency**.

Use several independent inner HTTPS connections / BusinessFlows over the same already-established outer lane. Application-side workers may synchronize their start, but no transport code may wait for peers or batch traffic.

Evidence must show distinct BusinessFlowIDs whose request intervals actually overlap, record maximum simultaneous active flows, preserve one outer connection, and keep TLS1.3, FEC off, production padding off and network injection disabled.

Do not mix in FEC-on, weak-network matrices, load/soak, classifier work or performance conclusions.
