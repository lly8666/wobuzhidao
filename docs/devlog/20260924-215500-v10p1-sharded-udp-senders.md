# 20260924-215500 V10.1 sharded UDP send workers

## V10 dense-repair canary result

V10 SOURCE_SHA `2f6a1e470e7b3d2967460971987a9dace5aa972d` Normal/5205/seed101 run `35999756079` is a new workflow_dispatch attempt 1 and has all five analyzer classifications PASS.

Functional/resource results are clean:

- all packet/raw/UDP/link/qdisc drops = 0;
- stress goodput C2S 10.00091 Mbps / S2C 10.00836 Mbps;
- probe stress p95 614.115 ms;
- outer/app C2S 5.47544x / S2C 5.61084x;
- global UDP backlog total peak 42 / 1024 and 21,472 / 9,150,464 bytes;
- global UDP overflow/stale/send error = 0;
- FreshBlocked / FreshWindowBypass / FreshEmitFailures = 0;
- RepairEvictionMaxScan = 1;
- FastRepairs C2S 23,690 / S2C 23,659.

However, process CPU is client 96.08s / server 100.85s. The same Normal/5205/seed101 V9 canary used 54.83s / 54.87s with nearly identical packet and outer-byte volume. V10 also shows repair reserve abandonment again (about 1.8k client and 2.5k server in the read-only stress window), while V9 had zero.

Therefore this V10 sample remains a historical five-class PASS but is **not** accepted as evidence that V10 is ready for final18.

## Root-cause hypothesis

V10 has four global sender workers consuming one FIFO. For a single hot UDP mapping, multiple workers may pop consecutive records for the same flow and then serialize on `state.sendMu`. Under the denser 20% repair regime, this creates runnable-worker/mutex contention without increasing delivered application work or wire volume.

The 30% samples have far fewer repair sends and did not expose the same CPU increase.

## V10.1 product delta

Keep all V10 budgets unchanged:

- one global maximum of 1024 pending UDP mapping records;
- byte maximum `1024 * MaxPayload`;
- four fixed sender workers;
- no per-flow queue;
- no kernel socket buffer change;
- no per-packet goroutine.

Change worker ownership:

1. each flow ID hashes deterministically to exactly one of the four sender shards;
2. each shard has one worker, so one flow can never have multiple workers contending to send it;
3. global queued+inflight records and bytes remain counted under one shared mutex/budget;
4. when the global record budget is full, enqueue compares only the four fixed shard heads and evicts the globally oldest queued datagram;
5. the eviction scan is therefore fixed at at most four entries, independent of flow count and backlog size;
6. diagnostics export the maximum fixed eviction scan.

This preserves bounded newest-first overload semantics while removing same-flow worker contention.

## Validation order

All validation stays in GitHub Actions.

First correctness/race only. If green, run one new exact-SHA Normal/5205/seed101 canary. It must retain roughly 23k real FastRepairs per direction, zero fresh blocking, zero local socket/link loss, strict queue bounds, `RepairEvictionMaxScan<=1`, UDP backlog eviction scan <=4, and materially remove the V10 CPU regression before any final18 is considered.
