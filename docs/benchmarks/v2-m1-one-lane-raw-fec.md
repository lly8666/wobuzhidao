# V2-M1 — pinned one-lane raw/FEC baseline qualification

Date: 2026-08-25

## Decision

**PASS.** The exact pinned udp2raw `20230206.0` + UDPspeeder `20230206.0` one-lane classic FakeTCP/FEC baseline was reproduced by local sandbox execution. This closes V2-M1 and admits **V2-M2 native DTLS 1.3** from ADR-0003. It does not admit custom FEC, two lanes, kernel-anchor work, native TUN/session work, or Auto.

## Exact pinned bytes

- udp2raw tag `20230206.0`, commit `e5ecd33ec4c25d499a14213a5d1dbd5d21e0dd63`, embedded git version `e5ecd33ec4`.
- udp2raw amd64 SHA-256 `c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c`; archive SHA-256 `503cf5781aa97e50b4954c6bc4622c3ea6be02f6a35def4bb3b3eaf95bd2c7e8`.
- UDPspeeder tag `20230206.0`, commit `61b24a369700c3d8248dd18fa9a524b778741454`, embedded git version `61b24a3697`.
- speederv2 amd64 SHA-256 `f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a`; archive SHA-256 `b64dd376a948995cb5da17d8eb171338ccd0553b9380e5164f8cb5ac4131bcaa`.

GitHub Actions was used only to relay the exact pinned release bytes and temporary iptables userspace files. Runtime qualification was performed locally on the exact binary hashes above.

## Final local topology and environment

```text
UDP test source
  → UDPspeeder client (mode 0; 20:10 or 20:20)
  → userspace 25 ms one-way delay + forward FEC-shard loss
  → udp2raw client --raw-mode faketcp -a
  → udp2raw server --raw-mode faketcp -a
  → UDPspeeder server
  → UDP echo
```

The host exposes `CAP_NET_RAW` but not host `CAP_NET_ADMIN`. To reproduce classic udp2raw behavior without kernel-RST contamination, the accepted runs executed inside `unshare -Urn`, brought loopback up, and used upstream `-a` RST-suppression rules on **both client and server** via `iptables-nft` inside that namespace. `XTABLES_LIBDIR` was explicitly set for the relayed iptables helper. No easy-faketcp, kernel anchor, alternate sequence mode, or product-code modification was used.

An earlier diagnostic series without complete firewall handling is excluded from qualification. Every accepted case completed a 64/64 serial warmup before the measured run.

## Profile

- Fixed 50 ms RTT: 25 ms each direction in the FEC-shard proxy.
- 200 measured UDP packets × 256 bytes; window 32.
- Seeds `260824`, `260825`, `260826`.
- Forward FEC-shard loss `0 / 1 / 5 / 10 / 15%` before udp2raw encapsulation.
- UDPspeeder mode 0, timeout 8 ms, fixed `20:10` and `20:20`.

## Three-seed median results

| FEC | Loss | p50 | p95 | p99 | Delivery | >100 ms | FEC-forward bytes/source | Product CPU* | Product RSS peak* |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 20:10 | 0% | 68.7 ms | 72.9 ms | 72.9 ms | 100% | 0% | 1.66x | 10 ms | 20.8 MiB |
| 20:10 | 1% | 69.3 ms | 71.8 ms | 71.9 ms | 100% | 0% | 1.67x | 30 ms | 20.8 MiB |
| 20:10 | 5% | 68.9 ms | 77.7 ms | 77.8 ms | 100% | 0% | 1.66x | 20 ms | 20.8 MiB |
| 20:10 | 10% | 68.9 ms | 70.5 ms | 70.5 ms | 100% | 0% | 1.66x | 20 ms | 20.8 MiB |
| 20:10 | 15% | 69.1 ms | 76.6 ms | 76.6 ms | 100% | 0% | 1.67x | 40 ms | 20.8 MiB |
| 20:20 | 0% | 68.7 ms | 70.1 ms | 70.1 ms | 100% | 0% | 2.22x | 40 ms | 20.9 MiB |
| 20:20 | 1% | 69.0 ms | 70.5 ms | 70.5 ms | 100% | 0% | 2.22x | 10 ms | 20.9 MiB |
| 20:20 | 5% | 68.5 ms | 70.6 ms | 70.6 ms | 100% | 0% | 2.22x | 20 ms | 20.9 MiB |
| 20:20 | 10% | 68.9 ms | 70.4 ms | 71.8 ms | 100% | 0% | 2.22x | 30 ms | 20.9 MiB |
| 20:20 | 15% | 68.8 ms | 71.0 ms | 71.0 ms | 100% | 0% | 2.22x | 20 ms | 20.9 MiB |

\* CPU/RSS are the sum of the two udp2raw and two UDPspeeder processes during the 200-packet measurement. CPU is based on `/proc` scheduler ticks and is intentionally treated as a coarse qualification receipt rather than a throughput-capacity benchmark.

## Interpretation

All 30 measured cases delivered 100% of application packets and had zero measured packets above 100 ms. The median p99 range across the ten profile points was roughly 70–78 ms; the worst individual accepted seed was about 92.5 ms. This is consistent with the historical M10-004 ~71 ms reference class and, most importantly, preserves the unordered recovery property that V1 lost to kernel TCP HOL.

`20:10` was sufficient for these independent-loss seeds through 15%; `20:20` spent roughly 2.22x FEC-forward bytes versus roughly 1.66x for `20:10` without a consistent latency advantage on this specific profile. This is evidence for later fixed-mode evaluation only; it is **not** an Auto-policy decision and says nothing yet about correlated/burst loss.

## Reproducibility artifacts

- Harness: `scripts/bench_v2_m1_raw_fec.py`.
- Raw 30-case CSV: `docs/benchmarks/data/v2-m1-one-lane-results.csv`.
- Three-seed medians: `docs/benchmarks/data/v2-m1-one-lane-median.csv`.
- Qualification receipt: `docs/benchmarks/data/v2-m1-one-lane-receipt.json`.
- `results.csv` SHA-256: `3eeb7851c1284d838817f6f3406516568e76469f0319056cc3bb63e2897e15e5`.
- `median.csv` SHA-256: `db22df67515253ee9e1bdc47196bc117e2485751043e92341d9cd61819695027`.

The committed harness was separately smoke-tested locally after the final client/server `-a` correction: `20:20 / 1% / seed 260824` delivered 200/200 with p99 about 71.8 ms.

## Exit gate

**V2-M1 is complete.** The next authorized main-path step is V2-M2 from ADR-0003: locally qualify pinned wolfSSL `v5.9.2-stable` / `ac01707f552c611fbd135cc723b2682b3e7f80f2`, then build a one-lane DTLS 1.3 shim between UDPspeeder and udp2raw using a real test X.509 chain and strict hostname verification. The first DTLS benchmark must measure incremental security overhead and verify that losing one DTLS application record does not block later source/repair records.

Kernel-anchor/real-return-packet work remains a later independent milestone and must not be mixed into V2-M2.
