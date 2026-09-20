# P5 full vs resumed inner HTTPS handshake candidate

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base docs HEAD: `6efd085ddf59376939c637507b7f47d86eaa9fc7`
- Current product qualification: `a9b92c26ac2f13b8ff0da2c321d5f2bb32100f5f` / Actions `35520092389` / 8/8 PASS
- Close-tail docs-only closure: `6efd085ddf59376939c637507b7f47d86eaa9fc7` / Actions `35521939589` / 8/8 PASS
- Candidate SOURCE_SHA: pending this commit
- Qualification: PENDING exact-SHA GitHub Actions

## Atom scope

This atom extends only the existing controlled real-HTTPS measurement harness to cover the P5 handshake dimension:

- one real TLS 1.3 full handshake;
- one real TLS 1.3 resumed handshake;
- both through the already-qualified formal runtimeentry / TunnelOwner / Normal lane path;
- the first inner HTTPS connection fully closes before the second is opened;
- exactly one outer client FakeTCP SYN and one stable outer lane remain required.

It does not add a certificate-chain matrix, weak-network injection, load, soak, classifier work, FEC-on testing, or padding enablement.

## Session provenance

The harness uses one explicit shared `tls.ClientSessionCache` wrapper backed by a Go LRU cache with capacity 8. The wrapper records:

- cache Get count;
- successful cache Hit count;
- non-nil Put count.

The two sequential inner HTTPS flows are required to behave as follows:

1. flow 1: `DidResume=false`, TLS 1.3, and at least one real session-state Put after the full handshake;
2. flow 2: `DidResume=true`, TLS 1.3, and at least one real cache Hit during the resumed handshake.

The cache is shared only between those two controlled inner HTTPS connections. The outer WBD TLS/admission session is unchanged and is not being called “resumed” by this evidence.

## Raw measurement extension

Each `https_flow` event additionally records:

- TLS version;
- negotiated cipher suite;
- handshake mode `full` or `resumed`;
- explicit resumed boolean;
- per-flow cache Get/Hit/Put deltas.

The manifest additionally records:

- `tls_version=TLS1.3`;
- expected handshake modes `[full,resumed]`;
- shared-LRU session-cache provenance and capacity;
- final cache Get/Hit/Put counters.

The existing raw packet/record capture, direction, timing, burst, business latency, wire bytes, capture-loss, network-drop, FEC, repair and padding accounting remain intact.

## Validator

`tools/check_p5_measurement.py` is strengthened to require:

- TLS 1.3 on both inner HTTPS flows;
- flow 1 full / `tls_resumed=false`;
- flow 1 stores a session ticket;
- flow 2 resumed / `tls_resumed=true`;
- flow 2 has a real session-cache hit;
- two distinct inner business FlowIDs;
- sequential close before next flow;
- one reused outer connection;
- FEC off, padding off, no network injection.

The success marker becomes:

`handshakes=full,resumed`

in addition to the prior `sequential_close=pass`.

## Qualification boundary

No local result is product qualification. The candidate must pass the full exact-SHA `next-foundation` workflow, including all existing platform/network regressions and the strengthened P5 gate.

Production padding remains `0/off`. Windows/Npcap physical remains P7 `NOT_RUN`. OpenWrt IPv6 remains `NOT_IMPLEMENTED`. No `old/` implementation is reused, so `docs/REUSE_LEDGER.json` is unchanged.
