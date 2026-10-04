# V2.2 shared-account two-session 100 Mbit weak-link qualification

Status: **QUALIFIED / RELEASE CAP FROZEN.** The current fixed `20:20` WBD FEC path with `legacy` FakeTCP recovery is qualified at a conservative **40 Mbit/s aggregate inner offered rate** on the 100 Mbit/s shared weak-link boundary. The corrected 40/60/80 sweep is now authoritative and reproduces the tail-latency cliff above 40 Mbit/s, so no 50 Mbit/s midpoint is required for the release path.

The protocol-freeze decision is recorded in `docs/architecture/ADR-0010-v2-protocol-freeze-40m-release-cap.md`.

## Boundary under test

Each point starts two simultaneous devices using the same configured Reality-front username/password. The front issues two distinct one-time tickets. Both devices then traverse one public WBD FakeTCP listener, separate raw associations, separate wolfSSL DTLS 1.3 workers, separate immutable LINK/LiveID sessions, and the shared `wbd-link-server-mux` / `session.DataPlane` boundary.

The measured direction is client -> server and the two application streams run concurrently:

- shared forward bottleneck: 100 Mbit/s;
- reverse links: 100 Mbit/s per client;
- packet size: 1200 bytes;
- offered interval: 2 seconds;
- RTT: 20 ms or 100 ms;
- measurement loss: random 20% in both directions;
- setup loss: 0%; RTT delay and the 100 Mbit/s rate remain active during setup;
- loss is enabled only after both LINK sessions are established and immediately before the offered interval;
- hosted `tc netem` is unseeded, so each point is a paired characterization sample rather than a deterministic delivery curve.

The harness meters each client LINK -> DTLS plaintext leg and classifies `WF` shard index `<20` as systematic and `>=20` as repair. FEC expansion is therefore measured from the live path rather than inferred from the nominal profile.

## Original authoritative 40 Mbit/s qualification

