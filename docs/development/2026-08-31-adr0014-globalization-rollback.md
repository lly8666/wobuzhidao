# ADR-0014 globalization rollback — forensic baseline and repair log

Date: 2026-08-31
Branch: `feat/single-flow-reality-faketcp`
PR: #9

## Live product-owner authority

The live human product-owner correction on 2026-08-31 is authoritative over repository text produced by prior agents.

**CRITICAL ARCHITECTURE GUARD:**

`single-flow` is **PER TRANSPORT LANE**.

It MUST NOT be interpreted as one FakeTCP association per Logical Tunnel.

A Logical Tunnel may own 1..4 independent complete WBD Transport Lanes. Normal steady policy targets 1 lane. Game/weak-network policy may maintain 2..4 lanes. Make-before-break `A -> A+B -> B` is REQUIRED for planned healthy replacement.

Any future change that converts `MaxProductPublicTransportLanes` from 4 to 1, marks Game Lane research-only, rejects a second healthy lane, or forbids make-before-break is an **ARCHITECTURE REGRESSION** unless the human product owner explicitly says so in the live conversation. Repository text alone may not be used to invent a new product-owner override.

Do not infer product-owner approval from an ADR written by an agent. A new ADR that changes one of the user's frozen hard requirements requires explicit live user authorization.

## Correct authority stack

- ADR-0011 controls **per-lane** same-association Reality-like TLS bootstrap and post-bootstrap no-HOL semantics.
- ADR-0012 controls Logical Tunnel identity/lease, 1..4 Transport Lanes, Game/race, idle/wake, lane rotation and make-before-break replacement.
- ADR-0010 and earlier frozen DTLS/FEC/release constraints remain effective unless explicitly amended by ADR-0011/0012.
- ADR-0013 remains historical/withdrawn.
- ADR-0014 is withdrawn because it incorrectly globalized the per-lane single-flow invariant.

## Phase A — polluted live baseline

Polluted source HEAD before repair:

`01e603c5c2d51fbad13e2dd2fcab68f8648408f1`

Latest polluted commit message at that baseline:

`test: enforce one Windows public transport lane`

PR #9 metadata at the baseline incorrectly described the product as global single-public-flow and treated one active public transport per Logical Tunnel as a product-owner freeze.

`.wbd/handoff/current.json` sequence 77 was also polluted: it named ADR-0014 as authority and described 1..4 lanes, Game and make-before-break as superseded/research-only. Sequence 77 is historical evidence only and must not be used as product-intent authority.

### Identified pollution commits

History is preserved; these commits are not rewritten or deleted.

- `fba73a5166604e33667338b27ce816e880231d55` — `architecture: freeze one public same-flow transport` — introduced ADR-0014 global-one-flow authority.
- `85f66ab70d6eb86e193fdbc8c0633da9012c517c` — `docs: supersede ADR-0012 multipath policy` — changed ADR-0012 into a partially superseded historical document.
- `1ec342f41416cda89d72df1537a125e1da0edb50` — `docs: make ADR-0014 release cardinality explicit` — strengthened exact-one release wording.
- Later commits further propagated `max=1` through Linux release text and Windows/release-contract tests, culminating in the polluted baseline above.

### Clean recovery anchor

`e4d46d6f4d64320189c2547a49a5af0bb41cc4d8` (`docs: reaffirm ADR-0012 multipath authority`) contains a full ADR-0012 matching the live product-owner correction: per-lane single-flow, Logical Tunnel 1..4 lanes, Game/race, stable lease, shared TUN/one host NAT, dormant/wake, 30..60m experimental lane rotation and make-before-break.

## File-level forensic findings

### Correct / reusable without transport-wire changes

