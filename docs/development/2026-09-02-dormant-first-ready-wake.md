# 2026-09-02 — DORMANT first-ready wake repair

## Authority

ADR-0012 remains the Logical Tunnel lifecycle authority. `single-flow` remains per Transport Lane. This repair does not modify FakeTCP wire behavior, Reality-like admission, DTLS, LINK, FEC, Game PacketID semantics, or Linux routing/NAT architecture.

## Cold-start finding

Live refresh found `.wbd/handoff/current.json` at checkpoint `66181a27b18b7b165fe87866877f333c670eb60b` while the branch HEAD was already `3a4de30134f7789ecbc22bc193b8afe94d34e040`. The old cursor therefore had no direct execution authority.

Repository authority documents were still aligned with ADR-0012, but Windows lifecycle execution contained a deterministic product mismatch: `Controller.Wake()` rebuilt every configured lane and waited for all requested Game lanes before publishing any target back to the running Game process. That contradicted the product requirement that the first READY lane resume forwarding immediately and optional Game lanes refill afterward.

## Repair

Substantive commit:

`c7a0622352889ff8906db940b3e1e2bb5df3d6b1` — `windows: resume on first ready wake lane`

`Controller.Wake()` now:

1. validates the desired 1..4 product lane count;
2. creates a fresh generation-bearing Logical Tunnel lane lifecycle for the wake epoch;
3. starts lane 1 and publishes the currently READY target set immediately;
4. starts later Game lanes and republishes the growing READY target set incrementally;
5. on any later failure, first returns Game to an empty target set and then retires started public lanes before returning to DORMANT.

The shared Game process, Wintun/TUN, routes, DNS/network state, and IPv6 fail-close state are not restarted by this change.

## Exact-head automated evidence

For `SOURCE_SHA=c7a0622352889ff8906db940b3e1e2bb5df3d6b1`:

- workflow `windows-linux-single-flow`, run `33595409896`;
- job `windows native wire producer` completed `success`;
- step `Qualify Windows protocol/runtime contracts` completed `success`;
- that step executes `go test ./cmd/wbd-winlin-wire-vector ./cmd/wbd-faketcp ./internal/windowsruntime -count=1`, therefore the focused Windows runtime regression suite including the new first-ready wake assertions passed on hosted Windows.

At the time of this control update, downstream Linux consumption/full-stack portions of the workflow were still incomplete and MUST NOT be reported as passed. Other skipped/path-filtered checks are not PASS evidence.

## Remaining lifecycle gap

`Controller.ReplaceLane()` currently builds and starts a same-logical-ID candidate in a private transport slot while the old lane remains alive, then switches the Game target for that logical LaneID to the candidate endpoint and retires the old transport. The current Game control contract rejects duplicate logical LaneIDs, so this is not yet evidence for the required bounded old+candidate data-plane race `A -> A+B -> B`.

The next atomic action is to inspect Game/race lane identity versus transport-incarnation identity and implement the smallest architecture-consistent bounded replacement overlap using the existing logical PacketID namespace, without creating a migration sequence and without changing mature transport wire behavior.

## Qualification status

- First-ready DORMANT wake: IMPLEMENTED=yes; focused hosted-Windows AUTOMATED-GREEN=yes; PHYSICAL-GREEN=no; RELEASE-QUALIFIED=no.
- Planned replacement bounded data-plane race: IMPLEMENTED=partial; AUTOMATED-GREEN=no; PHYSICAL-GREEN=no; RELEASE-QUALIFIED=no.
- Overall release: NOT RELEASE READY.
