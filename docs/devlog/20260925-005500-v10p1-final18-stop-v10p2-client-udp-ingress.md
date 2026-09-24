# 20260925-005500 V10.1 final18 stop; V10.2 client UDP ingress

Exact V10.2 candidate SOURCE_SHA: `56eb5413c3cf2e559b82026e8a5783508764e2f4`.

## V10.1 final18 is permanently stopped

V10.1 product SOURCE_SHA `e67b51968d9e5f707dfc9ef7d459ba5192008901` used a dedicated frozen ref and 18 new workflow_dispatch attempt-1 samples. The campaign is permanently **17 PASS / 1 CAPACITY_LIMITED**.

The sole failed sample is Normal / 5305 / seed303 run `36006942033`:

- CAPTURE PASS
- CORRECTNESS PASS
- INPUT_VALIDITY PASS
- ENVIRONMENT FAIL
- PERFORMANCE CAPACITY_LIMITED
- product client TPROXY `0.0.0.0:12345` / `client/ss_udp` drops = 2552
- receive queue reached about 1,049,472 bytes against a 1,048,576-byte receive buffer
- AF_PACKET/raw/server UDP/link/qdisc drops = 0
- stress goodput about 9.817 / 9.905 Mbps
- probe p95 about 616.53 ms
- stress FastRepairs about 350 / 287 per direction
- FreshBlocked / FreshWindowBypass / FreshEmitFailures = 0
- RepairEvictionMaxScan = 1

The failure is immutable and is not replaced by later diagnostics. P6 is NOT_RUN.

## Root cause

The OpenWrt client TPROXY UDP loop is synchronous:

`ReadMsgUDP -> BeforeBusiness -> UDPClient.ForwardUDP -> tunnel.Send -> next ReadMsgUDP`.

A tunnel send stall therefore stops draining the product-owned TPROXY UDP socket. This is the client-side mirror of the server mapping-socket coupling addressed in V10/V10.1.

## V10.2 product delta

V10.2 changes only the client TPROXY UDP ingress handoff:

- one global maximum of 1024 pending records;
- global payload byte ceiling `1024 * MaxPayload`;
- queued plus in-flight records/bytes share the same budget;
- four fixed worker shards;
- the same client endpoint deterministically stays on one shard, preserving per-flow FIFO and avoiding same-flow worker contention;
- `udpLoop` copies a valid intercepted datagram and immediately returns to draining the kernel socket;
- workers run `BeforeBusiness` and `ForwardUDP`;
- at global capacity, enqueue examines at most the four fixed shard heads and discards the globally oldest queued datagram;
- diagnostics expose queue/in-flight occupancy, record/byte peaks, overflow drops/bytes/age, gate errors, forward errors, close drops, workers and eviction max scan.

There is no kernel socket-buffer enlargement, per-flow large queue, per-packet goroutine, unbounded queue, or repair/FEC/wire/RTO/credit/threshold change.

## Repair quantity by loss regime

Current same-stack strict evidence remains intentionally non-monotonic. FastRepairs below are real shadow fast retransmissions, not FEC:

| Loss stage | Normal | Game4 |
| --- | ---: | ---: |
| lossless | 0 false repairs | 0 false repairs |
| 5% shoulder, 30s | about 6.0k-7.0k per direction | about 3.3k-4.4k per direction, four lanes aggregated |
| 20% stress, 60s | about 23.5k-24.3k per direction | about 11.7k-12.2k per direction, four lanes aggregated |
| 30% stress, 60s | about 0.29k-0.35k per direction | about 1.55k-1.69k per direction, four lanes aggregated |

At 30% loss, more gaps are forgiven/abandoned and fewer still-present ciphertext shadows remain timely enough for repair. The product intentionally does not become a reliable TCP byte stream.

## Validation order

All compile/test/race/performance remains GitHub Actions only.

1. Correctness/race plus privileged OpenWrt TPROXY coverage first.
2. Any failure stops performance.
3. If green, run one new exact-SHA Normal/5305/seed303 diagnostic canary.
4. Require client socket drops zero, strict client ingress record/byte bounds, client eviction scan <=4, server backlog bounded, fresh three counters zero, RepairEvictionMaxScan <=1 and all five analyzer classifications PASS.
5. Diagnostic canaries never replace the historical V10.1 CAPACITY_LIMITED sample.
