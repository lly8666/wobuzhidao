# ADR-0005: Fixed FEC scheduling, deferred Auto, multi-session accounts, split routing, and optional dual lane

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (updated 2026-08-25)

## Context

Focused first-arrival testing changed the FEC latency picture. The original WBD 20+20 block encoder waited for a full block (or its flush timer) before emitting systematic and repair shards. Changing the production fast path to emit each systematic source shard immediately reduced full-stack first-complete latency at 20% random loss from roughly 15.2 ms to 10.4 ms p50 at 20 ms RTT, and from roughly 55.7 ms to 50.4 ms p50 at 100 ms RTT, while still delivering 800/800 datagrams in both focused cases.

The remaining latency cost for a lost systematic source comes from waiting until enough repair equations exist. A fixed 20+20 tail-parity block is therefore a useful strong-loss reference, not the universal product optimum.

The product requirements now include:

- FEC that can be disabled or set to an explicit fixed profile;
- **Auto FEC deferred to a future advanced-research phase**, outside the current V2.2 implementation path;
- most performance/routing choices owned by the client, with the server validating and enforcing resource ceilings;
- multiple simultaneous sessions under one account identity;
- optional full-tunnel and China/non-China split capture on Windows, Linux, and OpenWrt without installing thousands of firewall rules;
- an optional browser-like TLS 1.3 connection-establishment Persona;
- a possible two-lane survival mode, but only after the one-lane fixed-mode surface is understood.

## Decision 1 — optimize FEC for first-complete datagram time, not block completion time

Let independent one-way packet loss be `p`, success probability `q = 1-p`, `K` systematic source shards, and `R` repair shards. For an ideal systematic MDS `(K+R,K)` block code, complete block recovery fails when more than `R` of the `K+R` transmitted shards are lost:

```text
P_fail(K,R,p)
  = P[Binomial(K+R,p) > R]
  = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l).
```

For `K=20`, the minimum `R` meeting representative iid block-failure targets illustrates why one fixed ratio is wasteful:

| random loss p | R for P_fail <= 1e-3 | overhead | R for P_fail <= 1e-5 | overhead |
| ---: | ---: | ---: | ---: | ---: |
| 1% | 3 | 1.15x | 4 | 1.20x |
| 5% | 6 | 1.30x | 8 | 1.40x |
| 10% | 9 | 1.45x | 12 | 1.60x |
| 15% | 12 | 1.60x | 16 | 1.80x |
| 20% | 15 | 1.75x | 20 | 2.00x |

These are planning bounds, not release promises. Burst loss, queueing, MTU, coding-window causality and implementation cost still require measurement.

### Scheduling result

For a source that survives its first transmission, the latency-minimizing systematic schedule is immediate transmission: deliberately waiting for a block cannot improve that source's first arrival.

For a source whose systematic transmission is lost, repairs should become available as early and as evenly as causality permits. If source opportunities are separated by `Delta` and repair/source ratio is `alpha = R/K`, a roughly uniform repair cadence has period about `Delta/alpha`; a lost source already covered by the coding window sees mean residual wait on the order of

```text
E[T_next_repair] ~= Delta / (2 alpha)
```

instead of waiting for a tail-only repair burst.

A useful lower bound treats missing source equations as repair debt. Loss creates debt at mean rate `p` per source opportunity and successfully received repair equations service debt at approximately `alpha(1-p)`. Necessary mean stability therefore requires

```text
alpha(1-p) > p
alpha > p/(1-p).
```

This is necessary, not sufficient, for a target p95/p99 tail.

### Capacity interaction

With payload offered load `B`, path capacity `C`, and repair ratio `alpha`, first-order utilization is

```text
rho = B(1+alpha)/C.
```

As `rho` approaches one, repair backlog and serialization delay can erase any recovery benefit. Therefore the current objective is not maximum redundancy; it is the best **fixed** profile for a chosen environment and capacity budget.

## Decision 2 — compare fixed schedulers before changing the live codec

The current WBD GF(256) 20+20 systematic Reed-Solomon implementation remains supported and benchmarked.

Its production invariant is:

1. emit every systematic source shard immediately;
2. emit repair when the selected fixed scheduler makes it available;
3. never intentionally delay an available source merely to fill a repair block.

The fixed-scheduler research set is:

- current `K`-block systematic RS with tail repairs;
- smaller/micro-block systematic RS;
- a causal/sliding-window systematic linear repair model that spreads independent repair equations earlier.

The deterministic simulator in `internal/fec/simulator.go` is an **offline qualification tool**, not an Auto controller. It models source generation, serialization, source-priority queuing, iid or burst loss, repair backlog, first-complete latency and finite-run drain time. No causal coding implementation replaces the current RS transport until it wins measured first-arrival tail, delivery, CPU/RSS and wire-efficiency gates.

## Decision 3 — current product FEC is `off | fixed`; Auto is reserved

Current protocol/configuration surface:

```text
fec.mode = off | fixed
fec.tx.k
fec.tx.r
fec.tx.flush
fec.tx.scheduler
fec.rx.*              # inferred/negotiated rather than manually duplicated
```

Uplink and downlink protection may differ.

`off` sends no proactive FEC repair. DTLS and FakeTCP shadow retransmission remain active.

`fixed` selects an admitted `K:R`/scheduler profile. Compatibility preset names remain:

- `weak-1.5x` = reference `20:10`;
- `weak-2x` = reference `20:20`.

