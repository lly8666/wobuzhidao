# ADR-0012: Stable logical tunnel, server-assigned address lease, and replaceable multipath transports

Status: **ACCEPTED / SUPERSEDES SESSION-LIFETIME, ONE-LANE, AND RAW-IP NETNS PRODUCT CLAUSES** (2026-08-30)

## Context

WBD has now proved three important but previously separate ideas:

1. a single WBD FakeTCP association can own its own complete lineage from SYN through Reality-like TLS bootstrap, DTLS 1.3, LINK/FEC and packet payload without using an ordinary kernel-TCP payload stream;
2. the later `internal/gamelane` race layer can copy one logical `PacketID` across 1..4 independent complete WBD associations and deliver the first arrival while suppressing later duplicates, with bounded out-of-order acceptance and no cross-lane HOL;
3. Windows raw-L3 delivery exposed a server-side identity problem because every Windows client was temporarily hard-coded as `10.66.0.2/30`. The per-LiveID Linux netns + veth + inner-NAT + host-NAT prototype isolates identical inner tuples, but it solves an address-model defect with permanent double NAT and per-session kernel topology.

Long-lived single FakeTCP associations also create an avoidable product risk. A logical VPN may stay enabled for hours, but the same public TCP-shaped 4-tuple should not be required to live forever. Public IP/NAT changes, network-interface changes, path failure, scheduled rotation and game-mode racing all need one coherent lifecycle model.

## Decision

### 1. Separate logical tunnel identity from transport identity

The product model is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> one stable server-assigned tunnel address lease
          -> one stable race SessionID / logical PacketID space while the tunnel exists
          -> zero or more replaceable Transport Lanes
```

A `Logical Tunnel` is the user-visible VPN session. A `Transport Lane` is disposable.

A lane owns one complete independent WBD transport epoch:

```text
FakeTCP association
  -> Reality-like TLS bootstrap/admission on that same association
  -> DTLS 1.3
  -> LINK
  -> lane-local fixed FEC state
```

`LiveID`, FakeTCP 4-tuple, DTLS state and LINK state belong to a lane/transport epoch. They do **not** own the tunnel address lease.

### 2. The single-flow invariant applies per lane, not per whole VPN lifetime

For every lane/transport epoch:

- exactly one public FakeTCP association owns Reality-like bootstrap through steady payload for that lane;
- no second ordinary kernel-TCP WBD payload connection is allowed;
- no `Reality TCP -> close -> new FakeTCP payload connection` shortcut is allowed;
- no ordinary kernel TCP byte stream may own sustained WBD payload;
- post-bootstrap delivery remains datagram/packet oriented and must preserve the no-HOL gate.

The whole logical tunnel may have more than one lane when product policy requires it. Normal mode targets one steady lane. Game/weak-network race mode may keep 2..4 independent lanes. Controlled migration may temporarily add a candidate lane before retiring an old one.

This does not revive V1 PR #2. V1 was rejected because redundancy/FEC sat above ordinary ordered kernel TCP lanes. The later Game Lane design uses independent WBD FakeTCP/DTLS/LINK associations, one logical PacketID across lanes, first-arrival delivery and duplicate suppression; it has no cross-lane ordered-delivery dependency.

### 3. Promote the Game Lane race semantics into the general tunnel multipath layer

The existing `internal/gamelane` semantics are the architectural foundation for transport racing and replacement:

- one logical payload gets one `PacketID`;
- copies may be emitted on 1..4 independent lanes;
- copies have lane-distinct envelopes before DTLS;
- each lane has its own FakeTCP tuple/sequence space, DTLS keys/nonces, LINK and FEC state;
- the first valid copy is delivered immediately;
- later copies of the same `PacketID` are suppressed;
- bounded out-of-order unique packets remain independently deliverable;
- there is no cross-lane HOL.

Do not introduce a second migration-only WBDP packet-sequence protocol unless a later ADR proves that the existing race envelope cannot cover the raw-IP tunnel path.

FEC remains lane-local. A FEC block must never be split across lanes or wait on another lane.

### 4. Server assigns a unique tunnel address lease

The server allocates each active logical tunnel/device a unique IPv4 tunnel address from a configurable pool. The initial implementation may use a configurable private pool such as `10.66.0.0/16`, but no address range is claimed to be universally collision-free.

The client must stop hard-coding `10.66.0.2/30` as a global identity. Address/prefix/route parameters are supplied by authenticated tunnel configuration. `/32` plus explicit routes is preferred if Windows qualification proves it reliable; a shared subnet prefix is an allowed fallback if required by Windows route/source-selection behavior.

The address lease is stable across lane rotation and short reconnect/replacement. It may be stable per device/installation when practical. Same-account devices receive distinct addresses.

Server ingress enforces anti-spoofing:

```text
raw IPv4 source == leased tunnel IPv4
```

Future IPv6 support applies the same rule to an assigned `/128`.

### 5. Linux raw-IP product data plane becomes shared TUN + one host NAT

The final Windows raw-L3 server direction is:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> race/multipath lanes
```

