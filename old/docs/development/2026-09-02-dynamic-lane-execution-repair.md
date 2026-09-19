# 2026-09-02 Windows dynamic-lane execution repair

## Scope

Atomic ADR-0012 rollback repair limited to Windows dynamic transport execution admission.

This phase deliberately does **not** redesign or modify mature FakeTCP ACK/SACK/RTO/recovery, same-association Reality-like bootstrap, pinned wolfSSL DTLS 1.3, LINK, FEC wire, Game scheduler policy, DORMANT triggers, or the Controller replacement state machine.

Per current authority, `single-flow` is **per Transport Lane**. One Logical Tunnel may own logical LaneIDs 1..4. A planned healthy make-before-break replacement may temporarily run old incarnation A plus candidate B for the **same logical LaneID**. Private transport slot 5 is overlap capacity only; it is not a fifth logical Game lane and does not create another Logical Tunnel PacketID namespace.

## Recovery point

Sequence 88 handoff identified the next bounded rollback defect:

- candidate bootstrap/plan construction had already been restored;
- `internal/windowsruntime/dynamic_lane.go` still contained the later global-one-flow rollback;
- runtime admission hard-coded logical ID 1 / slot 1;
- any existing public FakeTCP process caused `ErrOverlappingPublicFlow`;
- normal dynamic lanes 2..4 and a slot-5 same-ID replacement candidate were therefore unreachable.

The exact branch at phase start was:

```text
feat/single-flow-reality-faketcp
d87b031b7e622fb728a589c892510cb079eb96c3  handoff: sync sequence 88
```

The handoff's substantive checkpoint was `66181a27b18b7b165fe87866877f333c670eb60b`.

## Historical evidence

Before editing, Git history for `internal/windowsruntime/dynamic_lane.go` was inspected.

Relevant history:

```text
876f7f6bdacd4933212fd49c1aaf4e486044cd0a  windows: manage dynamic lane process groups above shared TUN
3359109ea3db83941b2d50b7d5be4257fc986e9f  windows: support transient candidate transport process groups
12abc3f7d97190c7aa50f3a297474308f8180532  windows: block second dynamic public flow
```

The pre-rollback `3359109e...` implementation already had the intended execution shape:

- logical LaneID range 1..4;
- independent per-lane process groups;
- process-name collision rejection;
- candidate failure rollback limited to the candidate process group;
- shared Game/TUN/routes left alive;
- logical lane enumeration deduplicated to 1..4.

That implementation was used as recovery evidence instead of redesigning the runtime.

## Repair implementation

Commit:

```text
2acfb847e472e1212e61a3f02045d88716a054a9  windows: restore dynamic lane overlap execution
```

Changes in `internal/windowsruntime/dynamic_lane.go`:

1. Restored logical LaneID admission for IDs 1..4.
2. Restored normal transport slot `slot == LaneID`.
3. Added explicit current-contract admission for private make-before-break slot 5.
4. Rejected cross-lane slot theft such as logical lane 1 using normal slot 2.
5. Removed the global `hasPublicFakeTCPLocked` exclusion from `StartDynamicLane`.
6. Preserved strict process-name collision rejection.
7. Preserved candidate-local rollback: only processes appended by the attempted dynamic start are stopped.
8. Preserved shared runtime state: Game/TUN/routes are not restarted or cleaned during dynamic lane start/stop.
9. Restored `StopDynamicLane` normal-slot support for logical IDs 1..4.
10. Updated `DynamicLaneIDs()` so candidate process names such as `link-1-candidate-s5` are attributed to logical LaneID 1. Slot 5 can never appear as a fifth logical lane.

No transport wire code changed.

## Direct contract tests

Commit:

```text
a58bc077220b20f3dbd161bd3d4d96dc2928f900  test: restore dynamic lane overlap contract
```

`internal/windowsruntime/dynamic_lane_test.go` now directly checks:

- normal logical IDs 1..4 are valid;
- same-ID private slot-5 candidates are valid;
- logical lane 5 is rejected;
- slot 6 is rejected;
- cross-lane normal slot theft is rejected;
- a failed dynamic lane 2 start rolls back only lane 2 and preserves lane 1 + shared runtime;
- an independent lane 2 can run alongside lane 1;
- stopping normal lane 1 leaves lane 2 alive;
- a logical lane 1 candidate in slot 5 coexists with old lane 1;
- logical enumeration remains `[1]` during old+candidate overlap;
- stopping the candidate leaves old lane 1 and shared Game/TUN alive;
- any process-name collision is rejected before runtime mutation.

## Live automation status at first exact-head refresh

Exact test HEAD:

```text
a58bc077220b20f3dbd161bd3d4d96dc2928f900
```

The PR #9 workflow matrix was successfully triggered. At the first commit-level refresh, `ci` and the main product/network qualification workflows were queued/pending; path-filtered workflows such as recovery AB variants were skipped by condition and are not counted as green evidence.

Representative run IDs at that refresh:

```text
ci                         33585237600  queued
single-flow-e2e            33585237538  queued
single-flow-link-fullstack 33585237836  queued
single-flow-no-hol         33585237754  queued
windows-linux-single-flow  33585237883  queued
product-lifecycle-e2e      33585237555  pending
game-lane-fullstack        33585237556  pending
mux-load-100m              33585237588  pending
windows-portable-bundle    33585237735  queued
linux-server-release       33585237755  queued
handoff-verify             33585237728  queued
```

No release qualification claim is made while these are incomplete.

## Current status

```text
Dynamic execution repair:
IMPLEMENTED: yes
DIRECT TESTS WRITTEN: yes
AUTOMATED-GREEN: pending exact-head Actions
PHYSICAL-GREEN: no / not part of this atomic phase
RELEASE-QUALIFIED: no
```

## Stop rule / next action

Do not wire `Controller.ReplaceLane` or trigger policy until this execution layer is green.

Next action:

1. wait for exact-head `ci` to execute and inspect any deterministic failure;
2. if direct tests are green, allow the broader exact-head matrix to complete far enough to rule out orchestration regressions;
3. append exact run results to this log;
4. advance handoff from sequence 88 with this repair and the next single bounded lifecycle defect.
