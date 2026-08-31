# 2026-08-31 — Architecture contract reconciliation

## Scope

This work is documentation-contract reconciliation only. It does **not** reopen FakeTCP, DTLS, LINK, FEC, Game Lane packet semantics, or the mature recovery core.

The substantive single-flow product baseline remains the code already present before this reconciliation. Repeated exact-head `go test ./... -count=1` executions stayed green while the only deterministic failures were Python tests asserting stale/canonical architecture wording.

## Frozen architecture during this work

Per ADR-0012 and the current project constitution:

- one Transport Lane owns one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage;
- real TLS 1.3 Reality-like bootstrap is the first protected payload phase of that same FakeTCP association;
- bounded reliable ordered behavior is allowed only during TLS/bootstrap;
- an explicit barrier switches the same association to DTLS/LINK/FEC packet/datagram semantics;
- there is no FIN/RST/new WBD payload SYN at that switch;
- after the barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP range is missing;
- a Logical Tunnel may own 1..4 independent Transport Lanes;
- mature TCP-like/FakeTCP recovery remains frozen unless deterministic qualification or physical evidence proves a lower-layer defect.

## Starting evidence

Feature branch: `feat/single-flow-reality-faketcp`

Substantive product head before this documentation reconciliation:

`9178c5666ca38db76adfdbc5120b653c2f5b382d`

Formal handoff before this work: sequence 72 on formal commit

`ae507f264cf06e764fb1d2efa99fabee0364f349`

The earlier failing CI run `33366816098` was important because `go test ./...` completed successfully; only the architecture/handoff Python contract failed on exact documentation wording. Therefore the first deterministic blocker was documentation, not transport.

## Reconciliation sequence

### 1. Project constitution canonical wording

Commit:

`60ce6c66beff4c40a1e9b3e1334ce71374db6148`

Message: `docs: restore canonical V2.4 contract wording`

Added a stable canonical wording section to `PROJECT_CONSTITUTION.md`, including the exact release-contract phrases required by the automated architecture test. No product code changed.

The next exact-head CI again completed all Go tests successfully and then exposed the next stale documentation phrase in `ARCHITECTURE.md`.

### 2. Architecture canonical wording

Commit:

`2904951b4a7dba9c9f2efbfbd3ef0d5ca2b25edc`

Message: `docs: restore canonical architecture contract wording`

Added the stable architecture phrases required by the automated contract, including:

- same association, no second WBD payload SYN;
- one public FakeTCP association per lane from SYN through bootstrap and steady payload;
- no separate ordinary kernel-TCP WBD payload connection;
- real TLS 1.3 ClientHello/ServerHello/Finished on the same lane sequence space;
- post-bootstrap earliest-complete datagram behavior;
- 40 Mbit/s aggregate-inner conservative release operating point.

Exact-head CI run `33369887991` again showed `go test ./... -count=1` green. The only failure moved forward to a stale roadmap milestone phrase.

### 3. Roadmap pivot milestone compatibility wording

Commit:

`43f82ddfe9485fa3e1ee29929f572b8c914b64f0`

Message: `docs: preserve roadmap contract milestone phrase`

Restored the stable automated-test milestone phrase:

`V2.4 LOGICAL-TUNNEL / MULTIPATH PIVOT ACTIVE`

while explicitly preserving the current V2.5 / ADR-0012 authority and ADR-0013 withdrawal.

Exact-head CI run `33370033257` once again completed all Go tests successfully. Its only failure was the next exact roadmap substring:

`V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association per lane`

Independent handoff-verify run `33370033135` failed for the same Python architecture test; the handoff schema verifier itself printed `HANDOFF_VERIFY_PASS` before that contract test ran.

### 4. Roadmap per-lane M6 wording

Commit:

`bd32f3f6af465d390ea76d1462dc1ef394666efd`

Message: `docs: align per-lane roadmap contract wording`

Changed the V2-M6 roadmap scope to the exact canonical phrase above. This is still documentation only and does not alter product behavior.

## Interpretation rules

The sequence of failures is evidence that the architecture test exposes stale wording one assertion at a time. It is **not** evidence of a new data-plane defect.

During this reconciliation:

- Go production tests repeatedly passed;
- no FakeTCP/DTLS/LINK/recovery implementation was modified;
- no release bound was relaxed;
- no test assertion was weakened or removed;
- ADR-0012 remains authoritative;
- ADR-0013 remains withdrawn;
- the per-lane single-flow/no-HOL design remains frozen.

Do not reopen the transport layer merely because another exact documentation phrase appears. Fix the smallest documentation contract mismatch and rerun the exact-head checks. Only new deterministic network qualification or physical Windows/Linux evidence may justify changing the mature transport path.

## Qualification rule after this log commit

Because this development log itself creates a new documentation-only HEAD, CI and handoff-verify must be evaluated on the new exact HEAD before declaring the documentation blocker closed.

Once those checks are green, the next engineering work should continue above the frozen transport layer: Logical Tunnel 1..4 lane lifecycle, Game Lane integration, make-before-break, idle/wake, packaging/qualification, or whichever first deterministic non-transport blocker the exact-head matrix exposes.
