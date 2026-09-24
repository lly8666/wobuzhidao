# 20260924-172000 V9 final18 stopped: server UDP mapping capacity-limited

Exact product SOURCE_SHA: `af6d1f21cc589c4ebfffc07efc38df3ce4ff97bf`.

## Final18 result

The dedicated V9 final18 used frozen ref `perf-fixed/af6d1f21cc589c4ebfffc07efc38df3ce4ff97bf-r2`. Exactly 18 new identities were dispatched; every strict sample is `workflow_dispatch`, attempt 1, exact product SHA. No V8 sample or V9 diagnostic canary was reused.

Result after reading all compact analyzer summaries:

- 17 / 18: all five classifications PASS
- 1 / 18: CAPACITY_LIMITED
- all six lossless samples retained zero FastRepairs
- all link/qdisc drops are zero
- no strict performance run was rerun

The campaign therefore stops. CAPACITY_LIMITED is not PASS, and P6 is NOT_RUN.

The immutable failing sample is Normal / 5305 / seed101:

- run `35978250504`
- job `107563733006`
- summary artifact `10798968472`, sha256 `046476c18a905a0c426c8d38fe2ac32f2446f4edd471d9ce349ed9ecf94597af`
- full artifact `10799441721`, sha256 `5c041b6488271e5bb8acbfad14cda8e1bad7effa7deb28ed3a425fe8ee3126d2`
- CAPTURE PASS
- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- ENVIRONMENT FAIL
- PERFORMANCE CAPACITY_LIMITED

## Important difference from the V8 capacity failures

This time AF_PACKET did not overflow:

- client packet-socket drops = 0
- server packet-socket drops = 0
- raw socket drops = 0
- link/qdisc drops = 0

The only local socket loss is `server/ss_udp = 2556`.

Read-only artifact analysis identifies the socket as `0.0.0.0:50934`, a product `platformflow.UDPServer` upstream mapping created by `net.ListenUDP`, not the target/biz generator.

All 2556 UDP drops happen in stress around elapsed 36–37s:

- first interval: +1492 drops; Recv-Q reaches 1,050,816 bytes against a 1,048,576 receive buffer
- next interval: +1064 drops, then the queue drains

## What V9 fixed, and what it exposed

The V9 receive shedding path did activate during the same event:

- LifecycleServer ready capacity remains 4096
- diagnostic peak = 4097 because accounting occurs before the O(1) oldest eviction
- explicit userspace overflow drops = 3890
- overflow age max about 823ms
- handler max about 1.313s
- raw packet socket still recorded zero drops

Therefore V9 succeeded at its intended layer: a full server ready queue no longer blocks the raw reader until AF_PACKET overflows.

But the downstream UDP mapping has a separate synchronous coupling. `platformflow.UDPServer.readUpstream` reads one datagram and then synchronously calls `state.tunnel.Send(...)`; it cannot drain the next UDP datagram until that send returns. During the observed downstream stall, its 1MiB kernel UDP queue fills and drops 2556 packets.

This is not acceptable as PASS and cannot be relabeled as netem loss.

## Business / performance effect

The failed sample still carries most traffic but is measurably below the passing 5305 samples:

- C2S stress goodput about 9.900 Mbps
- S2C stress goodput about 9.811 Mbps
- probe p95 about 620.0ms
- outer/app about 5.2979x C2S / 5.3904x S2C
- process CPU about 84.6s client / 87.4s server

Fresh transport invariants remain intact:

- FreshBlocked = 0
- FreshWindowBypass = 0
- FreshEmitFailures = 0
- RepairEvictionMaxScan = 1

## Resource-bound implication

`platformflow.MaxPayload` is 8936 bytes (9000 leased IPv4 limit - 20 IPv4 header - 44 platform frame header).

A per-flow 4096-record userspace queue would therefore permit roughly 36.6MiB of payload per flow before object overhead; with up to 4096 UDP flows that is not a valid bounded design.

The next design must use a strict global record and byte budget. Do not solve this by blindly increasing per-socket kernel receive buffers, per-flow giant queues, per-packet goroutines, or an unbounded sender queue.

Historical V8 final18 remains 16 PASS / 2 CAPACITY_LIMITED. Historical V9 final18 remains 17 PASS / 1 CAPACITY_LIMITED. None are overwritten.
