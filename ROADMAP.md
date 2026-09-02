# Roadmap

> **Status: PER-LANE SAME-FLOW + LOGICAL TUNNEL MULTIPATH ACTIVE.** ADR-0011 controls each lane; ADR-0012 controls Logical Tunnel 1..4-lane lifecycle. ADR-0014 is withdrawn/invalidated.

## Milestone map

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | raw FakeTCP + weak-network external baseline | **DONE** |
| V2-M2 | pinned wolfSSL DTLS 1.3 | **DONE** |
| V2-M3 | immutable LINK/control foundation | **DONE AS FOUNDATION** |
| V2-M4 | no-HOL FakeTCP/FEC qualification | **DONE / MUST REMAIN GREEN** |
| V2-M5 | Game/race 1..4-lane first-arrival/dedup | **PRODUCT FOUNDATION / REQUALIFY** |
| V2-M6 | Reality-like TLS bootstrap on each lane's same FakeTCP association | **IMPLEMENTED / MUST REMAIN PER-LANE** |
| V2-M7 | Windows Wintun raw-L3 + Npcap underlay | **IMPLEMENTED / RESTORE MULTI-LANE ORCHESTRATION** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED / REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9C | product 1..4-lane admission + Game/race wiring | **ACTIVE ROLLBACK REPAIR** |
| V2-M9D | payload-idle dormant/wake | **ACTIVE AFTER CARDINALITY REPAIR** |
| V2-M9E | make-before-break + unified lane replacement state machine | **ACTIVE AFTER CARDINALITY REPAIR** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED UNTIL SAME-HEAD AUTOMATION GREEN** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## Frozen product transport model

`single-flow` is per Transport Lane:

```text
Logical Tunnel
  -> stable TunnelID / server lease / PacketID race domain
  -> 0..4 independent Transport Lanes

Each lane:
  one raw FakeTCP SYN lineage / public 4-tuple / sequence space
    -> bounded reliable Reality-like real TLS 1.3 bootstrap on the same association
    -> explicit barrier: no FIN / RST / reconnect / new WBD payload SYN inside that lane
    -> pinned wolfSSL DTLS 1.3
    -> LINK
    -> lane-local FEC off or fixed 20:20
    -> packet/datagram payload without ordinary kernel-TCP HOL
```

Policy:

- Normal steady desired lanes = 1.
- Game / weak-network desired lanes = 2..4.
- Architectural ceiling = 4.
- Dormant/disconnected = 0.

A fifth lane is rejected. Planned replacement may briefly overlap old/candidate lanes under ADR-0012 make-before-break.

The mature TCP-like/FakeTCP recovery/FEC core is frozen unless a deterministic lower-layer qualification isolates a real defect.

## Game/race and replacement

Game/race is product behavior. Same logical PacketID may be copied through independent complete WBD lanes; first valid arrival wins, duplicates are suppressed, and bounded out-of-order unique packets deliver independently without cross-lane HOL.

Planned healthy replacement is:

```text
A -> build/qualify B -> A+B bounded race -> drain A -> B
```

Candidate B failure leaves A alive.

Game replacement rotates one lane at a time:

```text
A+B -> A+B+C -> B+C
```

A unified replacement lifecycle covers age rotation, NIC/default route/public IP changes, NAT/path/liveness failure, FakeTCP/DTLS/LINK failure, server request and manual reconnect, with generation fencing.

## Logical Tunnel identity + lease

The Logical Tunnel, not a lane/LiveID, owns stable TunnelID and the server-assigned IPv4 lease. Same-account devices receive distinct addresses; raw IPv4 ingress is accepted only when its source matches the lease.

Do not reintroduce a global hard-coded `10.66.0.2/30` identity.

## Idle / wake / age policy

- Default payload-idle guidance: 15m.
- `0`: never sleep due to payload idleness.
- Track `last_payload_activity` separately from `last_transport_activity`.
- PING/PONG/control does not refresh payload idle.
- DORMANT closes all lanes but preserves Logical Tunnel, lease, Wintun/routes/DNS.
- First new packet establishes the first healthy lane, then optional Game lanes refill.
- Each lane has an independent experimental randomized 30..60m soft age deadline; multi-lane rotation is staggered.

## Shared Linux TUN + one NAT

```text
Internet
  <-> one WBD public raw FakeTCP mux port
        <-> 1..4 independent lanes per Logical Tunnel as policy requires
        <-> per-lane DTLS/LINK
        <-> tunnel Game/race layer
        <-> shared WBD TUN
        <-> Linux root routing
        <-> one WBD-owned host NAT/SNAT
```

One public server port is not a one-lane-per-tunnel restriction.

Exit gates: distinct Logical Tunnels get distinct leases, identical inner tuples remain isolated, DNS/UDP/TCP pass, source spoofing is rejected, and firewall changes remain WBD-scoped.

## Windows product orchestration

One Wintun belongs to one Logical Tunnel. Restore product use of existing lane planning:

```text
Logical Tunnel
  -> 1..4 LaneBootstrap
      -> independent source port + FakeTCP child
      -> same-lane Reality-like TLS bootstrap
      -> DTLS
      -> LINK
  -> Game/race
  -> one Wintun
```

Normal mode uses one lane; Game/weak-network may use 2..4; replacement may overlap old/candidate lanes. There is no preliminary ordinary kernel-TCP Reality WBD connection.

## Frozen weak-network/release limits

- no ordinary kernel-TCP sustained WBD payload;
- no TCP-over-TCP HOL;
- pinned wolfSSL DTLS 1.3;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- FEC `off` or fixed systematic `20:20`, always lane-local;
- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- Windows Wintun raw L3;
- OpenWrt TPROXY + policy routing;
- WBD-scoped Linux firewall ownership;
- Windows IPv6 connected interval fail-closed until IPv6 qualification;
- deterministic Disconnect/Exit cleanup;
- Npcap packaging/licensing constraints unchanged;
- startup-latency optimization and Windows child-process slimming deferred.

## V2-M10 exact-head release gate

One exact substantive `SOURCE_SHA` must prove:

1. per-lane one-SYN same-association Reality-like real TLS 1.3 bootstrap;
2. no FIN/RST/reconnect/new WBD payload SYN inside a lane at bootstrap -> DTLS;
3. post-bootstrap no-HOL hole-bypass;
4. 1,2,3,4 active lanes accepted and fifth rejected;
5. normal desired=1, Game/weak desired=2..4;
6. Game first-arrival/dedup/out-of-order unique/no-cross-lane-HOL;
7. `A -> A+B -> B`, candidate failure preserving A, and `A+B -> A+B+C -> B+C`;
8. distinct leases, source-spoof rejection, shared TUN + one NAT DNS/UDP/TCP;
9. FEC `off` and `20:20` lane-local qualification;
10. Windows native/runtime and Linux raw/full-stack gates;
11. Windows portable and Linux amd64/arm64 artifacts from that exact HEAD;
12. final clean physical Windows 11 + Npcap -> Ubuntu ARM64 DNS/UDP/TCP + deterministic cleanup.

Until same-head automated gates are green, do not designate a new physical-test artifact as a release candidate.

## Deferred

- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps.
