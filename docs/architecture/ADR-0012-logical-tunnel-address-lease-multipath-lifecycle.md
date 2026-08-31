# ADR-0012: Stable logical tunnel, server-assigned address lease, and replaceable multipath transports

Status: **PARTIALLY SUPERSEDED BY ADR-0014 — 2026-08-31**

## Historical decision

ADR-0012 introduced two separable ideas:

1. a long-lived Logical Tunnel identity with a stable server-assigned address lease, independent of disposable transport identity;
2. 1..4 concurrent Transport Lanes, Game Lane first-arrival/dedup and make-before-break replacement.

The first idea remains useful product architecture. The second is no longer product policy.

## Current authority

The product owner subsequently froze the public network shape to **exactly one TCP-shaped WBD connection lineage for a connected Logical Tunnel**. ADR-0014 is authoritative for transport cardinality, Reality-like same-flow bootstrap, no-HOL steady state and replacement constraints.

Therefore these ADR-0012 clauses are **superseded**:

- 1..4 active public Transport Lanes;
- Game/weak-network policy maintaining 2..4 public lanes;
- `A -> A+B -> B` make-before-break public overlap;
- any statement that single-flow is only a per-lane invariant.

These ADR-0012 ideas remain valid:

- stable InstallationID / Logical Tunnel identity;
- server-assigned tunnel IPv4 lease bound to the Logical Tunnel rather than a disposable FakeTCP/DTLS/LINK epoch;
- source-address anti-spoofing against the lease;
- separating logical identity from one transport process lifetime;
- keeping the mature FakeTCP/DTLS/FEC data plane independent of ordinary kernel-TCP HOL.

## Preserved same-flow transport lineage

ADR-0012 also helped prove the transport phase shape that ADR-0014 now freezes globally:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap
  -> explicit barrier with no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality product connection and no sustained WBD payload over an ordinary kernel TCP byte stream.

## Historical evidence

Game Lane, multipath and make-before-break implementations/tests produced useful research evidence and may remain in-tree as non-product infrastructure. They must not override the ADR-0014 release ceiling of one active public transport for a connected Logical Tunnel.

See `docs/architecture/ADR-0014-global-single-flow-reality-like-bootstrap-final-freeze.md` for the controlling release contract.