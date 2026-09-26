# V2 200-Mbit/s public-link focused pass

Date: 2026-08-25

Workflow: `transport-200m`, run `32819141473`, head `747b0969d7d0295410d15a81b97c384a73db055e`.

## Purpose

Fast capacity/resource probe above TUN with the current external stack:

`rate probe -> UDPspeeder mode0 20:20 -> DTLS 1.3 -> udp2raw FakeTCP`

Each public direction was shaped to 200 Mbit/s. Because 20:20 adds approximately 100% FEC redundancy before DTLS/FakeTCP overhead, the inner generator offered 90 Mbit/s. The probe is request/echo, so this is a full-duplex stress case, not a one-way 90-Mbit/s download.

RTT points: 20 / 100 / 300 / 600 ms. Symmetric loss points per direction: 0 / 1 / 5 / 10 / 20%. Duration: 2 s offered-load phase, 1200-byte inner datagrams.

All four RTT jobs and the 20-row aggregate completed successfully.

## Direct observations

The current external-process stack is already capacity/CPU limited at the 90-Mbit/s full-duplex inner offered load. At 20 ms / 0% loss, only 39.217 Mbit/s of request/echo payload returned within the probe accounting window. The p99 was 282.5 ms. This occurred with zero configured packet loss and zero qdisc drops, so it is not a loss-recovery cliff.

At 20 ms / 0%:

- delivered ratio: 0.4357
- inner returned throughput: 39.217 Mbit/s
- p50 / p95 / p99: 173.3 / 266.9 / 282.5 ms
- aggregate peak RSS: 28,976 KiB
- client / server peak RSS: 13,688 / 15,288 KiB
- UDPspeeder RSS: 9,716 + 11,380 = 21,096 KiB
- DTLS RSS: 2,848 + 2,776 = 5,624 KiB
- udp2raw RSS: 1,124 + 1,132 = 2,256 KiB
- measured process CPU deltas: UDPspeeder 2.67 s, DTLS 2.70 s, udp2raw 2.29 s across both endpoints

UDPspeeder therefore accounts for about 73% of the measured userspace RSS in this point. CPU is distributed across all three layers; removing UDPspeeder is expected to reduce copies/socket hops and coding cost, but the first-pass data does not justify attributing all CPU pressure to FEC.

Across the full 20-case pass, aggregate RSS stayed roughly 28.3-30.5 MiB for both endpoints combined. No memory explosion was observed as RTT/loss increased.

## Important accounting limitation

Do **not** use the first-pass `delivery_ratio`, `inner_delivered_mbps`, or `cpu_core_fraction_*` columns to compare RTT points quantitatively.

The C request/echo probe waits for a post-send drain interval that grows with RTT. It counts responses received during that drain as delivered while the throughput denominator remains the 2-second offered-load interval. Likewise, the Python CPU delta includes the drain period while the `cpu_core_fraction_*` denominator was fixed at 2 seconds. This causes higher-RTT cases to receive a longer completion window and biases both delivery and CPU-core fractions.

The first pass is still valid for these conclusions:

1. The workflow, exact stack, 200-Mbit/s shaping and loss injection work.
2. The current external-process stack has a clear CPU/queueing capacity problem under 90-Mbit/s **full-duplex** inner request/echo load even at 20 ms / 0% loss.
3. RSS is stable and UDPspeeder dominates the memory footprint.
4. A second sizing probe should use one-way bulk traffic with a fixed measurement interval and a separate low-rate latency probe, or explicitly account for the real measurement wall/drain time.

## FEC simplification decision

UDPspeeder should remain the compatibility/oracle baseline while a WBD-owned FEC implementation is built and A/B tested.

The target is **not** a generic replacement for all UDPspeeder features. WBD only needs a narrow data-plane primitive:

- fixed systematic erasure coding, initially 20 data + 20 parity shards;
- bounded encode/decode block windows;
- a small flush timer for low traffic;
- explicit original packet-length/group metadata;
- fixed WBD MTU policy;
- no retransmission and no ordered stream;
- preallocated buffer pools;
- FEC before DTLS, matching the current architecture.

A single XOR parity shard is not equivalent to 20:20 because it only repairs one missing shard. Sending every packet twice is computationally cheap and can be retained as an optional low-CPU mode, but it is substantially weaker than a 20+20 MDS-style erasure code at high random loss.

A thin WBD protocol around a mature systematic Reed-Solomon implementation is the preferred first implementation. After the A/B benchmark, the codec can be vendored or replaced by a smaller owned GF(256) implementation if dependency size is still a concern.
