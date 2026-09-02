# 2026-09-02 Windows profile/runtime lane contract repair

## Scope

Atomic contract repair only. This phase does not modify Windows runtime production code or any transport wire implementation. It corrects stale tests that still encoded the withdrawn global-one-lane policy after the central ADR-0012 lane-count validator was restored to 1..4.

## Live state before the phase

After the core cardinality repair, production code already had the correct dependency chain:

```text
windowsgui.RuntimeProfileFile.Lanes
  -> windowsruntime.Profile.Lanes
  -> Profile.Validate()
  -> logicaltunnel.ValidateProductTransportLaneCount()
```

`internal/windowsgui/config.go` defaults omitted/zero `lanes` to Normal mode lane count 1 and otherwise delegates validation to `windowsruntime.Profile.Validate`.

`internal/windowsruntime/plan.go` already documents Normal=1 / Game=2..4 and delegates `Profile.Lanes` validation to the logicaltunnel product cardinality contract.

Therefore, after `MaxProductPublicTransportLanes` returned to 4, the **production profile/runtime validation became ADR-0012-correct without additional executable changes**.

## Stale tests found

Two direct tests still represented the withdrawn global-one-lane architecture:

- `internal/windowsgui/config_test.go`
  - `TestLoadRuntimeProfileAppliesGlobalSingleFlowDefaults`
  - `TestLoadRuntimeProfileAcceptsOnlyOneProductPublicTransport`
  - explicitly rejected lanes 2,3,4.
- `internal/windowsruntime/plan_test.go`
  - `TestBuildPlanUsesGlobalSingleFlowWindowsStack`
  - `TestProfileAllowsExactlyOneProductPublicTransport`
  - explicitly rejected lanes 2,3,4.

Per the development rule, this was classified as **test stale / implementation correct after central validator repair**. The implementation was not rolled back to satisfy stale tests.

## Changes

```text
dc4e28c16ccaca384f00b85cf625428496fec9b3  test: restore Windows profile lane range
fb24ffd57f387df8b6ed722706ae990f3e6977b0  test: restore Windows runtime lane range
```

Contracts now assert:

- default/omitted lane count -> 1 (Normal mode);
- explicit active lane counts 1,2,3,4 accepted;
- 5+ and negative values rejected;
- explicit JSON `lanes: 0` remains the existing default sentinel and normalizes to 1. This is configuration-default behavior, not an active zero-lane product state; DORMANT is a lifecycle state rather than a connected-profile lane count.

Test names were corrected to remove `GlobalSingleFlow`/`ExactlyOneProductPublicTransport` terminology.

## Frozen components

No change was made to:

- `internal/windowsgui/config.go` production behavior;
- `internal/windowsruntime/plan.go` production behavior;
- FakeTCP / Reality-like bootstrap / DTLS / LINK / FEC;
- candidate/make-before-break runtime;
- Game aggregation;
- server lane admission;
- DORMANT/wake or trigger handling.

## Exact-head automation snapshot

Live refresh after `fb24ffd57f387df8b6ed722706ae990f3e6977b0` showed the current workflow matrix still queued/pending. Relevant runs included:

```text
ci                         33583947191  queued
handoff-verify             33583947179  queued
game-settings-matrix       33583947157  queued
game-lane-fullstack        33583947136  queued
product-lifecycle-e2e      33583947140  queued
shared-tun-two-client      33583947172  queued
windows-portable-bundle    33583947113  queued
linux-server-release       33583947318  queued
mux-load-100m              33583947217  pending
```

Several unrelated/path-filtered workflows were skipped; they are **not** counted green.

## Status

```text
Windows profile/runtime 1..4 lane contract:
IMPLEMENTED: yes
AUTOMATED-GREEN: no / exact-head workflows pending
PHYSICAL-GREEN: no
RELEASE-QUALIFIED: no
```

## Next atomic action

Audit and restore the Windows candidate-lane construction contract that was explicitly disabled by the later global-one-lane rollback.

The next phase must be bounded to candidate construction/planning only. It should recover the existing ADR-0012 design rather than inventing a new migration protocol:

- candidate uses an independent transport slot/source port;
- candidate keeps the same logical LaneID while using a distinct transport incarnation/slot;
- candidate performs the same complete per-lane FakeTCP -> Reality-like bootstrap -> DTLS -> LINK lineage;
- no second logical PacketID sequence is created;
- the phase does not yet wire the full replacement state machine or trigger integration.

Inspect historical implementation/tests before editing; do not redesign mature TCP-like transport.