Upstream raw packets are authenticated to a logical tunnel, source-validated against its lease, then written to the shared TUN. Downstream packets read from the shared TUN are demultiplexed by leased destination address to the owning logical tunnel and emitted through that tunnel's currently active lane set.

The current per-session Linux netns/veth/double-NAT implementation is **superseded as a final product architecture**. It may remain as historical/correctness evidence, but new product work must not extend it. The earlier VRF/conntrack-zone prototype remains rejected and must not be revived.

### 6. Idle transport policy is based on real payload activity

Client policy exposes an idle transport timeout. Initial UI/default guidance is 15 minutes, with configurable values and `0` meaning **never sleep because of payload idleness**.

Internally track at least:

- `last_payload_activity`: real tunneled IPv4/IPv6 payload only;
- `last_transport_activity`: payload plus PING/PONG/control used for liveness.

PING/PONG/control must not reset the user-visible payload-idle timer.

When a non-zero idle timeout expires:

- close all active FakeTCP/DTLS/LINK lanes for that logical tunnel;
- keep the logical tunnel, leased IP, Wintun and capture/routing/DNS state while the user still considers the VPN connected;
- enter `DORMANT` with zero active lanes.

A new raw-IP packet wakes the tunnel. A small bounded wake buffer is allowed. The first healthy lane may immediately resume traffic; additional desired game/weak-network lanes are established afterward rather than blocking wake on the full lane count.

Explicit user Disconnect, not idle sleep, releases the logical tunnel and removes Wintun/routes/DNS/IPv6 policy.

### 7. Transport age rotation is independent of idle policy

Every active lane has an independent maximum-age/rotation policy. This policy applies even when idle timeout is non-zero but traffic is continuous, and it also applies when idle timeout is `0`.

Initial experimental product guidance is a per-lane randomized soft deadline in the 30..60 minute range. This is a policy default, not a wire-protocol constant. New lanes receive new randomized deadlines. Multi-lane mode must stagger replacement and enforce a minimum separation so healthy lanes are not intentionally rotated together.

If a tunnel becomes payload-idle before rotation, idle sleep wins and the lanes close instead of being needlessly rotated.

### 8. Replacement is make-before-break and reuses race/dedup

A healthy old lane is not destroyed merely because it reached a rotation deadline.

Replacement sequence:

```text
old lane ACTIVE
  -> create candidate lane
  -> FakeTCP + Reality-like bootstrap + DTLS + LINK
  -> authenticated TUNNEL_ATTACH to the existing Logical Tunnel
  -> prove bidirectional candidate health
  -> temporarily add candidate to the race set
  -> same PacketID may race over old + candidate
  -> first arrival delivers once; duplicate is suppressed
  -> old lane DRAINING
  -> stop new sends to old lane
  -> retire old lane after a short bounded drain/overlap
```

Candidate failure leaves the healthy old lane untouched and schedules a later retry/backoff.

Do not define a blind fixed five-second duplication interval as a wire invariant. A simple bounded overlap is acceptable for the first implementation; later qualification may make it RTT-aware or barrier-based.

Game mode normally has more than one healthy lane. Rotate one lane at a time: for example `A+B -> A+B+C -> B+C`. The logical PacketID/dedup layer makes the temporary extra copy safe. Never intentionally rotate all game lanes together.

### 9. One replacement state machine handles path change and failure

The client Transport Manager uses one replace mechanism for:

- scheduled age rotation;
- Windows network-interface/default-route/public-IP change;
- NAT mapping/path failure;
- missed-PONG / no-valid-RX dead-path detection;
- FakeTCP, DTLS or LINK child failure;
- server-requested replacement;
- manual reconnect.

Windows network-change notifications may trigger replacement immediately instead of waiting for heartbeat timeout. Heartbeat liveness remains a fallback detector.

Lane generations/epochs fence stale paths. Once a newer generation is committed, an older resurrected transport may drain briefly if authorized but can never reclaim active ownership.

