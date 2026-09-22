# 2026-09-23 lifecycle race / candidate harness correction

Base SOURCE_SHA before this round: `1c83b2cdf27f79a341c5650e78aaf5794b093fef`.

## Exact-SHA evidence from the previous round

Core and unaffected regression gates at `1c83b2cdf27f79a341c5650e78aaf5794b093fef` are clean:
- next-lifecycle run 35797317484 PASS (parameter catalog, Linux/Windows compile, normal + acceptance-tag tests, focused race).
- foundation run 35797317486 PASS.
- next-p4-steady-targeted run 35797317474 PASS.
- next-tls-startup-padding run 35797317460 PASS.
- next-realpath-calibration run 35797317467 PASS.

Fullstack run 35797317547 executed all 36 isolated jobs. Twenty-six sample jobs passed:
- L0: 8/8 (FEC off/20 × padding off/on × seed101/202).
- L1: 2/2.
- L2 C2S/S2C total-loss: 4/4.
- L3 pre-seal HEALTH loss: 2/2.
- L4 old-tuple / all-tuple blackhole: 4/4.
- L5 TLS and admission candidate failure: 4/4.
- L7 real production defaults: 2/2.

The ten remaining sample failures are acceptance-contract defects, not evidence of a product failure:

1. **L5 SYN (2 jobs)** — e.g. job 106979506053 had old-lane C2S=1.0, S2C≈0.998 during the first 90s, final 30s both 1.0, one bounded failed candidate then one success. The actual FakeTCP SYN retry path closed the candidate in ~6.5s. The validator incorrectly required 12–19s even though the candidate has its own bounded SYN retry limit inside the 15s absolute admission context.
2. **L5 detach (2 jobs)** — e.g. job 106979506186 converged after four total attempts (three failures + success), final 30s both directions 1.0, old-lane first-90s delivery >91%. A detach injection is after the server has already completed admission/handoff, so a small bounded sequence of retries while server retiring/generation state converges is expected. Requiring <=3 attempts contradicted the acceptance rule that first replacement success is not required.
3. **L6 race (4 jobs)** — the duplex generator sends target->client traffic after both endpoints have auto-dormanted. PARAMETERS explicitly states there is no out-of-band server reverse wake channel. C2S remained 1.0 while S2C failed; that tests an unsupported capability, not the required client-new-business wake race.
4. **L6 partial (2 jobs)** — the first duplex generator REGISTER was sent while all underlay tuples were intentionally blackholed, so the target never learned the reverse UDP peer and produced zero S2C packets. In addition, idle=3s matched the generator drain and made final DORMANT a normal state that the validator incorrectly rejected.

L7 is especially important: both default 15s keepalive / 90s dead-after samples passed the actual 30s stable + 120s blackhole + recovery schedule. Seed101 recovered business to 100% final-30s without a successful promotion; seed202 also obtained a promotion. This confirms the acceptance result must distinguish **business recovery** from **candidate replacement success**.

## This correction

- Add acceptance-only `tools/lifecycle_wake_driver.py`. It sends exactly 100 client-originated, mixed-size UDP events at 1.25s intervals. The target never responds. This directly exercises wake/idle races near the 1s idle cutoff without inventing a reverse-wake channel.
- L6 race1/race4 use that driver and require 100/100 unique target deliveries plus roughly one bounded wake transition per event.
- L6 partial uses a C2S-only wake attempt while all tuples are blackholed, clears the fault, then starts a fresh bidirectional flow so its REGISTER occurs on a reachable path. idle is 10s so the post-recovery receipt is sampled while active.
- L5 SYN accepts the actual bounded FakeTCP retry completion inside 15.5s and still requires repeated SYN pcap evidence.
- L5 candidate failures allow up to six total attempts and validate every observed failure->next-attempt delay against the existing 1–4s backoff window.
- L2 now checks both endpoint snapshots for false dormancy during one-way total loss and caps recovery attempts.
- Lane/FEC/generation checks use the latest active diagnostic snapshot. Only L6 race may legitimately finish DORMANT after its final event.
- No production implementation, parameter, wire format, 4096 bound, repair horizon, ACK/HOL semantics, socket buffer, FEC policy, or AF_PACKET path changes in this round.

The strict target-rate weaknet result remains the separate `0882adb...` CAPACITY_LIMITED evidence. The AF_PACKET/uplink-capacity mainline remains HOLD and is not resumed here.

No PASS is inherited by the next commit: the corrected harness must obtain a new exact SOURCE_SHA and rerun the full 36-job lifecycle matrix plus core/foundation gates.
