# P5 31 minute soak completed; validator schema repair

- Date: 2026-09-21
- Source SHA: `bb9c7ed10ce4a5e1af3db24ba18c6aa92f47f368`
- Actions: `35588122605`
- Scope: P5 soak only
- Product FEC: off
- Padding: off

## Product run result

Both independent soak jobs completed the full 1860 second measurement window.

Run 1, seed 20260935:
- HTTPS: 186/186 success
- application loss: 0
- duplicates: 0
- RTT p50/p95/p99/max: 602.134 / 1112.581 / 1113.499 / 1114.040 ms
- client/server generation advances: 58 / 58
- lifecycle errors: 0
- deterministic drop: C2S 1949/38965 = 5.0019%, S2C 1993/39864 = 4.9995%
- aggregate packet attempt rate: 42.381 pps
- injection queue p99: C2S 301.069 ms, S2C 301.068 ms
- final client/server TCP flows: 0 / 0
- final client/server BusinessFlows: 0 / 0
- final lanes: 2 active / 2 physical / 0 retiring on both endpoints
- memory plateau middle minutes 10..19 floor median: 16.598 MiB
- memory plateau late minutes 21..30 floor median: 16.383 MiB
- host CPU busy observation: 0.5455%
- max HeapAlloc/HeapInuse/Sys: 33.80 / 35.78 / 50.15 MiB
- GC delta: 98; pause delta 5.93 ms
- inner TLS rate: 0.014588 Mbps
- app goodput: 0.011196 Mbps
- outer wire rate: 0.137615 Mbps
- wire amplification: 9.433638x
- PCAP packets == outer events: 78859
- artifact: 10634387015
- digest: sha256:5749a86d7cddbc45da901a81b04c053e0cb40bc56f38c2eb0ca8dbae3c718901

Run 2, seed 20260936:
- HTTPS: 186/186 success
- application loss: 0
- duplicates: 0
- RTT p50/p95/p99/max: 602.329 / 1112.021 / 1113.229 / 1113.282 ms
- client/server generation advances: 58 / 58
- lifecycle errors: 0
- deterministic drop: C2S 1925/38503 = 4.9996%, S2C 1971/39411 = 5.0011%
- aggregate packet attempt rate: 41.889 pps
- injection queue p99: C2S 301.076 ms, S2C 301.084 ms
- final client/server TCP flows: 0 / 0
- final client/server BusinessFlows: 0 / 0
- final lanes: 2 active / 2 physical / 0 retiring on both endpoints
- memory plateau middle minutes 10..19 floor median: 16.773 MiB
- memory plateau late minutes 21..30 floor median: 16.227 MiB
- host CPU busy observation: 0.1828%
- max HeapAlloc/HeapInuse/Sys: 33.38 / 35.45 / 49.83 MiB
- GC delta: 94; pause delta 7.32 ms
- inner TLS rate: 0.014588 Mbps
- app goodput: 0.011196 Mbps
- outer wire rate: 0.137490 Mbps
- wire amplification: 9.424840x
- PCAP packets == outer events: 77944
- artifact: 10634426658
- digest: sha256:3f983ad7961a753b79e63d9c913f757a5819538288fa85966caf0969f7de09d4

All hard behavioral invariants were independently recomputed from the raw artifacts and passed, including deterministic drop provenance, non-dropped delivery completeness, 300 ms minimum queue residency, 186 distinct per-slot BusinessFlow IDs, final retirement/convergence, and memory plateau.

## Why the two soak jobs were red

The manifest used the project-wide schema `wbd-p5-https-measurement/v1`, via `p5MeasurementSchema`.

The new validator accidentally hard-coded `wbd-p5-measurement-v1`. Source SHA and harness SHA were both exact and correct. Therefore both jobs failed only at the first provenance condition:

`WBD_P5_SOAK_FAIL manifest schema/source/harness SHA`

The fix changes only the validator schema constant to the existing project schema. It does not change product code, workload, timing, loss, rotation, retirement, memory plateau, or any acceptance threshold.

## Separate fixed-FEC base red light

The same exact-SHA workflow initially failed the existing lossless fixed-FEC 20:10 measurement because the server decoder reported one reconstruction/recovered source. The exact same job was rerun at the same SHA after the workflow completed and passed as job `106313094093`.

This is recorded as a hosted timing intermittent. The lossless FEC qualification remains strict: reconstruction/recovered must be zero. No FEC assertion or product behavior was changed.
