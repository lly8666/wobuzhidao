# ADR-0010: Freeze V2.2 transport semantics at the 40 Mbit/s release operating point

Status: **ACCEPTED / PROTOCOL FROZEN FOR PLATFORM INTEGRATION** (2026-08-26)

## Context

V2.2 has completed the shared-account two-session transport/session fan-out and the corrected 100 Mbit/s shared-link capacity sweep required by the live handoff. The product priority remains earliest-complete inner datagram delivery with balanced independent LiveID sessions, not maximizing offered throughput at the cost of persistent queueing.

The corrected `mux-load-100m` workflow introduced at commit `71501859c6fc1aa1a6d1a6b048af5aebcf984732` ran cleanly on branch `dev/wbd-raw-fec-v2` at head `a3d8b05a875b2880c69cb4d6bada967eef8c17f9` as GitHub Actions run `32920925944`. Both `bench (20)` and `bench (100)` plus `aggregate` completed successfully and produced the filesystem-safe RTT and aggregate artifacts.

The fixed `20:20` / `legacy` results show a clear latency boundary:

| RTT | Aggregate inner offered | Delivery | Goodput | One-way p50 | One-way p99 |
|---:|---:|---:|---:|---:|---:|
| 20 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 12.940 ms | 21.768 ms |
| 20 ms | 60 Mbit/s | 0.7742 | 46.451 Mbit/s | 253.253 ms | 286.604 ms |
| 20 ms | 80 Mbit/s | 0.7587 | 60.699 Mbit/s | 619.032 ms | 956.262 ms |
| 100 ms | 40 Mbit/s | **1.0000** | **39.994 Mbit/s** | 52.152 ms | 60.711 ms |
| 100 ms | 60 Mbit/s | **1.0000** | **60.000 Mbit/s** | 137.377 ms | 210.995 ms |
| 100 ms | 80 Mbit/s | **1.0000** | **80.000 Mbit/s** | 538.317 ms | 913.954 ms |

The 60 Mbit/s point therefore already enters hundreds-of-milliseconds queueing at both RTTs, and the 80 Mbit/s point approaches one second of p99. This reproduces the cliff seen in the earlier diagnostic run while removing the earlier artifact-upload defect.

A 50 Mbit/s midpoint is optional research, not a release prerequisite. The current product does not have a requirement that justifies spending latency margin or reopening transport decisions merely to raise the qualification number above 40 Mbit/s.

## Decision 1 — release operating point

The conservative V2.2 release operating point on the current 100 Mbit/s weak shared-link qualification boundary is frozen at:

- **40 Mbit/s aggregate inner offered payload** for the qualified two-session fixed-`20:20` path;
- fixed systematic `20:20` tail-RS where FEC is enabled;
- `legacy` FakeTCP shadow recovery as the product default;
- setup loss 0% and measurement loss 20% for this loaded qualification surface.

Do not promote 50/60/80 Mbit/s into the release cap without a separate future benchmark decision. A future midpoint experiment must not delay platform release work.

## Decision 2 — protocol freeze boundary

Platform integration must treat the following as frozen unless a later explicit ADR reopens them with regression evidence:

- WBD-owned TCP-shaped raw FakeTCP carrier; no ordinary kernel TCP payload byte stream;
- UDP/datagram-like earliest-complete inner semantics;
- FEC off or the currently qualified fixed systematic `20:20` profile; no in-place runtime FEC epoch change;
- FEC shard -> independent DTLS 1.3 datagram -> FakeTCP ordering;
- pinned wolfSSL DTLS 1.3 implementation and its existing product verification policy;
- Reality-like TLS front as setup/admission only, never the sustained data plane;
- shared username/password admission issuing independent one-time tickets/LiveIDs;
- atomic ticket claim, peer/LiveID data demux, immutable per-LiveID `linkdata.Path` state;
- one public FakeTCP listener with independent raw associations and one DTLS worker per association for V2.2;
- `legacy` FakeTCP recovery default; `sack-rack` remains experimental;
- one raw lane as the release baseline.

OpenWrt or Windows routing convenience is not sufficient reason to alter these semantics.

## Decision 3 — next release work

Transport-capacity exploration stops here for the release path. Continue in roadmap order:

1. complete one clean OpenWrt **TPROXY + policy routing + underlay escape** end-to-end VPN integration using the frozen WBD association;
2. prove real DNS plus TCP and UDP application traffic traverses the association from clean routing/firewall state and that cleanup restores state;
3. complete one clean Windows **TUN/Wintun-class** end-to-end integration with explicit underlay endpoint escape and cleanup;
4. make those one-shot sequences release regressions.

The existing `scripts/openwrt_tproxy.sh` rule installer is infrastructure only; its own contract explicitly leaves the user-space transparent TCP/UDP adapter as the remaining platform-integration step.

## Pinned upstream boundary

No upstream versions are floated by this freeze. Current machine-readable locks remain authoritative:

- wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- udp2raw `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63` as the pinned external/raw baseline;
- UDPspeeder `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454` as the pinned external FEC baseline;
- quic-go `v0.61.0` (`579ee19`) remains a historical benchmark oracle only.

Any upstream upgrade is a separate workstream and does not occur incidentally during platform integration.
