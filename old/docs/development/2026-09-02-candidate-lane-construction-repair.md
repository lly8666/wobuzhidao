# 2026-09-02 Windows candidate-lane construction repair

## Scope

Atomic ADR-0012 rollback repair limited to **candidate lane bootstrap/plan construction**. This phase deliberately does not enable runtime overlap execution, replacement transitions, trigger handling or Game rotation.

No FakeTCP/Reality-like/DTLS/LINK/FEC wire code is changed.

## Drift

`internal/windowsruntime/candidate_lane.go` had been overwritten by the global-one-lane rollback:

- `BuildCandidateLaneBootstrap*` returned `ErrOverlappingPublicFlow` unconditionally;
- `BuildCandidateLanePlan*` returned `ErrOverlappingPublicFlow` unconditionally;
- `buildLanePlanForSlot` rejected candidate plans and any logical lane ID other than 1;
- `LaneGameTarget` rejected lane IDs beyond 1;
- comments claimed ADR-0015 required break-before-make.

Its current test required candidate construction to fail.

This directly contradicted ADR-0012 make-before-break and Game one-lane-at-a-time rotation.

## Historical evidence used instead of redesign

Git history for `candidate_lane.go` was inspected before editing.

The rollback commit was:

```text
5e24b006b605b298f26415875416709d9113f2b9  windows: forbid candidate public flow overlap
```

The rollback parent `bd1f91985d84c45896335b6d152f36d9b02f8237` and earlier candidate commits preserved the intended ADR-0012 implementation, including:

```text
b1b69c3e49c59422743ecb52877fc1be3f24dfec  windows: build same-logical-id make-before-break candidate slot
cf41a27bbd4a2eb2dce75975764c1346356ec3e9  windows: build authenticated normal and candidate lane plans
a6cfb8de53128af775a9b8beedd9526abe8b0366  windows: alternate private and normal slots across repeated MBB
7822b3ab6e21fd59307e65aea1721100cf4c6765  test: align candidate lane names with transport slot
```

This historical design was restored rather than inventing a new migration primitive.

## Restored contract

A candidate is a **new transport incarnation of an existing logical LaneID**, not a fifth logical Game lane and not a second logical PacketID space.

Default candidate slot:

```text
makeBeforeBreakCandidateSlot = 5
```

Logical IDs remain bounded to 1..4. Transport slot 5 is private overlap capacity for an unjoined replacement candidate.

Candidate bootstrap:

- uses the same server public endpoint;
- uses its own assigned dynamic FakeTCP source port;
- uses slot-specific loopback ports and state paths;
- performs the same complete per-lane FakeTCP + Reality-like setup;
- uses the same InstallationID so authenticated tunnel configuration attaches to the same Logical Tunnel/lease.

Candidate plan:

- preserves the same logical LaneID;
- keeps `Slot=5` by default;
- starts lane-local DTLS and LINK against slot-specific loopbacks;
- retains FEC as lane-local configuration;
- exposes the candidate LINK target to the existing Game/race layer.

`NextReplacementSlot` preserves the historical slot-5/normal-slot alternation for repeated replacements.

## Commits

```text
5f430c7242e8eba6944bebdd04893b9f59107a8c  windows: restore candidate lane construction
18729222b08500ceac108b897eeea1f6a359b700  test: restore candidate lane construction contract
```

The direct contract test proves:

- logical LaneID 4 may build a candidate in private slot 5;
- slot-specific FakeTCP/DTLS/LINK process identities and loopbacks are distinct;
- LaneGameTarget resolves the candidate slot;
- logical lane ID 5 is rejected;
- transport slot 6 is rejected;
- repeated replacement slot selection alternates between private slot 5 and the logical lane's normal slot.

The old runtime test that actually starts a candidate process group was **not** restored in this atomic phase because `dynamic_lane.go` still intentionally contains the next known rollback defect.

`ErrOverlappingPublicFlow` remains temporarily defined only so the still-unrepaired dynamic runtime compiles; candidate constructors no longer use it.

## Automation snapshot

Exact code/test HEAD:

```text
18729222b08500ceac108b897eeea1f6a359b700
```

At first exact-head refresh the relevant workflow matrix was queued/pending, including:

```text
ci                      33584315064  queued
handoff-verify          33584315039  queued
game-settings-matrix    33584315117  queued
game-lane-fullstack     33584315059  queued
product-lifecycle-e2e   33584315105  queued
windows-portable-bundle 33584315125  queued
linux-server-release    33584315083  queued
mux-load-100m           33584315095  pending
```

Skipped/path-filtered workflows are not green evidence.

## Status

```text
Candidate lane construction/planning:
IMPLEMENTED: yes
AUTOMATED-GREEN: no / pending exact-head CI
PHYSICAL-GREEN: no
RELEASE-QUALIFIED: no
```

## Next atomic action

Repair `internal/windowsruntime/dynamic_lane.go` execution admission only:

- allow independent normal lane IDs 1..4;
- allow a same-logical-ID candidate in private slot 5 to coexist with the old incarnation during bounded make-before-break;
- keep process names/slots collision-safe;
- preserve shared Game/TUN/routes;
- enumerate active logical LaneIDs without counting candidate slot 5 as a fifth logical lane;
- restore direct executor tests from pre-rollback history.

Do not yet implement Controller replacement triggers/state transitions in that phase.
