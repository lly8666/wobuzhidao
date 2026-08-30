# ADR-0010: Freeze V2 transport semantics at the 40 Mbit/s release operating point

Status: **ACCEPTED / AMENDED BY ADR-0011 AND ADR-0012** (original 2026-08-26; setup amendment 2026-08-29; tunnel/lane lifecycle amendment 2026-08-30)

> ADR-0011 reopened the former Reality/FakeTCP connection-boundary decision. ADR-0012 later separates the long-lived Logical Tunnel from replaceable Transport Lanes and reopens the old permanent `one raw lane` clause. The measured 40 Mbit/s release operating point, DTLS/FEC ordering, no-HOL requirement, FakeTCP recovery default and lane-local immutable transport semantics remain in force.

## Context

V2.2 completed the shared-account two-session transport/session fan-out and the corrected 100 Mbit/s shared-link capacity sweep. The product priority remains earliest-complete inner datagram delivery with balanced independent sessions, not maximizing offered throughput at the cost of persistent queueing.

The corrected `mux-load-100m` workflow introduced at commit `71501859c6fc1aa1a6d1a6b048af5aebcf984732` ran cleanly on branch `dev/wbd-raw-fec-v2` at head `a3d8b05a875b2880c69cb4d6bada967eef8c17f9` as GitHub Actions run `32920925944`.

The fixed `20:20` / `legacy` results showed the release latency boundary:

| RTT | Aggregate inner offered | Delivery | Goodput | One-way p50 | One-way p99 |
|---:|---:|---:|---:|---:|---:|
| 20 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 12.940 ms | 21.768 ms |
| 20 ms | 60 Mbit/s | 0.7742 | 46.451 Mbit/s | 253.253 ms | 286.604 ms |
| 20 ms | 80 Mbit/s | 0.7587 | 60.699 Mbit/s | 619.032 ms | 956.262 ms |
| 100 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 52.152 ms | 60.711 ms |
| 100 ms | 60 Mbit/s | **1.0000** | **60.000 Mbit/s** | 137.377 ms | 210.995 ms |
| 100 ms | 80 Mbit/s | **1.0000** | **80.000 Mbit/s** | 538.317 ms | 913.954 ms |

The 60/80 Mbit/s points consume unacceptable latency margin for the release target. A 50 Mbit/s midpoint remains optional research.

## Decision 1 — release operating point

The conservative release operating point on the current <=100 Mbit/s weak-link qualification boundary remains:

- **40 Mbit/s aggregate inner offered payload** across the logical tunnel;
- fixed systematic `20:20` tail-RS where FEC is enabled;
- `legacy` FakeTCP shadow recovery as the product default.

Do not promote 50/60/80 Mbit/s into the release cap without a separate benchmark decision. Multiple race lanes are redundancy paths; they do not authorize higher aggregate inner payload by themselves.

## Decision 2 — steady-state protocol freeze boundary

The following remain frozen unless a later explicit ADR reopens them with regression evidence:

- WBD-owned TCP-shaped raw FakeTCP carrier; no ordinary kernel TCP payload byte stream;
- UDP/datagram-like earliest-complete steady-state semantics;
- FEC `off` or currently qualified fixed systematic `20:20`; no in-place runtime FEC epoch change;
- FEC shard -> independent DTLS 1.3 datagram -> FakeTCP ordering **inside each lane**;
- pinned wolfSSL DTLS 1.3 implementation and existing product verification policy;
- shared username/password admission issuing independent one-time lane/session credentials;
- `legacy` FakeTCP recovery default; `sack-rack` remains experimental;
- LINK/FEC parameters are immutable for a transport lane/epoch;
- the <=100 Mbit/s qualification ceiling and 40 Mbit/s aggregate-inner release point.

### Amendment: per-lane single-flow setup

ADR-0011 requires, for every WBD lane:

```text
one FakeTCP SYN lineage
  -> temporary reliable ordered bootstrap mode
  -> real TLS 1.3 / Reality-like admission
  -> same lane 4-tuple and sequence space
  -> DTLS 1.3 / LINK / lane-local FEC datagrams
```

No second ordinary kernel-TCP WBD payload connection is permitted after successful admission for that lane. The reliable ordered adapter exists only for bounded TLS setup and is destroyed before no-HOL data mode.

### Amendment: lane-count / logical-tunnel lifecycle

ADR-0012 supersedes the old clause that made `one raw lane` a permanent architectural requirement.

The accepted model is now:

- normal mode steady baseline: one active Transport Lane;
- later Game Lane/race mode: 2..4 independent complete WBD lanes according to product policy/controller limits;
- controlled make-before-break replacement: a candidate lane may temporarily overlap an old lane;
- one logical PacketID may race across independent lanes with first-arrival delivery and duplicate suppression;
- FEC remains lane-local and never spans lanes;
- aggregate inner release operating point remains 40 Mbit/s until separately requalified.

This amendment does not revive rejected V1 PR #2. V1 used ordinary ordered kernel TCP lanes and inherited TCP HOL. ADR-0012 explicitly preserves the later WBD Game Lane no-cross-lane-HOL semantics instead.

## Decision 3 — release work after ADR-0012

Before platform release can rely on the new logical-tunnel architecture, preserve the old transport gates and add the new lifecycle/data-plane gates:

1. per lane, capture one WBD SYN lineage and one 4-tuple through TLS bootstrap and DTLS transition;
2. verify sequence-space continuity and no FIN/RST/new WBD payload SYN at the lane mode switch;
3. verify real TLS 1.3/SNI and no plaintext credentials/ticket;
4. verify post-switch later independent datagrams bypass an earlier sequence hole;
5. verify two logical tunnels receive distinct server-assigned IP leases;
6. verify shared server TUN + one host NAT, same inner source port to same target, and source-spoof rejection;
7. verify idle sleep/wake and make-before-break lane replacement using the existing race/dedup semantics;
8. verify game/race mode can replace one lane without losing its desired healthy redundancy;
9. then repeat physical Windows TUN and Linux/OpenWrt qualification on exact-source artifacts.

## Pinned upstream boundary

No upstream versions are floated by these amendments:

- wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- udp2raw `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63` as external/raw baseline;
- UDPspeeder `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454` as external FEC baseline;
- quic-go `v0.61.0` (`579ee19`) remains a historical benchmark oracle only.

Any upstream upgrade is a separate workstream.
