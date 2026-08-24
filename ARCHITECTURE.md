# Architecture v1

> **Status: REJECTED / historical only (2026-08-24).** M10-004 real-socket FEC qualification showed that adding erasure coding above multiple ordered TCP carriers does not bypass carrier HOL: 20:20 at 2.0x reached about 250 ms p99 with only 1% impairment while the udp2raw + UDPspeeder 20:20 reference remained about 72 ms p99 through 15% shard loss. See `docs/benchmarks/m10-004-fec-no-go.md`. Do not continue product work on this architecture without a new architecture decision that changes a fundamental carrier assumption.

## Public carrier

Each lane is independently established as:

```text
WBD protected inner session
        ↓
VLESS + XTLS Vision
        ↓
REALITY
        ↓
RAW transport
        ↓
real kernel TCP
        ↓
server:443
```

WBD never relies on public raw-FakeTCP behavior. Multiple lanes are separate genuine TCP connections and therefore separate kernel HOL/congestion domains.

## Logical transport

WBD borrows mature semantics rather than inventing a VPN-specific TCP clone:

- QUIC-like connection/session, streams, datagrams and bounded flow control.
- MPTCP-like logical data identity independent from per-lane transport identity, enabling cross-lane reinjection and deduplication.
- TCPLS-like use of multiple ordinary TCP/TLS carriers inside one higher-level secure logical session.
- Optional low-latency repair/FEC only after measured admission.

Conceptual stack:

```text
VPN / SOCKS / test flows
        ↓
WBD Flow Layer
  reliable STREAM
  expiring DATAGRAM
        ↓
WBD Session Layer
  session_id
  stream/flow identity
  logical offsets / datagram ids
  ACK ranges
  GAP hints
  flow control
  dedup / reorder
        ↓
Recovery Layer
  cross-lane reinjection
  rescue lane
  RBC 1.0x..2.0x
  optional FEC
        ↓
Lane Scheduler
  bounded logical flight per lane
  estimated-delivery-time metrics
        ↓
N independent real TCP + REALITY/Vision lanes
```

## Lane roles

The first benchmarked topologies are deliberately small:

- 1 lane: correctness/baseline.
- 2 symmetric bulk lanes.
- 2 bulk + 1 low-queue rescue/control lane.
- 4 symmetric lanes as a comparison, not an assumed optimum.

Control frames are not bound to a single lane. Any healthy lane may carry ACK, GAP, flow or session control.

## Logical identity

At minimum keep three identities separate:

1. `flow_id` + stream offset or datagram id: what application data this is.
2. `transmission_id`: one attempt to carry that logical data.
3. `lane_id`: which real TCP carrier carried the attempt.

Reinjecting a logical chunk changes transmission/lane identity, not application identity.

## Redundancy Budget Controller (RBC)

RBC owns a target intentional multiplier `M` in `[1.0, 2.0]` and decides how the protection budget is spent among:

- proactive repair/FEC,
- proactive cross-lane duplicate,
- gap-driven reinjection,
- source pacing/backpressure.

`weak-1.5x` targets `M=1.5`; `weak-2x` V1 uses full cross-lane replication; `auto` uses discrete levels and fast-up/slow-down hysteresis.

Primary feedback is logical delivery quality: ACK delay distribution, late-chunk ratio, gap age/rate, reinjection success, FEC recovery, lane stalls/disconnects and queue pressure. Kernel TCP telemetry is optional optimization only.

## Weak-network semantics

A logical item is operationally “lost” for latency purposes when it misses its soft delivery deadline, even if its original TCP lane will eventually recover it. STREAM data is eventually reliable; DATAGRAM data may expire at a hard deadline.

A rescue lane is evaluated because reinjected critical data must not sit behind bulk bytes already committed to another kernel socket queue.

## VPN integration boundary

Protocol development starts without TUN. Planned sequence:

1. deterministic flow/test generators,
2. WBD local TCP/UDP/SOCKS surface,
3. stock Xray REALITY/Vision carrier integration,
4. Linux TUN/VPN,
5. OpenWrt, Windows and finally Android.

Where possible, reuse Xray's existing TUN/netstack/platform support instead of building a second portable userspace TCP stack.

## Benchmark oracles

Every network profile must compare at least:

- native TCP,
- native UDP datagram,
- QUIC stream/datagram,
- WBD single TCP,
- WBD multi-lane variants.

Metrics: p50/p95/p99 delivery latency, maximum HOL stall, goodput, recovery latency, intentional redundancy, observed network bytes, CPU, memory and socket count.
