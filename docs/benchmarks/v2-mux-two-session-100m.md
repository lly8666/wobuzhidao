# V2.2 shared-account two-session 100 Mbit weak-link qualification

Status: **qualified characterization checkpoint** for the current fixed `20:20` WBD FEC path and `legacy` FakeTCP recovery. The conservative qualified aggregate inner rate remains **40 Mbit/s** pending any narrower midpoint qualification.

Authoritative 40 Mbit/s GitHub Actions run: `32918572671` (`mux-load-100m`, run #5), substantive head `d29856d017a08dd3f9d7d291e4179ac53ca3c614`. All four network matrix jobs and the aggregate job completed successfully.

## Boundary under test

Each point starts two simultaneous devices using the same configured Reality-front username/password. The front issues two distinct one-time tickets. Both devices then traverse one public WBD FakeTCP listener, separate raw associations, separate wolfSSL DTLS 1.3 workers, separate immutable LINK/LiveID sessions, and the shared `wbd-link-server-mux`/`session.DataPlane` service boundary.

The measured direction is client -> server and the two application streams run concurrently:

- shared forward bottleneck: 100 Mbit/s;
- reverse links: 100 Mbit/s per client;
- packet size: 1200 bytes;
- offered interval: 2 seconds;
- RTT: 20 ms or 100 ms;
- measurement loss: random 20% in both directions;
- setup loss: 0%; RTT delay and the 100 Mbit/s rate remain active during setup;
- loss is enabled only after both LINK sessions are established and immediately before the offered interval;
- hosted `tc netem` has no deterministic seed here, so each run is a paired diagnostic/qualification sample rather than deterministic release statistics.

Separating setup from measurement is deliberate. Earlier harness attempts applied 20% loss before FakeTCP/DTLS establishment and produced random pre-data handshake failures. Those failures did not exercise either FEC mode and therefore contaminated a sustained data-path comparison.

The harness also inserts a transparent meter only on each client LINK -> DTLS plaintext leg. It counts the actual datagrams emitted by the live path and classifies `WF` shard index `<20` as systematic and `>=20` as repair. Therefore FEC expansion below is measured, not inferred from the nominal 20+20 profile.

## Authoritative paired result at 40 Mbit/s aggregate inner

| RTT | Delivery off -> 20:20 | Goodput off -> 20:20 | One-way p50 off -> 20:20 | One-way p99 off -> 20:20 | Stack CPU off -> 20:20 | Peak RSS off -> 20:20 | FEC plaintext expansion |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 ms | 0.8022 -> **1.0000** | 32.083 -> **39.994 Mbit/s** | 10.497 -> 11.977 ms | 11.061 -> 20.678 ms | 3.57 -> 4.20 s | 80,248 -> 106,336 KiB | 2.1622x |
| 100 ms | 0.8062 -> **1.0000** | 32.242 -> **39.994 Mbit/s** | 50.502 -> 52.531 ms | 50.770 -> 61.185 ms | 3.60 -> 4.18 s | 82,636 -> 104,368 KiB | 2.1597x |

Paired deltas:

- RTT 20 ms: delivery **+19.779 percentage points**, goodput **+7.911 Mbit/s**, p50 **+1.480 ms**, p99 **+9.617 ms**, CPU **+17.65%**, peak RSS **+32.51%**.
- RTT 100 ms: delivery **+19.383 percentage points**, goodput **+7.752 Mbit/s**, p50 **+2.029 ms**, p99 **+10.415 ms**, CPU **+16.11%**, peak RSS **+26.30%**.

Both fixed-FEC streams individually delivered all 4166 planned datagrams at both RTTs. The off-mode streams remained balanced rather than starving one LiveID: at RTT 20 ms both were exactly 0.8022 delivery in this run; at RTT 100 ms they were 0.8012 and 0.8111.

## Wire and recovery accounting at 40 Mbit/s

At RTT 20 ms, fixed `20:20` emitted 10,464,992 systematic bytes and 11,153,280 repair bytes on the measured LINK -> DTLS plaintext legs. At RTT 100 ms it emitted 10,464,992 systematic bytes and 11,128,160 repair bytes. The small amount above an exact 2x inner ratio comes from the WBD FEC shard/header and partial-block behavior.

Forward qdisc drops approximately doubled because fixed FEC deliberately put roughly twice as many packets on the shared link: 1657 -> 3470 drops at RTT 20 ms and 1618 -> 3452 at RTT 100 ms. Despite those additional dropped outer packets, the live decoder reconstructed every planned inner datagram in both fixed-FEC cases.

Client FakeTCP retransmit bytes were lower with fixed FEC in this sample: 45,214 -> 20,448 bytes at RTT 20 ms (-54.8%) and 34,216 -> 21,726 bytes at RTT 100 ms (-36.5%). This is diagnostic only; current product recovery remains `legacy`, and this run does not justify reopening the previously closed `sack-rack` default decision.

## 60/80 Mbit/s ceiling sweep diagnostic

A follow-up sweep was started at commit `621653e30f8bd7e9d9d26bce598077d8391043ad`, Actions run `32920381126`, over aggregate inner 40/60/80 Mbit/s, RTT 20/100 ms, and FEC `off`/`20:20`. Every fixed-20:20 network measurement cited below completed and printed a complete `result.json`, but that first sweep workflow used the literal `20:20` in the GitHub artifact name. GitHub rejected `:` during artifact upload. Therefore run `32920381126` is **diagnostic log evidence, not an authoritative aggregate artifact run**.

The workflow was subsequently corrected and amortized at commit `71501859c6fc1aa1a6d1a6b048af5aebcf984732`: one build per RTT, six sequential points per RTT, filesystem-safe `20x20` artifact naming, and workflow concurrency so stale sweeps can be canceled. A later clean run must be used before promoting any load above 40 Mbit/s to qualified status.

The diagnostic fixed-20:20 measurements already show the load ceiling cliff:

| RTT | Aggregate inner offered | Delivery | Goodput | One-way p50 | One-way p99 | Measured plaintext expansion | Session fairness |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 11.977 ms | 20.678 ms | 2.162x | both streams 1.0000 |
| 20 ms | 60 Mbit/s | 0.7558 | 45.346 Mbit/s | 250.567 ms | 305.948 ms | 2.095x | 0.7571 vs 0.7544 |
| 20 ms | 80 Mbit/s | 0.9971 | 79.762 Mbit/s | 530.701 ms | 915.466 ms | 2.094x | 0.9974 vs 0.9968 |
| 100 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 52.531 ms | 61.185 ms | 2.160x | both streams 1.0000 |
| 100 ms | 60 Mbit/s | **1.0000** | **60.000 Mbit/s** | 116.341 ms | 181.819 ms | 2.095x | both streams 1.0000 |
| 100 ms | 80 Mbit/s | 0.9401 | 75.206 Mbit/s | 623.151 ms | 994.520 ms | 2.094x | 0.9414 vs 0.9388 |

For one paired control available directly from the same diagnostic logs, RTT 100 ms / 60 Mbit/s with FEC off delivered 0.8030 at 48.178 Mbit/s with p99 51.143 ms. Fixed 20:20 restored 100% delivery and the full 60 Mbit/s offered rate, but p99 rose to 181.819 ms. That is useful recovery, but it is already a large queueing cost for a latency-first product.

The non-monotonic delivery at RTT 20 ms (60 Mbit/s worse than 80 Mbit/s in this random-loss sample) is exactly why these unseeded points must not be treated as deterministic delivery curves. The robust signal is the **tail-latency cliff**: both RTTs move from tens of milliseconds at 40 Mbit/s to hundreds of milliseconds at 60 Mbit/s and roughly one second by 80 Mbit/s, while the two sessions remain balanced rather than one LiveID starving the other.

## Current decision and continuation

The shared-account two-session fan-out is qualified at **40 Mbit/s aggregate inner offered rate** under the present 100 Mbit/s weak-link / 20% measured-loss boundary. Fixed `20:20` provides a large delivery gain there with bounded first-complete latency and no ordinary TCP HOL behavior.

Do **not** raise the operational/qualification cap to 60 or 80 Mbit/s from the diagnostic sweep. In particular, 60 Mbit/s is not a generally safe point: the RTT 20 ms sample already collapsed to 75.6% delivery with ~306 ms p99, and RTT 100 ms reached ~182 ms p99 even while delivery stayed complete. 80 Mbit/s is clearly outside the acceptable latency region at both RTTs.

The next transport action, if more capacity than 40 Mbit/s is worth pursuing, is a **single narrow midpoint characterization around 50 Mbit/s aggregate inner** at RTT 20/100 ms using the corrected two-job workflow. Keep FEC fixed at `20:20`, recovery fixed at `legacy`, setup loss at 0%, measurement loss at 20%, and do not alter protocol semantics while finding that boundary. If the clean corrected sweep/midpoint does not materially improve the latency margin, retain 40 Mbit/s as the conservative release cap and move on to protocol freeze plus OpenWrt TPROXY and Windows TUN one-shot release integration.
