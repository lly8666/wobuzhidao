# ADR-0005: Fixed FEC scheduling, deferred Auto, shared-account multi-session, split routing, and optional dual lane

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (updated 2026-08-26)

ADR-0006 is authoritative for link-parameter timing: V2.2 uses immutable per-association `LINK_INIT/LINK_ACCEPT`; there is no runtime config epoch or mid-session FEC switching. ADR-0008 is authoritative for the Reality-like front and shared username/password admission.

## Context

Focused first-arrival testing changed the FEC latency picture. The original WBD 20+20 block encoder waited for a full block (or flush timer) before emitting source and repair shards. Changing the fast path to emit each systematic source immediately reduced full-stack first-complete latency at 20% random loss from roughly 15.2 ms to 10.4 ms p50 at 20 ms RTT and from roughly 55.7 ms to 50.4 ms at 100 ms RTT, while still delivering 800/800 datagrams.

The remaining latency cost for a lost systematic source comes from waiting for enough repair equations. Fixed 20+20 tail parity is therefore a strong-loss reference, not a universal optimum.

Current requirements include fixed FEC that may be disabled, a 100 Mbit/s weak-link ceiling, client-owned routing choices, multiple simultaneous sessions/devices using one shared personal account, global/China/non-China capture, a Reality-like connection front, OpenWrt TPROXY and Windows TUN/Wintun-class capture. **Auto FEC remains future advanced research.**

## Decision 1 — optimize FEC for first-complete datagram time

For independent one-way packet loss `p`, systematic source count `K`, and repair count `R`, an ideal systematic MDS `(K+R,K)` block fails when more than `R` transmitted shards are lost:

```text
P_fail(K,R,p)
  = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l).
```

For `K=20`, minimum `R` for representative iid block-failure targets is:

| random loss p | R for P_fail <= 1e-3 | overhead | R for P_fail <= 1e-5 | overhead |
| ---: | ---: | ---: | ---: | ---: |
| 1% | 3 | 1.15x | 4 | 1.20x |
| 5% | 6 | 1.30x | 8 | 1.40x |
| 10% | 9 | 1.45x | 12 | 1.60x |
| 15% | 12 | 1.60x | 16 | 1.80x |
| 20% | 15 | 1.75x | 20 | 2.00x |

These are planning bounds, not release promises. Burst loss, queueing, MTU, coding causality and implementation cost require measurement.

### Scheduling result

A source that is available should be transmitted immediately. Deliberately waiting for a block cannot improve that source's first arrival.

For a source whose systematic transmission is lost, repairs should become available as early and evenly as causality permits. With source spacing `Delta` and repair/source ratio `alpha=R/K`, an evenly spread repair cadence has mean residual wait on the order of:

```text
E[T_next_repair] ~= Delta / (2 alpha)
```

A simple repair-debt lower bound is:

```text
alpha(1-p) > p
alpha > p/(1-p)
```

This is necessary, not sufficient, for p95/p99 recovery targets.

With payload offered load `B` and path capacity `C`, first-order utilization is:

```text
rho = B(1+alpha)/C
```

As `rho` approaches or exceeds one, repair backlog and serialization delay can erase recovery gains. Therefore fixed profiles are qualified against both loss and capacity pressure. Current release-critical capacity qualification uses `C <= 100 Mbit/s`.

## Decision 2 — compare fixed schedulers before changing live codec

The current WBD GF(256) systematic 20+20 Reed-Solomon path remains the qualified live fixed codec.

Research compares:

- current K-block systematic RS with tail repairs;
- smaller/micro-block systematic RS;
- causal/sliding-window systematic linear repair with earlier repair equations.

`internal/fec/simulator.go` is an offline qualification tool, not an Auto controller and not a declaration of live support. No research scheduler replaces the live codec until it wins first-arrival tail, delivery, CPU/RSS and wire-efficiency gates under iid, burst and 100 Mbit/s capacity stress.

## Decision 3 — current product FEC is `off | fixed`

The conceptual client configuration is:

```text
fec.mode = off | fixed
fec.k
fec.r
fec.flush
fec.scheduler
```

`off` sends no proactive repair. DTLS and FakeTCP shadow retransmission remain active.

`fixed` selects a profile supported by the live implementation. Compatibility names remain:

- `weak-1.5x` = `20:10` reference;
- `weak-2x` = `20:20` reference.

At present only WBD live `20:20` tail-RS and FEC off are admitted. `20:10`, micro and causal values remain reference/research until implemented and qualified.

`auto` remains reserved future advanced research and is rejected in the current product path.

Per ADR-0006, the fixed/off choice and all link-defining parameters are proposed once in `LINK_INIT`, accepted exactly or rejected by the server, and then remain immutable until reconnect.

## Decision 4 — one shared account may own multiple simultaneous device sessions

WBD is a personal server, not a multi-tenant identity platform. Admission is deliberately simpler than the earlier per-device-token proposal.

