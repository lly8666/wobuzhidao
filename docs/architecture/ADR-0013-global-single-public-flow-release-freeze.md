# ADR-0013: Global single-public-flow release freeze

Status: **WITHDRAWN / SUPERSEDED BY REAFFIRMED ADR-0012** (2026-08-31)

## Historical context

ADR-0013 temporarily interpreted the single-flow requirement as a **global** invariant for the whole connected Logical Tunnel: one public transport total, no Game Lane, and break-before-make replacement.

That interpretation was rejected by the product owner on 2026-08-31. The controlling rule is again ADR-0012:

- a connected Logical Tunnel may own **1..4 independent Transport Lanes**;
- normal steady mode targets one lane;
- Game/weak-network policy may keep 2..4 lanes;
- planned replacement is **make-before-break** and may temporarily overlap old + candidate lanes;
- Game Lane `PacketID` / first-arrival / duplicate-suppression semantics are the shared multipath/replacement layer;
- replacement rotates one healthy lane at a time.

## What remains valid from this experiment

ADR-0013 correctly strengthened a different invariant which remains mandatory **per lane**:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap
  -> explicit bootstrap barrier; no FIN and no second WBD payload SYN
  -> DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality connection for a lane and no sustained WBD payload over an ordinary kernel TCP byte stream.

The single-flow work, Npcap filtering fixes, same-association bootstrap tests, no-HOL tests and other evidence produced while ADR-0013 was active remain useful technical evidence. Only the **global one-transport count** and **break-before-make** policy are withdrawn.

## Current authority

For transport count, Game Lane, migration, rotation, idle/wake and Logical Tunnel lifecycle, **ADR-0012 is authoritative**.

Any release-contract test, configuration, lifecycle code or documentation that still requires `MaxProductPublicTransportLanes == 1`, rejects a second lane for the same TunnelID, disables Game Lane as a product path, or requires break-before-make must be corrected.
