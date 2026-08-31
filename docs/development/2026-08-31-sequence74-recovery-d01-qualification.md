# 2026-08-31 — Sequence 74 recovery and d01 exact-head qualification

## Why this log exists

Conversation restoration lost a large amount of development history. GitHub did not lose the work. This file records the live repository state recovered before continuing so later sessions do not repeat already-completed transport work or follow stale handoff next-steps.

## Live repository state recovered

Canonical branch:

- `dev/wbd-raw-fec-v2`
- live head at recovery: `9d001a6a6d6fb7b6bd9d7c5328c675219cbbf221`
- canonical handoff: sequence 74

Feature branch:

- `feat/single-flow-reality-faketcp`
- live substantive head at recovery: `d01e15a20646b3fd901378940b07546a074443b7`
- PR: #9 `Single-flow release: Firefox-like bootstrap, no-HOL switch, Windows portable`

Sequence 74 is useful historical evidence but its recorded feature head `faa4f3209886a45a6fcdacbd11da820312032f88` is stale. The feature branch advanced after that checkpoint.

## Architecture authority after recovery

ADR-0014 is the current product-owner final freeze. Where older logs, ADR-0012 or withdrawn/superseded clauses conflict, ADR-0014 controls.

The product invariant is global, not per-lane:

- exactly one active WBD public TCP-shaped connection lineage for one connected Logical Tunnel;
- one client/server 4-tuple;
- one FakeTCP SYN/SYN-ACK/ACK lineage and one FakeTCP sequence space;
- FakeTCP owns the public flow from the first SYN;
- real TLS 1.3 Reality-like admission runs over the bounded reliable/ordered bootstrap phase of that same FakeTCP association;
- the bootstrap barrier does not FIN, RST or create another WBD SYN;
- the same association then carries pinned wolfSSL DTLS 1.3, LINK and FEC/datagram VPN payload;
- sustained payload must not inherit ordinary kernel-TCP HOL;
- no concurrent Game lane, replacement lane, multipath lane or make-before-break public overlap is product behavior;
- the mature FakeTCP recovery/FEC/TCP-like data plane remains frozen absent deterministic lower-layer evidence.

Recent commits after the sequence-74 qualified head further enforce this final global-one-flow contract, including removal of the Linux product Game public hop and release tests that reject obsolete/extra Windows public transport children.

## What was already done and must not be repeated

The repository already contains and has historically qualified:

- same-association Firefox-120-like TLS 1.3 / Reality-like bootstrap;
- protected admission and Logical Tunnel ticket/lease binding;
- explicit same-flow switch to pinned wolfSSL DTLS 1.3;
- one-SYN/same-sequence-space packet invariants;
- post-bootstrap no-HOL hole-bypass qualification;
- FEC `off` and fixed systematic `20:20` qualification;
- Linux raw/netns full-stack and weak-network qualification;
- Windows hosted runtime/TUN qualification and native Windows -> Linux single-flow wire qualification;
- Windows portable and Linux amd64/arm64 build workflows;
- release-contract checks that reject multiple product public transports.

Do not reopen the mature TCP-like/FakeTCP core merely because conversation context is incomplete.

## Live CI observation at `d01e15a2...`

The Actions query for the live feature head returned 15 completed runs. No run in that exact-head result had `conclusion=failure` and none remained in progress. `windows-linux-single-flow` was among the successful exact-head runs.

This is strong ordinary push evidence, but it is not sufficient release authority by itself.

## Remaining qualification gap

The corrected `release-qualification-kick.yml` is the authoritative exact-head aggregator. It dispatches and waits for 13 explicit opt-in workflows and also resolves/waits for 8 exact-candidate push workflows, requiring every child to have the exact aggregator SHA and `conclusion=success` before emitting `WBD_RELEASE_QUALIFICATION_PASS`.

No release-qualification aggregator run was present in the live `d01e15a2...` exact-head run set. Therefore the next action is qualification, not speculative product changes.

## Execution rule for this recovery

1. Record this recovery log first.
2. Make the `QUALIFICATION_KICK.md` generation update the final branch change.
3. Freeze the resulting exact candidate HEAD while the aggregator runs; do not move the branch during qualification.
4. If the aggregator fails, inspect only the first deterministic child failure and fix the smallest justified layer.
5. If it succeeds, record the exact aggregator/child evidence and then select the next product/lifecycle task above the frozen TCP-like data plane.
6. Before ending the workstream, update and verify the canonical handoff from sequence 74 to the new live state.

## Delivery rule

Do not offer new Windows/Linux artifacts merely because ordinary CI is green. Matching hosted artifacts require the exact-head authoritative aggregate to pass first. Physical Windows 11 + Npcap/NIC/NAT/ISP -> Ubuntu ARM64 remains final acceptance and cannot be fabricated by CI.
