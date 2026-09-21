# P5 fixed-FEC 20:20 weaknet candidate

- Date: 2026-09-21
- Branch: `next/tlslike-dataplane`
- Authority before write: `849222ed232ed71b034c787f76abb3d17e49e073`
- Previous 40 Mbps closure regression: Actions `35576437098`, 22/22 PASS.

## Why this atom exists

P3 already qualifies fixed 20:20 correctness as one supported profile, and P5 already has a lossless fixed-FEC 20:10 capability/cost point. There has not been a P5 fixed-FEC 20:20 qualification under the prescribed 300 ms weak-network profiles.

This atom answers that question without turning 20:20 into a production recommendation.

## Jobs

Four independent Actions jobs are added. Every job runs one scenario, one deterministic seed and one raw artifact only:

- 5% -> 20% -> 5%, seed 20260921
- 5% -> 20% -> 5%, seed 20260922
- 5% -> 30% -> 5%, seed 20260923
- 5% -> 30% -> 5%, seed 20260924

These are intentionally the same seeds used by the existing FEC-off weaknet qualification, so the scenario/drop generator is as comparable as practical. FEC changes the emitted packet sequence, so packet identities are not treated as paired samples.

## Fixed profile

- FEC: fixed 20:20
- runtime defaults: 8 ms flush, 8 max blocks
- production default remains FEC off
- padding remains off
- no FEC profile sweep or adaptive policy

## Multiple checks from each single run

Each run preserves the existing raw PCAP/events/network/requests/resources evidence and one independent validator computes:

- deterministic drop provenance and actual phase drop ratios,
- application success/loss/duplicate/integrity,
- RTT p50/p95/p99,
- final-5% third-consecutive-success recovery,
- phase PPS,
- injection queue residency p50/p95/p99/max and peak depth,
- host aggregate/per-core CPU, memory and GC,
- FEC source/parity shards, full/partial blocks and settled pending sources,
- decoder reconstruction events and recovered sources,
- recovery-expired missing sources,
- transport repair retransmits,
- wire amplification,
- capture completeness.

A run must actually exercise FEC reconstruction; source/parity inventories must be nonzero and settled. Application loss in finite random loss is reported rather than hard-required to zero, matching the existing P5 weaknet acceptance rule.

CPU/PPS/memory are observational within each VM and are not cross-VM absolute gates.
