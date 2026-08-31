# ADR-0013: Global single-public-flow release freeze

Status: **HISTORICAL / WITHDRAWN — NOT PRODUCT AUTHORITY — 2026-08-31**

## Historical context

ADR-0013 was an experiment that attempted to freeze one public transport for the whole connected Logical Tunnel and to use break-before-make replacement. That experiment was withdrawn.

It MUST NOT be used to infer current product cardinality or lifecycle policy.

Current authority is:

- ADR-0011 for **per-Transport-Lane** same-association Reality-like TLS bootstrap and post-bootstrap no-HOL behavior;
- ADR-0012 for Logical Tunnel identity/lease, 1..4 independent product Transport Lanes, Game/race, idle/wake, lane rotation and make-before-break;
- ADR-0014 is separately retained only as withdrawn evidence of a later incorrect globalization of the same per-lane invariant.

## Evidence retained from ADR-0013

The transport-shape work remains useful when scoped to one lane:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap
  -> explicit same-flow barrier, no FIN/RST/reconnect/new WBD payload SYN inside the lane
  -> DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

Npcap filtering fixes, same-association bootstrap tests and post-switch no-HOL tests remain qualification evidence for each lane.

## Withdrawn clauses

The following ADR-0013 clauses remain invalid:

- global `public transport count = 1` for a Logical Tunnel;
- Game Lane retirement/research-only status;
- rejection of a second healthy independent lane;
- break-before-make as the planned healthy replacement model;
- any claim that per-lane single-flow forbids Logical Tunnel multipath.

History is preserved, but ADR-0013 has no authority over ADR-0012.
