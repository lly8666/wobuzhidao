# V2 fixed-FEC deterministic simulator smoke

Status: **repository simulator implemented; CI unit tests PASS; these numbers are offline model smoke, not live-network qualification**.

## Purpose

This smoke exists to answer one narrow question before changing the live WBD codec: under the same fixed loss trace and path budget, how do immediate-source tail-block RS, micro-block RS, and causal/sliding repair differ in first-complete latency, delivery and repair backlog?

Auto FEC is not part of this experiment and is intentionally deferred.

## Simulator

Implementation:

- `internal/fec/simulator.go`
- `internal/fec/simulator_test.go`
- `cmd/wbd-fec-sweep`

The simulator:

- generates source datagrams at the configured payload offered rate;
- never intentionally waits for a repair block before making a source ready;
- gives ready sources priority over ready repairs on the serialized link;
- applies deterministic iid loss or a two-state burst trace using a fixed seed;
- models tail-block MDS recovery, micro-block MDS recovery, and deterministic full-rank causal linear repair over a recent window;
- records direct/repaired delivery, p50/p95/p99/max first-complete latency, wire ratio, repair backlog and finite-run drain time.

The causal model is an offline coding-schedule model, not a claim that the live transport already implements sliding-window FEC.

Repository CI run `32851722051` completed `go test ./...` and handoff tests successfully after the simulator and fixed-FEC architecture changes were committed.

## Smoke A — low offered load

Common parameters:

```text
samples=2000
payload=1200 B
simulated shard header=56 B
RTT=50 ms (25 ms one-way)
offered payload=20 Mbit/s
path capacity=200 Mbit/s
seeds=260825,260826,260827
baseline tail/causal ratio=20:10
micro ratio=5:3
causal window=20
```

Values below are averages across the three deterministic seeds.

### iid loss

| loss | schedule | delivery | p95 ms | p99 ms | wire x | max ready repairs |
| ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 1% | off | 0.9893 | 25.050 | 25.050 | 1.047 | 0 |
| 1% | tail 20:10 | 1.0000 | 25.073 | 25.073 | 1.570 | 10 |
| 1% | micro 5:3 | 1.0000 | 25.050 | 25.067 | 1.675 | 3 |
| 1% | causal 20:10 | 1.0000 | 25.050 | 25.084 | 1.570 | 1 |
| 5% | off | 0.9493 | 25.050 | 25.050 | 1.047 | 0 |
| 5% | tail 20:10 | 1.0000 | 25.242 | 32.368 | 1.570 | 10 |
| 5% | micro 5:3 | 0.9998 | 25.067 | 26.860 | 1.675 | 3 |
| 5% | causal 20:10 | 1.0000 | 25.084 | 25.580 | 1.570 | 1 |
| 10% | off | 0.9027 | 25.050 | 25.050 | 1.047 | 0 |
| 10% | tail 20:10 | 1.0000 | 29.465 | 33.362 | 1.570 | 10 |
| 10% | micro 5:3 | 0.9950 | 26.071 | 27.037 | 1.675 | 3 |
| 10% | causal 20:10 | 1.0000 | 25.580 | 26.700 | 1.570 | 1 |
| 20% | off | 0.8055 | 25.050 | 25.050 | 1.047 | 0 |
| 20% | tail 20:10 | 0.9975 | 32.119 | 34.062 | 1.570 | 10 |
| 20% | micro 5:3 | 0.9718 | 26.604 | 27.089 | 1.675 | 3 |
| 20% | causal 20:10 | 0.9995 | 27.340 | 31.980 | 1.570 | 1 |

Interpretation:

- At iid 5-10%, the causal 20:10 model materially reduces the repair tail versus tail-block 20:10 at the same modeled wire ratio.
- Micro 5:3 shows low latency among delivered packets, but its lower delivery at 10-20% means p99 alone is misleading. Delivery and latency must be read together.
- `off` naturally reports propagation-floor latency only for surviving direct packets; its falling delivery ratio is the missing part of the metric.

### burst length 4

| loss | schedule | delivery | p95 ms | p99 ms | wire x |
| ---: | --- | ---: | ---: | ---: | ---: |
| 5% | tail 20:10 | 0.9892 | 25.073 | 32.021 | 1.570 |
| 5% | causal 20:10 | 0.9962 | 25.061 | 32.140 | 1.570 |
| 10% | tail 20:10 | 0.9780 | 28.518 | 33.435 | 1.570 |
| 10% | causal 20:10 | 0.9940 | 29.588 | 35.060 | 1.570 |
| 20% | tail 20:10 | 0.9180 | 30.784 | 33.903 | 1.570 |
| 20% | causal 20:10 | 0.9460 | 31.820 | 39.787 | 1.570 |

Interpretation:

- Causal/windowed repair improves modeled delivery in these burst traces but can produce a longer upper recovery tail because a burst may remove several nearby sources/repairs from the same window.
- Therefore the live codec must not be switched from block RS to causal repair based on iid results alone.
- Follow-up fixed sweeps should vary causal window, repair cadence/ratio and burst length while preserving identical traces across schedulers.

## Smoke B — capacity pressure

Parameters are the same except iid loss is fixed at 10%, one seed is used, and payload offered rate is increased against the same 200 Mbit/s path.

| offered | schedule | nominal utilization | delivery | p99 ms | max ready repairs | drain ms |
| ---: | --- | ---: | ---: | ---: | ---: | ---: |
| 100 Mbit/s | tail 20:10 | 0.75 | 1.000 | 27.025 | 10 | 0.553 |
| 100 Mbit/s | causal 20:10 | 0.75 | 1.000 | 25.388 | 1 | 0.100 |
| 140 Mbit/s | tail 20:10 | 1.05 | 1.000 | 57.899 | 277 | 13.995 |
| 140 Mbit/s | causal 20:10 | 1.05 | 1.000 | 56.790 | 270 | 13.665 |
| 160 Mbit/s | tail 20:10 | 1.20 | 1.000 | 91.772 | 615 | 30.965 |
| 160 Mbit/s | causal 20:10 | 1.20 | 1.000 | 92.624 | 611 | 30.790 |

This validates an important planning invariant: once `payload_rate * (1+repair_ratio)` exceeds capacity, repair backlog grows rapidly and FEC recovery latency degrades even though source generation remains prioritized. A fixed profile therefore needs an explicit operating envelope; more redundancy is not automatically lower latency.

## Next fixed-mode work

1. Sweep causal windows such as 8/12/20/32 and fixed ratios such as 20:4, 20:8, 20:10, 20:12, 20:20 under identical iid/burst traces.
2. Add a repository-backed simulator workflow/receipt once the parameter surface is narrowed enough to avoid noisy CI volume.
3. Keep live WBD transport on the already-qualified systematic block path until a candidate wins both delivery and tail latency under burst + capacity tests.
4. Implement runtime `fec.mode=off|fixed` configuration epochs without reconnect.
5. Keep Auto FEC deferred to future advanced research.
