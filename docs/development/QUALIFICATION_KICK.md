# Exact-head release qualification kick

## Purpose

WBD release qualification must be tied to one exact candidate source HEAD. Many expensive Windows/Linux workflows intentionally use path filters, so documentation-only reconciliation commits do not automatically re-run every release gate even when product code is unchanged.

This file is the durable, non-product trigger for the feature-branch qualification aggregator in `.github/workflows/release-qualification-kick.yml`.

Changing the `Kick generation` value below is allowed only when the intent is to re-run the complete hosted release qualification matrix. It must never be used to hide or bypass a deterministic failure.

## Rules

1. The aggregator is CI infrastructure only. It does not modify FakeTCP, DTLS, LINK, FEC, Game Lane, Logical Tunnel or runtime product semantics.
2. The aggregator uses existing release-authority workflows; it does not duplicate their test logic.
3. Before dispatching, it verifies that `feat/single-flow-reality-faketcp` still resolves to the aggregator run's own `GITHUB_SHA`. If the branch moved, the aggregator fails instead of creating mixed-head evidence.
4. A successful aggregator is authoritative: it must wait for every required child workflow, verify the immutable child `head_sha` equals its own exact candidate SHA, require the expected event (`workflow_dispatch` or `push`), and require `conclusion=success`.
5. The aggregator must not accept an older success, a pull-request merge SHA, a run merely because it started, or a run created before the exact candidate commit.
6. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 remains final acceptance. Hosted qualification must be green first.
7. Mature TCP-like/FakeTCP recovery remains frozen unless a deterministic qualification failure isolates a defect there.
8. Matching Windows/Linux artifacts may be delivered only after the exact candidate HEAD has the required hosted release gates green.

## Explicit workflow-dispatch authority set

The aggregator dispatches these workflows against the feature branch and then waits for the newly-created `workflow_dispatch` run on the exact candidate SHA:

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

## Exact-candidate push authority set

These workflows already run automatically for the final aggregator commit. The aggregator waits for their `push` run on the same exact candidate SHA and requires success:

- `ci.yml`
- `faketcp-native.yml`
- `faketcp-pcap-20loss.yml`
- `fullstack-first-arrival.yml`
- `openwrt-tcp-tproxy.yml`
- `single-flow-e2e.yml`
- `single-flow-no-hol.yml`
- `single-flow-tcp-persona.yml`

Together these cover hosted native Windows -> Linux single-flow wire + Linux raw/netns full stack; Windows portable and TUN qualification; Windows administrator TUN and raw-IP gateway checks; Linux server release and WBD-owned firewall qualification; Game Lane research regression; FakeTCP recovery and 20% pcap behavior; first-arrival weak-network behavior; 100 Mbit mux/load release operating-point qualification; OpenWrt; one-SYN/same-sequence-space E2E; startup stress; no-HOL; and TCP/Reality-like persona checks.

## Kick generation

`2026-08-31-seq77-handoff-green`

## Evidence recording

The aggregator prints one `WBD_RELEASE_CHILD_PASS` line for every required child and emits `WBD_RELEASE_QUALIFICATION_PASS` only after the full exact-head matrix succeeds. Record the candidate SHA, authoritative aggregator run ID, child run IDs/results and any physical-only acceptance gap in a dated file under `docs/development/` and in the next canonical `.wbd/handoff/current.json` sequence. Do not call queued/in-progress runs green.
