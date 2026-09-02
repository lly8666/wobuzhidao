# ADR-0014: Historical global single-flow experiment

Status: **HISTORICAL / WITHDRAWN — NOT PRODUCT AUTHORITY — 2026-09-02**

## Historical note

ADR-0014 attempted to globalize the valid per-Transport-Lane single-flow invariant into one public flow for the whole Logical Tunnel and to replace make-before-break with break-before-make. That interpretation is withdrawn.

The current live human instruction restores **ADR-0012** as the authoritative Logical Tunnel lifecycle/multipath model. ADR-0011 remains the authority for same-association Reality-like TLS bootstrap and no-HOL steady-state transport semantics inside each individual lane.

Do not use this file as current product authority.

## Evidence that remains useful

The technical work remains valid when scoped to one Transport Lane:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap
  -> protected admission / authenticated tunnel configuration
  -> explicit in-band barrier with no FIN/RST/reconnect/new payload SYN
  -> pinned wolfSSL DTLS 1.3 on the same FakeTCP association
  -> LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> no-HOL packet/datagram VPN payload
```

The mature FakeTCP ARQ/recovery/FEC wire should not be redesigned merely to satisfy architecture contract tests. Reality-like fidelity remains evidence-driven and should be evaluated with reproducible packet/handshake traces rather than unsupported percentage claims.

## Withdrawn clauses

The following clauses are invalid product directions:

- one public transport for the entire Logical Tunnel lifetime;
- maximum product lane count of 1;
- Game/weak-network multipath disabled or research-only;
- rejecting a legitimate second complete WBD lane for the same Logical Tunnel;
- break-before-make planned healthy replacement;
- forbidding bounded old+candidate overlap during replacement;
- claiming per-lane single-flow semantics prohibit Logical Tunnel multipath.

## Current authority

Use:

- ADR-0011: per-lane same-association Reality-like bootstrap and no-HOL data plane;
- ADR-0012: stable Logical Tunnel identity/lease, `1..4` lanes, Game/race, DORMANT/wake, age rotation, generation fencing and make-before-break;
- ADR-0013: historical/withdrawn;
- ADR-0015: historical/withdrawn after a later explicit live human correction.
