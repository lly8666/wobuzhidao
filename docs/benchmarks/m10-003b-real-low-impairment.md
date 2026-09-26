# M10-003B — real two-lane low-impairment qualification

Date: 2026-08-24

## Scope

This gate extends M10-003A without changing RBC/FEC policy. It adds a reusable localhost userspace fault path around **two independent real kernel TCP WBD lanes**, routes the existing `lane.Pool`, `session.Receiver`, logical ACK/GAP handling, and `recovery.StreamSender` through that path, and records native TCP/UDP baselines beside WBD normal/Auto.

The impairment is intentionally described as a **userspace lane hold**, not kernel TCP packet loss. The proxy reads complete WBD frames, assigns each an arrival+delay release time, and preserves frame order inside each TCP lane. Normal propagation delay therefore does not become serialized service time. Holding one source frame still creates genuine TCP head-of-line blocking for later frames on that same lane; recovery can bypass that HOL only by reinjecting the logical bytes on the alternate lane.

M10-003A's existing 40–60 ms RTT / 0% normal gate is unchanged. M10-003B also has a clean two-lane check with a single outstanding logical frame so both real lanes are exercised without manufacturing cross-lane reorder.

## Deterministic profiles

### Clean two-lane sanity

- 16 samples, 256-byte payloads.
- 25 ms one-way in each direction (50 ms target RTT).
- 0% injected impairment.
- Window 1; source frames still alternate lane 1 / lane 2.
- Required: 100% completion/delivery, zero GAP/reinjection, Auto remains 1.0x.

### Low impairment

- Seed `7001`.
- 100 samples, 256-byte payloads.
- 25 ms one-way in each direction (50 ms target RTT).
- 1% of source logical frames selected deterministically; the selected lane-1 source frame receives an additional 200 ms forward hold.
- Window 4, 10 ms source spacing.
- This is the first staged recovery gate. No RBC/FEC tuning is performed.

### Moderate impairment

- Seed `8001`.
- 50 samples, 256-byte payloads.
- Per-direction propagation is seeded in 40–75 ms, giving an 80–150 ms RTT schedule; observed target mean RTT for this seed is 116.264351 ms.
- 2% of source logical frames selected deterministically; the selected lane-1 source frame receives an additional 300 ms forward hold.
- Window 4, 50 ms source spacing so ordinary propagation spread does not itself manufacture cross-lane gaps.
- This stage was run only after the ~1% stage repeatedly completed successfully.

## Recovery correctness fix exposed by the real two-lane path

The first low-impairment runs exposed an ACK/GAP crossing case. `StreamSender.ReinjectGap` already treated a stale GAP as a no-op when the entire flow had been pruned, but returned `ErrUnknownGap` when the stale GAP covered an already-ACKed subrange while other records for the same flow were still outstanding. Independent carrier control frames can legitimately cross in exactly that state.

M10-003B adds a narrow `ReinjectGapCrossLaneSafe` wrapper for the real multi-lane path. It converts `ErrUnknownGap` to a no-op only when the GAP is wholly covered by accumulated logical ACK ranges, and adds focused regressions proving that truly unknown, non-ACKed gaps still remain errors. The core `ReinjectGap` contract stays strict.

## Local qualification snapshot

Command set:

```text
gofmt check on changed Go files
go vet ./internal/benchmark ./internal/recovery
go test ./internal/benchmark ./internal/recovery -count=1 -v
go test -race ./internal/benchmark ./internal/recovery -count=1 -v
```

The non-race qualification run completed successfully with these observations:

| Profile | Mode | Target mean RTT | Mean | p50 | p95 | p99 | Completion / delivery | Late ratio | Intentional payload bytes | Reinjection | GAPs | Final multiplier |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| clean | WBD normal | 50 ms | 52.813 ms | 50.815 ms | 82.083 ms | 82.083 ms | 1.000 / 1.000 | n/a | 4096 / 4096 source | 0 | 0 | 1.0x |
| clean | WBD Auto | 50 ms | 51.145 ms | 50.829 ms | 56.199 ms | 56.199 ms | 1.000 / 1.000 | n/a | 4096 / 4096 source | 0 | 0 | 1.0x |
| ~1% | native TCP | 50 ms | 53.484 ms | 51.187 ms | 54.330 ms | 60.002 ms | 1.000 / 1.000 | 0.010 | 25600 | 0 | 0 | 1.0x |
| ~1% | native UDP | 50 ms | 53.350 ms | 51.129 ms | 54.227 ms | 56.366 ms | 1.000 / 1.000 | 0.010 | 25600 | 0 | 0 | 1.0x |
| ~1% | WBD normal | 50 ms | 56.334 ms | 51.485 ms | 103.950 ms | 130.636 ms | 1.000 / 1.000 | 0.060 | 26624 / 25600 source | 1024 B | 4 | 1.0x |
| ~1% | WBD Auto | 50 ms | 56.126 ms | 51.461 ms | 104.090 ms | 130.622 ms | 1.000 / 1.000 | 0.060 | 26624 / 25600 source | 1024 B | 4 | 2.0x |
| ~2% | native TCP | 116.264 ms | 123.686 ms | 113.645 ms | 145.451 ms | 409.006 ms | 1.000 / 1.000 | 0.020 | 12800 | 0 | 0 | 1.0x |
| ~2% | native UDP | 116.264 ms | 123.685 ms | 113.584 ms | 145.413 ms | 409.107 ms | 1.000 / 1.000 | 0.020 | 12800 | 0 | 0 | 1.0x |
| ~2% | WBD normal | 116.264 ms | 158.305 ms | 151.809 ms | 230.136 ms | 280.987 ms | 1.000 / 1.000 | 0.080 | 13312 / 12800 source | 512 B | 2 | 1.0x |
| ~2% | WBD Auto | 116.264 ms | 158.306 ms | 151.807 ms | 230.235 ms | 281.024 ms | 1.000 / 1.000 | 0.080 | 13312 / 12800 source | 512 B | 2 | 1.25x |

The intentional payload accounting stayed well below the hard 2.0x ceiling: ~1% WBD used 1.04x source payload in this snapshot; ~2% WBD used 1.04x. A single held TCP-lane source frame can require multiple logical reinjections because the held frame also HOL-blocks later source frames on that same carrier; this is expected behavior of the real TCP path, not additional injected loss.

`LateRatio` remains a wall-clock observation. It is **not** fed into Auto in this localhost gate because host descheduling, especially under the race detector, is not network-quality evidence. Auto receives logical delivery count plus receiver-observed GAP events. This preserves M10-003A's separation of host noise from network-path signals while still exercising the unchanged Auto controller.

The race-instrumented run also passed. As expected, race instrumentation inflated wall-clock RTT/late observations, but correctness, completion, dedup/recovery, budget ceiling, and control-path assertions remained green.

## Gate result

M10-003B is qualified for the staged ~50 ms/~1% and 80–150 ms/~2% userspace fault profiles. This result does **not** authorize 10–20% impairment work, RBC/FEC tuning, production FEC, REALITY/Vision behavior changes, or TUN integration. Those remain outside this atomic step.
