# Roadmap

> **Status: V2.2 ACTIVE.** Core transport: packet/datagram → FEC → DTLS 1.3 → TCP-shaped FakeTCP. TUN/platform qualification is temporarily deferred while the transport-only `20:20` matrix is characterized.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE**; exact one-lane raw/FEC baseline qualified |
| V2-M2 | native DTLS 1.3 security shim | **DONE**; pinned wolfSSL DTLS 1.3 + X.509/hostname validation qualified |
| V2-M3 | minimal native session/control | **DONE**; framing/auth/liveness/stats/close/reconnect/fixed protection config qualified |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED**; historical evidence only |
| V2-M5 | optional two raw lanes | **DEFERRED** |
| V2-M6 | Linux/OpenWrt native L3/TUN core | **M6A IMPLEMENTED; M6B DEFERRED TO EXTERNAL REAL-DEVICE TESTING** |
| V2-M7 | Windows client | **DEFERRED UNTIL PLATFORM TEST PHASE** |
| V2-M8A | optional TLS Persona bootstrap | admitted, implementation later |
| V2-M8B-T | transport-only fixed-20:20 characterization | **CURRENT**; RTT/loss/TCP-like/UDP-like/CPU/RSS matrix |
| V2-M8B-P | platform hardening | later TUN/OpenWrt/Linux/Windows/MTU/routing/device-resource tests |
| V2-M9 | adaptive protection research | only if measurements justify it |
| V2-M10 | release qualification | final platform + security + transport regression |

## V2-M8B-T current gate

The immediate goal is to characterize the already-qualified one-lane stack **without TUN**:

```text
application datagrams
  → UDPspeeder mode0 20:20
  → DTLS 1.3
  → udp2raw FakeTCP
  → impaired underlay
  → reverse stack
```

The first matrix is frozen to reduce test volume:

- RTT: `20, 50, 100, 200, 400, 600 ms`;
- symmetric random loss per direction: `0, 1, 5, 10, 20, 30, 40%`;
- FEC: mode 0 `20:20` only;
- seeds: at least three;
- each case starts fresh namespaces and fresh FakeTCP/DTLS/FEC state;
- network impairment exists before FakeTCP/DTLS establishment.

Nominal full matrix: `6 × 7 × 3 = 126` cases.

### Required result columns

Connection:

- FakeTCP/DTLS establishment pass/fail;
- ready/handshake elapsed time;
- failure stage/reason.

UDP-like data behavior:

- sent/delivered/delivery ratio;
- p50/p95/p99/max RTT;
- goodput;
- out-of-order completion;
- later-datagram bypass count/rate as no-HOL evidence.

TCP-like outer behavior:

- TCP-shaped packet counts by direction;
- SYN/SYN-ACK/ACK/PSH/FIN/RST flags;
- duplicates;
- sequence/ACK progression diagnostics;
- RST/control-loop anomalies.

Resources:

- CPU time per udp2raw/UDPspeeder/DTLS process;
- combined CPU time;
- CPU ms per delivered MiB;
- peak RSS per process/component;
- combined peak RSS;
- transmitted bytes / delivered payload bytes.

`40%` loss is a stress/cliff-finding point and may legitimately fail. It is not a release promise.

### Decision rule after first matrix

Do **not** immediately start another full factorial sweep.

1. identify handshake failure cliff;
2. identify delivery/p99 cliff;
3. identify CPU-per-MiB and RSS growth regions;
4. inspect whether outer FakeTCP state becomes abnormal;
5. inspect whether later datagrams still bypass earlier lost/delayed datagrams;
6. only around informative boundaries add targeted follow-up cases.

Possible targeted follow-ups include `20:10`, burst loss, MTU or UDPspeeder timing, but only where the first surface shows value.

## M6 platform state

M6A packet-preserving WBDP/TUN code and the M6B privileged real-TUN harness remain in-tree. They are not deleted. Actual TUN/OpenWrt qualification may be performed later by external real-device testers and must produce repository-backed receipts.

## TLS Persona admission rule

Persona remains optional and separate from steady-state transport. Initial config names remain `off/native/chrome/firefox/safari/edge`; randomized is not admitted yet.

## Removed/deferred work

- ordinary kernel TCP as product data carrier;
- kernel-anchor integration;
- Xray/VLESS/Vision as the VPN data plane;
- WireGuard inner glue;
- Android/no-root;
- multi-lane until measurements justify it.