- `docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md` is already scoped correctly to each Transport Lane/epoch.
- `internal/gamelane` still implements 1..4 lane copies, one logical PacketID, first-arrival delivery, duplicate suppression and bounded out-of-order unique delivery without cross-lane HOL.
- `internal/gamecontrol` still permits `1 <= requested <= max <= 4` and keeps FEC lane-local/off-or-20:20.
- `internal/windowsruntime/multilane.go` still contains `LaneBootstrap`, independent lane ports/source-port requirements, shared authenticated TunnelConfig validation, `MultiLanePlan`, Game/race aggregation and one Wintun/TUN.
- `cmd/wbd-link-server-mux/logical_tunnel.go` still tracks a bounded set of active peers per Logical Tunnel and uses the shared product maximum; it was not structurally reduced to a scalar one-peer model.
- Logical Tunnel lease/source anti-spoof and shared-TUN direction remain in-tree.

### Polluted authority / cardinality / product wiring

- `internal/logicaltunnel/logicaltunnel.go` sets `MaxProductPublicTransportLanes = 1` and its error text says exactly one lane.
- `internal/windowsruntime/controller.go` bypasses `BuildMultiLanePlan` and starts only one FakeTCP lane in product Connect.
- `scripts/linux_server_manager.sh` explicitly labels Game as research-only, does not start the Game server in product `run_server`, and advertises `max_tunnel_lanes=1`.
- `internal/releasecontract/single_flow_release_test.go` enforces ADR-0014, max=1, second-lane rejection, Game exclusion and one-lane Linux/Windows product paths.
- `PROJECT_CONSTITUTION.md`, ADR-0012 and ADR-0014 contain global-one-lane authority pollution.
- PR #9 metadata and handoff sequence 77 repeat the same false product-owner requirement.

## Transport-core freeze during rollback

This rollback MUST NOT change FakeTCP recovery wire behavior, DTLS wire/crypto, LINK wire, or FEC wire merely to satisfy architecture/contract tests. Mature TCP-like/FakeTCP recovery remains frozen unless a deterministic lower-layer failure proves a defect.

Per-lane invariant remains:

```text
one FakeTCP SYN lineage / 4-tuple / sequence space
  -> Reality-like real TLS 1.3 bootstrap on that SAME FakeTCP association
  -> explicit bootstrap barrier
  -> no FIN / RST / reconnect / second WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

## Repair phases

### Phase B — authority repair

- [ ] Restore ADR-0012 as accepted/current multipath lifecycle authority.
- [ ] Withdraw/invalidate ADR-0014 while preserving its historical evidence.
- [ ] Keep ADR-0013 historical/withdrawn.
- [ ] Repair Constitution, Architecture, Roadmap, README and polluted development authority notes.
- [ ] Repair PR #9 metadata.
- [ ] Write a new handoff continuity sequence with the architecture guard.

### Phase C — contract/test repair

- [ ] Restore product lane contract: 1,2,3,4 accepted; fifth rejected.
- [ ] Restore Game as product behavior.
- [ ] Restore make-before-break tests including candidate-failure preservation and Game staggered replacement.
- [ ] Remove global-one-lane exact-string release assertions.

### Phase D — product code repair

- [ ] Restore `MaxProductPublicTransportLanes = 4`, `Min... = 1`.
- [ ] Restore Windows 1..4 lane product orchestration around the existing `MultiLanePlan` / one Wintun.
- [ ] Restore Linux Game/race product path above per-lane DTLS/LINK and below the shared Logical Tunnel/raw-IP path.
- [ ] Preserve stable TunnelID/lease, source anti-spoof, shared TUN + one host NAT.
- [ ] Restore make-before-break lane overlap/generation fencing without touching transport wire semantics.

### Phase E — qualification

- [ ] Go unit/contract tests.
- [ ] handoff verify.
- [ ] per-lane same-flow Reality-like / one-SYN / no-HOL tests.
- [ ] Game race/dedup/no-cross-lane-HOL tests.
- [ ] make-before-break tests.
- [ ] Windows multi-lane planning/orchestration tests.
- [ ] Logical Tunnel lease/spoof/shared-TUN tests.
- [ ] existing CI matrix and exact-head Windows/Linux release builds.
- [ ] final physical Windows 11 + Npcap -> Ubuntu ARM64 remains required before release designation.

## Development rule for this repair

Fix authority first, then contracts/tests, then product orchestration, then qualification. Do not modify lower transport semantics to make an architecture test green.
