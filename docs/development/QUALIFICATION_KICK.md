# Exact-head release qualification kick

## Purpose

WBD release qualification must be tied to one exact candidate source HEAD. Many expensive Windows/Linux workflows intentionally use path filters, so documentation-only reconciliation commits do not automatically re-run every release gate even when product code is unchanged.

This file is the durable, non-product trigger for the feature-branch qualification dispatcher in `.github/workflows/release-qualification-kick.yml`.

Changing the `Kick generation` value below is allowed only when the intent is to re-run the complete hosted release qualification matrix. It must never be used to hide or bypass a deterministic failure.

## Rules

1. The dispatcher is CI infrastructure only. It does not modify FakeTCP, DTLS, LINK, FEC, Game Lane, Logical Tunnel or runtime product semantics.
2. The dispatcher uses existing release-authority workflows; it does not duplicate their test logic.
3. Before dispatching, it verifies that `feat/single-flow-reality-faketcp` still resolves to the dispatcher run's own `GITHUB_SHA`. If the branch moved, the dispatcher fails instead of creating mixed-head evidence.
4. Every dispatched workflow result must later be checked by exact `head_sha`; a successful dispatcher only means the workflows were requested, not that release qualification passed.
5. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 remains final acceptance. Hosted qualification must be green first.
6. Mature TCP-like/FakeTCP recovery remains frozen unless a deterministic qualification failure isolates a defect there.
7. Matching Windows/Linux artifacts may be delivered only after the exact candidate HEAD has the required hosted release gates green.

## Hosted release-authority set

The dispatcher requests the existing workflows that cover:

- hosted native Windows -> Linux single-flow wire + Linux raw/netns full stack;
- Windows portable bundle;
- Windows TUN build;
- Windows administrator TUN smoke;
- Windows raw-IP gateway qualification;
- Linux server release bundles;
- Linux WBD-owned firewall qualification;
- Game Lane full stack;
- FakeTCP native recovery;
- FakeTCP 20% loss pcap;
- full-stack first-arrival weak-network matrix;
- 100 Mbit mux/load release operating-point qualification;
- OpenWrt TCP TPROXY and one-shot full stack;
- single-flow startup stress and LINK full stack where those workflows expose `workflow_dispatch`.

Single-flow one-SYN E2E, no-HOL and persona workflows that run on every feature-branch push remain independent required gates and are checked on the same candidate HEAD.

## Kick generation

`2026-08-31T16:00+08-seq1`

## Evidence recording

After a kick, record the exact candidate SHA and run IDs/results in a dated file under `docs/development/` and in the next canonical `.wbd/handoff/current.json` sequence. Do not call queued/in-progress runs green.