- one configured `username/password` pair identifies the shared account;
- the same pair may create multiple simultaneous sessions/devices;
- the credentials are sent once inside a recognized TLS 1.3 front connection, where TLS already supplies encryption and integrity;
- server recognition uses bounded constant-time equality checks only; there is no required application-layer password KDF/challenge protocol for the personal path;
- every successful login receives a fresh random one-time ticket;
- live state is keyed by the independent ticket/session identity, never by username alone;
- a simple overall resource limit such as `max-conns` may bound simultaneous work;
- no per-device credential database, revocation table, device-token rotation or username-based single-session lock is required.

The ticket is consumed by the later DTLS/WBD association. Once a front ticket is accepted, the data association does not repeat a second bearer `AUTH` exchange.

## Decision 5 — routing/capture policy is client-side and must exclude underlay

Client capture modes are:

```text
capture.mode = off | global | only-cn | only-non-cn
```

Every full/split mode has an **underlay escape invariant**: WBD server endpoint(s), Reality-like bootstrap endpoint(s), and required local-link traffic continue through the original physical/default route and never recurse into the VPN capture path.

### OpenWrt

The final OpenWrt product uses **TPROXY plus policy routing**, not a TUN device. Selected TCP/UDP traffic is redirected to a local transparent adapter. Packet marks and dedicated route tables return marked traffic locally while explicit WBD underlay destinations are exempted before broad capture.

Prefer compact nftables interval sets/ipsets for `only-cn` and `only-non-cn`. Do not create one firewall rule per China prefix. Installation and cleanup must be idempotent and WBD-owned state must be removable after failed startup.

The existing Linux TUN implementation remains a protocol/regression harness and a Linux experiment path; it does not satisfy the OpenWrt release gate.

### Windows

Use a **TUN/Wintun-class L3 adapter**. Global capture uses broad tunnel routes with explicit `/32` and `/128` endpoint escape routes through the original gateway.

For `only-cn` / `only-non-cn`, compare compact aggregated routes with a small WFP/equivalent interception layer backed by user-space longest-prefix classification. Do not install thousands of persistent Windows Firewall rules.

CIDR membership is longest-prefix matching, not exact-address hashing. The domestic prefix database is versioned, atomically replaced, and supports IPv4 and IPv6.

## Decision 6 — Reality-like front appearance is client-selected; certificate verification is optional in personal mode

The client selects `persona = off | native | chrome | firefox | safari | edge` from the supported set where implemented. Browser-profile work affects only connection establishment; it does not replace steady-state DTLS/FakeTCP data transport.

The preferred product join is ADR-0008's same-entry front: one ClientHello is classified, recognized traffic is locally taken over on the same TCP socket, and unrecognized traffic continues byte-for-byte to the fixed fallback target.

The personal client may explicitly set server certificate/hostname verification off. A configured SNI can therefore be used with an unrelated self-signed WBD certificate. This gives encrypted TLS records without server certificate identity authentication and is an intentional personal-use setting. It must be visible in logs/configuration rather than happening silently.

WBD may use public speed-test sites as **measurement baselines** when studying network treatment. It does not need those services' private keys and does not route sustained VPN data through them.

The older target-mirror/witness diagnostic remains a compatibility experiment, not the preferred connection join.

## Decision 7 — dual lane is an optional survival mode, not default

Two lanes are not developed as `duplicate everything twice` by default. Shared bottlenecks or correlated loss can make blind 2-4x traffic wasteful.

Future modes may include:

- `striped`: sources and independent repairs distributed across lanes;
- `hedged`: selected second-lane duplicates/repairs;
- `survival`: explicit emergency source duplication plus independent repair, potentially above 2x and requiring separate qualification.

Two lanes do not require unrelated FEC algorithms. They require independent lane IDs, sequence spaces, coding equations/seeds/window phases and schedules so repair traffic is complementary rather than byte-identical duplication.

## Configuration ownership summary

Client/session-owned at establishment: capture mode, fixed FEC profile, directional preferences, optional Persona profile, optional future lane mode.

Server/operator-owned: one shared username/password, simple overall connection/resource limits, local certificate/private key, fallback target, supported protocol/code versions, and hard memory/CPU/wire/MTU/lane ceilings.

Negotiated exactly once: immutable LinkConfig for the new association. Unsupported proposals are rejected rather than silently rewritten.

## Consequences / implementation order

1. Finish 100 Mbit/s FakeTCP/FEC/DTLS/recovery qualification and freeze the transport semantics.
2. Keep immutable `LINK_INIT/LINK_ACCEPT`; no runtime config epochs.
3. Finish ADR-0008 same-entry Reality-like front with simple shared username/password and one-time ticket admission.
4. Implement the smallest data-session demultiplexing model keyed by independent ticket/session identity so one shared account can keep several live associations concurrently.
5. Run the complete protocol/unit/pcap/100 Mbit/s regression set before platform packet capture is allowed to change.
6. Connect the frozen session adapter to OpenWrt TPROXY, perform one clean end-to-end VPN success, then do the same with Windows TUN/Wintun-class.
7. Revisit fixed dual-lane experiments only if one-lane 100 Mbit/s qualification shows a meaningful cliff.
8. Keep Auto FEC outside the V2.2 implementation sequence.
