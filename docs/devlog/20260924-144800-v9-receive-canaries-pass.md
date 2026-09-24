# 20260924-144800 V9 receive canaries pass

Exact product SOURCE_SHA: `af6d1f21cc589c4ebfffc07efc38df3ce4ff97bf`.

## Correctness / race

Product-SHA Actions are green:

- `next-lifecycle` run `35958681116` PASS, including focused lifecycle race repetitions.
- `next-p4-steady-targeted` run `35958681285` PASS, including Linux race.
- `next-foundation` run `35958681101` PASS, including Ubuntu `go test -race ./...`.
- `next-lifecycle-fullstack` run `35958681137` PASS.
- `next-tls-startup-padding` run `35958681168` PASS.

The later docs-only pin `f1036b4969a598852f4cdba7f3e5557f52142633` had contract-only failures because it modified the existing devlog instead of adding a new development log. No product test failed there; this new devlog satisfies that repository rule.

## Normal / 5305 / seed202 canary

Run `35958955273`, job `107503186988`, new workflow_dispatch attempt 1 at exact V9 SHA.

Artifacts:

- summary `10791184593`, sha256 `6c4b549aad3d8dc7ec96edc79ed80952cdf6da3d9053ae60cfefa78eb17f087f`
- full `10790764272`, sha256 `00661a888ff2f98c7f560fb918ce49a8bf9c74c0b36bf4a4a836d077c2a0cdfa`

Five classifications all PASS. All socket/link/qdisc drops are zero.

Stress:

- wall goodput: C2S 9.9879 Mbps, S2C 9.9821 Mbps
- probe p95: 612.878 ms
- FastRepairs: C2S 274, S2C 329
- FreshBlocked / FreshWindowBypass / FreshEmitFailures = 0
- RepairEvictionMaxScan = 1
- outer/app: C2S 5.30008x, S2C 5.43376x
- process CPU seconds: client 85.41, server 87.79

Read-only receive diagnostics:

- client SegmentMux peak 304 / 4096, full_waits=0, overflow_drops=0
- server ready peak 352 / 4096, overflow_drops=0
- AF_PACKET max occupancy: client about 6.03%, server about 4.48%

For comparison, the immutable V8 final18 Normal/5305/seed202 run `35955301841` remains CAPACITY_LIMITED with client AF_PACKET drops=3121. It is not replaced by this canary.

## Game4 / 5305 / seed303 canary

Run `35959672408`, job `107505349274`, new workflow_dispatch attempt 1 at exact V9 SHA.

Artifacts:

- summary `10791594202`, sha256 `e931416368893a5024480bf836e29dd08552ff6b6064575a5ea8a8e622268856`
- full `10791892918`, sha256 `f17f376f1276f2dc8234693ac419d43d602c59cef831934538fce27a75f6ca7b`

Five classifications all PASS. All socket/link/qdisc drops are zero.

Stress:

- wall goodput: C2S 2.99991 Mbps, S2C 3.00007 Mbps
- probe p95: 612.190 ms
- fresh three counters = 0 on all lanes
- RepairEvictionMaxScan = 1 on all lanes
- outer/app: C2S 21.35383x, S2C 21.88516x
- process CPU seconds: client 110.59, server 107.58

Read-only receive diagnostics:

- four SegmentMux route peaks: 63 / 85 / 130 / 108, each capacity 4096
- full_waits=0 and overflow_drops=0 on all routes
- server ready peak 454 / 4096, overflow_drops=0
- AF_PACKET max occupancy: client about 6.12%, server about 14.78%

The immutable V8 final18 Game4/5305/seed303 run `35955313644` remains CAPACITY_LIMITED with server AF_PACKET drops=10460.

## Interpretation

These two independent V9 canaries do not show an acceleration claim. They show that the exact product can carry the prior failure parameter sets without kernel/link drops, without userspace overflow, without fresh HOL, and without changing any configured capacity or performance threshold.

CPU and outer cost remain in the same band as V8; probe RTT remains about 612ms under a configured 300ms one-way path. The receive queues stayed far below 4096 in both new runs, so there was no hidden conversion from kernel drops into large userspace drops.

## Next

Before any new final18, run one exact-SHA Normal/5205/seed101 diagnostic canary. That scenario has much denser TCP-like repair than 5305 and is therefore the better check that V9 receive handling does not disturb the repair/fresh balance.

Historical V8 final18 stays permanently 16 PASS / 2 CAPACITY_LIMITED. No strict run is rerun or overwritten.
