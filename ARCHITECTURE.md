# Architecture v2.x

> **Status: ACTIVE MAINLINE DESIGN. ADR-0012 is authoritative for Logical Tunnel identity, 1..4 Transport Lanes, Game/race and lifecycle. ADR-0011 controls same-association Reality-like setup inside each lane.**

## Core model

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned IPv4 lease
          -> shared SessionID / PacketID race namespace
          -> 1..4 independent replaceable Transport Lanes
```

`single-flow` applies to **each independent Transport Lane**. It is not a global one-flow-per-Logical-Tunnel-lifetime rule.

Every lane owns:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap on the SAME association
  -> protected account + Logical Tunnel admission
  -> explicit in-band barrier
  -> no FIN/RST/reconnect/new WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality WBD connection. Sustained outer WBD payload never becomes an ordinary kernel TCP byte stream.

## Game/race

**Game/race operates above independent complete WBD lanes.** Normal mode targets one lane; Game/weak-network mode targets 2..4.

One logical payload has one PacketID. Copies may race over lanes. **The first valid arrival is delivered once**; later copies are suppressed. Bounded out-of-order unique packets deliver independently, so there is no cross-lane HOL. FEC is lane-local and never spans lanes.

## Identity and lease

TunnelID and server-assigned tunnel IPv4 belong to the Logical Tunnel/device installation, not to a lane. Same-account different installations receive distinct active leases. Short lane replacement should preserve the lease where possible.

Raw IPv4 ingress enforces `inner source == leased tunnel IPv4`; mismatch is a hard anti-spoof drop.

## Make-before-break replacement

```text
A ACTIVE
  -> build B completely
  -> authenticate/attach B to the same Logical Tunnel
  -> health gate
  -> A+B bounded race with existing PacketID/dedup
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves A active. Game rotates one lane at a time, e.g. `A+B -> A+B+C -> B+C`.

Every lane incarnation has generation/epoch fencing. Stale FakeTCP RX, DTLS/LINK callbacks, timers, goroutines and candidates must not mutate newer state.

Age, NIC/default-route/public-IP/NAT changes, missed PONG/no RX, FakeTCP/DTLS/LINK failure, server request and manual reconnect converge on one replacement lifecycle.

## Idle / DORMANT / wake

`last_payload_activity` is real tunneled payload only. PING/PONG/control update transport liveness but do not refresh payload idle.

Default payload-idle guidance is 15 minutes. `idle_timeout=0` disables idle-induced sleep only, not lane age rotation.

DORMANT closes all lanes while preserving Logical Tunnel, lease, Wintun, routes and DNS/NRPT. First real payload wakes the tunnel; first READY lane resumes traffic and optional Game lanes attach afterward.

Explicit Disconnect/Exit is different and deterministically cleans WBD-owned routes, DNS/NRPT, IPv6 fail-closed rules, firewall/runtime state and releases lease according to policy.

Lane soft age is initially randomized around 30..60 minutes and staggered across active lanes.

## Windows stack

```text
one Wintun / raw L3
  -> Logical Tunnel / lease / PacketID domain
  -> Game/race aggregation
  -> Lane A: LINK -> DTLS -> Reality-like bootstrap + FakeTCP
  -> Lane B: LINK -> DTLS -> Reality-like bootstrap + FakeTCP
  -> optional Lane C/D
```

Each lane uses an independent source port/public tuple. Windows IPv6 remains fail-closed through ACTIVE/DORMANT/replacement until qualified.

## Linux stack

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux
       <-> independent lane tuples
       <-> per-lane DTLS + LINK
       <-> Logical Tunnel manager + Game/race
       <-> one shared WBD TUN
       <-> Linux root routing
       <-> one WBD-owned host NAT/SNAT
```

One public server port does not mean one lane per tunnel. Per-session netns/veth/double NAT and VRF/conntrack-zone remain historical/reference only. Firewall helpers manipulate WBD-owned state only.

## Frozen boundaries

- mature FakeTCP ACK/SACK/RTO/recovery remains frozen; `legacy` default, `sack-rack` experimental;
- pinned wolfSSL DTLS 1.3 stays crypto authority;
- FEC only `off` or fixed systematic `20:20`, lane-local and immutable per lane/epoch;
- source packets do not wait merely to fill an FEC block;
- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release point across the Logical Tunnel, not per lane;
- Windows Wintun/raw L3 and OpenWrt TPROXY + policy routing remain final capture directions;
- one public WBD server port;
- deterministic WBD-owned cleanup and no secret/per-packet INFO logging;
- startup RTT redesign, DTLS HRR optimization, LINK bootstrap coalescing, Windows child slimming and new FEC ratios are deferred.

## Qualification

One exact substantive SOURCE_SHA must prove per-lane same-association bootstrap/no-HOL; active counts 1..4 and fifth rejection; Normal=1 and Game=2..4; race/dedup/no-cross-lane-HOL; `A -> A+B -> B` with candidate-failure preservation; one-lane-at-a-time Game rotation; generation fencing; DORMANT/wake; unified triggers; distinct leases/source anti-spoof; shared-TUN DNS/UDP/TCP; FEC off/20:20; exact-head Windows/Linux gates and same-source artifacts; then physical Windows 11 + Npcap -> Ubuntu ARM64 lifecycle/cleanup qualification.

## Retired / invalid product shapes

- ordinary kernel TCP as sustained outer WBD carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- global `MaxProductPublicTransportLanes = 1`;
- Game/multipath disabled or research-only;
- planned break-before-make replacement of a healthy lane;
- rejecting a legitimate second complete lane;
- cross-lane FEC or a second migration PacketID space;
- per-LiveID netns/veth/double NAT as final product.
