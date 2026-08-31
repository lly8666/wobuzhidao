# ADR-0014: Global single public flow with Reality-like bootstrap and no-HOL steady state

Status: **WITHDRAWN / INVALIDATED — INCORRECT GLOBALIZATION OF PER-LANE SINGLE-FLOW — 2026-08-31**

## Withdrawal authority

This ADR is **not product architecture authority**.

It incorrectly expanded the valid ADR-0011 rule — one complete same-association FakeTCP/Reality-like/DTLS/LINK/FEC flow **per Transport Lane** — into a false rule that a whole Logical Tunnel may own only one public Transport Lane.

It also incorrectly claimed `PRODUCT-OWNER FINAL FREEZE` without explicit live human authorization and incorrectly superseded ADR-0012's 1..4 Transport Lane, Game/race and make-before-break lifecycle architecture.

The live human product-owner correction on 2026-08-31 explicitly invalidates those claims.

Current authority is:

- ADR-0011: per-lane same-association Reality-like bootstrap and no-HOL steady transport;
- ADR-0012: Logical Tunnel identity/lease, 1..4 independent Transport Lanes, Game/race, idle/wake, lane rotation and make-before-break;
- ADR-0010 and earlier compatible DTLS/FEC/release constraints;
- ADR-0013: historical/withdrawn only.

Do not infer product-owner approval from an ADR written by an agent. A new ADR that changes a frozen human product requirement requires explicit live human authorization.

## What this ADR got right technically

The following technical statements remain valid, but their authority belongs to ADR-0011/ADR-0012 rather than this withdrawn ADR:

For **each individual Transport Lane**:

```text
one raw FakeTCP SYN lineage / public 4-tuple / sequence space
  -> bounded reliable ordered bootstrap adapter on that same FakeTCP association
  -> real TLS 1.3 Reality-like ClientHello / ServerHello / Finished
  -> protected account admission + Logical Tunnel/lane binding
  -> explicit bootstrap barrier, with no FIN/RST/reconnect/new WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3 on the same FakeTCP association
  -> LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

FakeTCP owns each public lane from its first SYN onward. The temporary ordered adapter exists only to satisfy the short TLS/bootstrap setup and is destroyed at the barrier. Sustained WBD payload must not be carried by ordinary kernel TCP.

Reality-like fidelity remains evidence-driven: real TLS 1.3, configured SNI, protected credentials, plausible TCP/TLS persona and no second WBD payload SYN **inside a lane**. A numeric similarity percentage is not a release criterion without a reproducible pcap metric.

The mature FakeTCP recovery/FEC core remains frozen unless a deterministic lower-layer qualification isolates a real defect.

## What this ADR got wrong and is explicitly withdrawn

The following statements are invalid and MUST NOT be used by product code, tests, docs, handoff or release qualification:

- that `single-flow` is a global Logical Tunnel invariant;
- exactly one public WBD 4-tuple for a connected Logical Tunnel;
- no simultaneous second independent WBD Transport Lane;
- `MaxProductPublicTransportLanes = 1`;
- Game/multipath is research-only;
- a second healthy lane must be rejected;
- make-before-break `A -> A+B -> B` is forbidden;
- planned replacement must be break-before-make;
- Windows product Controller must forever start only one FakeTCP child;
- Linux product path must omit Game/race because one public server port supposedly implies one lane per tunnel;
- ADR-0012 multipath/lifecycle clauses are superseded;
- an agent-authored document may label a new architecture a product-owner final freeze without live human approval.

## Correct scope: per-lane single-flow + tunnel multipath

The correct product model is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IP lease
          -> stable logical PacketID/race space
          -> 0..N replaceable Transport Lanes
```

Product policy/ceiling:

```text
Normal steady mode:      1 active lane
Game / weak-network:     2..4 active lanes
Logical Tunnel maximum:  4 active public lanes
Dormant:                 0 active lanes
```

Example:

```text
Logical Tunnel
├─ Lane A = one complete single-flow FakeTCP association
├─ Lane B = one complete single-flow FakeTCP association
├─ Lane C = one complete single-flow FakeTCP association
└─ Lane D = one complete single-flow FakeTCP association
```

The lanes do not share FakeTCP sequence space, DTLS nonce/key state or FEC blocks. They share Logical Tunnel identity/lease and the logical PacketID race domain.

## Replacement correction

Planned healthy replacement uses ADR-0012 make-before-break:

```text
A ACTIVE
  -> build B through FakeTCP + same-lane Reality bootstrap + DTLS + LINK
  -> attach B to the same Logical Tunnel
  -> prove health
  -> bounded A+B race/dedup overlap
  -> stop new sends to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves A untouched. Game mode may rotate one healthy lane at a time, e.g. `A+B -> A+B+C -> B+C`.

## Historical evidence retention

This file is retained so the repository records how the erroneous globalization happened. Its invalid global-cardinality clauses are evidence only and must not be copied into current authority, tests, release contracts, PR metadata or handoff.
