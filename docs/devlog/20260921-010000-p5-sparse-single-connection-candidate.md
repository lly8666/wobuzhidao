# P5 sparse single-connection candidate

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base docs HEAD: `2d284b00a7c1ceff7f6b7943ad50cc9cb6baf83d`
- Current product qualification: `e2c1d6685f678df8fa4157f97a29de54ef84b803` / Actions `35524123958` / 8/8 PASS
- Certificate-chain docs closure: `2d284b00a7c1ceff7f6b7943ad50cc9cb6baf83d` / Actions `35524275373` / 8/8 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Atom scope

This atom covers only the P5 **sparse single-connection** workload dimension.

It adds a second, independent measurement scenario instead of mutating the already-qualified certificate-chain/full-resumed scenario. The existing `TestP5ControlledHTTPSMeasurementHarness` and validator continue to run unchanged.

The new sparse scenario uses:

- one established outer FakeTCP + protected TLS/admission Normal lane;
- one inner BusinessFlow;
- one real TLS 1.3 connection with normal certificate verification;
- one HTTP/1.1 keep-alive connection;
- three sequential GET requests;
- no concurrent business flow;
- FEC off;
- production padding off;
- no weak-network injection.

## Sparse arrival schedule

The controlled request workload uses application-idle targets:

- request 1: 0 ms prior idle;
- request 2: 150 ms after the previous response completes;
- request 3: 300 ms after the previous response completes.

The implementation does **not** use `time.Sleep`. It computes an explicit application-arrival deadline relative to the previous response completion and waits on a timer only in the test workload generator.

This spacing is measurement input, not transport behavior:

- the transport does not wait for another business packet;
- the transport does not batch;
- no production pacing or padding policy changes;
- manifest field `transport_batch_wait=none`;
- arrival model is recorded as `explicit-deadline-timer-after-previous-response`.

The validator requires actual application idle to be at least the scheduled workload gap.

## Persistent-flow evidence

The inner TCP flow is opened exactly once before the TLS handshake.

After every response, before the next scheduled request, the harness requires:

- client `TCPFlows()==1`;
- server `TCPFlows()==1`.

Only after the third response does the client close TLS. The harness then requires both flow registries to reach zero.

All three request events carry the same positive BusinessFlowID. The validator rejects any second flow, any concurrency, or a changed outer connection.

## Raw event schema

The sparse artifact uses the same `wbd-p5-https-measurement/v1` raw PCAP and outer/TLS-like event stream, with two additional event types:

- `https_sparse_request`
- `https_sparse_connection`

Each request records:

- request ordinal;
- persistent BusinessFlowID;
- outer connection ID;
- scheduled application gap;
- actual application idle;
- request start and completion timestamps;
- request-response/business latency;
- HTTP status and response bytes;
- per-request c2s/s2c wire bytes.

The connection event records:

- the single BusinessFlowID;
- request count;
- close completion;
- TLS version/cipher;
- full/resumed state;
- handshake timing;
- verified controlled certificate-chain fingerprints.

The manifest separately records application loss/duplicates, capture loss, injected network drop, FEC recovery, repair retransmits and padding bytes.

## Actions gate

The existing `p5-https-measurement-base` Actions job is extended, not replaced.

It now runs both:

- the already-qualified 3-flow certificate/full-resumed test and `tools/check_p5_measurement.py`;
- the new sparse single-connection test and `tools/check_p5_sparse_measurement.py`.

The sparse raw files are stored separately under `artifacts/p5-sparse-single`. The Actions artifact uploads both directories.

Expected sparse validator marker:

`WBD_P5_SPARSE_SINGLE_CONNECTION_PASS ... requests=3 business_flows=1 outer_connections=1 scheduled_idle_ms=0,150,300 fec=off padding=off`.

## Qualification boundary

No local result is qualification authority. The exact candidate SHA must pass all eight `next-foundation` jobs, including both P5 validators.

This atom does not cover:

- natural concurrency;
- load or soak;
- weak-network conditions;
- FEC-on;
- padding-on;
- classifier work;
- performance conclusions.

Windows/Npcap physical remains P7 `NOT_RUN`; OpenWrt IPv6 remains `NOT_IMPLEMENTED`. No `old/` implementation is reused, so `docs/REUSE_LEDGER.json` is unchanged.
