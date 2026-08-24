# Roadmap

> **V1 is closed. V2 starts from ADR-0002.** PR #2 / M10-004 remains the rejected multi-ordinary-TCP history. The milestones below are the only authorized mainline work for `dev/wbd-raw-fec-v2`.

The roadmap is gate-based. Do not skip a failed gate by adding more complexity.

| Milestone | Scope | Exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart + new branch/PR/handoff | fresh agent lands on V2 without reading V1 as active work |
| V2-M1 | reproduce pinned one-lane udp2raw + UDPspeeder baseline | exact pinned versions run locally; clean and 1/5/10/15% impairment results reproduce the ~72 ms p99 reference class with hashes/receipts |
| V2-M2 | kernel TCP anchor / real-return-packet feasibility | packet capture proves real handshake/control behavior without payload entering kernel TCP stream, no RST/ACK state conflict, and later raw payload bypasses a lost earlier payload |
| V2-M3 | one-lane V2 harness | OpenWrt/Linux-oriented one-lane raw/FEC service with normal, 1.5x and 2.0x fixed modes; local benchmark and long-run correctness pass |
| V2-M4 | optional two-lane raw symbol striping | same total byte budget; repeatable p95/p99 win vs one lane under independent loss; correlated/burst loss measured; otherwise reject two-lane default |
| V2-M5 | virtual L3 glue | WireGuard or equivalent minimal UDP/IP point-to-point link operates through V2 without custom TUN code |
| V2-M6 | stock Xray composition | stock Xray client/server VLESS + Vision + REALITY works over private V2/WireGuard link; weak-network result does not regress to V1 TCP-HOL behavior |
| V2-M7 | OpenWrt packaging | reproducible package/service, firewall/raw capability setup, reboot persistence, resource profile |
| V2-M8 | Linux endpoint packaging | reproducible privileged Linux client/server packaging and service management |
| V2-M9 | Windows client | Npcap + upstream easy-faketcp-compatible client path, local integration and failure-recovery tests |
| V2-M10 | fixed-mode weak-network qualification | 40–60 ms/0%, ~50 ms/1%, 80–150 ms/~2%, 150–300 ms/10% and 20%, correlated burst, 250–600 ms/~30% or worse |
| V2-M11 | Auto protection admission | only after fixed 1.0/1.5/2.0 modes are real-path qualified; must reduce bytes on good links without harming tail latency on weak links |
| V2-M12 | hardening | multi-hour runs, reconnect, address change, lane failure, CPU/RAM/FD bounds, upgrade/rollback |

## V2-M1 exact first task

Do not start by writing a new protocol.

1. Restore/build the pinned udp2raw `20230206.0` and UDPspeeder `20230206.0` assets already referenced in `deps/oracle-lock.json`.
2. Verify exact SHA-256 and source commits.
3. Run the existing M10-004 reference topology locally as a **V2 product baseline**, not as an oracle.
4. Produce machine-readable results for 0/1/5/10/15% impairment at ~50 ms RTT.
5. Confirm `20:10` and `20:20` behavior before touching lane count, Xray, WireGuard or kernel-anchor code.

## V2-M2 real-return-packet gate

The requested "real TCP return packet" idea is tested separately from FEC so failures are diagnosable.

The test must answer:

- Does the OS own a real TCP anchor 4-tuple after the handshake?
- Which SYN/SYN-ACK/ACK/RST/keepalive packets are kernel-generated vs raw-generated?
- Can raw data-bearing packets use the lane without causing the kernel to reject ACK state?
- Can packet N+1 be delivered when packet N is dropped?
- Is any application payload waiting in the kernel TCP receive/send queue?

If the answer to the last question is yes, the experiment fails because V1 HOL has been reintroduced.

## Two-lane stop rule

Do not assume two lanes are always better. If two independent raw lanes at the same total 1.5x/2.0x byte budget do not improve tail latency under the agreed real fault profiles, keep one lane. No 3/4-lane escalation is authorized without a new benchmark decision.

## Xray stop rule

Do not place stock REALITY/Vision ordinary TCP outside the raw/FEC carrier and call that V2. That topology is allowed only as a negative-control benchmark because it restores the exact ordered carrier assumption rejected by M10-004.

## Platform stop rule

Android is removed. Do not spend cycles on unrooted/mobile portability. Windows support can require Npcap/admin privileges. OpenWrt/Linux privileged raw access is acceptable.
