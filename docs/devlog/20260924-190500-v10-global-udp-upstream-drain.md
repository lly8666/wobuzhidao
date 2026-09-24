# 20260924-190500 V10 global UDP upstream drain

## Trigger

V9 SOURCE_SHA `af6d1f21cc589c4ebfffc07efc38df3ce4ff97bf` permanently stopped its dedicated final18 at 17 PASS / 1 CAPACITY_LIMITED.

The immutable failure is Normal/5305/seed101 run `35978250504`. AF_PACKET/raw/link/qdisc drops are all zero, so V9 successfully removed the previous raw-reader amplification. The remaining local loss is 2556 kernel UDP drops on the product-owned `platformflow.UDPServer` upstream mapping socket.

Read-only code review confirms the coupling: `readUpstream` performs one `ReadFromUDPAddrPort`, then synchronously calls `state.tunnel.Send`, and only after that call returns can it drain the next mapping datagram. During the observed ~1.3s downstream stall, the 1MiB mapping Recv-Q fills.

## V10 product delta

V10 does not enlarge any kernel socket buffer and does not allocate a large queue per flow.

A single `UDPServer` now owns one global bounded upstream-send backlog:

- global pending budget: exactly 1024 records across all UDP mappings;
- global payload byte ceiling: `1024 * MaxPayload` (about 8.7 MiB);
- queued and currently sending records both count against the same budget;
- no per-flow buffered send queue is added;
- four fixed sender workers are shared across the server;
- each flow retains a small send mutex so a single mapping is not concurrently submitted through its `TunnelFlow`;
- `readUpstream` copies a datagram then immediately performs an O(1) enqueue;
- at global record capacity, exactly one oldest queued datagram is discarded and the newest is retained;
- if a queued record belongs to a retired flow it is discarded as stale;
- queue occupancy, bytes, overflow drops/bytes/age, stale drops, send errors and close drops are exported in runtime diagnostics.

The record and byte budget is independent of `DefaultMaxUDPFlows=4096`; worst-case backlog does not multiply by the number of mappings.

Unchanged: SegmentMux 4096, LifecycleServer ready 4096, active repair 4096, repair reserve 1024, FEC20:20, wire format, RTO/horizon/repair credits, packet-socket buffers, loss thresholds and analyzer thresholds.

## Intended semantics

This remains partial reliability. When downstream sending is stalled long enough to exhaust the global budget, V10 intentionally discards the oldest unsent UDP mapping reply so the product can keep draining kernel UDP sockets and retain newer traffic. It does not attempt reliable UDP delivery.

## Validation order

No local compile/test/race/performance.

1. GitHub Actions platformflow/runtimeentry unit and race coverage, targeted, foundation and lifecycle.
2. Any failure stops performance.
3. If green, one new Normal/5305/seed101 strict diagnostic canary at the exact V10 product SHA.
4. Require all analyzer classifications PASS, all socket/link/qdisc drops zero, global queue total <=1024 and bytes <= budget, explicit overflow accounting if exercised, fresh three counters zero, and RepairEvictionMaxScan <=1.
5. Historical V8 and V9 failed samples remain immutable and are never replaced.
