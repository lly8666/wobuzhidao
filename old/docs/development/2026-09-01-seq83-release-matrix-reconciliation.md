# 2026-09-01 sequence 83 release-matrix reconciliation

## Context

Development resumed from canonical handoff sequence 79. The active product branch is `feat/single-flow-reality-faketcp`. Mature FakeTCP/TCP-like ACK/SACK/RTO/FEC behavior remains frozen. The current task is exact-head Windows/Linux qualification for the per-Transport-Lane single-public-flow product before any new physical artifact is handed to the user.

The user's latest physical Windows evidence was an older single-flow candidate that failed during FakeTCP startup with `faketcp: not ipv4/tcp`, then timed out waiting for the same-flow Reality ticket. The durable sequence-79 handoff records that unrelated-frame Npcap ingress filtering was added after that physical run, so the old failure is historical acceptance evidence rather than proof that the current candidate still has the same bug.

## Live refresh

The feature branch was refreshed at `7822b3ab6e21fd59307e65aea1721100cf4c6765` before this reconciliation. Its exact-head check-run set contained 36 completed checks with no observed failure or skipped conclusion. Notably, the combined hosted Windows + Linux qualification was green, and Linux raw/netns full-stack qualification was green for both FEC `off` and `20:20`.

That automatic push evidence is necessary but not sufficient for release authority because several expensive product/release workflows intentionally use path filters.

## Deterministic release-contract defect found

`docs/development/QUALIFICATION_KICK.md` and `.github/workflows/release-qualification-kick.yml` had drifted:

- the authority document listed `mux-load-100m.yml` among the 18 workflow-dispatch gates;
- the executable aggregator listed `product-lifecycle-e2e.yml` instead and did not dispatch `mux-load-100m.yml`;
- both still described the total as 27 child gates (18 dispatch + 9 exact-SHA push).

Dropping either gate is unjustified. `product-lifecycle-e2e.yml` is mandatory for the restored Logical Tunnel 1..4/Game/make-before-break product layer, while `mux-load-100m.yml` is mandatory release evidence for the frozen weak-network operating point and ensures the one-public-flow load path is not silently omitted by path filters.

## Fix policy

The release authority is strengthened rather than weakened:

- retain all existing 18 workflow-dispatch gates;
- add `mux-load-100m.yml` to the executable dispatch set;
- resulting exact-head matrix becomes 19 workflow-dispatch gates + 9 exact-SHA push gates = 28 child gates;
- add a repository release-contract test that requires both the executable workflow and the authority document to contain the same mandatory gate names and the 19/9/28 authority marker.

No FakeTCP/TCP-like transport, DTLS wire, LINK wire, FEC wire, Reality-like bootstrap wire, Game packet semantics, or route behavior is changed by this reconciliation.

## Qualification rule after this change

Any commit that moves the feature branch invalidates prior release authority. The final candidate must remain branch HEAD while `release-qualification-kick` dispatches and waits for all 19 opt-in workflows and resolves all 9 exact-SHA push workflows. Only a successful `WBD_RELEASE_QUALIFICATION_PASS ... total_children=28` on that immutable HEAD can authorize matching Windows/Linux artifacts for final physical Windows 11 Npcap/NIC/NAT/ISP -> Ubuntu ARM64 acceptance.
