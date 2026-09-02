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
| V2-M7 | Windows Wintun raw-L3 + Npcap underlay | **MULTI-LANE EXECUTION RESTORED / REQUALIFY** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED / REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9C | product 1..4-lane admission + Game/race wiring | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9D | payload-idle dormant/wake | **IMPLEMENTED: AUTOMATIC PAYLOAD-IDLE DORMANT + FIRST-PAYLOAD/FIRST-READY WAKE; NON-IDLE TRIGGERS PENDING** |
| V2-M9E | make-before-break + unified lane replacement state machine | **BOUNDED SAME-LANEID OLD+CANDIDATE RACE + RANDOMIZED STAGGERED 30..60M AGE ROTATION IMPLEMENTED; UNIFIED TRIGGERS PENDING** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED UNTIL ONE EXACT-SOURCE MATRIX + ARTIFACTS + PHYSICAL EVIDENCE** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## 2026-09-02 live reconciliation

The takeover baseline is `0d65698d1601951169a807d94c0eaa8c09c6531f`, 16 commits ahead of stale handoff checkpoint `c7a0622352889ff8906db940b3e1e2bb5df3d6b1`. Current code/tests, not the stale continuation cursor, are authoritative for completion state:

- same logical LaneID old+candidate bounded Game race is implemented; candidate failure preserves the old transport;
- product logical-lane ceiling remains 1..4 while bounded replacement has private `4+1` physical-incarnation capacity;
- real app/TUN payload activity is tracked independently of control traffic;
- automatic payload-idle DORMANT and payload-triggered wake are implemented, including first-READY publication before optional lane refill;
- randomized per-incarnation 30..60m lane-age deadlines are implemented in controller policy, collision-deconflicted by at least one minute and serialized through the existing `ReplaceLane` path;
- the remaining lifecycle work is unified replacement triggers for network/path/liveness/transport failures and explicit server/manual triggers.

CI green at any one push is not release readiness. V2-M10 still requires one exact substantive source across the complete release matrix, Windows/Linux artifacts, and physical Windows 11 + Npcap -> Ubuntu ARM64 evidence.

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
- Architectural ceiling = 4 logical lanes.
- Dormant/disconnected = 0.

A fifth logical lane is rejected. Planned replacement may briefly overlap old/candidate physical incarnations under ADR-0012 make-before-break, with an architectural maximum of four logical lanes plus one bounded private replacement incarnation.

The mature TCP-like/FakeTCP recovery/FEC core is frozen unless a deterministic lower-layer qualification isolates a real defect.

## Game/race and replacement

Game/race is product behavior. Same logical PacketID may be copied through independent complete WBD lanes; first valid arrival wins, duplicates are suppressed, and bounded out-of-order unique packets deliver independently without cross-lane HOL.

Planned healthy replacement is:

```text
A -> build/qualify B -> A+B bounded race -> drain A -> B
```

Candidate B failure leaves A alive. The bounded race is implemented with old and candidate physical incarnations carrying the same logical LaneID/PacketID namespace; it is no longer a pending gap.

Game replacement rotates one lane at a time:

```text
A+B -> A+B+C -> B+C
```

Randomized 30..60m soft age rotation now converges on this same replacement lifecycle: each active incarnation gets its own randomized deadline, multi-lane deadlines are deconflicted by at least one minute, only the earliest due lane is replaced at a time, candidate failure preserves the old lane and schedules a bounded retry, and a successful 1<->5 physical-slot promotion receives a fresh age deadline. The remaining unified-trigger work is NIC/default route/public IP changes, NAT/path/liveness failure, FakeTCP/DTLS/LINK failure, server request and manual reconnect, all with the existing generation fencing. This is lifecycle work, not transport-wire work.

## Logical Tunnel identity + lease

The Logical Tunnel, not a lane/LiveID, owns stable TunnelID and the server-assigned IPv4 lease. Same-account devices receive distinct addresses; raw IPv4 ingress is accepted only when its source matches the lease.

Do not reintroduce a global hard-coded `10.66.0.2/30` identity.

## Idle / wake / age policy

- Product default payload idle timeout: **15m (900s)** when `idle_timeout` is omitted.
- Explicit `idle_timeout=0`: never sleep due to payload idleness; lifecycle monitoring and lane-age rotation remain enabled.
- Track `last_payload_activity` separately from `last_transport_activity`.
- Only real application/TUN payload refreshes payload idle; PING/PONG/control does not.
- DORMANT closes all lanes but preserves Logical Tunnel, lease, Wintun/routes/DNS.
- First new payload wakes the tunnel; forwarding resumes on the first READY lane, then optional Game lanes refill incrementally.
- Each active lane incarnation has an independent randomized 30..60m soft age deadline. Multi-lane deadlines are staggered/deconflicted, DORMANT clears age state, wake rebuilds fresh deadlines, and replacements run one lane at a time through existing make-before-break.

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

One Wintun belongs to one Logical Tunnel. Product execution now admits logical lanes 1..4 plus a private replacement transport slot that does not become a fifth logical Game lane:

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

Normal mode uses one lane; Game/weak-network may use 2..4; replacement may overlap old/candidate transports. DORMANT wake publishes the first READY lane immediately and then refills later Game lanes. There is no preliminary ordinary kernel-TCP Reality WBD connection.

The randomized/staggered 30..60m lane-age scheduler is now implemented in the Windows controller without touching FakeTCP/Reality/DTLS/LINK/FEC wire. The remaining Windows lifecycle gap is convergence of NIC/default-route/public-IP/NAT changes, missed-PONG/no-RX, transport failure, server request and manual reconnect onto the existing generation-fenced replacement path.

## Frozen weak-network/release limits

- no ordinary kernel-TCP sustained WBD payload;
- no TCP-over-TCP HOL;
- pinned wolfSSL DTLS 1.3;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- FEC `off` or fixed systematic `20:20`, always lane-local;
- functional/lifecycle qualification prioritizes FEC `off`; `20:20` is compatibility smoke only for this lifecycle phase, with parameter research deferred;
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
4. 1,2,3,4 active lanes accepted and fifth logical lane rejected while bounded replacement respects the 4+1 physical-incarnation ceiling;
5. normal desired=1, Game/weak desired=2..4;
6. Game first-arrival/dedup/out-of-order unique/no-cross-lane-HOL;
7. `A -> A+B -> B`, candidate failure preserving A, `A+B -> A+B+C -> B+C`, and staggered 30..60m age replacement through the same lifecycle;
8. distinct leases, source-spoof rejection, shared TUN + one NAT DNS/UDP/TCP;
9. lifecycle/functionality with FEC `off`, plus fixed `20:20` lane-local compatibility smoke; FEC parameter research is deferred;
10. Windows native/runtime and Linux raw/full-stack gates;
11. Windows portable and Linux amd64/arm64 artifacts from that exact `SOURCE_SHA`;
12. final clean physical Windows 11 + Npcap -> Ubuntu ARM64 DNS/UDP/TCP/lifecycle + deterministic cleanup.

A direct push CI-green result is necessary but not sufficient. The complete release-qualification matrix must finish on the same `SOURCE_SHA`, artifacts must be attributable to that source, and physical evidence must be fresh and exact-source before release-ready may be claimed.

## Deferred

- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps;
- FEC parameter research beyond fixed `20:20` compatibility smoke.
