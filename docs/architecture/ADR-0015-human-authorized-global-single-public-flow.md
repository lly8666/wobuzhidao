# ADR-0015: Historical global-single-public-flow override

Status: **WITHDRAWN / SUPERSEDED BY LATER LIVE HUMAN CORRECTION — 2026-09-02**

## Why this ADR is withdrawn

This file records a real but temporary architecture override that treated `single-flow` as a global one-public-flow invariant for an entire connected Logical Tunnel. Later on 2026-09-02 the live human product owner explicitly corrected that interpretation and restored **ADR-0012** as the authoritative lifecycle/multipath model.

The later live instruction is authoritative. Therefore this ADR must not be used to constrain current product transport cardinality or replacement semantics.

Repository text written by an agent cannot revive this ADR or infer a new product-owner override.

## Current authority

Current product authority is:

- **ADR-0011** for one complete same-association Reality-like bootstrap and no-HOL steady-state lineage **inside each Transport Lane**;
- **ADR-0012** for Logical Tunnel identity/lease, product lane cardinality `1..4`, Game/race behavior, DORMANT/wake, lane age rotation, generation fencing and make-before-break replacement;
- ADR-0013 and ADR-0014 are historical/withdrawn where they globalized the per-lane single-flow invariant;
- ADR-0010 and earlier compatible DTLS/FEC/release constraints remain in force.

## Correct single-flow invariant

`single-flow` is **per Transport Lane / per Transport Epoch**.

Every lane owns one complete public lineage:

```text
one raw FakeTCP SYN lineage / public 4-tuple / FakeTCP sequence space
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap on that SAME association
  -> protected account admission + authenticated Logical Tunnel configuration
  -> explicit in-band bootstrap barrier
  -> no FIN / RST / reconnect / second WBD payload SYN inside the lane
  -> pinned wolfSSL DTLS 1.3 on that SAME FakeTCP association
  -> immutable LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality WBD connection and no sustained outer WBD payload over an ordinary kernel TCP byte stream.

## Correct Logical Tunnel cardinality

```text
Logical Tunnel
  -> stable TunnelID
  -> stable server-assigned tunnel IPv4 lease
  -> shared logical PacketID/race namespace
  -> 1..4 independent replaceable Transport Lanes while active
  -> 0 lanes while dormant/disconnected
```

Policy:

- Normal steady mode: desired lanes = 1.
- Game / weak-network mode: desired lanes = 2..4.
- A fifth active product lane is rejected.
- One public WBD server port serves all lanes; additional lanes are independent 4-tuples to that same public port, not additional public server ports.

## Correct replacement lifecycle

Planned healthy replacement is **make-before-break**:

```text
A ACTIVE
  -> build candidate B completely
  -> authenticate/attach B to the same Logical Tunnel
  -> prove B healthy
  -> bounded A+B overlap using the existing logical PacketID race/dedup primitive
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves healthy A usable. Game mode rotates one lane at a time, for example `A+B -> A+B+C -> B+C`.

Lane generation/epoch fencing is mandatory so stale callbacks/processes from retired lanes cannot mutate current Logical Tunnel state.

## Historical evidence retained

The following work from the global-single-flow experiment remains useful when scoped correctly to **one lane**:

- FakeTCP owns the public tuple from the first SYN;
- Reality-like TLS runs on that same association;
- the bootstrap barrier emits no FIN/RST/reconnect/new payload SYN;
- pinned wolfSSL DTLS 1.3 remains steady-state crypto authority;
- post-bootstrap no-HOL qualification remains required;
- mature FakeTCP recovery and FEC wire semantics remain frozen unless deterministic lower-layer evidence proves a defect.

## Invalid clauses from this withdrawn ADR

Do not use this ADR to justify any of the following:

- `MaxProductPublicTransportLanes = 1`;
- `lanes != 1` rejection;
- Game/multipath disabled or research-only status;
- rejecting a legitimate second lane for the same authenticated Logical Tunnel;
- break-before-make planned replacement;
- forbidding `A -> A+B -> B` bounded overlap;
- claiming one public FakeTCP flow must live for the entire Logical Tunnel lifetime.