`auto` remains a reserved future value and is **not implemented, negotiated, or accepted in the current product path**. The existing control-plane reserved Auto value continues to be rejected. Any future Auto controller requires a separate milestone with estimator, hysteresis, capacity inference, failure behavior and extensive qualification; it is not a prerequisite for V2.2.

Configuration changes are versioned by a monotonically increasing **config epoch** and take effect only at a coding-window boundary. Old-epoch shards remain decodable until their bounded receive windows expire. Changing between admitted `off` and `fixed` profiles must not require reconnecting DTLS/FakeTCP.

The server advertises capabilities and hard ceilings such as supported code/scheduler versions, maximum repair ratio, maximum coding window, maximum lanes and MTU constraints, then accepts, clamps or rejects a client's fixed proposal. A client cannot force unbounded server memory, CPU or wire amplification.

## Decision 4 — one account may own multiple simultaneous device sessions

The current bearer authorization is upgraded to a minimal account/session model; this is not a SaaS account platform.

- `username` identifies an account principal.
- session state is keyed by at least `(account_id, session_id)`, never username alone.
- the same username may have multiple simultaneous sessions/devices.
- each device should preferably use a distinct high-entropy access token/key so one device can be revoked independently.
- authentication remains inside the already authenticated DTLS association.
- server policy may cap concurrent sessions per account, but FEC/routing/Persona choices remain session-local and client-proposed.

Human-memorable passwords are not required for the first implementation. If added later, they require a proper password KDF.

## Decision 5 — routing/capture policy is client-side and must exclude the underlay

Client capture modes are:

```text
capture.mode = off | global | only-cn | only-non-cn
```

Every full/split mode has an explicit **underlay escape invariant**: WBD server endpoint(s), Persona/bootstrap endpoint(s), and required local-link control traffic continue through the original physical/default route and never recurse into the tunnel.

### Linux / OpenWrt

Use TUN plus policy routing. Prefer a small number of `ip rule`/route-table rules and platform-native `nftables` interval sets when packet marking is useful. China prefixes are loaded as a compact interval/prefix set; do not materialize one firewall rule per prefix.

CIDR membership is longest-prefix matching, not an exact-address hash lookup. The portable classifier may use a radix/Patricia structure; Linux/OpenWrt may use superior native prefix/interval sets.

### Windows

Use Wintun-class L3 I/O. For global capture, broad tunnel routes coexist with explicit `/32` and `/128` escape routes to actual WBD/Persona endpoints through the original gateway.

For `only-cn` / `only-non-cn`, compare a compact aggregated route set against a small Windows Filtering Platform/equivalent interception layer backed by a user-space longest-prefix classifier. Do not install thousands of persistent Windows Firewall rules.

The domestic prefix database is versioned, atomically replaced, and supports IPv4 and IPv6.

## Decision 6 — Persona mimics a browser ClientHello profile, not a third-party identity

The browser-like ClientHello originates at the client, so the client selects `persona = off | native | chrome | firefox | safari | edge` from the server-advertised supported set.

The TLS endpoint hostname, certificate and private key are operator-controlled server assets. The client validates a normal certificate chain and hostname. Browser-profile implementations are pinned and pcap-qualified rather than trusting a library's moving `Auto` alias.

WBD may use public services such as speed-test sites as **measurement baselines** when studying whether a network treats traffic classes differently. It must not present an unrelated third-party hostname/certificate as WBD's own identity, borrow a third-party private key, or disable certificate validation. If the goal is to obtain ordinary TLS treatment, the supported path is a standards-compliant browser-like ClientHello to an endpoint/domain the operator is authorized to use.

Persona remains a connection-establishment preflight and does not replace steady-state DTLS 1.3.

## Decision 7 — dual lane is an optional survival mode, not the default

Two lanes are not developed as `duplicate everything twice` by default. If both lanes share a bottleneck or correlated loss process, blind duplication can consume 2-4x wire capacity without proportional recovery benefit.

Future fixed lane modes may include:

- `striped`: sources distributed across lanes with independent repairs;
- `hedged`: normally one source copy with selected second-lane copies/repairs;
- `survival`: explicit emergency source duplication plus independent repair, potentially above 2x total wire cost and therefore requiring a separate admission decision.

Two lanes do not require unrelated FEC algorithms. They require independent lane IDs, sequence spaces, coding equations/seeds/window phases and schedules so repair traffic is complementary rather than byte-identical duplication.

## Configuration ownership summary

Client/session-owned: capture mode, FEC `off|fixed` profile, directional preferences, Persona profile, optional future lane mode.

Server/operator-owned: account credentials/caps, certificate/private key and allowed Persona hostnames, supported protocol/code versions, hard memory/CPU/wire/MTU/lane ceilings.

Negotiated: effective fixed FEC profile/epoch, lane parameters when admitted, MTU-compatible limits, selected Persona profile/allowed hostname combination.

## Consequences / implementation order

1. Finish deterministic fixed-scheduler qualification: tail-block RS vs micro-block RS vs causal/sliding repair under iid/burst loss and capacity pressure.
2. Implement negotiated config epochs and runtime `fec.mode=off|fixed` switching without reconnect.
3. Implement the optional TLS Persona preflight with pinned, pcap-qualified browser profiles and normal certificate validation against operator-controlled endpoint identities.
4. Upgrade bearer auth to account + device-token + concurrent multi-session state.
5. Implement Linux/OpenWrt capture policies and then Windows Wintun/global/split policies with underlay escape tests.
6. Revisit fixed dual-lane `striped/hedged/survival` experiments only if one-lane qualification shows a meaningful cliff.
7. **Auto FEC remains future advanced research and is intentionally outside this implementation sequence.**
