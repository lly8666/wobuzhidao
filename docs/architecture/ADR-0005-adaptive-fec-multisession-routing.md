# ADR-0005: Fixed FEC scheduling, deferred Auto, multi-session accounts, split routing, and optional dual lane

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (updated 2026-08-25)

ADR-0006 is authoritative for link-parameter timing: V2.2 uses immutable per-association `LINK_INIT/LINK_ACCEPT`; there is no runtime config epoch or mid-session FEC switching.

## Context

Focused first-arrival testing changed the FEC latency picture. The original WBD 20+20 block encoder waited for a full block (or flush timer) before emitting source and repair shards. Changing the fast path to emit each systematic source immediately reduced full-stack first-complete latency at 20% random loss from roughly 15.2 ms to 10.4 ms p50 at 20 ms RTT and from roughly 55.7 ms to 50.4 ms at 100 ms RTT, while still delivering 800/800 datagrams.

The remaining latency cost for a lost systematic source comes from waiting for enough repair equations. Fixed 20+20 tail parity is therefore a strong-loss reference, not a universal optimum.

Current requirements also include fixed FEC that may be disabled, client-owned performance/routing choices with server resource ceilings, several simultaneous sessions under one account, global/China/non-China capture, optional browser-like TLS Persona, and possible later two-lane survival modes. **Auto FEC remains future advanced research.**

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

As `rho` approaches or exceeds one, repair backlog and serialization delay can erase recovery gains. Therefore fixed profiles are qualified against both loss and capacity pressure.

## Decision 2 — compare fixed schedulers before changing live codec

The current WBD GF(256) systematic 20+20 Reed-Solomon path remains the qualified live fixed codec.

Research compares:

- current K-block systematic RS with tail repairs;
- smaller/micro-block systematic RS;
- causal/sliding-window systematic linear repair with earlier repair equations.

`internal/fec/simulator.go` is an offline qualification tool, not an Auto controller and not a declaration of live support. No research scheduler replaces the live codec until it wins first-arrival tail, delivery, CPU/RSS and wire-efficiency gates under iid, burst and capacity stress.

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

## Decision 4 — one account may own multiple simultaneous device sessions

The current bearer authorization will evolve into a minimal account/session model:

- `username` identifies an account principal;
- state is keyed by at least `(account_id, session_id)`, never username alone;
- the same username may have multiple simultaneous sessions/devices;
- each device should preferably use a distinct high-entropy access token/key so it can be revoked independently;
- authentication remains inside the authenticated DTLS association;
- server policy may cap concurrent sessions per account.

Human-memorable passwords are not required for the first implementation. If added later, they require a proper password KDF.

## Decision 5 — routing/capture policy is client-side and must exclude underlay

Client capture modes are:

```text
capture.mode = off | global | only-cn | only-non-cn
```

Every full/split mode has an **underlay escape invariant**: WBD server endpoint(s), Persona/bootstrap endpoint(s), and required local-link traffic continue through the original physical/default route and never recurse into the tunnel.

### Linux / OpenWrt

Use TUN plus policy routing. Prefer a small number of `ip rule`/route-table rules and compact platform-native interval/prefix sets. Do not create one firewall rule per China prefix.

CIDR membership is longest-prefix matching, not exact-address hashing. A portable radix/Patricia structure is acceptable; kernel-native prefix/interval sets may be superior.

### Windows

Use Wintun-class L3 I/O. Global capture uses broad tunnel routes with explicit `/32` and `/128` endpoint escape routes through the original gateway.

For `only-cn` / `only-non-cn`, compare compact aggregated routes with a small WFP/equivalent interception layer backed by user-space longest-prefix classification. Do not install thousands of persistent Windows Firewall rules.

The domestic prefix database is versioned, atomically replaced, and supports IPv4 and IPv6.

## Decision 6 — Persona profile is client-selected; endpoint identity is operator-owned

The client selects `persona = off | native | chrome | firefox | safari | edge` from the supported set.

The TLS endpoint hostname, certificate and private key are identities the operator is authorized to use. The client validates a normal certificate chain and hostname. Browser profile implementations are pinned and pcap-qualified rather than trusting a moving library Auto alias.

WBD may use public services such as speed-test sites as **measurement baselines** when studying network treatment. It does not borrow their private keys/certificates or present an unrelated third-party identity as the WBD endpoint.

Persona remains a connection-establishment preflight and does not replace steady-state DTLS 1.3.

## Decision 7 — dual lane is an optional survival mode, not default

Two lanes are not developed as `duplicate everything twice` by default. Shared bottlenecks or correlated loss can make blind 2-4x traffic wasteful.

Future modes may include:

- `striped`: sources and independent repairs distributed across lanes;
- `hedged`: selected second-lane duplicates/repairs;
- `survival`: explicit emergency source duplication plus independent repair, potentially above 2x and requiring separate qualification.

Two lanes do not require unrelated FEC algorithms. They require independent lane IDs, sequence spaces, coding equations/seeds/window phases and schedules so repair traffic is complementary rather than byte-identical duplication.

## Configuration ownership summary

Client/session-owned at establishment: capture mode, fixed FEC profile, directional preferences, Persona profile, optional future lane mode.

Server/operator-owned: account credentials/caps, certificate/private key and allowed Persona hostnames, supported protocol/code versions, hard memory/CPU/wire/MTU/lane ceilings.

Negotiated exactly once: immutable LinkConfig for the new association. Unsupported proposals are rejected rather than silently rewritten.

## Consequences / implementation order

1. Finish fixed-scheduler offline qualification.
2. Implement and integrate immutable `LINK_INIT/LINK_ACCEPT`; no runtime config epochs.
3. Implement optional TLS Persona with pinned, pcap-qualified browser profiles and normal certificate validation.
4. Upgrade bearer auth to account + device-token + concurrent multi-session state.
5. Implement Linux/OpenWrt capture policies and then Windows Wintun/global/split policies with underlay escape tests.
6. Revisit fixed dual-lane experiments only if one-lane qualification shows a meaningful cliff.
7. Keep Auto FEC outside the V2.2 implementation sequence.
