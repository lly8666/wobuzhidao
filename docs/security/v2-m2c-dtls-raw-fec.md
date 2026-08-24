# V2-M2 — Native DTLS 1.3 over UDPspeeder/FakeTCP qualification

Date: 2026-08-25

## Decision

**PASS.** The one-lane V2.1 security/carrier stack is admitted for the next milestone:

```text
upper UDP
  -> UDPspeeder mode 0 FEC
  -> WBD wolfSSL DTLS 1.3 shim
  -> udp2raw FakeTCP
  -> public path
```

M2A already proved the exact pinned wolfSSL DTLS 1.3 build and native X.509 chain/hostname verification. M2B proved a missing encrypted application record does not block a later record. M2C now proves that the shim composes with the exact M1 raw/FEC carrier without recreating ordered-stream HOL.

## Primary 20:20 matrix

| Loss | delivery | p50 ms | p95 ms | p99 ms | p99 delta vs M1 | security bytes vs M1 FEC | DTLS CPU ms | peak RSS KiB |
|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 0% | 100% | 69.7 | 70.4 | 70.5 | -0.2 | +5.24% | 20 | 27016 |
| 1% | 100% | 69.1 | 71.8 | 71.9 | +1.2 | +5.28% | 10 | 27032 |
| 5% | 100% | 70.2 | 75.9 | 76.2 | +7.4 | +5.41% | 10 | 27080 |
| 10% | 100% | 69.2 | 69.7 | 69.8 | -2.1 | +5.30% | 10 | 27044 |
| 15% | 100% | 69.1 | 81.4 | 82.1 | +6.2 | +5.35% | 20 | 27052 |

All 15 primary cases delivered 200/200 payloads after 64/64 warmup. Loss was applied to encrypted DTLS application datagrams after FEC encoding and before udp2raw.

## Secondary 20:10 regression

| Loss | delivery | p99 ms | p99 delta vs M1 | security bytes vs M1 FEC |
|---:|---:|---:|---:|---:|
| 0% | 100% | 72.3 | -0.6 | +5.45% |
| 1% | 100% | 71.9 | +0.0 | +5.44% |
| 15% | 100% | 79.0 | -0.1 | +5.35% |

All 9 secondary cases delivered 200/200. This is a regression check, not a replacement for the 20:20 primary matrix.

## Outliers and repeat diagnostics

- Primary 20:20 / 5% / seed 260824 reached p99 97.9 ms; the same case repeated at 87.1 ms.
- Primary 20:20 / 15% / seed 260825 reached p99 112.5 ms with 16% of samples above 100 ms; the same deterministic drop pattern repeated at 86.4 ms.
- Secondary 20:10 / clean / seed 260824 reached p99 90.5 ms; repeat was 84.1 ms.

These values are preserved in the committed raw results. Repeats are diagnostics only and do not replace the formal matrix. The variation is consistent with sandbox scheduling sensitivity rather than a deterministic ordered-record stall: delivery stayed complete and repeated identical drop patterns did not reproduce the worst tail.

## Security overhead

Across the primary matrix, median encrypted forward bytes are about 5.2-5.6% above the corresponding M1 UDPspeeder FEC datagram bytes. Product peak RSS was about 27 MiB in this sandbox. CPU counters are coarse (10 ms scheduler ticks); DTLS typically accounted for about 10-20 ms CPU during a 200-payload case.

## Accepted runtime

- wolfSSL: `v5.9.2-stable` / `ac01707f552c611fbd135cc723b2682b3e7f80f2`
- quiet-default shim source SHA-256: `b5b8a1031c045af973b27c18178205f2057330f28e2bc5350ba82a5556d272a1`
- locally tested shim binary SHA-256: `63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a`
- M2C harness SHA-256: `4604c5ed4808738c964d79dd75380aa709ff53d6ac259e58fdb354b4d6306e86`
- `WBD_DTLS_TRACE=1` enables per-record debugging; it is off by default because synchronous trace I/O measurably inflated tail latency.

## Gate result

The M2 security shim is admitted. Continue to V2-M3 native WBD session/control **inside DTLS**. Do not add another cryptographic layer, reliable public byte stream, two lanes, kernel-anchor, TUN, or Auto as part of this transition.
