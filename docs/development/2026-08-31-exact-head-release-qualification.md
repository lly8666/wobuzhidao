# Exact-head release qualification aggregation

Date: 2026-08-31
Branch: `feat/single-flow-reality-faketcp`
Baseline inspected: `8571f077925bd6bdaa925239f4f5058b7fd03d82`

## Context

The single-public-flow architecture is now release-sensitive: Windows and Linux must be qualified from the same immutable source commit before matching artifacts may be delivered. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 remains final acceptance, but hosted qualification must be complete first.

The mature TCP-like/FakeTCP recovery, DTLS/FEC sustained data-plane semantics, and the no-HOL contract remain frozen. This work changes CI qualification authority only.

## Deterministic gap found

`release-qualification-kick.yml` checked that the feature branch still pointed to its own `GITHUB_SHA`, dispatched eleven workflows, checked the branch again, and then printed a PASS marker. It did **not** wait for the child workflow runs, verify each child run's immutable `head_sha`, or aggregate child conclusions.

Therefore a green dispatcher only proved that GitHub accepted the dispatch requests. It did not prove release qualification.

A second drift existed between `docs/development/QUALIFICATION_KICK.md` and the workflow. The document described FakeTCP recovery, 20% pcap, first-arrival, OpenWrt and single-flow persona/no-HOL gates as release authority, but the dispatcher did not itself prove all of those gates on the candidate SHA.

## Exact-head authority model

The corrected aggregator must distinguish two classes.

### Explicit workflow-dispatch gates

These are path-filtered or intentionally opt-in expensive gates. The aggregator dispatches them against `feat/single-flow-reality-faketcp`, then waits for a `workflow_dispatch` run whose `head_sha` equals the aggregator's own `GITHUB_SHA`, and requires `conclusion=success`:

- `windows-linux-single-flow.yml`
- `windows-portable-bundle.yml`
- `windows-tun-build.yml`
- `windows-tun-admin-smoke.yml`
- `windows-rawip-gateway.yml`
- `linux-server-release.yml`
- `linux-server-firewall.yml`
- `game-lane-fullstack.yml`
- `mux-load-100m.yml`
- `single-flow-startup-stress.yml`
- `single-flow-link-fullstack.yml`
- `faketcp-recovery-ab.yml`
- `openwrt-fullstack-one-shot.yml`

### Candidate-push gates

These workflows already run automatically on every relevant push. The final aggregator commit itself must have a completed `push` run with `head_sha == GITHUB_SHA` and `conclusion=success`:

- `ci.yml`
- `faketcp-native.yml`
- `faketcp-pcap-20loss.yml`
- `fullstack-first-arrival.yml`
- `openwrt-tcp-tproxy.yml`
- `single-flow-e2e.yml`
- `single-flow-no-hol.yml`
- `single-flow-tcp-persona.yml`

The aggregator must not accept an older successful run, a run from another SHA, a pull-request merge SHA, or a run that merely started successfully.

## Required aggregator behavior

1. Verify the branch points at `GITHUB_SHA` before any dispatch.
2. Record the dispatch start time for each explicit workflow.
3. Dispatch all explicit workflows without serially waiting, so expensive gates can run in parallel.
4. Resolve the new child run by workflow identity, event, creation time and exact `head_sha`.
5. Wait until every explicit child is `completed` and require `success`.
6. Resolve and wait for every required candidate-push workflow on the same exact `GITHUB_SHA`.
7. Verify the feature branch still points at `GITHUB_SHA` after all children finish.
8. Emit one authoritative `WBD_RELEASE_QUALIFICATION_PASS` marker only after every required child passes.
9. Emit per-child evidence containing workflow, event, run id, head SHA and conclusion so the handoff can preserve exact evidence.

## Why this is required

The user-facing delivery rule is stricter than ordinary CI convenience: Windows portable and Linux ARM64 artifacts must be demonstrably built and tested from one source state. A dispatcher that only requests work cannot establish this invariant.

This change deliberately does not touch FakeTCP ARQ, FEC, DTLS wire semantics, LINK, logical tunnel routing, Game Lane or product runtime behavior.

## Next atomic action

Update `docs/development/QUALIFICATION_KICK.md`, then make `release-qualification-kick.yml` the final branch change. Let that exact commit run its full matrix without moving the branch. If it fails, inspect the first deterministic child failure; do not deliver artifacts until the authoritative aggregate is green.
