# 2026-08-30 Architecture Pivot: Logical Tunnel + Shared TUN + Multipath Lane Lifecycle

> **STOP-OLD-WORK NOTICE.** ADR-0012 is now the controlling architecture decision. Any in-progress work that treats per-LiveID Linux netns + veth + double NAT as the final Windows raw-IP product design must stop after preserving evidence. Do not continue expanding that topology.

This document is the execution contract for an agent that was already developing the older raw-IP gateway path when ADR-0012 was accepted.

## Immediate first actions

1. Live-refresh PR #9, its head branch and Actions before editing. Never assume a remembered SHA is still current.
2. Read, in order:
   - `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`;
   - `PROJECT_CONSTITUTION.md`;
   - `ARCHITECTURE.md`;
   - `ROADMAP.md`;
   - this file;
   - `docs/development/2026-08-30-rawip-gateway.md` as historical evidence only;
   - `internal/gamelane/**` and the game-lane full-stack tests before changing multipath semantics.
3. Stop adding product behavior to `wbd-ip-gateway-server` netns/veth/double-NAT design. Preserve commits/tests as historical reference or behavioral requirements; do not delete evidence merely to make the tree look clean.
4. Do not start Windows process slimming, startup-latency optimization, DTLS HRR removal, LINK bind/init coalescing or unrelated GUI cleanup during this pivot.

## The architecture you are now implementing

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - server-assigned unique tunnel IPv4 lease
          - stable race SessionID / PacketID space while tunnel is active
          - desired lane count
          - 0..N active Transport Lanes

Transport Lane
  - lane id + generation/epoch
  - one FakeTCP 4-tuple/sequence space
  - Reality-like TLS bootstrap on that same FakeTCP association
  - one DTLS 1.3 association
  - one LINK session
  - lane-local FEC state
```

Normal mode: desired lanes = 1.

Game/weak-network race mode: desired lanes may be 2..4 according to the existing controller and policy limits.

Migration/replacement may temporarily add a candidate lane before retiring an old lane, but never exceeds the product maximum without an explicit later decision.

## Do not misunderstand the two historical multilane designs

### Rejected V1

PR #2 used ordinary ordered kernel TCP lanes. FEC/redundancy above them remained subject to each kernel TCP stream's HOL. That design stays rejected forever unless a new ADR changes a fundamental assumption.

### Later Game Lane

`internal/gamelane` is **not** the rejected V1 design. It intentionally races one logical `PacketID` over 1..4 independent complete WBD associations. Each lane has independent FakeTCP tuple/sequence space, independent DTLS and independent LINK/FEC. First arrival is delivered, later copies are deduplicated, bounded out-of-order unique packets are allowed, and there is no cross-lane HOL.

Preserve that semantic contract. Promote/reuse it for general tunnel migration rather than replacing it with a second migration-only packet-ID layer.

## Raw-IP server pivot

### Stop implementing

Do not continue the final-product path:

```text
LiveID
  -> private netns
  -> per-session TUN
  -> veth /30
  -> inner NAT
  -> host NAT
```

Do not revive the older VRF/conntrack-zone prototype.

### Implement instead

```text
Internet
  <-> one WBD-owned host NAT
  <-> Linux root routing
  <-> shared WBD TUN
  <-> Logical Tunnel manager
  <-> race/multipath lane set
