# Roadmap

> **Status: V2.2 ACTIVE.** Core goal: OpenWrt/Linux ↔ Linux/Windows personal VPN using TUN → WBD packets → FEC → DTLS 1.3 → TCP-shaped FakeTCP. Optional TLS Persona is isolated from the steady-state datagram data plane.

The roadmap is gate-based. Completed evidence is preserved; optional research cannot block the core VPN.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; exact pinned one-lane raw/FEC baseline locally qualified |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; pinned wolfSSL DTLS 1.3, X.509/hostname validation and weak-network behavior qualified |
| V2-M3 | minimal native session/control | **DONE**; framing/auth/liveness/stats/close/reconnect/fixed protection config qualified |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** by product scope clarification; historical evidence only, no further work |
| V2-M5 | optional two raw lanes | **DEFERRED**; cannot block one-lane product path |
| V2-M6 | Linux/OpenWrt native L3/TUN core | **CURRENT**; real IPv4/IPv6 packets cross a packet-preserving WBD datagram adapter and then the qualified one-lane FEC/DTLS/FakeTCP composition |
| V2-M7 | Windows client | required Npcap/easy-faketcp-compatible raw path + Wintun/equivalent interoperates with OpenWrt/Linux server |
| V2-M8A | optional TLS Persona bootstrap | real TLS 1.3 preflight with explicit `off/native/browser-profile` policy, operator-controlled cert verification, bounded bootstrap binding, fingerprint/fragmentation tests |
| V2-M8B | product hardening + large parameter sweep | long-duration/fault/MTU/RTT/loss/burst matrix; select measured defaults for fixed modes and timers |
| V2-M9 | adaptive protection research | Auto admitted only if M8B measurements justify it; otherwise remain fixed-mode |
| V2-M10 | release qualification | OpenWrt/Linux ↔ Linux/Windows end-to-end regression with security, MTU, routing, reconnect, performance and optional Persona cases |

## M6 current gate

M6 is deliberately split so the project gets a usable data plane before more optional features.

### M6A — packet-preserving TUN adapter

- one TUN read equals one WBD IP datagram;
- one decoded WBD IP datagram equals one TUN write;
- exact length validation;
- IPv4 and IPv6 accepted;
- bounded MTU;
- counters for packets/bytes/drops/errors;
- no stream reassembly.

### M6B — Linux/OpenWrt integration

- create/configure TUN with privileged product path;
- connect the packet adapter to the existing local UDPspeeder → DTLS shim → udp2raw composition;
- verify bidirectional ICMP/UDP and representative TCP-over-the-VPN traffic;
- routing and MTU documented;
- reconnect does not corrupt packet boundaries.

### M6C — impairment qualification

Run real packet traffic through the complete one-lane path at 0/1/5/10/15% plus burst loss and at several RTTs. Record p50/p95/p99/max, delivery, bytes, CPU/RAM and FEC recovery.

## TLS Persona admission rule

TLS Persona is optional and must not block M6/M7.

Initial policy values:

- `off`
- `native`
- `chrome`
- `firefox`
- `safari`
- `edge`
- later `randomized` only after explicit qualification

The implementation should use a maintained uTLS-style library rather than hand-encoding browser ClientHello bytes.

Qualification must include:

- standard TLS 1.3 handshake success;
- operator-controlled chain + hostname validation;
- negotiated ALPN;
- ClientHello total bytes and TCP segment count;
- MTU-sensitive fragmentation;
- repeated handshake latency/failure rate;
- fail-closed behavior when Persona is required.

The bootstrap remains separate from the FakeTCP/DTLS data lane.

## Large test / optimization rule

Do not optimize from intuition. After M6 + M7 core interoperability:

1. establish one immutable default baseline;
2. sweep one parameter family at a time;
3. use multiple seeds/runs;
4. compare same payload and intentional byte budget;
5. reject changes that improve mean latency while materially worsening p99, delivery, CPU/RAM or burst-loss behavior;
6. store machine-readable receipts.

Primary tuning targets:

- `normal`, `20:10`, `20:20`;
- UDPspeeder grouping/timing parameters that are actually exposed by the pinned version;
- TUN MTU;
- reconnect/backoff/keepalive;
- DTLS record sizing only when standards-compliant and supported by the library;
- Persona profile and handshake size/fragmentation.

## Removed/deferred work

- ordinary kernel TCP as product data carrier;
- kernel-anchor product integration;
- Xray/VLESS/Vision as the VPN data plane;
- WireGuard inner glue;
- Android/no-root;
- multi-lane unless measurements justify it.
