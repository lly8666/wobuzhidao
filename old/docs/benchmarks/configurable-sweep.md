# Configurable upper-bound sweep

## Goal

One JSON file should be enough to generate the experiment matrix. The point is to find which combination works best under each network condition, not to hand-edit Go code for every run.

## Parameters available now

| Area | Parameters |
| --- | --- |
| Real WBD carrier | `lane_counts` from 1 to 16 |
| WBD control | `normal` / `auto` |
| Load | outstanding `windows`, sample count, payload size, source spacing |
| Network | RTT min/max, seeded impairment basis points, extra hold, soft deadline, same-lane burst length |
| FEC plan | on/off, data shards, parity shards, mode, timeout, interleave |
| External comparator | pinned udp2raw + UDPspeeder, including 20:10 and 20:20 FEC presets |

`configs/bench/upper-bound-sweep.json` is the first sweep file. It currently expands to real WBD cases plus experimental FEC cases and external-oracle cases.

## Important boundary

FEC parameters are already part of the experiment configuration, but an enabled WBD FEC case is marked `wbd-fec-experimental` and `runnable=false` until a real carrier-side FEC experiment is implemented and locally qualified. This prevents the test framework from pretending a parameter has an effect when it does not.

The fixed WBD `weak-1.5x` and `weak-2x` modes are also intentionally not exposed by the real fault runner yet: a multiplier label without an actual duplicate/FEC spender would be misleading. `normal` and `auto` are runnable because the current real path genuinely exercises gap-driven reinjection.

## External comparator

The primary raw/FEC comparator is pinned as:

- udp2raw `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63`, `--raw-mode faketcp`.
- UDPspeeder `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454`, `--mode 0`.
- primary FEC: `-f20:20 --timeout 8`, i.e. one parity packet per source packet in aggregate, approximately 2.0x payload traffic.
- secondary sanity comparator: `-f20:10 --timeout 8`, approximately 1.5x.

The intended chain is application -> UDPspeeder -> udp2raw -> network -> udp2raw -> UDPspeeder -> application. It is an external upper-bound/reference oracle only; raw FakeTCP remains outside the WBD product architecture.

## Usage

Generate all cases:

```text
go run ./cmd/wbd-sweep -config configs/bench/upper-bound-sweep.json
```

Run one case by exact generated ID:

```text
go run ./cmd/wbd-sweep -config configs/bench/upper-bound-sweep.json -run <case-id>
```

Each generated case says whether it is runnable. External binaries are blocked until their exact bytes are restored and SHA-qualified locally.
