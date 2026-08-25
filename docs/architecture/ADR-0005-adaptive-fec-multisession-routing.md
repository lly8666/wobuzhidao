# ADR-0005: Adaptive FEC, multi-session accounts, client-driven policy, split routing, and optional dual lane

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (2026-08-25)

## Context

Focused first-arrival testing changed the FEC latency picture. The original WBD 20+20 block encoder waited for a full block (or its flush timer) before emitting systematic and repair shards. Changing the production fast path to emit each systematic source shard immediately reduced full-stack first-complete latency at 20% random loss from roughly 15.2 ms to 10.4 ms p50 at 20 ms RTT, and from roughly 55.7 ms to 50.4 ms p50 at 100 ms RTT, while still delivering 800/800 datagrams in both focused cases.

The remaining latency cost for a *lost* systematic source comes from waiting until enough repair equations exist. A fixed 20+20 tail-parity block is therefore a useful strong-loss reference, not the universal product optimum.

At the same time the product requirements now include:

- FEC that can be disabled, fixed, or later adapted automatically;
- most performance/routing choices owned by the client, with the server validating and adapting rather than requiring per-client static configuration;
- multiple simultaneous sessions under one account identity;
- optional full-tunnel and China/non-China split capture on Windows, Linux, and OpenWrt without installing thousands of firewall rules;
- an optional browser-like TLS 1.3 connection-establishment Persona;
- a possible two-lane survival mode, but only when one-lane adaptive protection is insufficient.

## Decision 1 — optimize FEC for first-complete datagram time, not block completion time

Let independent one-way packet loss be `p`, success probability `q = 1-p`, `K` systematic source shards, and `R` repair shards. For an ideal systematic MDS `(K+R,K)` block code, complete block recovery fails when more than `R` of the `K+R` transmitted shards are lost:

```text
P_fail(K,R,p)
  = P[Binomial(K+R,p) > R]
  = sum_{l=R+1}^{K+R} C(K+R,l) p^l (1-p)^(K+R-l).
```

For `K=20`, the minimum `R` meeting a representative block-failure target illustrates why one fixed ratio is wasteful:

| random loss p | R for P_fail <= 1e-3 | overhead | R for P_fail <= 1e-5 | overhead |
| ---: | ---: | ---: | ---: | ---: |
| 1% | 3 | 1.15x | 4 | 1.20x |
| 5% | 6 | 1.30x | 8 | 1.40x |
| 10% | 9 | 1.45x | 12 | 1.60x |
| 15% | 12 | 1.60x | 16 | 1.80x |
| 20% | 15 | 1.75x | 20 | 2.00x |

These are ideal iid-loss block probabilities, not release promises. Burst loss, correlated lanes, finite queues, MTU, code non-ideality, and estimation error require measured safety margin.

### Scheduling result

For a source that survives its first transmission, the latency-minimizing systematic schedule is immediate transmission: deliberately waiting for a block can never improve that source's first arrival.

For a source whose systematic transmission is lost, repairs should be made available as early and as evenly as causality allows. If source opportunities are separated by `Delta` and repair/source ratio is `alpha = R/K`, a roughly uniform repair cadence has period about `Delta/alpha`; a lost source that is already covered by the causal coding window sees mean residual wait on the order of

```text
E[T_next_repair] ~= Delta / (2 alpha)
```

rather than waiting approximately half of a `K`-packet block before a tail-only repair burst. The exact decode time is the first time the receiver has enough independent equations covering the missing source.

A useful queueing lower bound treats missing source equations as repair debt. Loss creates debt at mean rate `p` per source opportunity and successfully received repair equations service debt at approximately `alpha(1-p)`. A necessary mean-stability condition is therefore

```text
alpha(1-p) > p
alpha > p/(1-p).
```

This condition is necessary but not sufficient for a desired p95/p99/p99.9 tail. The binomial/empirical tail constraint chooses a larger `alpha`.

### Bandwidth/queue interaction

