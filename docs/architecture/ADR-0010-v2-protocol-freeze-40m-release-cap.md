# ADR-0010: Freeze V2.2 transport semantics at the 40 Mbit/s release operating point

Status: **ACCEPTED / AMENDED BY ADR-0011** (original 2026-08-26; setup-boundary amendment 2026-08-29)

> ADR-0011 reopens only the former Reality/FakeTCP connection-boundary decision. The measured steady-state release operating point, DTLS/FEC ordering, no-HOL requirement, FakeTCP recovery default and one-lane decision remain in force.

## Context

V2.2 completed the shared-account two-session transport/session fan-out and the corrected 100 Mbit/s shared-link capacity sweep. The product priority remains earliest-complete inner datagram delivery with balanced independent LiveID sessions, not maximizing offered throughput at the cost of persistent queueing.

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

- **40 Mbit/s aggregate inner offered payload**;
- fixed systematic `20:20` tail-RS where FEC is enabled;
- `legacy` FakeTCP shadow recovery as the product default;
- one raw lane.

Do not promote 50/60/80 Mbit/s into the release cap without a separate benchmark decision.

## Decision 2 — steady-state protocol freeze boundary

The following remain frozen unless a later explicit ADR reopens them with regression evidence:

- WBD-owned TCP-shaped raw FakeTCP carrier; no ordinary kernel TCP payload byte stream;
- UDP/datagram-like earliest-complete steady-state semantics;
- FEC `off` or the currently qualified fixed systematic `20:20` profile; no in-place runtime FEC epoch change;
- FEC shard -> independent DTLS 1.3 datagram -> FakeTCP ordering;
- pinned wolfSSL DTLS 1.3 implementation and existing product verification policy;
- shared username/password admission issuing independent one-time tickets/LiveIDs;
- atomic ticket claim, peer/LiveID demux and immutable per-LiveID link state;
- one public FakeTCP raw listener with independent raw associations and one DTLS worker per WBD association;
- `legacy` FakeTCP recovery default; `sack-rack` remains experimental;
- one raw lane as the release baseline.

### Amendment: Reality/FakeTCP setup boundary

The older V2.2 clause that treated Reality-like TLS admission as a **separate public ordinary TCP connection** is superseded by ADR-0011.

The product now requires:

```text
one FakeTCP SYN lineage
  -> temporary reliable ordered bootstrap mode
  -> real TLS 1.3 / Reality-like admission
  -> same 4-tuple and sequence space
  -> DTLS 1.3 / LINK / FEC datagrams
```

No second public SYN is permitted after successful admission, and the product server must not rely on a parallel kernel TCP Reality listener on the WBD public port.

This amendment does **not** authorize steady-state TCP stream delivery. The reliable ordered adapter exists only for bounded TLS setup and is destroyed before no-HOL DTLS/FEC payload mode.

## Decision 3 — release work after the setup-boundary correction

Before platform release work can rely on the corrected architecture, add explicit single-flow gates:

1. capture exactly one public WBD SYN lineage and one 4-tuple through TLS bootstrap and DTLS transition;
2. verify sequence-space continuity and no FIN/RST/new SYN at the mode switch;
3. verify real TLS 1.3/SNI and no plaintext credentials/ticket;
4. verify post-switch later independent datagrams bypass an earlier sequence hole;
5. then repeat OpenWrt TPROXY and Windows TUN one-shot qualification using the same association.

## Pinned upstream boundary

No upstream versions are floated by this amendment:

- wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- udp2raw `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63` as external/raw baseline;
- UDPspeeder `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454` as external FEC baseline;
- quic-go `v0.61.0` (`579ee19`) remains a historical benchmark oracle only.

Any upstream upgrade is a separate workstream.
