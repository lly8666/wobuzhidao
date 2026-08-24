# Roadmap

> **Status: HALTED at M10 on 2026-08-24.** The current multi-real-TCP architecture failed the M10-004 FEC architecture gate. Do not advance M11-M19 on this design. Resume only with a new architecture decision that changes a fundamental carrier/product assumption. See `docs/benchmarks/m10-004-fec-no-go.md`.

The roadmap is gate-based. A later milestone is not admitted merely because it is interesting.

| Milestone | Scope | Exit gate |
| --- | --- | --- |
| M0 | repository bootstrap, handoff schemas, benchmark contract | fresh-agent reconstruction + local handoff tests |
| M1 | WBD frame/session codec | deterministic encode/decode, malformed-input tests |
| M2 | STREAM/DATAGRAM semantics | reliable stream and expiring datagram simulator tests |
| M3 | one real TCP lane abstraction | end-to-end bytes/datagrams on local sockets |
| M4 | multi-lane scheduler + reorder/dedup | independent lane reorder/stall tests |
| M5 | logical ACK ranges + GAP hints | gap-driven recovery tests |
| M6 | cross-lane reinjection | measurable HOL reduction vs single TCP |
| M7 | bounded flight + 2-bulk/1-rescue experiment | rescue data avoids bulk queue delay |
| M8 | RBC normal/1.5x/2x/auto framework | bounded multiplier accounting + fast-up/slow-down hysteresis tests |
| M9 | same-budget FEC admission experiment | experimental admit only if repair measurably beats reinjection in repeatable cases |
| M10 | UDP + QUIC oracle benchmark harness | **FAILED architecture gate at M10-004: FEC above multiple ordered TCP carriers did not bypass HOL; current architecture rejected** |
| M11 | adaptive RBC tuning | blocked by M10 no-go |
| M12 | sliding-window FEC research | blocked by M10 no-go |
| M13 | username/password + protected inner session | blocked by M10 no-go |
| M14 | stock REALITY/Vision integration | blocked by M10 no-go |
| M15 | Linux VPN | blocked by M10 no-go |
| M16 | long-duration/fault qualification | blocked by M10 no-go |
| M17 | OpenWrt | blocked by M10 no-go |
| M18 | Windows | blocked by M10 no-go |
| M19 | Android | blocked by M10 no-go |
