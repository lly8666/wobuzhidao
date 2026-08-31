# ADR-0012: Stable logical tunnel, server-assigned address lease, and replaceable multipath transports

Status: **ACCEPTED / REAFFIRMED 2026-08-31 / SUPERSEDES ADR-0013 GLOBAL-SINGLE-TRANSPORT CLAUSES**

## Context

WBD has proved three important but previously separate ideas:

1. a single WBD FakeTCP association can own its own complete lineage from SYN through Reality-like TLS bootstrap, DTLS 1.3, LINK/FEC and packet payload without using an ordinary kernel-TCP payload stream;
2. `internal/gamelane` can copy one logical `PacketID` across 1..4 independent complete WBD associations and deliver the first arrival while suppressing later duplicates, with bounded out-of-order acceptance and no cross-lane HOL;
3. a long-lived user-visible VPN must not tie identity/address continuity to one immortal public 4-tuple.

ADR-0013 temporarily changed the release rule to exactly one public transport for the entire connected Logical Tunnel and break-before-make replacement. The product owner explicitly rejected that change on 2026-08-31 and reaffirmed the original ADR-0012 rule: **1..4 lanes, Game Lane, and make-before-break are the current product architecture.**

The essential reconciliation is:

- **single-flow is a per-lane invariant**;
- **multipath is a Logical Tunnel invariant**;
- every lane begins with one raw FakeTCP SYN lineage, carries bounded Reality-like real TLS 1.3 setup on that same association, crosses an explicit barrier without FIN/new SYN, and then carries DTLS/LINK/FEC/raw-IP without ordinary kernel-TCP HOL;
- a Logical Tunnel may own 1..4 such independent lanes according to policy.

## Decision

### 1. Separate logical tunnel identity from transport identity

The product model is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> one stable server-assigned tunnel address lease
          -> one stable race SessionID / logical PacketID space while the tunnel exists
          -> 1..4 replaceable Transport Lanes while active
          -> zero lanes only while dormant/disconnected
```

A `Logical Tunnel` is the user-visible VPN session. A `Transport Lane` is disposable.

A lane owns one complete independent WBD transport epoch:

```text
raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable Reality-like TLS 1.3 bootstrap on that same association
  -> explicit bootstrap barrier; no FIN and no second WBD payload SYN
  -> DTLS 1.3
  -> LINK
  -> lane-local fixed FEC state
  -> packet/datagram payload
```

`LiveID`, FakeTCP 4-tuple, DTLS state and LINK state belong to a lane/transport epoch. They do **not** own the tunnel address lease.

### 2. The single-flow invariant applies per lane, not per whole VPN

For every lane/transport epoch:

- exactly one public FakeTCP association owns Reality-like bootstrap through steady payload for that lane;
- no preliminary ordinary kernel-TCP Reality connection is allowed;
- no `Reality TCP -> close -> new FakeTCP payload connection` shortcut is allowed;
- no ordinary kernel TCP byte stream may own sustained WBD payload;
- post-bootstrap delivery remains datagram/packet oriented and must preserve the no-HOL gate.

The whole Logical Tunnel may have more than one lane. Normal mode targets one steady lane. Game/weak-network race mode may keep 2..4 independent lanes. Controlled migration may temporarily add a candidate lane before retiring an old one.

This does not revive V1 PR #2. V1 was rejected because redundancy/FEC sat above ordinary ordered kernel TCP lanes. Current Game Lane uses independent WBD FakeTCP/DTLS/LINK associations, one logical PacketID across lanes, first-arrival delivery and duplicate suppression, with no cross-lane ordered-delivery dependency.

### 3. Game Lane semantics are the general multipath/replacement layer

The existing `internal/gamelane` semantics are the architectural foundation:

- one logical payload gets one `PacketID`;
- copies may be emitted on 1..4 independent lanes;
- copies have lane-distinct envelopes before DTLS;
- each lane has its own FakeTCP tuple/sequence space, DTLS keys/nonces, LINK and FEC state;
- the first valid copy is delivered immediately;
- later copies of the same `PacketID` are suppressed;
- bounded out-of-order unique packets remain independently deliverable;
- there is no cross-lane HOL.

FEC remains lane-local. A FEC block must never be split across lanes or wait on another lane.

### 4. Server assigns a unique tunnel address lease

The server allocates each active Logical Tunnel/device a unique IPv4 tunnel address from a configurable pool. The initial implementation may use a configurable private pool such as `10.66.0.0/16`, but no address range is claimed to be universally collision-free.

The client must not hard-code `10.66.0.2/30` as a global identity. Address/prefix/route parameters are supplied by authenticated tunnel configuration. `/32` plus explicit routes is preferred where platform qualification supports it.

The address lease is stable across lane rotation and short reconnect/replacement. Same-account devices receive distinct addresses.

Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

Future IPv6 support applies the same rule to an assigned `/128`.

### 5. Linux raw-IP data plane is shared TUN + one host NAT

The selected Windows raw-L3 server direction is:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> race/multipath lanes
```