Protection itself can increase latency when it saturates the path. With payload offered load `B`, path capacity `C`, and repair ratio `alpha`, the first-order utilization is

```text
rho = B(1+alpha)/C.
```

As `rho` approaches 1, queue delay harms every packet. Therefore the product objective is **not maximum redundancy**. It is the smallest repair schedule that reaches the selected recovery-tail target while retaining adequate capacity headroom.

## Decision 2 — keep systematic RS as a fixed reference; research causal streaming repair

The current WBD GF(256) 20+20 systematic Reed-Solomon implementation remains supported and benchmarked.

Its production schedule is:

1. emit every systematic source shard immediately;
2. for the current fixed block implementation, emit repair shards when the block becomes encodable;
3. never delay an available source merely to make the FEC block prettier.

The next FEC research task compares three schedulers under identical offered load and loss traces:

- current `K`-block systematic RS with tail repairs;
- smaller/micro-block systematic RS, reducing repair causality delay at some coding/header cost;
- a causal/sliding-window systematic linear repair code that can emit independent repair equations continuously over a recent source window.

No causal coding implementation replaces the current RS path until it wins first-arrival tail, delivery, CPU, RSS, and wire-efficiency qualification.

## Decision 3 — FEC is an optional, per-direction client policy

The product configuration surface becomes:

```text
fec.mode = off | fixed | auto
fec.tx.k
fec.tx.r
fec.tx.flush
fec.tx.scheduler
fec.rx.*              # normally inferred/negotiated, not manually duplicated
```

Uplink and downlink protection may differ because observed loss and capacity may differ.

`off` sends no proactive FEC repair. DTLS and FakeTCP shadow retransmission remain active.

`fixed` selects an admitted `K:R`/scheduler profile. `20:10` and `20:20` remain reference presets, not the only legal future values.

`auto` is a client-side controller. It estimates directional loss, recovery latency, RTT, delivered goodput, and queue pressure, chooses an admitted protection profile, and proposes it to the peer.

Configuration changes are versioned by a monotonically increasing **config epoch** and take effect only at a coding-window boundary. Old-epoch shards remain decodable until their bounded receive windows expire. Changing FEC does not require reconnecting DTLS/FakeTCP.

The server is adaptive, not policy-less. It advertises capabilities and hard ceilings (supported code/scheduler versions, maximum repair ratio, maximum coding window, maximum lanes, MTU constraints), then accepts, clamps, or rejects a client's proposal. A client must not be able to force unbounded server memory, CPU, or wire amplification.

## Decision 4 — one account may own multiple simultaneous device sessions

The current single bearer-token authorization model is upgraded to a minimal account/session model; this is not a SaaS account platform.

- `username` identifies an account principal.
- session state is keyed by at least `(account_id, session_id)` and never by username alone.
- the same username may have multiple simultaneous sessions/devices.
- each device should preferably use a distinct high-entropy device access token/key under the account so one device can be revoked without rotating every other device.
- authentication remains inside the already authenticated DTLS association.
- server policy may cap concurrent sessions per account, but performance/routing/FEC choices remain session-local and client-proposed.

Human-memorable passwords are not required for the first implementation. If added later, they require a proper password KDF and are not stored/compared as raw SHA-256 secrets.

## Decision 5 — routing/capture policy is client-side and must exclude the underlay

Client capture modes are:

```text
capture.mode = off | global | only-cn | only-non-cn
```

Every full/split mode has an explicit **underlay escape invariant**: the WBD server endpoint(s), bootstrap/Persona endpoint(s), and required local-link control traffic must continue through the original physical/default route and must never recurse into the WBD tunnel.

### Linux / OpenWrt

Use TUN plus policy routing. Prefer a small number of `ip rule`/route-table rules and platform-native `nftables` interval sets when packet marking is useful. China prefixes are loaded as a compact interval/prefix set; do not materialize one rule per prefix.

