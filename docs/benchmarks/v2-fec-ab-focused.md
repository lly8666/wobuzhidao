# Focused WBD FEC vs UDPspeeder 20:20 A/B

Date: 2026-08-25

Purpose: compare the first packet-preserving WBD-owned systematic 20+20 FEC candidate against the pinned UDPspeeder mode0 20:20 oracle without changing the qualified DTLS 1.3 or udp2raw-compatible FakeTCP layers.

## Fixed comparison stack

Variant A:

`probe -> UDPspeeder mode0 20:20 -> pinned wolfSSL DTLS 1.3 -> pinned udp2raw FakeTCP`

Variant B:

`probe -> WBD systematic GF(256) 20+20 -> same pinned wolfSSL DTLS 1.3 -> same pinned udp2raw FakeTCP`

Each case creates fresh namespaces and fresh FEC/DTLS/FakeTCP state. Impairment is installed before FakeTCP/DTLS establishment.

## Focused surface

- public link: 200 Mbit/s per direction
- one-way inner bulk offer: 75 Mbit/s
- bulk active-send duration: 2 s
- packet size: 1200 bytes
- RTT: 20 / 100 / 300 / 600 ms
- symmetric independent random loss per direction: 0 / 1 / 5 / 10 / 20%
- 20 paired network points, 40 transport cases total

The hosted Ubuntu 24.04 `tc netem` build does not accept an explicit random-loss seed token, so this focused pass records the requested seed but marks it unapplied. It is a first paired diagnostic surface, not a deterministic release qualification.

## Corrected measurement split

The previous 200-Mbit request/echo probe mixed an RTT-dependent post-send drain with a fixed 2-second throughput/CPU denominator. This A/B does not use that cross-RTT accounting.

Bulk throughput is one-way with a fixed active-send denominator. End-to-end process CPU is divided by the actual measured case wall time. Latency is a separate low-rate request/echo probe, so queue saturation from the bulk phase does not redefine the latency measurement window.

## Required comparison outputs

For each paired RTT/loss point record:

- FakeTCP + DTLS establishment outcome;
- one-way bulk delivery ratio and delivered Mbit/s;
- separate low-rate p50/p95/p99 latency;
- FEC-only CPU delta and peak RSS;
- whole userspace stack CPU delta and peak RSS;
- client/server resource split;
- outer qdisc bytes and drops.

UDPspeeder remains the oracle baseline until WBD is at least competitive on delivery/tail latency and materially better on resource use. This focused pass does not authorize removal of UDPspeeder by itself.
