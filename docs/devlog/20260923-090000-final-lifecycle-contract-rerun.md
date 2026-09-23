# 2026-09-23 final lifecycle contract rerun

Base SOURCE_SHA: `9567e5746eecd9b240d61a5f732641b3ac21975c`.

## Reviewed exact-SHA evidence

`next-lifecycle` 35801881011, `next-foundation` 35801880957 and `next-p4-steady-targeted` 35801880982 all PASS at the base SHA.

`next-lifecycle-fullstack` 35801880941 ran all 36 isolated samples. Thirty-three samples PASS. Only three sample jobs fail:

- L4 all-tuple seed101, job 106993850139: the temporary all-underlay blackhole caused three bounded failed candidates. After the fault cleared, the still-authoritative old tuple recovered naturally; final 30s C2S and S2C delivery are both 1.0, with no candidate promotion/generation bump. The validator incorrectly required `RecoverySucceeded>0` and a generation advance even for the temporary-all-tuples subcase.
- L6 race4 seed101, job 106993851015: blackhole phase exercised failed wakes and bounded retry; post-clear wake succeeded; strict clear-path 100-event target completed without delivery/corruption/unexpected errors. The only validator error is “insufficient wake cycles attempts=1 succeeded=1”.
- L6 race4 seed202, job 106993851133: same, with strict phase attempts=2/succeeded=2. The only validator error is the arbitrary >=10 wake-cycle count.

Both L6 race1 seeds PASS at the same SHA. This confirms the `65ff2ef...` PeerFIN product correction fixes the unilateral-server-dormancy packet-loss defect; the remaining race4 failures do not show lost business.

## Contract correction

The acceptance definition already says recovery is “30s sustained deliverability plus bounded state convergence” and explicitly says the first high-loss lane replacement need not succeed.

- `l4_old_tuple`: permanent old 4-tuple blackhole still requires a successful new TLS candidate and generation advance.
- `l4_all_tuple`: all tuples are temporarily blackholed. Candidate attempts must occur and remain bounded, but after clear the original authoritative tuple may recover before any promotion. Final 30s delivery, active lane state, zero candidate/retiring residue, stable lease and bounded retries are the hard gates. A post-clear promotion, if it happens, still has the 25s budget.
- L6 strict phase starts from an observed both-DORMANT state, requires at least one successful wake, no clear-path wake failures, and 100/100 unique delivery. There is no protocol requirement that 100 business events create ten, ninety or one hundred separate wake transitions.

No production Go code changes in this round.

## Fresh performance evidence

The product changed at `65ff2ef...`, so the older `0882adb...` strict target-rate result cannot be the sole exact-SHA evidence for the final source. The strict workflow trigger is expanded to lifecycle/runtime product files, and this commit changes that workflow itself, forcing a new 18-job target-rate matrix:

- Normal: one lane, FEC20:20, 10 Mbps each direction.
- Game: four lanes, FEC20:20, 3 Mbps logical business each direction.
- lossless, 5→20→5 and 5→30→5 profiles, 300ms one-way path, three independent seeds.
- one sample per runner.
- separated FEC parity, Game replication, repair, health, padding and handshake/reconnect accounting retained.

If AF_PACKET drops again appear at the lossless boundary, record the exact new SHA and raw boundary as CAPACITY_LIMITED/PERFORMANCE_FAIL. Do not resume or alter the user-held AF_PACKET/uplink-capacity work.
