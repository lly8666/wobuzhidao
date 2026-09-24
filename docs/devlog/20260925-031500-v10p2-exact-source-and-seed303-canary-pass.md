# 20260925 V10.2 exact-source correctness and Normal/5305/seed303 canary PASS

Exact product SOURCE_SHA: `56eb5413c3cf2e559b82026e8a5783508764e2f4`.

## Exact-source correctness/race/privileged coverage

The branch head is still the docs-only `df391d9ee017eb3dc55ae385e8847559ecf3f2e8`; its parent `56eb5413c3cf2e559b82026e8a5783508764e2f4` remains the V10.2 product candidate.

Existing product-SHA push runs `36031020170` (targeted) and `36031020171` (foundation) failed only at the repository contract with `Each change needs STATUS update`; their downstream product jobs were skipped. They are not product compile/test/race failures.

To remove the exact-SHA evidence gap without moving or force-updating the product branch, main commit `1aeace341928805baa297357045ed54dc3533b05` added a one-shot verification workflow. Run `36039856693` triggered from main, but every job explicitly checked out `56eb5413c3cf2e559b82026e8a5783508764e2f4` and first required `git rev-parse HEAD == SOURCE_SHA`.

All jobs PASS:

- Linux job `107769035755`: V10.2 OpenWrt client queue unit coverage, `go test ./...`, `go build ./...`, full `go test -race ./...`, targeted steady core/race, lifecycle core/focused race.
- Windows job `107769035789`: exact-source foundation/unit/build plus targeted packages and client/server builds.
- privileged OpenWrt job `107769035359`: exact-source real TCP/UDP TPROXY ownership and socket-tunnel adapter; both required PASS markers present.

No performance threshold, repair/FEC/wire parameter, capacity, or kernel socket buffer changed.

## Existing exact-SHA seed303 canary

Remote state had already advanced outside the product branch: fixed ref `perf-fixed/56eb5413c3cf2e559b82026e8a5783508764e2f4` and the canonical main fixed-ref request already pointed at V10.2.

Run `36031678003` is the required single Normal / 5305 / seed303 sample:

- event: `workflow_dispatch`
- attempt: 1
- exact head SHA: `56eb5413c3cf2e559b82026e8a5783508764e2f4`
- mode/rate/lanes: Normal / 10 Mbps each direction / 1 lane
- summary artifact: `10823360900`
- full artifact: `10823620900`

It is retained as the one sample and is not rerun.

Analyzer classifications:

- CAPTURE PASS
- CORRECTNESS PASS
- ENVIRONMENT PASS
- INPUT_VALIDITY PASS
- PERFORMANCE PASS

Local drops are all zero: client/server `ss_udp`, AF_PACKET/`ss_packet`, `ss_raw`, and link deltas.

Stress traffic:

- eventual goodput C2S/S2C: 9.981681 / 9.976207 Mbps
- probe p95: 612.893291 ms
- outer IP / app raw input: 5.299306x / 5.434162x
- process CPU: client 93.77 s / server 92.06 s
- strict analyzer directional FastRepairs: C2S 277 / S2C 319
- ACK/control IP bytes during stress: 21,118,348 / 21,107,612

The CPU figure is not declared an improvement or regression because V10.1 Normal/5305/seed303 was CAPACITY_LIMITED and is not a clean same-condition performance baseline.

## V10.2 client UDP ingress bounds

Artifact readers over full artifact `10823620900` show:

- capacity 1024 records / 9,150,464 payload bytes
- 4 fixed workers
- total peak 40 records
- total bytes peak 20,960
- queue peak 30 records / 15,200 bytes
- in-flight peak 1 record / 1,200 bytes
- overflow drops/bytes = 0
- gate errors = 0
- forward errors = 0
- close drops = 0
- eviction max scan = 0

The previous V10.1 client TPROXY socket boundary did not recur: client `ss_udp` max rmem was only 11,712 / 1,048,576 and drops remained zero in all stages.

## Server UDP / receive / transport

Server UDP remains bounded:

- capacity 1024 records / 9,150,464 bytes
- total peak 37 records / 19,440 bytes
- queue peak 17 records / 9,056 bytes
- in-flight peak 1 record / 1,200 bytes
- overflow/stale/send/close errors = 0
- eviction max scan = 0

SegmentMux remains bounded: peak 350 / 4096, bytes peak 168,185, full waits 0.

Transport invariants remain:

- FreshBlocked = 0
- FreshWindowBypass = 0
- FreshEmitFailures = 0
- RepairEvictionMaxScan = 1

At 30% stress the strict analyzer reports FastRepairs 277 / 319 by direction, while high abandonment/forgiveness remains expected bounded-loss behavior. Artifact-reader endpoint-stage reserve peaks are below 1024 during stress (client 724, server 619); reserve drop/expiry remain zero. The canary does not attempt to increase repair rate.

## Qualification state

V10.1 final18 remains permanently 17 PASS / 1 CAPACITY_LIMITED. Run `36006942033` and its client TPROXY `ss_udp` drop=2552 remain immutable historical evidence. This V10.2 canary does not replace it.

V10.2 is **not qualified**. No new final18 has started and P6 remains NOT_RUN.

Next, run one new independent exact-SHA Normal/5205/seed101 diagnostic sample to compare against the clean V10.1 dense-repair baseline (about 23.6k FastRepairs per direction, CPU 50.64 / 52.52 s). If it remains clean, decide whether Game4/5305/seed101 and lossless/seed101 are needed before any new final18. Any FAIL or CAPACITY_LIMITED stops the chain.
