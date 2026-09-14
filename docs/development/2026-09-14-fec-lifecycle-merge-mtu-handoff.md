# 2026-09-14 FEC lifecycle merge + MTU follow-up handoff

## Purpose

This record is the substantive development handoff after merging the bounded FEC recovery work into `release/20260914-fec-baseline`. It is written so a completely new agent can resume without chat history.

## Current branch and merged state

- Repository: `lly8666/wobuzhidao`
- Active integration branch: `release/20260914-fec-baseline`
- FEC lifecycle PR: #13 `fix(fec): bound heavy recovery lifetime`
- PR head before merge: `999369bb7b9af7896bebbfa0aea90dd8758dfb52`
- Merge commit: `f8951a7065001a0850e178dd0bfb182cb4973ae2`
- Release baseline before PR: `d6f4a7010f8695a2d2fd43a5872c30bd9b38733f`
- Frozen preview/FEC640 baseline: `b8ddd5a48a0c1d7dbed0e593004fe290edd86aba`
- DataPlane FEC wire ownership fix: `bfca8bdc8bc42840607a1a5cc6de18b806e02bb8`

The published preview/tag remains unchanged. The release branch is the post-preview development line.

## What was merged in PR #13

The six commits merged above the FEC640 baseline are:

1. `73228a0b72dcc9b6cd1885ff6cd6ba9624e71131` — constant-time decoder pressure counters.
2. `46b82b4833e8cf58fdd5873b03546c56617d59f5` — move detailed LINK/FEC pressure scans off packet hot path.
3. `1f478df414bc21fc548a4e6d0b28fe0d5cbb715f` — focused `internal/fec` + `internal/linkdata` unit/race gate.
4. `335d00129930251cb6210dfd4af3f0d28555c77e` — bound incomplete heavy FEC recovery by a 2 second absolute deadline.
5. `e10de07f96a55c84f923c372e272039f36a778b6` — first paired 5 Mbps 5%→30%→5% validation workflow.
6. `999369bb7b9af7896bebbfa0aea90dd8758dfb52` — repeated paired validation workflow.

### Bounded recovery semantics

After 2 seconds, an incomplete heavy decoder block is compacted into the existing retired representation. Heavy parity/reconstruction payload is released. Late systematic source packets must still be first-deliverable, and duplicate suppression must remain correct. This is a lifecycle/state bound, not a change to FEC geometry or MTU.

## FEC validation evidence

Reference profile used for paired business validation:

- 5 Mbps each direction
- 300 ms one-way delay
- 120 seconds total
- 5% loss for 0–30 s, 30% for 30–90 s, 5% for 90–120 s
- native `maxBlocks=640`

Two clean stats-only vs bounded pairs are authoritative for this lifecycle conclusion.

### Clean pair 1 — run 34803505742

- heavy peak in flight: 214 → 18 (-91.6%)
- max active missing sources: 1422 → 116 (-91.8%)
- reconstruct success: 18170 → 18098 (-0.40%)
- raw spike application loss: 1.616% → 1.793%
- bounded sample saw about 0.143 percentage-point harsher actual netem loss, so the raw +0.177 pp application-loss delta is not attributable to the 2 s deadline alone.

### Clean pair 2 — run 34805760375

- heavy peak in flight: 219 → 14 (-93.6%)
- max active missing sources: 1320 → 88 (-93.3%)
- reconstruct success: 18148 → 18175 (+0.15%)
- raw spike application loss: 2.294% → 1.632%
- bounded sample saw about 0.197 percentage-point easier actual netem loss.

The business-loss direction reversed across the two clean pairs as the random netem realization changed. The lifecycle-state reduction repeated strongly, while reconstruct success stayed within ±0.4%. This supports the intended claim: the 2 s deadline bounds long-lived heavy recovery state without evidence of systematic large-scale recovery truncation.

A third repeated pair is intentionally excluded from release evidence: bounded-r3 had one skipped send slot, and stats-r3 recorded 855 UDP `RcvbufErrors`.

### CPU interpretation

Do not market this as a CPU optimization. LINK CPU was effectively neutral/slightly higher in the clean pairs (about +0.001 to +0.002 core). The demonstrated benefit is bounded heavy FEC recovery state/memory lifetime and lower lifecycle pressure.

## Known CI state around the FEC merge

Before `335d001`, the stats-only parent `1f478df` already had unrelated repository-wide red checks. Therefore these are not FEC deadline regressions:

