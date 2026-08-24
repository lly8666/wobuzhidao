# Roadmap

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
| M10 | UDP + QUIC oracle benchmark harness | reproduce M9 candidates across realistic loss/stall/reorder profiles before any FEC wire commitment |
| M11 | adaptive RBC tuning | fast-up/slow-down without oscillation |
| M12 | sliding-window FEC research | only if M9 data justifies complexity |
| M13 | username/password + protected inner session | authenticated session/lane join |
| M14 | stock REALITY/Vision integration | local full E2E with unchanged carrier stack |
| M15 | Linux VPN | local sandbox qualification |
| M16 | long-duration/fault qualification | restart/stall/memory soak |
| M17 | OpenWrt | router build/profile |
| M18 | Windows | Wintun/service profile |
| M19 | Android | unrooted VpnService profile |
