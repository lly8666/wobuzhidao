# V2.2 shared-account two-session 100 Mbit weak-link qualification

Status: **qualified characterization checkpoint** for the current fixed `20:20` WBD FEC path and `legacy` FakeTCP recovery. This is not a protocol-freeze load ceiling.

Authoritative GitHub Actions run: `32918572671` (`mux-load-100m`, run #5), substantive head `d29856d017a08dd3f9d7d291e4179ac53ca3c614`. All four network matrix jobs and the aggregate job completed successfully.

## Boundary under test

Each point starts two simultaneous devices using the same configured Reality-front username/password. The front issues two distinct one-time tickets. Both devices then traverse one public WBD FakeTCP listener, separate raw associations, separate wolfSSL DTLS 1.3 workers, separate immutable LINK/LiveID sessions, and the shared `wbd-link-server-mux`/`session.DataPlane` service boundary.

The measured direction is client -> server and the two application streams run concurrently:

- shared forward bottleneck: 100 Mbit/s;
- reverse links: 100 Mbit/s per client;
- inner offered rate: 20 Mbit/s per stream, 40 Mbit/s aggregate;
- packet size: 1200 bytes;
- offered interval: 2 seconds;
- RTT: 20 ms or 100 ms;
- measurement loss: random 20% in both directions;
- setup loss: 0%; RTT delay and the 100 Mbit/s rate remain active during setup;
- loss is enabled only after both LINK sessions are established and immediately before the offered interval;
- hosted `tc netem` has no deterministic seed here, so the run is a paired diagnostic/qualification sample rather than deterministic release statistics.

Separating setup from measurement is deliberate. Earlier harness attempts applied 20% loss before FakeTCP/DTLS establishment and produced random pre-data handshake failures. Those failures did not exercise either FEC mode and therefore contaminated a sustained data-path comparison.

The harness also inserts a transparent meter only on each client LINK -> DTLS plaintext leg. It counts the actual datagrams emitted by the live path and classifies `WF` shard index `<20` as systematic and `>=20` as repair. Therefore FEC expansion below is measured, not inferred from the nominal 20+20 profile.

## Paired results

| RTT | Delivery off -> 20:20 | Goodput off -> 20:20 | One-way p50 off -> 20:20 | One-way p99 off -> 20:20 | Stack CPU off -> 20:20 | Peak RSS off -> 20:20 | FEC plaintext expansion |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 ms | 0.8022 -> **1.0000** | 32.083 -> **39.994 Mbit/s** | 10.497 -> 11.977 ms | 11.061 -> 20.678 ms | 3.57 -> 4.20 s | 80,248 -> 106,336 KiB | 2.1622x |
| 100 ms | 0.8062 -> **1.0000** | 32.242 -> **39.994 Mbit/s** | 50.502 -> 52.531 ms | 50.770 -> 61.185 ms | 3.60 -> 4.18 s | 82,636 -> 104,368 KiB | 2.1597x |

Paired deltas:

- RTT 20 ms: delivery **+19.779 percentage points**, goodput **+7.911 Mbit/s**, p50 **+1.480 ms**, p99 **+9.617 ms**, CPU **+17.65%**, peak RSS **+32.51%**.
- RTT 100 ms: delivery **+19.383 percentage points**, goodput **+7.752 Mbit/s**, p50 **+2.029 ms**, p99 **+10.415 ms**, CPU **+16.11%**, peak RSS **+26.30%**.

Both fixed-FEC streams individually delivered all 4166 planned datagrams at both RTTs. The off-mode streams remained balanced rather than starving one LiveID: at RTT 20 ms both were exactly 0.8022 delivery in this run; at RTT 100 ms they were 0.8012 and 0.8111.

## Wire and recovery accounting

At RTT 20 ms, fixed `20:20` emitted 10,464,992 systematic bytes and 11,153,280 repair bytes on the measured LINK -> DTLS plaintext legs. At RTT 100 ms it emitted 10,464,992 systematic bytes and 11,128,160 repair bytes. The small amount above an exact 2x inner ratio comes from the WBD FEC shard/header and partial-block behavior.

Forward qdisc drops approximately doubled because fixed FEC deliberately put roughly twice as many packets on the shared link: 1657 -> 3470 drops at RTT 20 ms and 1618 -> 3452 at RTT 100 ms. Despite those additional dropped outer packets, the live decoder reconstructed every planned inner datagram in both fixed-FEC cases.

Client FakeTCP retransmit bytes were lower with fixed FEC in this sample: 45,214 -> 20,448 bytes at RTT 20 ms (-54.8%) and 34,216 -> 21,726 bytes at RTT 100 ms (-36.5%). This is diagnostic only; current product recovery remains `legacy`, and this run does not justify reopening the previously closed `sack-rack` default decision.

## Interpretation and continuation

This checkpoint qualifies the shared-account two-session fan-out at the present **40 Mbit/s aggregate inner offered rate** under 20% random loss: the two LiveIDs remain independent and balanced, fixed `20:20` restores full application delivery, and first-complete latency remains bounded without ordinary TCP HOL behavior.

It does **not** establish the safe aggregate load ceiling. Measured fixed-FEC plaintext expansion is about 2.16x, so 40 Mbit/s inner already produces roughly mid-80-Mbit/s LINK plaintext before DTLS/FakeTCP overhead. The next transport qualification should therefore keep protocol/FEC semantics frozen and sweep aggregate inner offered load upward (for example 40 -> 60 -> 80 Mbit/s at RTT 20/100 ms, 20% measured loss) to find where the 100 Mbit/s shared bottleneck turns the fixed profile from recovery gain into persistent queue/tail-latency collapse. Do not remove or retune fixed `20:20` before that load ceiling is measured.
