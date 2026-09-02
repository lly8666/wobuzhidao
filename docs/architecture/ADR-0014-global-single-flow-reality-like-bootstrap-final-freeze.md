# ADR-0014: Global single public flow with Reality-like bootstrap and no-HOL steady state

Status: **HISTORICAL — SUPERSEDED BY ADR-0015 — 2026-09-02**

## Historical note

This file records an earlier architecture dispute. It first attempted to globalize the single-flow invariant, was later withdrawn in favor of a 1..4-lane Logical Tunnel design, and is now superseded by **ADR-0015**, which records the explicit live human product-owner requirement.

Do not use this file as current product authority.

## Current authority

ADR-0015 is authoritative for shipping product behavior:

```text
one connected Logical Tunnel
  = exactly one simultaneous public WBD FakeTCP association
  = exactly one public 4-tuple / SYN lineage
```

That one association performs:

```text
FakeTCP SYN lineage
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap
  -> protected admission / authenticated tunnel configuration
  -> explicit in-band barrier, no FIN/RST/reconnect/new SYN
  -> pinned wolfSSL DTLS 1.3 on the same association
  -> LINK
  -> FEC off or fixed systematic 20:20
  -> no-HOL packet/datagram VPN payload
```

The first setup phase may temporarily need ordered stream semantics for real TLS. Those semantics end at the explicit barrier and must not become ordinary kernel-TCP HOL for sustained VPN payload.

## What remains historically useful

The old discussions correctly identified several technical boundaries that remain valid:

- FakeTCP, not an ordinary kernel TCP socket, owns the public WBD transport from the first SYN.
- Reality-like setup must occur on that same association rather than on a separate preliminary TCP connection.
- The transition to DTLS/LINK sends no FIN/RST and performs no reconnect/new payload SYN.
- The mature FakeTCP recovery/FEC core should not be redesigned merely to satisfy an architecture string test.
- Reality-like fidelity must be established by reproducible packet/handshake evidence rather than an unsupported numeric percentage.

## What is no longer product behavior

The intermediate 1..4-public-lane product model is no longer authorized for shipping behavior. In particular:

- Game/weak-network mode may not create 2..4 simultaneous public WBD flows for one tunnel;
- a second concurrent public transport for the same TunnelID is rejected;
- planned replacement may not use `A -> A+B -> B` public-flow overlap;
- Windows shipping orchestration may not start multiple FakeTCP children for one connected tunnel;
- candidate/make-before-break public transport is not a shipping path;
- server acceptance of up to four simultaneous peers for one TunnelID is not a shipping rule.

Historical multipath code may remain for research if it is unreachable from shipping configuration and guarded by product release tests.

## Replacement under current authority

Planned replacement is break-before-make at the public-flow boundary:

```text
A ACTIVE
  -> stop new inner sends
  -> detach/stop A LINK + DTLS + FakeTCP
  -> confirm A public transport is gone
  -> create B
  -> same-flow Reality-like TLS bootstrap -> DTLS -> LINK
  -> B ACTIVE
```

There is never a simultaneous A+B public interval for one Logical Tunnel.

## Authority rule

Agent-authored ADR text is not evidence of a product-owner override. The 2026-09-02 live human instruction recorded in ADR-0015 is the current cardinality authority unless a later explicit human instruction changes it.
