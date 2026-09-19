# 2026-08-31 — Single-flow authority recovery and continuation

## Why this log exists

Chat/session recovery repeatedly truncated recent implementation history. The repository is therefore the durable source of truth. This note records the exact recovery performed on 2026-08-31 before further product work.

## User hard requirement

The product requirement is globally single-flow for one connected Logical Tunnel:

- exactly one public TCP-shaped WBD 4-tuple / FakeTCP SYN lineage / FakeTCP sequence space at a time;
- FakeTCP owns that public flow from the first SYN;
- the first bounded seconds carry real TLS 1.3 Reality-like setup on that same FakeTCP association;
- the current client persona is Firefox 120 uTLS where technically practical;
- credentials/admission are protected by TLS;
- bootstrap -> steady-state is an in-band barrier with no FIN, RST, reconnect or second WBD SYN;
- steady transport is pinned wolfSSL DTLS 1.3 -> LINK -> FEC -> packet/datagram payload and must not acquire ordinary kernel-TCP HOL;
- the mature TCP-like/FakeTCP recovery/FEC core is frozen unless a deterministic lower-layer test isolates a defect.

ADR-0014 is the controlling architecture decision. ADR-0012 remains useful only for compatible Logical Tunnel identity and server-assigned lease concepts. Its 1..4 public-lane, Game multipath and make-before-break overlap clauses are superseded.

## Recovery finding

Live refresh found the repository in a mixed-authority state:

- PR #9 metadata, `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, ADR-0012 and ADR-0014 already described the ADR-0014 global single-flow product;
- feature branch `.wbd/handoff/current.json` was stale at sequence 61 and still claimed ADR-0012 1..4 public lanes / Game / make-before-break were product policy;
- `ROADMAP.md` likewise still described the V2.5 multipath product;
- `tests/test_handoff.py` still asserted old V2.4/V2.5 lane wording and failed `handoff-verify` even though `scripts/verify_handoff.py` itself passed.

This mixed state explained repeated contradictory recovery behavior across interrupted chat sessions.

## Exact evidence before repair

Feature branch before repair:

- branch: `feat/single-flow-reality-faketcp`
- observed head: `2108490e5853d9ef854c80cd1a8da89c8ec11e0f`
- PR: #9, `[V2.6] Global single-flow Reality-like bootstrap + no-HOL steady transport`
- failing handoff-verify run: `33390347489`
- failing job: `99482302935`

The failing job printed `HANDOFF_VERIFY_PASS`, then failed only in `tests/test_handoff.py::test_v24_lane_and_tunnel_architecture_invariants_are_persisted` because it required the retired phrase `Each Transport Lane has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage` and additional 1..4-lane/Game/make-before-break wording.

No product data-plane failure was established by that red gate.

## Repair commits in this recovery

1. `02c94ded6649800105da25e649f4478a33eda0af` — `docs: align roadmap with ADR-0014 single-flow freeze`
   - rewrote `ROADMAP.md` around ADR-0014;
   - one connected Logical Tunnel now has exactly one active public WBD transport;
   - Game/multipath/make-before-break remain research-only;
   - replacement is break-before-replace unless a future ADR preserves one visible lineage;
   - release gate requires exact-head Windows/Linux/same-flow/no-HOL qualification before physical delivery.

2. `433c98e7459b078c18a17257689a249a409e9d3e` — `test: enforce ADR-0014 single-flow handoff contract`
   - replaced stale V2.4/V2.5 multipath assertions in `tests/test_handoff.py`;
   - tests now assert ADR-0014 global one-flow, same-association real TLS bootstrap, no second WBD SYN, post-barrier no-HOL behavior, ADR-0012 partial supersession and the pinned wolfSSL/FEC release lock;
   - TCP-like production code was not changed.

## Product implementation state recovered from PR #9

PR #9 currently states and implements the intended high-level product line:

```text
one raw FakeTCP SYN / one public 4-tuple / one FakeTCP sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap on that same association
  -> explicit barrier with no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

The Windows controller no longer needs a preliminary ordinary-TCP Reality product connection for this architecture. Linux exposes one raw public mux path and no parallel ordinary kernel-TCP Reality product listener on that public WBD port.

## Physical evidence already observed

Earlier physical single-flow testing produced two important shapes:

1. Successful server path at least once:
   - `WBD_SINGLE_FLOW_BOOTSTRAP_READY`
   - DTLS `BOUND`
   - `WBD_DTLS_SERVER_PEEK`
   - `WBD_DTLS_SERVER_ACCEPT_PASS`
   - `READY role=server version=DTLSv1.3`
   - `WBD_LINK_MUX_SESSION_READY`

2. Windows failure shape on another build:
   - FakeTCP/Npcap child started;
   - `wbd-faketcp handshake: faketcp: not ipv4/tcp` after receiving unrelated traffic;
   - Reality ticket file never appeared;
   - connect failed at the single-flow ticket readiness timeout.

Subsequent work added Windows Npcap ingress filtering so unrelated ARP/IPv6/UDP/unrelated TCP noise is discarded before FakeTCP parsing. Do not assume that historical physical failure remains current; requalify exact current candidate before delivery.

## Development rule from here

1. Treat ADR-0014, `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, current `ROADMAP.md`, PR #9 and refreshed handoff as one authority set.
2. Do not reintroduce ADR-0012 1..4 public lanes / Game / make-before-break product behavior.
3. Do not modify mature FakeTCP recovery/FEC unless a deterministic failing lower-layer qualification requires it.
4. Fix the first deterministic failing layer only.
5. Before any new artifact is delivered to physical testing, one exact substantive source HEAD must have current green:
   - main CI/release contract;
   - handoff verify;
   - single-flow one-SYN E2E;
   - Firefox120 Reality-like persona;
   - post-bootstrap no-HOL hole-bypass;
   - FEC off and 20:20 weak-network regression;
   - Windows native/runtime/portable/admin/raw-IP qualification;
   - Linux raw/netns/firewall/release/raw-IP qualification;
   - combined Windows-runtime -> Linux-server qualification;
   - matching Windows/Linux `SOURCE_SHA` release artifacts.
6. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 is final environment acceptance, not a substitute for automated qualification.

## Next atomic action

Observe the new exact feature HEAD after the ADR-0014 roadmap/test repair. Confirm `handoff-verify` first. Then enumerate all same-head Windows/Linux mandatory gates and identify any remaining deterministic product failure. If no transport failure exists, continue only above the frozen transport layer (single-active-transport lifecycle, Logical Tunnel lease/shared-TUN integration, cleanup/packaging qualification).