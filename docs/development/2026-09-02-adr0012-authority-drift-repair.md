# 2026-09-02 ADR-0012 authority drift repair

## Purpose

This log records a live architecture/state rebuild after the active PR accumulated a contradictory global-one-public-flow rollback. It is durable recovery evidence; chat summaries are not required to reconstruct this decision.

## Live refresh before editing

At the start of this repair:

```text
PR: #9
branch: feat/single-flow-reality-faketcp
base: dev/wbd-raw-fec-v2
PR HEAD: f42641f2b6f0210bb6dace506e6636f7b7d3de1f
canonical dev HEAD: 2b036a7f92034946010e4baaed1b836df40d4afa
handoff continuity_sequence: 84
handoff checkpoint: 88b00fec9c8735e7ca4aa0366b7f02aae44da2fd
HEAD == checkpoint: no
```

The handoff snapshot also named an older PR #9 candidate, so its continuation cursor was classified stale and not executed.

The current PR HEAD Actions were mostly queued at the refresh snapshot. No exact-head release-green conclusion was carried forward.

## Authority finding

The live product owner explicitly restored the architecture in ADR-0012 and required correction before further feature work.

Current authority after this repair:

- ADR-0011: per-Transport-Lane same-association Reality-like TLS bootstrap and post-bootstrap no-HOL semantics.
- ADR-0012: Logical Tunnel identity/address lease, product `1..4` Transport Lanes, Game/race behavior, DORMANT/wake, lane rotation, generation fencing and make-before-break replacement.
- ADR-0013: historical/withdrawn.
- ADR-0014: historical/withdrawn.
- ADR-0015: historical/withdrawn after a later explicit live human correction.

`single-flow` means one FakeTCP association/tuple/sequence lineage **per lane/transport epoch**, not one public flow for the entire Logical Tunnel lifetime.

## Drift found

The repository contained two incompatible architecture sets at the same time.

Correct ADR-0012-aligned evidence already present:

- `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`;
- `ROADMAP.md`;
- `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`;
- `CONTINUE_HERE.md`;
- README's V2.4 logical-tunnel/multipath description;
- existing Game/race and Windows multilane planning foundations.

Contradictory rollback evidence:

- `PROJECT_CONSTITUTION.md` named ADR-0015 as authority and required global one public flow;
- `ARCHITECTURE.md` disabled product Game/multilane and required break-before-make;
- ADR-0014 and ADR-0015 repeated the global-one-lane rule;
- PR #9 body repeated that rule;
- executable code set `MaxProductPublicTransportLanes = 1` and validated only `n == 1`;
- Windows candidate-lane constructors were hard-disabled by `ErrOverlappingPublicFlow`;
- recent tests/commits explicitly required a second public flow to be rejected.

## Components explicitly frozen / not to rewrite

This correction is not permission to redesign the mature transport wire.

Preserve and requalify:

- FakeTCP SYN/ACK/SACK/RTO/recovery and the legacy default;
- per-lane same-association Reality-like real TLS bootstrap;
- explicit no-FIN/no-RST/no-new-SYN bootstrap barrier inside a lane;
- pinned wolfSSL DTLS 1.3;
- immutable LINK;
- FEC release wire `off` or fixed systematic `20:20`, lane-local only;
- <=100 Mbit/s weak-link qualification ceiling and 40 Mbit/s aggregate-inner conservative release point;
- Logical Tunnel InstallationID/TunnelID/lease manager and raw IPv4 source anti-spoof foundation;
- shared Linux TUN + one WBD-owned host NAT product direction;
- Windows Wintun raw-L3 and IPv6 fail-closed boundary.

## Atomic phase completed here: authority/docs only

No executable transport code was changed in this phase.

Changes:

1. ADR-0015 was marked withdrawn and rewritten as historical evidence.
2. ADR-0014 was marked historical/withdrawn and its global-one-lane clauses invalidated.
3. `PROJECT_CONSTITUTION.md` was restored to ADR-0012 authority and 1..4-lane/Game/make-before-break semantics.
4. `ARCHITECTURE.md` was restored to the stable Logical Tunnel + 1..4 complete lanes model.
5. PR #9 title/body was rewritten so it no longer advertises global-one-lane shipping behavior.
6. `ROADMAP.md`, `CONTINUE_HERE.md` and README were inspected and already expressed the ADR-0012 multipath direction, so they were not rewritten merely for churn.

Substantive commits in this authority repair:

```text
3cf346af51088b4c77b9793484d2bfd8cd455d6f  docs: withdraw global single-flow override
15b8972d1f4b7f2ece132b096bcacacfcaa40fb2  docs: restore ADR-0012 over ADR-0014
23098e26fd7d6acf163ebf62533676131dcc277a  docs: restore ADR-0012 project constitution
ce62411121de46dd4878ecd6d56a921559fe1388  docs: restore multipath mainline architecture
```

PR metadata was also updated after `ce624111...`.

## Status taxonomy

Architecture authority reconciliation:

```text
IMPLEMENTED: yes
AUTOMATED-GREEN: not yet
PHYSICAL-GREEN: not applicable to docs-only phase
RELEASE-QUALIFIED: no
```

No product capability is declared release-ready by this documentation repair.

## Next atomic action

Restore the minimum executable cardinality contract without touching FakeTCP/DTLS/LINK/FEC wire semantics:

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 4
ValidateProductTransportLaneCount: accept 1..4, reject 0 and 5+
```

Update only the directly conflicting contract tests in that atomic phase. Do not yet combine candidate replacement, DORMANT, generation fencing or trigger integration into the same commit.

After the cardinality contract phase, update the handoff again before proceeding to candidate/Game make-before-break restoration.
