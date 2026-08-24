# M10 local benchmark baseline

Status: first deterministic profile/report layer and real kernel TCP/UDP fault-proxy smoke are implemented. QUIC is pinned as the next oracle but is not yet linked into the Go 1.23 project.

## Why there are two benchmark layers

The report matrix is deterministic: every profile uses versioned integer steps, fixed seeds and fixed chunk sizes. This keeps p50/p95/p99 and protection-byte comparisons reproducible across CI and local runs.

A separate localhost socket smoke uses real `tcp4` and `udp4` kernel sockets through a fault proxy. Its wall-clock numbers are not used as authoritative percentiles; the test only qualifies the semantic claim that one serial TCP stall holds later stream records while independently scheduled UDP datagrams bypass the stalled datagram.

Run the deterministic report with:

```text
go run ./cmd/wbd-bench -pretty
```

## Standard profile v1

All deterministic profiles use eight 1024-byte chunks and 10 ms per logical step.

| Profile | Original-arrival shape |
| --- | --- |
| clean | all chunks at step 1 |
| reordered | selected chunks at steps 2/3 while later chunks arrive earlier |
| single-stall | chunk 2 held until step 8 |
| burst-same-xor | chunks 0 and 1 held until step 8 |
| burst-cross-xor | chunks 1 and 2 held until step 8 |
| final-chunk-stall | final chunk/FIN held until step 8 |

The WBD soft-deadline candidate fires at step 4 and its rescue copy becomes usable at step 5. It is an experiment, not a wire or timer commitment.

## Representative 1.5x results

| Profile | Native TCP p95 | Native UDP p50 / p95 | WBD reinjection p95 | WBD tail-deadline p95 | 1.5x duplicate p95 | 1.5x XOR p95 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| clean | 10 ms | 10 / 10 ms | 10 ms | 10 ms | 10 ms | 10 ms |
| reordered | 30 ms | 10 / 30 ms | 20 ms | 20 ms | 30 ms | 10 ms |
| single-stall | 80 ms | 10 / 80 ms | 20 ms | 20 ms | 10 ms | 10 ms |
| burst-same-xor | 80 ms | 10 / 80 ms | 20 ms | 20 ms | 80 ms | 80 ms |
| burst-cross-xor | 80 ms | 10 / 80 ms | 20 ms | 20 ms | 80 ms | 10 ms |
| final-chunk-stall | 80 ms | 10 / 80 ms | 80 ms | 50 ms | 80 ms | 10 ms |

The table intentionally keeps the pairwise-XOR failure profile. M10 therefore does not promote pairwise XOR to a production FEC format. At 2.0x, full duplication remains the proactive latency ceiling in this model; the XOR candidate spends only half its entitlement on parity and can use remaining budget for gap reinjection.

## Real localhost socket smoke

The qualified test inserts a 35 ms delay into record/datagram 1 while the other delays are 1 ms. Typical local observations were approximately:

```text
TCP: [~1.5ms, ~37ms, ~38ms, ~39ms]
UDP: [~1.3ms, ~35ms, ~1.3ms, ~1.3ms]
```

Exact wall-clock values are deliberately not asserted. The invariant is ordering: TCP record 2 cannot pass delayed TCP record 1, while UDP datagram 2 arrives comfortably before delayed UDP datagram 1.

## QUIC oracle pin

The planned oracle is `github.com/quic-go/quic-go` tag `v0.61.0`, release commit short SHA `579ee19`. That release requires Go 1.25.0, while the WBD module remains Go 1.23 in this checkpoint. M10 therefore records the pin in `deps/oracle-lock.json` and defers integration to an isolated compatible oracle build/toolchain instead of silently upgrading the project.

## Decision after this checkpoint

- Keep gap-driven reinjection as the low-overhead reliable-stream baseline.
- Keep a bounded soft-deadline candidate because GAP-only recovery is blind to a missing final extent.
- Keep pairwise XOR experimental; its same-group double-stall failure remains a hard counterexample.
- Do not freeze an FEC wire frame yet.
- Next benchmark work should add the pinned QUIC stream/datagram oracle and broaden the fault matrix beyond discrete stalls into repeatable jitter/loss distributions before RBC/FEC policy is tuned.
