# 2026-09-23 lifecycle fullstack matrix correction

Base SOURCE_SHA before this round: `0882adb70ddb85d8d5dbd5e6be8618f3d7132fcd`.

## Raw evidence reviewed before editing

- `next-lifecycle-fullstack` run 35791010503 created all 15 planned jobs, but every job failed in harness startup. Job 106959215169 (L0) shows the formal binaries built successfully, then the sample passed literal `${server_args[@]}` / `${client_args[@]}` as a single argument. Both processes printed Usage and exited; `kill -0` failed and the validator then lacked `manifest.json`. This is a harness failure, not a product lifecycle verdict.
- `next-strict-weaknet` run 35791010526 executed all 18 formal target-rate samples and aggregate job 106961188033. All 18 have CORRECTNESS=PASS and CAPTURE=PASS; all 18 have ENVIRONMENT=FAIL and PERFORMANCE=CAPACITY_LIMITED. INPUT_VALIDITY is PASS in 17/18; Normal/5305/seed202 is INPUT_VALIDITY=FAIL.
- The earliest target-rate abnormal boundary already appears in lossless control samples, before 20%/30% injected loss. Normal/lossless/seed101 job 106959215720 reports server `ss_packet` drops 1,356,170 with C2S only 0.72/0.55/0.60 Mbps versus 10 Mbps. Game/lossless/seed101 job 106959215595 reports server `ss_packet` drops 1,454,505 with C2S 0.48/0.31/0.34 Mbps versus logical 3 Mbps. Some samples also show client/server UDP socket drops.
- This confirms the strict target throughput gate is not qualified. The existing AF_PACKET/uplink-capacity investigation remains HOLD exactly as requested; this round does not alter packet socket buffers, 4096, repair/HOL/ACK behavior, FEC policy, or the capacity datapath.

## Corrections in this round

1. Fix Bash array expansion so client/server receive the real argument vector.
2. Expand the real-process lifecycle matrix to 36 isolated runner jobs:
   - every L1-L7 scenario has two independent seeds (101/202);
   - L0 executes FEC parity 0/20 x TLS startup padding off/on, also two seeds.
3. Add a lifecycle aggregate gate that requires all 36 exact-SHA summaries.
4. Add non-secret diagnostic receipts for effective keepalive/dead/reconnect/dormant/rotation timing and per-lane FEC parity. Credentials remain absent.
5. L0 deliberately writes opposite JSON values and uses explicit CLI values; validator checks the effective runtime values, FEC profile, padding enable state, 1s health cadence, and that startup-only padding does not touch UDP.
6. L3 drops HEALTH pre-seal on both endpoints in 1/2/3 consecutive groups and requires normal business delivery without dormancy.
7. L5 no longer blackholes the authoritative old lane. A 70s timed rotation initiates the candidate while the old lane stays usable. SYN/TLS/admission/detach failures are injected independently; validator requires old-lane delivery, counted failure, bounded retry delay, eventual generation advance/recovery, candidate/resource bounds, and stage receipt. SYN additionally checks source-port pcap and the candidate timeout window.
8. L6 requires at least 100 low-rate alternating demand packets in the 1-lane and 4-lane race jobs; the partial 4-lane wake retains lane-3 admission-failure evidence.
9. L7 still uses unmodified production defaults. Validator now checks effective 15s keepalive / 90s dead-after, first recovery suspicion near the real 90s budget, recovery after blackhole clear, and sustained final 30s delivery.
10. L1 adds a second dormant precondition, sparse traffic, and pure downlink continuity.

## Parameter / architecture boundary

No CLI parameter, default, wire format, FEC profile, buffer bound, 4096 limit, repair horizon, or ACK/HOL semantic changes. Therefore `docs/PARAMETERS.md/json` intentionally do not change and the parameter catalog remains a gate.

## Actions requirement

No PASS is claimed in this commit. The new exact SHA must pass `next-lifecycle`, foundation, and the 36-sample `next-lifecycle-fullstack` aggregate before lifecycle functional acceptance can be recorded. The strict 10/3 Mbps performance gate remains CAPACITY_LIMITED from 0882 evidence and cannot be closed by lifecycle correctness.
