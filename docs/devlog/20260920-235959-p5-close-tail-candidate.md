# P5 platformflow close-tail candidate

- Date: 2026-09-20
- Branch authority: `next/tlslike-dataplane`
- Base authority HEAD: `b3171a9f8f9b6b34c1794b680742f8ca25ad0258`
- Current product qualification: `043da6298a2be091b55fb470b63c69e605bb61d8`
- Product Actions: `35516700104` / 8/8 PASS
- Docs-only closure: `b3171a9f8f9b6b34c1794b680742f8ca25ad0258` / Actions `35517168976` / 8/8 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Why this atom exists

The first P5 measurement-base atom qualified two independent real HTTPS flows over one existing outer lane, but its failed second candidate preserved a concrete lifecycle defect:

`74f1b602bc664446f34d5a8c42e5a4d21391439e` / Actions `35516466731`

After the first inner HTTPS connection completed `Connection: close`, a legitimate tail frame could arrive after the corresponding platformflow server flow had already been removed. The old behavior treated non-Close frames for any missing FlowID as `platformflow: malformed frame: unknown TCP server flow`, which propagated upward and could terminate the runtime before the next HTTPS connection.

This atom fixes only that retired-flow lifecycle seam. It does not redesign TCP reliability.

## Minimal implementation

`internal/platformflow/tcp.go` adds a bounded retired-FlowID tombstone set shared by the client/server TCP lifecycle policy:

- default retention: 5 seconds;
- default cap: 4096 retired FlowIDs;
- retirement is recorded only when an actually active flow is removed;
- a recent retired FlowID makes late TCP Data/ACK/Close idempotent no-ops;
- on the server, a late duplicate TCPOpen for a recent retired FlowID is ignored and does not redial the upstream;
- an unknown FlowID that was never retired, or whose tombstone expired, still fails closed for Data/ACK;
- existing unknown TCPClose idempotence remains unchanged;
- oldest tombstones are evicted at capacity and expired tombstones are removed on lookup/add.

The 5s/4096 bounds are lifecycle metadata only. This atom does not change chunk size, send window, retransmit count, RTO, FEC profile, padding, lane count, or wire topology.

## Direct regression

`internal/platformflow/platformflow_test.go` adds a focused regression that checks:

- retired Data/ACK/Close are harmless on both client and server;
- a retired duplicate Open does not dial/recreate a server flow;
- an unrelated unknown ACK remains `ErrMalformed`;
- an expired tombstone no longer suppresses unknown-flow errors;
- tombstone capacity is bounded and evicts the oldest entry.

## P5 fullstack regression tightened

The existing dedicated P5 Actions gate is strengthened rather than replaced.

For each of the two real HTTPS flows, the harness now:

1. opens a distinct inner TCP connection through the existing `platformflow.Client.AddTCP` path;
2. completes a real TLS handshake and HTTP request;
3. sends `Connection: close`, closes TLS, and waits until **both client and server platformflow TCP flow counts are zero**;
4. records a positive `business_flow_id` and `flow_closed=true`.

Only after flow 1 is fully retired on both sides may flow 2 start. The validator requires two distinct inner FlowIDs while the outer FakeTCP initial SYN count remains exactly one and the outer lane ref remains unchanged.

The scenario manifest now explicitly records `sequential_close_before_next=true`, and the gate marker requires `sequential_close=pass`.

## Governance / non-claims

- No code is copied from `old/`; `docs/REUSE_LEDGER.json` is unchanged.
- Archive source SHA remains `b5c848f4e9afdffd15d1bc451560edf4e9390a35`.
- Production padding remains 0/off.
- FEC remains off in this measurement atom.
- No weak-network matrix, load/soak conclusion, classifier, or parameter tuning is added.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s repair horizon is unchanged.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 remains `NOT_IMPLEMENTED`.

## Qualification rule

Local results are not qualification authority. The candidate must pass the full `next-foundation` workflow at its exact SOURCE_SHA, including Windows/Linux active Go tests, Linux race, existing network gates, and the strengthened P5 HTTPS measurement gate.

Until that happens, `docs/STATUS.json.last_tested_source_sha` remains the already-qualified P5 measurement-base SHA `043da6298a2be091b55fb470b63c69e605bb61d8`.
