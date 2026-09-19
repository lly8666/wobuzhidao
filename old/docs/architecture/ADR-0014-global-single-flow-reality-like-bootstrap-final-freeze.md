# ADR-0014: Historical global single-flow experiment

Status: **WITHDRAWN / INVALIDATED — NOT PRODUCT AUTHORITY — 2026-09-02**

## Historical correction

ADR-0014 **incorrectly expanded** the valid per-Transport-Lane single-flow invariant into one public flow for an entire Logical Tunnel and paired that with break-before-make replacement. That product interpretation is withdrawn.

A later explicit live human product-owner correction restored **ADR-0012** as the authoritative Logical Tunnel lifecycle/multipath model. ADR-0011 remains the authority for same-association Reality-like TLS bootstrap and no-HOL steady-state semantics inside each individual Transport Lane.

## Evidence retained from the experiment

The following remains correct when scoped to one lane:

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

FakeTCP owns the public tuple from the first SYN. The mature FakeTCP recovery/FEC wire should not be redesigned merely to satisfy an architecture string test. Reality-like fidelity remains evidence-driven.

## Invalid clauses

Do not use ADR-0014 to justify:

- one public transport for the whole Logical Tunnel lifetime;
- `MaxProductPublicTransportLanes = 1`;
- Game/weak-network multipath disabled or research-only;
- rejecting a legitimate second complete lane for the same Logical Tunnel;
- break-before-make planned healthy replacement;
- forbidding bounded `A -> A+B -> B` overlap;
- claiming per-lane single-flow semantics prohibit Logical Tunnel multipath.

## Current authority

- ADR-0011: per-lane same-association Reality-like bootstrap and no-HOL data plane.
- ADR-0012: stable Logical Tunnel identity/lease, product lanes 1..4, Game/race, DORMANT/wake, lane age rotation, generation fencing, make-before-break and unified replacement.
- ADR-0013: historical/withdrawn.
- ADR-0015: historical/withdrawn after a later explicit live human correction.
