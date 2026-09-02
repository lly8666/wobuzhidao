# 2026-08-31 — ADR-0012 multipath authority restoration and implementation log

Status: ACTIVE DEVELOPMENT LOG

## Why this log exists

Conversation recovery repeatedly lost important implementation history. This file is the durable project record for the 2026-08-31 correction that restored ADR-0012 as the current product architecture.

## Product-owner correction

The authoritative rule is:

- a user-visible Logical Tunnel may own **1..4 simultaneous WBD Transport Lanes**;
- Game Lane is a current product mechanism, not retired research;
- planned replacement is **make-before-break**;
- old + candidate lane overlap is permitted during bounded replacement;
- Game mode replaces one lane at a time so redundancy is preserved;
- **single-flow applies per lane**, not globally to the whole VPN;
- the mature TCP-like/FakeTCP recovery/data-plane core should remain frozen unless a deterministic failing qualification proves a defect below the lifecycle layer.

Each lane must still satisfy the same no-HOL single-flow lineage:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like real TLS 1.3 bootstrap on that association
  -> explicit bootstrap barrier, no FIN/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload with no ordinary kernel-TCP HOL
```

## How the conflict happened

ADR-0013 temporarily changed the release policy to a global single public transport for the whole Logical Tunnel, retired Game Lane from the product path, and required break-before-make. It also drove code changes that:

- set `internal/logicaltunnel.MaxProductPublicTransportLanes = 1`;
- made the LINK server reject a second concurrent transport for the same TunnelID;
- added release-contract tests that failed if architecture/constitution mentioned 1..4 lanes or make-before-break;
- rewrote `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md` and `ROADMAP.md` around the global-one-flow interpretation.

The 2026-08-31 product-owner correction explicitly rejects that interpretation. ADR-0012 is reaffirmed and ADR-0013 is retained only as historical evidence.

## Evidence recovered from repository history

The pre-supersession ADR-0012 text was recovered from commit `b6b6e80b4d09d719f6d710745a7350f5de2bba58`. It explicitly defines:

- Logical Tunnel identity/address lease above disposable Transport Lanes;
- 1..4 independent complete WBD associations;
- Game Lane PacketID / first-arrival / duplicate suppression;
- no cross-lane HOL;
- lane-local FEC;
- shared Linux TUN + one host NAT;
- payload-idle DORMANT with zero lanes and wake;
- independent lane age rotation;
- `A -> A+B -> B` make-before-break replacement;
- one-lane-at-a-time Game rotation;
- one replacement state machine for age/path/network/liveness/child-failure events.

The existing handoff sequence 60 also still described these ADR-0012 semantics, proving the later global-one-flow freeze conflicted with the repository's own continuity record.

## Changes completed in this correction

1. `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`
   - restored accepted/reaffirmed status;
   - explicitly states single-flow is per lane;
   - restores 1..4 lanes, Game Lane and make-before-break;
   - preserves mature no-HOL/FakeTCP/DTLS/FEC constraints.

2. `docs/architecture/ADR-0013-global-single-public-flow-release-freeze.md`
   - changed to WITHDRAWN / superseded by reaffirmed ADR-0012;
   - retains only useful per-lane same-association/no-HOL evidence;
   - explicitly identifies global-one-transport and break-before-make as withdrawn.

## Code conflicts confirmed and still being corrected

- `internal/logicaltunnel/logicaltunnel.go` currently caps product lanes at 1 and must become 1..4.
- `cmd/wbd-link-server-mux/logical_tunnel.go` currently maps one TunnelID to one active peer and rejects a second lane; it must permit up to four independent lane peers for one Logical Tunnel.
- `cmd/wbd-link-server-mux/logical_tunnel_transport_test.go` currently proves break-before-make/reject-second semantics and must be replaced with 1..4 / fifth-rejected / make-before-break tests.
- `internal/releasecontract/single_flow_release_test.go` currently freezes the withdrawn ADR-0013 semantics and must be rewritten to freeze ADR-0012 instead.
- `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, and `ROADMAP.md` still contain global-single-flow wording and must be aligned.
- Windows product lifecycle still needs inspection/integration so policy can request 1..4 per-lane single-flow children and Game Lane can own PacketID racing/replacement without changing the TCP-like core.

## Qualification strategy

Do not hand new artifacts to the physical tester until one exact substantive source HEAD has green evidence for the corrected architecture. Minimum target matrix:

- Go CI / release-contract tests;
- Logical Tunnel 1..4 lane admission and fifth-lane rejection;
- make-before-break `A -> A+B -> B`, candidate failure keeps A, and one-at-a-time Game rotation;
- Game Lane first-arrival/dedup/no-cross-lane-HOL;
- same-lane single-flow Reality-like bootstrap -> DTLS -> LINK continuity;
- no-HOL post-bootstrap hole-bypass;
- shared TUN + one host NAT / lease isolation / anti-spoof;
- Windows build, capability/admin smoke, portable bundle;
- Linux server release/firewall/full-stack;
- exact-source Windows/Linux artifact identity;
- physical Windows 11 + Npcap -> Ubuntu ARM64 only after automated qualification.

## Current next atomic action

Restore the product lane-count/server admission contract to 1..4, replace the withdrawn second-lane rejection tests with bounded multipath and make-before-break tests, then run CI before wiring additional Windows lifecycle behavior.