```

The server allocates a unique tunnel IPv4 address per logical tunnel/device from a configurable pool. Do not hard-code every Windows client to `10.66.0.2/30`.

Ingress raw-IP invariant:

```text
packet source IPv4 == logical tunnel leased IPv4
```

Mismatch is a hard drop/security event. Do not let a lane choose another tunnel's source address.

Downlink demux is by destination lease address after Linux routing/reverse NAT returns the packet to the shared TUN.

## Address/config negotiation

The tunnel address lease belongs to the Logical Tunnel, not to LiveID or the current lane.

Authenticated tunnel configuration must carry enough information for Windows to configure Wintun and routes. Prefer testing `/32` + explicit route behavior first because Wintun is L3; use a shared-prefix fallback if physical Windows qualification requires it. Keep the address pool configurable because no RFC1918/CGNAT range is collision-free on every client network.

A short reconnect/lane replacement should resume the same logical tunnel and leased address where possible. Same-account devices must receive different active addresses.

## Tunnel and lane lifecycle

Required logical tunnel states:

```text
DISCONNECTED
CONNECTING
ACTIVE
DORMANT
WAKING
MIGRATING / REPLACING
DEGRADED
```

A simpler internal representation is acceptable if it preserves these observable semantics.

### Idle sleep

Track real payload activity separately from transport liveness.

- `last_payload_activity`: tunneled IP payload only.
- `last_transport_activity`: payload plus PING/PONG/control.

PING/PONG must not reset the user's payload-idle timer.

Client setting `idle_transport_timeout`:

- initial product default: 15 minutes;
- configurable;
- `0` means never sleep **because of idleness**, not never replace a FakeTCP lane.

When non-zero idle timeout expires, close all active lanes and enter DORMANT while retaining Logical Tunnel identity, lease, Wintun and capture/routing/DNS state.

A new Wintun packet triggers wake. Use a bounded wake buffer only. Start traffic after the first healthy lane is ready, then restore additional desired game lanes in the background.

### Age rotation

Every active lane has its own randomized age deadline. Initial experiment/default range: 30..60 minutes. This is a product policy value, not a wire constant.

Age rotation applies regardless of whether idle timeout is zero. Continuous traffic must not create an immortal FakeTCP association.

Multi-lane mode must stagger deadlines and enforce a minimum rotation separation. Never intentionally rotate all healthy game lanes at once.

### Make-before-break replacement

For old lane A and candidate B:

1. keep A active;
2. fully establish B: FakeTCP -> Reality-like bootstrap -> DTLS -> LINK;
3. authenticate `TUNNEL_ATTACH`/equivalent to the existing logical tunnel;
4. prove B has working bidirectional control/data path;
5. add B to the race set temporarily;
6. permit the same logical PacketID to race over A+B;
7. rely on first-arrival + dedup so downstream receives one payload;
8. mark A DRAINING and stop assigning new logical packets to it;
9. retire A after a short bounded drain/overlap.

If B fails at any stage, A remains usable. Back off and retry later.

Game mode example:

```text
A + B
  -> build C
  -> A + B + C briefly
  -> retire A
  -> B + C