GitHub Actions run `32918572671` (`mux-load-100m`, run #5), substantive head `d29856d017a08dd3f9d7d291e4179ac53ca3c614`, established the first authoritative 40 Mbit/s two-session point.

| RTT | Delivery off -> 20:20 | Goodput off -> 20:20 | One-way p50 off -> 20:20 | One-way p99 off -> 20:20 | Stack CPU off -> 20:20 | Peak RSS off -> 20:20 | FEC plaintext expansion |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 ms | 0.8022 -> **1.0000** | 32.083 -> **39.994 Mbit/s** | 10.497 -> 11.977 ms | 11.061 -> 20.678 ms | 3.57 -> 4.20 s | 80,248 -> 106,336 KiB | 2.1622x |
| 100 ms | 0.8062 -> **1.0000** | 32.242 -> **39.994 Mbit/s** | 50.502 -> 52.531 ms | 50.770 -> 61.185 ms | 3.60 -> 4.18 s | 82,636 -> 104,368 KiB | 2.1597x |

Both fixed-FEC streams individually delivered all 4166 planned datagrams at both RTTs. The two LiveIDs remained balanced.

At RTT 20 ms, fixed `20:20` emitted 10,464,992 systematic bytes and 11,153,280 repair bytes on the measured plaintext legs. At RTT 100 ms it emitted 10,464,992 systematic bytes and 11,128,160 repair bytes. Fixed FEC intentionally increased outer packet pressure while reconstructing every planned inner datagram at the qualified point.

## Historical diagnostic 60/80 sweep

Run `32920381126` at commit `621653e30f8bd7e9d9d26bce598077d8391043ad` completed the cited network measurements but used literal `20:20` in GitHub artifact names, causing fixed jobs to fail at artifact upload. It remains **diagnostic log evidence only**.

That diagnostic already showed the important signal: p99 moved from tens of milliseconds at 40 Mbit/s to hundreds of milliseconds at 60 Mbit/s and about one second at 80 Mbit/s, while per-session delivery remained balanced. The workflow was corrected at `71501859c6fc1aa1a6d1a6b048af5aebcf984732` to build once per RTT, run six points sequentially, use filesystem-safe `20x20` artifact names, and cancel stale same-workflow runs.

## Authoritative corrected 40/60/80 sweep

The corrected workflow completed cleanly on branch `dev/wbd-raw-fec-v2`, head `a3d8b05a875b2880c69cb4d6bada967eef8c17f9`, as GitHub Actions run **`32920925944`** (`mux-load-100m`, run #15).

All three jobs succeeded:

- `bench (20)` — SUCCESS;
- `bench (100)` — SUCCESS;
- `aggregate` — SUCCESS.

The run produced the unexpired artifacts `mux-load-rtt20-sweep`, `mux-load-rtt100-sweep`, and `mux-load-100m-sweep-summary`.

### Corrected sweep result

| RTT | Aggregate inner offered | Delivery off -> 20:20 | Goodput off -> 20:20 | One-way p50 off -> 20:20 | One-way p99 off -> 20:20 | Fixed plaintext expansion | Fixed stream gap |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 20 ms | 40 Mbit/s | 0.799 -> **1.000** | 31.94 -> **39.99 Mbit/s** | 10.50 -> **12.94 ms** | 11.04 -> **21.77 ms** | 2.157x | 0.00 pp |
| 20 ms | 60 Mbit/s | 0.800 -> **0.774** | 47.97 -> **46.45 Mbit/s** | 10.70 -> **253.25 ms** | 64.28 -> **286.60 ms** | 2.095x | 0.32 pp |
| 20 ms | 80 Mbit/s | 0.596 -> **0.759** | 47.69 -> **60.70 Mbit/s** | 23.22 -> **619.03 ms** | 176.32 -> **956.26 ms** | 2.094x | 0.76 pp |
| 100 ms | 40 Mbit/s | 0.799 -> **1.000** | 31.96 -> **39.99 Mbit/s** | 50.50 -> **52.15 ms** | 50.66 -> **60.71 ms** | 2.170x | 0.00 pp |
| 100 ms | 60 Mbit/s | 0.800 -> **1.000** | 48.01 -> **60.00 Mbit/s** | 50.55 -> **137.38 ms** | 50.94 -> **211.00 ms** | 2.095x | 0.00 pp |
| 100 ms | 80 Mbit/s | 0.785 -> **1.000** | 62.79 -> **80.00 Mbit/s** | 52.74 -> **538.32 ms** | 164.47 -> **913.95 ms** | 2.094x | 0.00 pp |

The corrected sample changes some random-loss delivery values relative to the historical diagnostic, as expected for unseeded `netem`, but it reproduces the robust product signal:

- 40 Mbit/s remains bounded: fixed `20:20` delivers 100% with p99 about 21.8 ms at RTT20 and 60.7 ms at RTT100;
- 60 Mbit/s already enters persistent hundreds-of-milliseconds queueing at both RTTs; RTT20 also loses delivery/goodput despite FEC;
- 80 Mbit/s approaches one second p99 at both RTTs;
- the two LiveIDs remain balanced, so the failure mode is shared-link queue pressure rather than session starvation.

This is sufficient clean evidence to close the capacity cursor. A 50 Mbit/s midpoint could narrow the numerical cliff in future research, but it cannot change the release conclusion without making extra throughput a product requirement. It is therefore not on the release critical path.

## Release decision

The qualified V2.2 release operating point is **40 Mbit/s aggregate inner offered payload** under the present 100 Mbit/s shared-link / 20% measurement-loss boundary.

For this release decision:

- FEC remains fixed systematic `20:20` where enabled;
- FakeTCP recovery remains `legacy`;
- DTLS, ticket/LiveID session semantics, association fan-out and one-lane raw carrier semantics remain unchanged;
- 50/60/80 Mbit/s are not release operating points;
- `sack-rack` remains experimental and is not reopened by this sweep.

Capacity exploration stops here for V2.2 release work. Continue with the frozen transport/session protocol into one clean OpenWrt TPROXY end-to-end VPN qualification, then one clean Windows TUN/Wintun-class qualification. Platform integration must not change already-qualified transport semantics merely to make routing easier.
