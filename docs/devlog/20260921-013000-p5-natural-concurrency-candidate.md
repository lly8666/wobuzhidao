# P5 natural concurrency candidate

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base docs HEAD: `8cb12a3d8cb266a0cf570221506d1eec56bd9cb3`
- Current product qualification: `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e` / Actions `35524525129` / 8/8 PASS
- Sparse docs-only closure retry: `8cb12a3d8cb266a0cf570221506d1eec56bd9cb3` / Actions `35529614401` / 8/8 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Atom scope

This atom covers only P5 **natural concurrency**.

It adds three independent real inner HTTPS connections / BusinessFlows over one already-established outer FakeTCP + protected TLS/admission Normal lane.

It does not add:

- FEC-on;
- weak-network injection;
- a load matrix;
- soak/long-run duration;
- classifier work;
- padding enablement;
- transport pacing or packet batching.

Production padding remains `0/off`.

## Application start model

Each worker independently:

1. opens a distinct inner TCP BusinessFlow;
2. performs a real TLS 1.3 full handshake;
3. verifies the same controlled Root -> Intermediate -> leaf `target.test` certificate chain;
4. reports ready.

Only after all three workers are ready and both platformflow registries actually report three active TCP BusinessFlows does the application close one start-barrier channel.

Workers then immediately issue one real HTTPS request each.

There is no fixed sleep in the concurrency generator and no transport wait-for-peer/batching behavior.

The controlled server returns a fixed 128 KiB body per request. This is workload payload, not a transport delay. It makes the request intervals measurable without introducing a timer-based response delay.

## Concurrency evidence

The raw concurrency events record, per BusinessFlow:

- distinct positive BusinessFlowID;
- request start and completion timestamps;
- business latency;
- HTTP status and response bytes;
- TLS handshake time/version/cipher and `DidResume=false`;
- certificate verification provenance;
- normal final close;
- per-flow inner TLS ciphertext bytes in each direction.

A summary event records:

- application barrier release timestamp;
- actual simultaneous client BusinessFlow count;
- actual simultaneous server BusinessFlow count;
- maximum overlapping request intervals calculated from raw timestamps.

The independent validator recalculates interval overlap from raw events and requires at least two request intervals to genuinely overlap. It also requires both client and server registries to have held all three BusinessFlows concurrently.

## Per-flow wire-byte scope

There is no active-tree primitive that can truthfully attribute all encrypted **outer** wire records to one BusinessFlow when flows overlap.

Therefore this atom does not subtract overlapping global outer counters and call the result per-flow outer cost.

Instead:

- per-flow wire bytes are counted exactly at each independent inner TLS socket and labeled `inner-tls-ciphertext-counted-at-application-socket`;
- aggregate outer wire bytes remain separately recorded by the existing SegmentIO recorder;
- raw outer PCAP and TLS-like record events remain available for aggregate/later attribution analysis.

This preserves the distinction between per-flow inner TLS traffic and tunnel-wide outer wire cost.

## Actions gate

The existing dedicated P5 measurement job is strengthened in place. It now runs and independently validates:

1. certificate/full-resumed measurement;
2. sparse single-connection measurement;
3. natural-concurrency measurement.

The combined artifact upload adds `artifacts/p5-natural-concurrency` with:

- `outer.pcap`;
- `events.jsonl`;
- `manifest.json`;
- `summary.json`;
- `test.log`.

Expected concurrency markers include:

`WBD_P5_NATURAL_CONCURRENCY_CAPTURED ... flows=3 outer_connections=1 max_business_flows=3 ...`

and

`WBD_P5_NATURAL_CONCURRENCY_PASS ... flows=3 outer_connections=1 max_business_flows=3 ...`.

## Qualification boundary

The candidate must pass the complete exact-SHA `next-foundation` workflow. The prior P5 certificate/full-resumed and sparse validators remain required on the same SHA.

No local result is qualification authority. No `old/` implementation is reused, so `docs/REUSE_LEDGER.json` is unchanged.

Windows/Npcap physical remains P7 `NOT_RUN`; OpenWrt IPv6 remains `NOT_IMPLEMENTED`.
