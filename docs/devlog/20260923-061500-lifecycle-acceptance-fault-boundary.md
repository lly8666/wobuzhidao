# 2026-09-23 weaknet lifecycle acceptance fault boundary

Base branch HEAD before this round: `5ecea583aab307ea64f35871036b586e0df9e2fd`. The product lifecycle implementation under qualification remains `84c466f81860c3e87aac3b571a9bce419018aabc`; this round changes only acceptance instrumentation plus the exact code call sites needed to expose deterministic failure boundaries.

## Scope

- Added `internal/acceptancefault` with mutually exclusive build-tag implementations. Normal production builds compile `Consume` to an unconditional false no-op.
- `-tags lifecycleacceptance` may consume bounded JSON control rules from `WBD_LIFECYCLE_ACCEPTANCE_CONTROL` and writes each consumed event to `WBD_LIFECYCLE_ACCEPTANCE_EVENTS`.
- Added explicit pre-seal HEALTH drop boundary in `runtimeowner.sendHealth`; dropped acceptance health never reaches TLS sealing, FEC, padding, repair credit, or the wire.
- Added candidate TLS and admission failure boundaries on the client admission path, and a detach failure boundary after protected admission but before client FakeTCP detach/promotion.
- No CLI/config parameter was added; `PARAMETERS.md/json` therefore intentionally do not change.
- No change to 4096 outstanding, repair horizon, ACK/HOL behavior, FEC profile, global buffers, or AF_PACKET receive capacity work.
- The prior AF_PACKET/uplink-capacity main task remains HOLD exactly as recorded in STATUS.

## Acceptance intent

These hooks exist only because `LIFECYCLE_ACCEPTANCE.md` forbids packet-length guessing for HEALTH loss and requires SYN/TLS/admission/detach candidate failures to be distinguished. SYN remains a real router/firewall fault; TLS/admission/detach and pre-seal HEALTH can now carry direct raw event receipts.

## Actions

This commit itself must obtain a new exact `SOURCE_SHA`. `next-lifecycle` now compiles/tests both the normal build and the acceptance-tag build. No PASS is claimed in this log before Actions completes. The real-process L0-L7 matrix and strict 10/3 Mbps weaknet matrix remain NOT_RUN at the start of this round.
