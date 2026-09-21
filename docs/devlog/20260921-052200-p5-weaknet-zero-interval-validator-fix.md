# P5 weak-network zero-interval validator fix

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Base HEAD: `02e2d47ec2f50db05b67d33654aa007c77559f7c`
- Failed Actions: `35564104849`
- Current product qualification remains: `b7c0b3d334ba1c52f9f53479eb8a2f4e4de692e3` / Actions `35555303253`
- Scope: validator-only fix plus STATUS/devlog

## Preserved result

The second weak-network exact-SHA candidate kept all eight pre-existing required jobs green. Both dedicated weak-network matrix jobs ran their full 120 s scenario and failed only in the independent validator.

Run 1 / seed 20260921:

- scheduled 40;
- success 36;
- application loss 4;
- repair retransmits 3469;
- wire amplification 8.581570;
- artifact `10623074371`;
- digest `sha256:493346210c4555dc28170804fcdf0cc4d5e728a416a32f605fe58a24f0f31c51`.

Run 2 / seed 20260922:

- scheduled 40;
- success 38;
- application loss 2;
- repair retransmits 3673;
- wire amplification 8.953445;
- artifact `10623188618`;
- digest `sha256:f8131306a65585f3507bfebf91e737498c984b9945a64ee31805d6d44f2d725b`.

Both validators stopped at:

`outer interval/burst raw evidence`

The previous phase-ordinal file-order defect is therefore confirmed fixed: validation progressed beyond that stage on both runs.

## Root cause

The shared P5 raw-event struct declares `InterArrivalNS` with `json:",omitempty"`.

That means a legitimate exact zero interval is represented by an absent JSON property. Zero is expected for:

- the first packet/record in a direction because there is no predecessor;
- multiple TLS-like records captured from the same outer segment at the same timestamp.

The validator treated absent `inter_arrival_ns` as missing evidence rather than the schema's zero representation.

## Fix

Only the weak-network validator changes.

For outer packet events it still requires:

- positive packet length;
- burst ID;
- burst wire bytes;
- interval value, with absent `inter_arrival_ns` normalized to 0;
- non-negative interval.

For TLS-like record events it still requires:

- positive record length;
- interval value, again normalizing schema omission to 0;
- non-negative interval.

The recorder is intentionally unchanged so the measurement path is not perturbed to satisfy the checker.

## Scope unchanged

No change to:

- 300 ms one-way delay;
- 30/60/30 phase clock;
- deterministic 5% -> 20% -> 5% injection;
- real HTTPS workload;
- FEC off;
- padding off;
- repair logic;
- CPU/PPS/memory/GC sampling;
- one Actions VM per weak-network run.

Hosted VM performance remains observational. No absolute CPU, PPS, or throughput threshold is introduced.

## Next

Create a new exact SOURCE_SHA and run the full ten-job `next-foundation` qualification. Preserve both earlier failed runs regardless of the next outcome.