```

Do not define a fixed five-second double-send window as a protocol invariant. Initial implementation may use a simple bounded overlap; RTT/barrier optimization is later work.

### Unified replacement triggers

Use one replacement state machine for:

- scheduled age rotation;
- Windows network/default-route/public-IP change;
- NAT/path change or dead path;
- missed PONG / no valid RX;
- FakeTCP child failure;
- DTLS child failure;
- LINK child failure;
- server-requested replacement;
- manual reconnect.

Windows network-change events may trigger immediately. Heartbeat timeout is the fallback detector.

Use lane generation/epoch fencing so a stale old path cannot resurrect and become active after a newer lane generation commits.

## Single-flow rule after the pivot

Do not write or test the old statement `one VPN session = one public flow until Disconnect`.

The correct invariant is:

> Every **lane/transport epoch** has one public FakeTCP SYN lineage and one continuous FakeTCP association carrying Reality-like bootstrap, DTLS, LINK/FEC and payload for that lane. There is no ordinary kernel-TCP WBD payload connection. The logical tunnel may intentionally have multiple independent lanes in game/race mode or briefly during make-before-break replacement.

Tests that assert exactly one public flow for a normal one-lane transport epoch remain useful. Tests must not reject a legitimate second independent WBD lane created by configured game mode or controlled migration.

## Preserved hard metrics and boundaries

Do not regress any of these while implementing ADR-0012:

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- `legacy` FakeTCP shadow recovery is default; `sack-rack` is experimental;
- steady payload must never depend on ordinary kernel TCP ordered delivery/HOL;
- bootstrap-only reliable ordering remains bounded and per lane;
- pinned wolfSSL DTLS 1.3 remains steady-state cryptographic authority;
- release FEC is `off` or fixed systematic `20:20`; FEC state is lane-local and immutable for a lane;
- systematic source delivery must not wait merely to fill a FEC block;
- Windows product capture is Wintun/raw L3;
- OpenWrt final capture is TPROXY + policy routing;
- Linux product server exposes one public WBD port;
- WBD firewall helpers modify/clean WBD-owned state only and never flush the user's host ruleset;
- Windows IPv6 remains fail-closed for the entire connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit must restore routes, DNS/NRPT, IPv6 and WBD firewall state;
- Npcap release/licensing constraints remain unchanged;
- secrets/credentials/tickets/resume secrets must never be logged;
- retain non-secret session correlation IDs, first-boundary markers and counters without per-packet INFO logging.

## Implementation phases

### Phase 0 — stop and preserve

- Stop netns product expansion.
- Mark existing netns qualification as superseded-product/reference-only.
- Fix stale handoff/current pointers before further implementation.
- Keep historical tests and commits where they preserve useful correctness evidence.

### Phase 1 — logical tunnel + lease model

- Introduce Logical Tunnel identity distinct from LiveID/lane.
- Add server lease allocator and authenticated tunnel configuration.
- Remove product assumption that every Windows client is `10.66.0.2/30`.
- Enforce ingress source-address binding.

### Phase 2 — shared TUN + one NAT

- One shared Linux TUN.
- Root routing and one WBD-owned host NAT for the configured tunnel pool.
- Lease-destination downlink demux to logical tunnel.
- Two-tunnel simultaneous raw-IP qualification.

### Phase 3 — reuse/promote race layer

- Adapt the existing Game Lane PacketID/race/dedup semantics so raw-IP Logical Tunnel payload can use 1..4 dynamic lanes.
- Keep lane-local DTLS/LINK/FEC.
- Add lane generation/lifecycle attach/detach.

### Phase 4 — idle sleep / wake

- Separate payload-idle from liveness timers.
- DORMANT closes lanes only.
- Bounded wake buffer.
- First healthy lane resumes traffic before desired-lane fill completes.

### Phase 5 — make-before-break replacement

- Manual forced A->B replacement first.
- Candidate failure safety.
- Dedup proof.
- Then scheduled randomized age rotation.
- Then network-change/dead-path/child-failure triggers.

### Phase 6 — physical release qualification

Only after deterministic automated gates pass, build exact-same-SOURCE_SHA Windows and Linux packages and run physical Windows 11 -> Ubuntu ARM64 qualification.

## Mandatory qualification scenarios

Automated tests must prove all of these before calling the pivot complete:

1. two logical tunnels receive different leased IPv4 addresses;
2. both bind the same inner TCP source port `40000` and reach the same Internet target/port simultaneously;
3. DNS-style UDP, generic UDP and TCP work through shared TUN + one host NAT;
4. spoofed source IP from the wrong lease is rejected;
5. lease cleanup/reconnect/reuse is deterministic;
6. idle timeout puts the tunnel DORMANT without removing Wintun/lease and a new packet wakes it;
7. one-lane A->B make-before-break causes no application-level duplicate delivery;
8. game mode keeps at least its desired healthy redundancy while only one lane is rotated at a time;
9. candidate failure does not damage the current healthy lane set;
10. network-change and missed-PONG events use the same replacement path;
11. FEC `off` and `20:20` remain qualified per lane;
12. full-stack packet path is FakeTCP -> Reality-like -> DTLS -> LINK -> race/raw-IP -> shared TUN -> Internet;
13. final physical Windows 11 -> Ubuntu ARM64 DNS/UDP/TCP and cleanup pass.

## Explicit non-goals during this pivot

Do not:

- optimize handshake RTT yet;
- remove DTLS HRR cookie yet;
- merge LINK bind/init yet;
- implement 0-RTT/resumption shortcuts yet;
- refactor Windows child processes into one binary/module yet;
- redesign FEC profiles;
- reintroduce kernel TCP as payload carrier;
- revive VRF/conntrack-zone or expand netns/double NAT as final product;
- claim release readiness before the new shared-TUN/multipath/physical gates pass.

## Done condition

This pivot is done only when the repository's current architecture documents, handoff and tests all agree on the same model, the shared-TUN/unique-lease path is green, lane lifecycle replacement is green, and the final exact-source physical Windows/Linux test passes. A green historical netns test alone is not completion.