Upstream raw packets are authenticated to a Logical Tunnel, source-validated against its lease, then written to the shared TUN. Downstream packets read from the shared TUN are demultiplexed by leased destination address to the owning Logical Tunnel and emitted through that tunnel's active lane set.

The per-session Linux netns/veth/double-NAT implementation is historical/correctness evidence, not final product architecture. The earlier VRF/conntrack-zone prototype remains rejected.

### 6. Idle transport policy is based on real payload activity

Client policy exposes an idle transport timeout. Initial guidance is 15 minutes, with configurable values and `0` meaning **never sleep because of payload idleness**.

Track at least:

- `last_payload_activity`: real tunneled IPv4/IPv6 payload only;
- `last_transport_activity`: payload plus PING/PONG/control used for liveness.

PING/PONG/control must not reset the user-visible payload-idle timer.

When a non-zero idle timeout expires, close all active lanes but keep the Logical Tunnel, leased IP, Wintun and capture/routing/DNS state and enter `DORMANT`. A new raw-IP packet wakes the tunnel; the first healthy lane may resume traffic before optional redundant lanes finish establishing.

### 7. Transport age rotation is independent of idle policy

Every active lane has an independent maximum-age/rotation policy. Initial experimental guidance is a randomized 30..60 minute soft deadline. Multi-lane mode staggers replacement and must not intentionally rotate all healthy lanes together.

### 8. Replacement is make-before-break and reuses race/dedup

```text
old lane ACTIVE
  -> create candidate lane
  -> FakeTCP + same-lane Reality-like bootstrap + DTLS + LINK
  -> authenticated TUNNEL_ATTACH to the existing Logical Tunnel
  -> prove bidirectional candidate health
  -> temporarily add candidate to the race set
  -> same PacketID may race over old + candidate
  -> first arrival delivers once; duplicate is suppressed
  -> old lane DRAINING
  -> stop new sends to old lane
  -> retire old lane after a short bounded drain/overlap
```

Candidate failure leaves the healthy old lane untouched. Game mode rotates one lane at a time, for example `A+B -> A+B+C -> B+C`.

### 9. One replacement state machine handles path change and failure

Use one replacement mechanism for scheduled rotation, network/default-route/public-IP change, NAT/path failure, liveness failure, FakeTCP/DTLS/LINK child failure, server-requested replacement and manual reconnect. Lane generations fence stale paths.

### 10. Liveness and long-lived logical VPN behavior

Keepalive is lane liveness/NAT-maintenance traffic, not logical user activity. The product goal is a long-lived Logical Tunnel, not an immortal FakeTCP 4-tuple.

### 11. Server restart is not transparent-mobility scope

A server restart may destroy host conntrack/NAT state. WBD does not promise preservation of arbitrary application TCP flows across server restart.

### 12. Startup-latency optimization remains deferred

Do not combine this architecture correction with DTLS HRR removal, LINK bind/init coalescing, abbreviated resume admission or Windows child-process slimming.

## Preserved hard release constraints

- WBD outer payload carrier remains WBD-owned raw TCP-shaped FakeTCP; no ordinary kernel-TCP sustained payload path;
- no TCP-over-TCP HOL dependency;
- temporary reliable ordering is bootstrap-only inside each FakeTCP lane;
- pinned wolfSSL DTLS 1.3 remains steady-state cryptographic authority;
- FEC release wire remains `off` or fixed systematic `20:20` unless a later ADR reopens it;
- `legacy` FakeTCP recovery remains the default; `sack-rack` remains experimental;
- <=100 Mbit/s weak-link qualification ceiling and 40 Mbit/s aggregate-inner conservative release point remain;
- Linux server exposes one public WBD port; multiple lanes are independent 4-tuples to that same port;
- Windows Wintun/raw-L3, IPv6 fail-closed interval, scoped cleanup and Npcap licensing constraints remain;
- secrets must not be logged;
- mature TCP-like/FakeTCP recovery is frozen unless deterministic qualification proves a defect below this lifecycle layer.

## Qualification gates

Before artifact delivery, exact-head automation must prove at minimum:

1. each lane has one SYN lineage through Reality-like TLS bootstrap -> barrier -> DTLS -> LINK -> payload, with no second WBD payload SYN;
2. a connected Logical Tunnel accepts 1..4 active lanes and rejects a fifth;
3. Game/race first-arrival delivery and duplicate suppression work across independent lanes without cross-lane HOL;
4. scheduled `A -> A+B -> B` replacement is make-before-break and does not duplicate delivered payload;
5. candidate failure leaves the old healthy lane usable;
6. Game mode replaces one lane at a time while retaining desired redundancy;
7. distinct Logical Tunnels receive distinct leased IPs and remain isolated;
8. raw-IP source spoofing across leases is rejected;
9. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
10. FEC `off` and `20:20` remain lane-local and qualified;
11. exact-head Windows and Linux builds/full-stack tests pass before physical Windows 11 -> Ubuntu ARM64 qualification.

## Supersession note

ADR-0013's global `public transport count = 1`, Game Lane retirement and break-before-make clauses are **withdrawn**. Historical evidence from that experiment remains useful, but it is not current product policy.