- generic `ci`: stale MTU range assertions; targeted FEC/LINK packages pass.
- `faketcp-native`: existing SACK/RACK 10% loss smoke requires strict 1000/1000 delivery; the parent produced 991/1000 and the later head produced 986/1000 after unit/build passed.
- `apply-server-negotiated-mtu-fix`: historical/stale MTU workflow; not authority for the merged FEC lifecycle conclusion.

The merged PR had `mergeable=true`; it was intentionally merged by user authorization despite known pre-existing unstable checks.

## MTU status: NOT fully fixed

Do not tell the user or a future agent that MTU is fully fixed.

What is already present:

- a unified path-budget derivation exists and is the source of truth for actual encapsulation budgeting;
- the FEC ownership corruption bug is fixed separately at the DataPlane ownership boundary;
- recent full-stack tests did not reproduce transport-integrity corruption under the tested packet/path combinations.

What remains unresolved:

1. `cmd/wbd-link-server-mux/mtu_policy_test.go` contains stale expectations that treat 1461/1501 as universally illegal inner MTUs even though the protocol representation bounds are now wider.
2. `linkPolicyForInnerMTU` validates a protocol-representable value but does not itself prove the value fits this association's actual carrier/interface budget and enabled FEC/Game overhead.
3. The full `LINK_INIT → policy validation → association activation` path must be audited to ensure every negotiated value is checked against the real per-association path budget before activation.
4. Shared server associations must validate against their own connection budget; do not use one global inner-MTU default to cover all clients.
5. An oversize backend/application datagram must have an explicit product behavior. The current service loop can return after `Outbound` error, which can leave an apparently active association with the backend read loop gone. The fix must either deliberately drop/count and continue for the known oversize case, or deliberately close/notify the association; do not silently leave a dead data path.

### Current 1500-path budget reference

For connection MTU 1500 with the current IPv4/TCP carrier, FEC20:20 and Game enabled, preserve this budget chain:

`1500 connection → 1460 carrier payload → 1428 DTLS plaintext → 1372 LINK plaintext → 1332 inner IP`

Critical guardrails:

- Do **not** change the FEC decoder payload upper bound from 1372 to 1428. The original corruption was not an MTU arithmetic bug.
- Do not widen MTU to hide framing/ownership errors.
- Do not copy another independent set of subtract-constant formulas; reuse the canonical path-MTU derivation.
- Distinguish protocol representation bounds from transport/association bounds: a value fitting the wire field is not automatically safe on a 1500-byte carrier.
- Do not silently shrink a peer proposal unless the existing protocol explicitly negotiates and confirms the smaller value.
- Keep the published preview tag unchanged; post-preview fixes stay on new branches/commits.

### Minimum MTU acceptance criteria

A new MTU-fix branch should not be considered complete until all of the following are covered:

- update the stale range tests using actual protocol min/max boundaries; keep 1461/1501 legal when they are within representation bounds;
- for the current 1500/FEC/Game transport budget, LINK plaintext 1372 is accepted and 1373 is rejected before data transfer;
- test the FEC-off budget separately through the canonical derivation;
- run two simultaneous associations with different valid budgets and prove one does not mutate/override the other;
- reject over-budget negotiation during setup rather than waiting for packet send failure;
- after an over-budget backend packet, send a later legal packet and prove behavior matches the chosen explicit policy (continue or clean close), with no hidden dead channel;
- verify the edge using real FEC + native DTLS + FakeTCP and prove the boundary packet stays in one carrier; do not substitute a fake fixed DTLS header test as sole evidence;
- test at least one smaller connection budget and one explicitly supported large-MTU environment in addition to 1500.

## Next development task

The FEC lifecycle phase is complete and merged. The next engineering task is the MTU follow-up.

Start from the live `release/20260914-fec-baseline` head after refreshing it. Create a dedicated MTU branch; keep test-contract repair, association-budget enforcement/oversize handling, and any additional harness work in separate commits where practical.

First code-reading path:

1. `MTU_FOLLOWUP.md`
2. `internal/pathmtu` derivation and its tests
3. `cmd/wbd-link-server-mux/mtu_policy.go` and tests
4. LINK_INIT/control policy validation path
5. association activation path in server mux
6. `internal/linkdata.Path.Outbound` call sites and the server `serviceLoop` error behavior
7. native DTLS/FEC/FakeTCP boundary tests/workflows

The first atomic action is to trace the negotiated MTU value from LINK_INIT through activation and identify the exact point where a per-association carrier-derived maximum must be enforced. Do not edit the wire format unless that trace proves it is necessary.

## Repository handoff rule

After this substantive record, `.wbd/handoff/current.json` must be refreshed in a separate final handoff-only commit. Its `checkpoint_based_on_head_sha` must point to the commit that adds this record, not to the handoff-only commit itself.
