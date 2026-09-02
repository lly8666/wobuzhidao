# 2026-09-02 Multipath dynamic-lane qualification log

## Scope

This log records the resumed qualification work for `feat/single-flow-reality-faketcp` after handoff sequence 88. The active requirement is to qualify the Windows/Linux same-association transport end to end before delivering another package. The mature FakeTCP ACK/SACK/RTO/FEC data plane is frozen unless new evidence directly requires a core change.

## Live refresh

- Formal branch `dev/wbd-raw-fec-v2`: `d30e74d4d96301c5178fe2b98993cf8f2306b6f7` (`handoff: sequence 88 multipath first-activation rollback`).
- Active feature branch: `feat/single-flow-reality-faketcp` at `f7712a63f9f5131ea687e04d2044de3849d06b76`.
- PR #9 is open but currently reports `mergeable_state=dirty`; this must be resolved before formal promotion/qualification.
- Handoff sequence 88 remains the authoritative architectural recovery point, but its tested feature SHA predates the current feature HEAD.

## Current architecture constraints

- One public TCP-shaped association per lane.
- The same association carries the TLS-1.3-looking / Reality-like bootstrap, admission, takeover, DTLS 1.3, then LINK/TUN traffic. No second SYN is allowed for a lane.
- ADR-0012 allows 1..4 independent lanes; each lane owns its own complete same-association path and transport source port.
- Dynamic lane work is orchestration/admission/hold/fallback work only. Do not change FakeTCP retransmission/SACK/RTO/FEC wire semantics without new evidence.
- Current front-phase camouflage bytes/persona remain frozen in this qualification task.

## 2026-09-02 Actions triage

The latest feature HEAD has completed Actions, but completion is not equivalent to qualification: the failed-runs query reports 22 failed workflow runs/checks on `f7712a63...`.

Confirmed failures so far:

1. `game-settings-matrix` run `33586547156`, job `settings` (`100111785003`) fails in `Measure real logical inner pacing (UDP)`. This workflow exercises pacing ratios/timing and is not directly in the newest Windows lane-lifecycle diff. Exact failure output is not available through the current connector, so it must be treated as unclassified until rerun/evidence.
2. `windows-faketcp-persona` run `33586547100`, job `packet_persona` (`100111784634`) fails in the combined Windows packet/single-flow/runtime/diagnostics test step. The latest HEAD changes only Windows runtime dynamic-lane lifecycle code/tests, so the runtime subset is the first suspect; the job has been re-run on the exact same HEAD to distinguish deterministic logic failure from timing/flakiness.

## Qualification rule

No Windows or Linux package will be delivered from this head until the exact tested head has green automated Windows and Linux same-flow qualification, plus the relevant multipath/lane lifecycle and release gates. Failures will be classified into deterministic product blockers, downstream/aggregate failures, or timing-only flakes before any code change.
