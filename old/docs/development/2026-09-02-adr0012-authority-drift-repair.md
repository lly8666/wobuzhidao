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

The handoff snapshot also named an older PR #9 candidate, so its continuation cursor was classified stale and not executed. The current PR HEAD Actions were mostly queued at the refresh snapshot; no exact-head release-green conclusion was carried forward.

## Authority finding

The live product owner explicitly restored ADR-0012 and required correction before further feature work.

Current authority after this repair:

- ADR-0011: per-Transport-Lane same-association Reality-like TLS bootstrap and post-bootstrap no-HOL semantics.
- ADR-0012: Logical Tunnel identity/address lease, product `1..4` Transport Lanes, Game/race behavior, DORMANT/wake, lane rotation, generation fencing and make-before-break replacement.
- ADR-0013: historical/withdrawn.
- ADR-0014: historical/withdrawn/invalidated.
- ADR-0015: historical/withdrawn after a later explicit live human correction.

`single-flow` means one FakeTCP association/tuple/sequence lineage per lane/transport epoch, not one public flow for the entire Logical Tunnel lifetime.

## Drift found

Correct ADR-0012-aligned evidence already present included ADR-0012 itself, `ROADMAP.md`, `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`, `CONTINUE_HERE.md`, README, existing Game/race code and Windows multilane planning foundations.

Contradictory rollback evidence included:

- `PROJECT_CONSTITUTION.md` naming ADR-0015 as authority and requiring global one public flow;
- `ARCHITECTURE.md` disabling product Game/multilane and requiring break-before-make;
- ADR-0014/0015 repeating global-one-lane semantics;
- PR #9 body repeating that rule;
- executable `MaxProductPublicTransportLanes = 1` and `n == 1` validation;
- Windows candidate builders hard-disabled by `ErrOverlappingPublicFlow`;
- recent tests/commits requiring second-lane rejection.

## Frozen components

This correction does not authorize transport-wire redesign. Preserve and requalify mature FakeTCP SYN/ACK/SACK/RTO/recovery (`legacy` default), per-lane same-association Reality-like real TLS bootstrap, the no-FIN/no-RST/no-new-SYN barrier inside one lane, pinned wolfSSL DTLS 1.3, immutable LINK, FEC `off`/fixed systematic `20:20` lane-local behavior, <=100 Mbit/s qualification ceiling, 40 Mbit/s aggregate-inner conservative release point, Logical Tunnel lease/source anti-spoof foundation, shared Linux TUN + one WBD-owned host NAT, Windows Wintun raw-L3 and IPv6 fail-closed.

## Atomic phase: authority/docs only

No executable transport code was changed in this phase.

Changes:

1. ADR-0015 was withdrawn and retained as historical evidence only.
2. ADR-0014 was marked `WITHDRAWN / INVALIDATED` and its global-one-lane clauses explicitly invalidated.
3. `PROJECT_CONSTITUTION.md` was restored to ADR-0012 authority, product lane range 1..4, Game and make-before-break.
4. `ARCHITECTURE.md` was restored to stable Logical Tunnel + 1..4 complete per-lane same-flow transports.
5. PR #9 title/body was rewritten to advertise ADR-0012 rather than global-one-lane shipping behavior.
6. `ROADMAP.md`, `CONTINUE_HERE.md` and README were inspected and already expressed the ADR-0012 multipath direction, so they were not rewritten merely for churn.
7. PR #9's current handoff architecture contract test was inspected. It is ADR-0012-aligned and intentionally asserts machine-readable phrases for per-lane single-flow, 1..4 lanes, Game, make-before-break and withdrawn ADR-0014. Documentation was tightened to satisfy that correct contract rather than weakening the test.

Substantive commits in this authority repair:

```text
3cf346af51088b4c77b9793484d2bfd8cd455d6f  docs: withdraw global single-flow override
15b8972d1f4b7f2ece132b096bcacacfcaa40fb2  docs: restore ADR-0012 over ADR-0014
23098e26fd7d6acf163ebf62533676131dcc277a  docs: restore ADR-0012 project constitution
ce62411121de46dd4878ecd6d56a921559fe1388  docs: restore multipath mainline architecture
5303d43d62b2ee0cf955586809be9d3710c950d7  docs: record ADR-0012 authority drift repair
df56678bfafce0c96d6201b0fc9bbd154d56431c  docs: lock machine-readable ADR-0012 constitution
b447a5eba37b1265225fd37b048bae759130fac2  docs: lock machine-readable multipath architecture
59ce14e8dee96b33bcd95d2629001013c4063f9c  docs: satisfy ADR-0012 authority contract
```

PR metadata was updated during this phase. The final substantive checkpoint for the phase is the commit immediately before the handoff-only commit written after this log update.

## Status taxonomy

```text
Architecture authority reconciliation:
IMPLEMENTED: yes
AUTOMATED-GREEN: not yet (must use post-repair exact HEAD)
PHYSICAL-GREEN: not applicable to docs-only phase
RELEASE-QUALIFIED: no
```

No product capability is declared release-ready by this documentation repair.

## Next atomic action

Restore only the minimum executable cardinality contract without touching FakeTCP/DTLS/LINK/FEC wire semantics:

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 4
ValidateProductTransportLaneCount: accept 1..4, reject 0 and 5+
```

Update only directly conflicting cardinality tests in that phase. Do not combine candidate replacement, DORMANT, generation fencing or trigger integration into the same atomic change. Update handoff again before proceeding to candidate/Game make-before-break restoration.