A plain hash table is not the canonical representation for arbitrary CIDR membership. The portable classifier uses longest-prefix matching (radix/Patricia-style structure); Linux/OpenWrt may use nftables interval sets or equivalent kernel prefix structures.

### Windows

Use Wintun-class L3 I/O. For global capture, install broad tunnel routes while preserving explicit `/32` and `/128` escape routes to the actual server/bootstrap endpoints through the original gateway. Avoid a large Windows Firewall rule set.

For `only-cn` / `only-non-cn`, the implementation gate will compare:

- a reasonably compact aggregated route set; and
- a single/few Windows Filtering Platform or equivalent interception/filter rules backed by a user-space longest-prefix classifier.

The design must choose the approach with stable routing semantics and minimal persistent system mutation. It must not install thousands of per-prefix firewall rules.

The domestic prefix database is versioned, atomically replaced, and has an explicit source/build receipt. IPv4 and IPv6 are both first-class.

## Decision 6 — Persona profile is client-selected; certificate/domain authority is server-owned

The browser-like ClientHello originates at the client, so the client selects `persona = off | native | chrome | firefox | safari | edge` from the server-advertised supported set.

The TLS endpoint hostname, certificate, and private key are server/operator assets. The server advertises the allowed operator-controlled Persona hostname(s); the client validates the normal certificate chain and hostname. The client may select among allowed names but cannot invent arbitrary SNI/certificate identities.

No third-party private key or unrelated website certificate is borrowed. Browser-like fingerprinting changes the TLS preflight appearance; DTLS 1.3 remains the steady-state product security layer.

## Decision 7 — dual lane is an optional survival mode, not the default

Two lanes are not developed as `duplicate everything twice` by default. If two lanes share a physical bottleneck or correlated loss process, blind duplication can consume 2-4x wire capacity without proportional recovery benefit.

A future lane controller may admit:

- `striped`: sources distributed across lanes; independent repairs distributed across both, near `1+alpha` intentional overhead;
- `hedged`: normally one source copy, with selective duplicate/extra repair on the second lane when measured risk warrants it;
- `survival`: explicit emergency mode that may duplicate sources and add independent repair on both lanes. It may exceed the normal 2x one-lane protection budget only after a separate qualification/admission decision.

Two lanes do **not** require two unrelated FEC algorithms. They require independent coding equations and schedules: distinct lane IDs, sequence spaces, coding seeds/matrices/window phases, and preferably non-identical repair payloads. The receiver deduplicates/reconstructs by original datagram ID.

A lane benefit controller must measure cross-lane loss/latency correlation. Auto mode must not enable a second lane merely because single-lane packet loss is high.

## Configuration ownership summary

Client/session-owned: capture mode, FEC off/fixed/auto policy, target protection/tail level, directional preferences, Persona profile, optional lane policy.

Server/operator-owned: account credentials/caps, certificate/private key and allowed Persona hostnames, supported protocol/code versions, hard memory/CPU/wire/MTU/lane ceilings.

Negotiated: effective FEC profile/epoch, lane parameters, MTU-compatible limits, selected Persona hostname/profile combination where applicable.

This split keeps deployment simple while preventing a compromised or misconfigured client from forcing unbounded server resource use.

## Consequences / next implementation order

1. Build a deterministic FEC schedule simulator/sweeper that compares tail-block RS, micro-block RS, and a causal streaming-repair model across iid and burst loss while including offered-load/capacity queue cost.
2. Add negotiated config epochs and `off/fixed` runtime switching first; then admit `auto` only after estimator/controller tests.
3. Implement the optional TLS Persona preflight with pinned, pcap-qualified browser profiles and normal certificate validation.
4. Upgrade bearer auth to account + device-token + multi-session state.
5. Implement Linux/OpenWrt capture policies and then Windows Wintun/global/split policies with underlay escape tests.
6. Revisit two-lane `striped/hedged` experiments only after one-lane adaptive FEC has a measured failure/cliff surface.
