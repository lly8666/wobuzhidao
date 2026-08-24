# Roadmap

> **Status: V2.1 ACTIVE.** V1 multi-ordinary-TCP is permanently rejected. ADR-0002 changed the carrier to unordered FakeTCP/FEC. ADR-0003 removes Xray/WireGuard and makes WBD itself the DTLS 1.3-secured VPN/session implementation.

The roadmap is gate-based. Later work is not admitted merely because it is attractive.

| Milestone | Scope | Exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | PR #2 frozen as rejected evidence; PR #3 + handoff point to V2 |
| V2-M1 | exact one-lane udp2raw `20230206.0` + UDPspeeder `20230206.0` product baseline | local exact-hash reproduction; ~50 ms RTT over 0/1/5/10/15% impairment; 20:10 and 20:20 where practical; p50/p95/p99/delivery/bytes/CPU recorded |
| V2-M2 | native WBD DTLS 1.3 security shim on one raw lane | pinned wolfSSL DTLS 1.3 build locally qualified; real X.509 server cert + hostname validation; UDPspeeder source/repair datagrams each carried as DTLS application data; 0/1/5/10/15% results preserve the M1 weak-network result class with bounded security overhead |
| V2-M3 | native WBD session/control inside DTLS | version/config framing, optional username/password/token after Finished, keepalive, session stats and reconnect work without custom crypto; malformed/auth tests |
| V2-M4 | kernel-anchor / real-return-packet experiment | packet capture proves or rejects real OS handshake/control + raw payload coexistence with no kernel payload HOL/retransmission dependency; fallback to classic udp2raw remains valid |
| V2-M5 | optional two raw lanes, one DTLS association per lane | same-total-byte-budget comparison beats one secured lane repeatably in at least one justified impairment family and does not regress correlated-loss cases enough to invalidate admission |
| V2-M6 | Linux/OpenWrt native L3/TUN VPN path | real IP packets cross WBD DTLS/FEC/FakeTCP path; routing, MTU, DNS and reconnect qualification; no Xray/WireGuard dependency |
| V2-M7 | Windows client | Npcap/easy-faketcp + native DTLS/FEC + Wintun/equivalent integration passes interoperability with OpenWrt/Linux server |
| V2-M8 | fixed-mode product hardening | `normal`, `weak-1.5x`, `weak-2x` configuration, resource bounds, long-duration/fault tests |
| V2-M9 | adaptive protection research | Auto admitted only if real measurements justify it; total intentional source/repair bytes remain <=2.0x unless constitution changes |
| V2-M10 | release qualification | normal, 1%, 2%, 150–300 ms/10–20%, correlated burst, and 250–600 ms/~30% profiles plus CPU/RAM/MTU/route/security regression |

## V2-M1 immediate rule

Do not start DTLS implementation until the exact pinned raw/FEC baseline is reproduced locally. Temporary GitHub Actions relay helpers may fetch/build bytes, but Actions PASS is not runtime qualification.

## V2-M2 security gate

The DTLS milestone must prove **real protocol use**, not appearance:

- DTLS 1.3 handshake completes over the FakeTCP-provided datagram path;
- server presents an operator-controlled real certificate;
- client verifies CA chain and expected hostname;
- product mode rejects invalid/expired/mismatched certificates;
- 0-RTT is disabled initially;
- after Finished, all source/repair datagrams are DTLS application data records;
- no switch to custom encryption and no detector-specific TLS/browser fingerprint shaping;
- FEC repair remains independent: loss of record N does not prevent later record N+1 or a repair record from being verified/decrypted.

Initial implementation pin is recorded in `deps/security-lock.json`.

## Two-lane admission rule

A second lane is not "more redundancy for free". At the same overall source+repair budget, compare:

- one lane + one DTLS association;
- two raw lanes + two independent DTLS associations + one shared FEC decoder.

Test independent loss, correlated loss and burst loss. If two lanes do not produce repeatable p95/p99 benefit, keep one lane as the product default.

## Removed roadmap items

The following are explicitly removed from V2.1:

- Xray/VLESS/Vision/REALITY integration;
- WireGuard composition;
- Android/no-root support;
- multi-ordinary-TCP carrier work;
- V1 RBC/reinjection/rescue-lane development.