### 10. Liveness and long-lived logical VPN behavior

Keepalive remains useful for a currently active lane, but it is liveness/NAT-maintenance traffic, not logical user activity. Healthy lanes can therefore remain alive through short quiet periods, close after the configured payload-idle timeout, and rotate before becoming excessively long-lived.

The product goal is a long-lived logical VPN, not an immortal FakeTCP 4-tuple.

### 11. Server restart is not transparent-mobility scope

A server restart may destroy host conntrack/NAT state. WBD does not promise to preserve every application TCP flow across server restart. The continuity goal is short underlay failure, network change, lane failure and planned rotation while the server-side logical tunnel/NAT state remains available.

### 12. Startup-latency optimization is deferred

Do not combine this architecture pivot with startup-RTT optimization. DTLS HRR-cookie removal, LINK bind/init coalescing, abbreviated resume admission and other handshake-latency work remain backlog items until the new logical-tunnel/shared-TUN/multipath lifecycle is functionally qualified.

## Preserved hard release constraints

ADR-0012 does **not** relax these constraints:

- WBD outer payload carrier remains WBD-owned raw TCP-shaped FakeTCP; no ordinary kernel-TCP sustained payload path;
- no TCP-over-TCP HOL dependency;
- temporary reliable ordering is bootstrap-only inside each FakeTCP association;
- pinned wolfSSL DTLS 1.3 remains steady-state cryptographic authority;
- FEC release wire remains `off` or fixed systematic `20:20` unless a later ADR reopens it;
- `legacy` FakeTCP recovery remains the default; `sack-rack` remains experimental;
- weak-link qualification ceiling remains <=100 Mbit/s and conservative release operating point remains 40 Mbit/s aggregate inner payload;
- Windows product capture remains raw L3 through Wintun/TUN;
- OpenWrt final capture remains TPROXY + policy routing;
- Linux server product exposes one public WBD port; internal services remain private implementation details;
- Linux firewall management is WBD-owned and must not flush or replace the user's host ruleset;
- Windows IPv6 remains fail-closed for the full connected interval until a real IPv6 tunnel path is qualified;
- Disconnect/Exit must restore WBD-owned route, DNS/NRPT, IPv6 and firewall state;
- Npcap release/licensing constraints remain in force; WBD does not improperly redistribute the Free Edition;
- credentials, passwords, tickets, resume secrets and other identities/secrets must not be logged;
- non-secret diagnostic correlation IDs, first-boundary markers and counters are retained without per-packet INFO spam;
- Windows child-process slimming/refactoring is a separate later workstream.

## Qualification gates added by this ADR

Before this architecture can become release-acceptable, automated qualification must prove at minimum:

1. two logical tunnels receive different leased IPs;
2. both can simultaneously bind the same inner TCP source port `40000` and reach the same Internet target/port;
3. DNS-style UDP, generic UDP and TCP pass through shared TUN + one host NAT;
4. a tunnel attempting to inject a source address belonging to another lease is rejected;
5. disconnect/reconnect and lease reuse/cleanup are deterministic;
6. non-zero idle timeout closes transport lanes without destroying the logical tunnel, and a new packet wakes it;
7. scheduled `A -> B` replacement is make-before-break and does not duplicate downstream payload;
8. game/race mode keeps its desired lane redundancy while replacing only one lane at a time;
9. candidate failure leaves the old healthy lane usable;
10. network-change and heartbeat-dead-path events enter the same replacement state machine;
11. FEC `off` and fixed `20:20` remain qualified with lane-local state;
12. full stack works through FakeTCP -> Reality-like bootstrap -> DTLS -> LINK -> race/raw-IP -> shared TUN -> Internet;
13. final physical Windows 11 -> Ubuntu ARM64 qualification passes DNS/UDP/TCP, cleanup and the preserved hard release constraints.

## Superseded clauses

ADR-0012 supersedes current-product text that says any of the following:

- one entire VPN session must use one public 4-tuple until Disconnect;
- `one raw lane` is a permanent architectural requirement rather than the normal-mode steady baseline;
- LiveID/FakeTCP association owns the tunnel IP identity;
- all Windows clients remain `10.66.0.2/30`;
- per-LiveID Linux netns + veth + two NAT layers is the selected final product raw-IP gateway;
- later Game Lane research is equivalent to the rejected V1 ordinary-TCP multilane architecture.

Historical benchmark/dev-log evidence remains valid as history. Conflicting current-policy clauses must point to this ADR instead of silently coexisting.
