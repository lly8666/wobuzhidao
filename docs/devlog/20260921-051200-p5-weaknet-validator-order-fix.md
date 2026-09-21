# P5 weak-network validator ordinal-order fix

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base HEAD: `3aaf10c8dc37c9d55cb63d1fe38ae8d88406b376`
- Failed Actions: `35559257580`
- Current product qualification remains: `b7c0b3d334ba1c52f9f53479eb8a2f4e4de692e3` / Actions `35555303253`
- Scope: validator-only fix plus STATUS/devlog; no product/runtime/workload change

## Failure preserved

The first 5% -> 20% -> 5% weak-network candidate completed both prescribed 120 s runs. All eight pre-existing required jobs passed. Both new weak-network matrix jobs failed only after their harnesses completed, at the same independent-validator assertion:

`phase ordinal sequence low5-initial/c2s`

Run 1 / seed 20260921 captured:

`scheduled=40 success=40 loss=0 repair=3608 wire_amp=8.656139`

Artifact:

- ID `10621273952`
- digest `sha256:a42160d15f792fd1965fa30031f34ea1f815ae76945dcfbc331364d1cac9c031`

Run 2 / seed 20260922 captured:

`scheduled=40 success=38 loss=2 repair=3808 wire_amp=9.468501`

Artifact:

- ID `10621626494`
- digest `sha256:34f71b1046a59cc9783ca15f47bf577e0400a6de656b4e3e41b0544e647521c2`

This failure is retained and is not rewritten as a product PASS.

## Root cause

Each directional weak-network link assigns `phase_ordinal` while holding its own link mutex. The raw JSONL writer is shared and is intentionally outside that hot link lock. Concurrent runtime emitters can therefore write already-numbered `network_decision` events in a different file-line order from their assigned packet ordinals.

The validator incorrectly required the JSONL line order itself to equal:

`1,2,3,...,N`

That ordering is not part of the raw event identity. Each event already carries:

- direction;
- phase;
- monotonic `phase_ordinal`;
- relative timestamp;
- seed;
- deterministic drop score;
- drop decision;
- queue enter time;
- packet length.

The validator already recomputes the drop decision from those fields. Requiring file-line order adds a false serialization assumption and would encourage holding the measurement-path lock across JSON encoding, which is undesirable measurement perturbation.

## Fix

Only `tools/check_p5_weaknet_measurement.py` changes:

- keep validating every event's phase, timestamp, seed, score and decision;
- keep rejecting missing or duplicate ordinals;
- compare the **sorted ordinal set** with the exact contiguous range instead of treating JSONL line order as packet order.

No change is made to:

- runtimeentry/TunnelOwner/lane;
- 300 ms delay;
- 30/60/30 phase clock;
- 5% -> 20% -> 5% deterministic drop policy;
- HTTPS workload or request deadlines;
- FEC strategy (still off);
- padding (still 0/off);
- repair;
- resource sampling;
- wire-amplification accounting.

## Actions isolation / VM performance

The weak-network gate remains a dedicated `p5-weaknet-5-20-5` matrix job. Each matrix child gets its own Ubuntu Actions VM and runs exactly one 120 s scenario with one seed and one artifact.

CPU/PPS/memory/GC are raw observations for that VM and are independently recomputed. There is no absolute throughput/CPU threshold that would turn hosted VM variance into a protocol verdict. Future load qualification should keep the same one-job/one-load isolation.

## Next

Run the full `next-foundation` workflow on the new exact SOURCE_SHA. Qualification requires all ten jobs on that SHA to pass, including both 120 s weak-network runs and their independent validators.

If a later validator stage exposes another current-atom defect, preserve this failure and fix only that defect. Do not mix in 5% -> 30% -> 5%, load, soak, FEC tuning, or padding enablement.
