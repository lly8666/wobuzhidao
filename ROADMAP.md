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
| V2-M9D | payload-idle dormant/wake | **IMPLEMENTED: AUTOMATIC PAYLOAD-IDLE DORMANT + FIRST-PAYLOAD/FIRST-READY WAKE** |
| V2-M9E | make-before-break + unified lane replacement state machine | **AGE + PROCESS/LINK LIVENESS + LOCAL WINDOWS PATH TRIGGERS IMPLEMENTED; EXPLICIT SERVER/MANUAL + ROUTE-REBIND/NAT OBSERVABILITY GAPS REMAIN** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED UNTIL ONE EXACT-SOURCE MATRIX + ARTIFACTS + PHYSICAL EVIDENCE** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## 2026-09-03 lifecycle checkpoint

Substantive source `fef0820d63736e037a790a87ef4de9e35ce39e26` completes the current local Windows underlay/path-change phase. All 16 ordinary push workflows observed on the PR branch for that exact SHA completed successfully with queued=0, in_progress=0 and failure=0. This is exact-source lifecycle evidence, **not** release authorization.

Current code/tests establish:

- same logical LaneID old+candidate bounded Game race; candidate failure preserves the old transport;
- product logical-lane ceiling 1..4 with bounded 4+1 physical-incarnation capacity;
- the physical spare is a rotating token across slots 1..5, so sequential multi-lane replacement does not assume slot 5 is permanently free;
- real app/TUN payload activity is separate from control traffic; PING/PONG/control do not refresh payload idle;
- automatic payload-idle DORMANT and payload-triggered wake, including first-READY publication;
- randomized per-incarnation 30..60m lane-age deadlines, staggered and serialized through the same replacement lifecycle;
- authoritative FakeTCP/DTLS/LINK child exit and LINK no-RX/missed-PONG terminate the current incarnation and converge through replacement;
- connected Windows path observation detects changes in source IPv4, Npcap packet device/NIC, source MAC, and next-hop MAC/default-route identity;
- connected physical-path discovery explicitly excludes WBD's own pinned server escape route so the monitor does not self-lock onto the pre-change route;
- path convergence replaces current logical lanes one at a time with expected-generation fencing; discovery failure is fail-open, candidate failure preserves both healthy A and the last-known-good underlay baseline;
- DORMANT does not perform path replacement; Wake rediscovers the physical underlay before rebuilding lanes.

Remaining lifecycle/orchestration gaps are narrower and explicit:

1. `RouteForeign`/direct-prefix kernel routes recorded at initial apply still need a safe physical-route rebind strategy after an underlay change; raw WBD lanes may already have migrated while those direct routes still reference the old physical path.
2. Explicit server-request/manual-reconnect triggers still need to converge on the same generation-fenced replacement API; do not invent a second replacement state machine.
3. Public external-IP/NAT mapping change detection remains unimplemented because the current local underlay observer has no authoritative public-NAT signal. Do not fake this by relabeling local source/default-route changes.

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

A fifth logical lane is rejected. Planned replacement may briefly overlap old/candidate physical incarnations under ADR-0012 make-before-break. Physical transport slots are bounded to 1..5; only four are authoritative at once and the one unused slot is the make-before-break spare token. Promotion retires the old physical slot, which becomes the next spare. None of these physical slots creates a fifth logical LaneID or a second PacketID namespace.

The mature TCP-like/FakeTCP recovery/FEC core is frozen unless deterministic lower-layer qualification isolates a real defect.

## Game/race and replacement

Game/race is product behavior. Same logical PacketID may be copied through independent complete WBD lanes; first valid arrival wins, duplicates are suppressed, and bounded out-of-order unique packets deliver independently without cross-lane HOL.

Planned healthy replacement is:

```text
A -> build/qualify B -> A+B bounded race -> drain A -> B
```

Candidate B failure leaves A alive. Age, process-exit, LINK liveness, and local Windows underlay/path triggers now converge on this same generation-fenced lifecycle rather than duplicating break-before-make logic.

Randomized 30..60m soft age rotation assigns each active incarnation its own deadline, deconflicts multi-lane deadlines by at least one minute, replaces only one due lane at a time, retries candidate failure with a bounded delay, and gives each promoted incarnation a fresh age deadline.

## Idle / wake / liveness policy

- Product default payload idle timeout: **15m (900s)** when `idle_timeout` is omitted.
- Explicit `idle_timeout=0`: never sleep due to payload idleness; lifecycle monitoring and lane-age rotation remain enabled.
- Only real application/TUN payload refreshes payload idle; PING/PONG/control does not.
- DORMANT closes all lanes but preserves Logical Tunnel, lease, Wintun/routes/DNS.
- First new payload wakes the tunnel; forwarding resumes on the first READY lane, then optional Game lanes refill incrementally.
- Each active lane incarnation has an independent randomized 30..60m soft age deadline.
- Current authoritative FakeTCP/DTLS/LINK process exit and LINK no-RX/missed-PONG feed the same replacement lifecycle; stale retired-incarnation exits are ignored by generation/physical-plan fencing.

## Windows physical-path convergence

One Wintun belongs to one Logical Tunnel. The connected monitor observes local underlay identity only:

```text
SourceIP + PacketDevice + SourceMAC + NextHopMAC
```

A change is not applied by mutating the controller baseline first. Instead, the discovered underlay is supplied to a same-LaneID candidate. Only after candidate transport qualification, bounded Game overlap and lifecycle promotion does the controller commit that underlay as the new known-good baseline. Candidate failure therefore preserves old A and its old baseline.

Because the connected route table contains a WBD-owned pinned `/32` server escape route, monitor discovery must exclude that WBD-owned route before selecting the current physical path. Otherwise the observer would report the old path indefinitely after a real default-route/NIC switch.

This phase does **not** claim public external-IP/NAT mapping detection, and it does **not** yet claim that `RouteForeign` direct-prefix kernel routes are rebound transactionally to the new physical route.

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

One public server port is not a one-lane-per-tunnel restriction. Distinct Logical Tunnels retain distinct leases; raw ingress remains source-lease checked.

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

## V2-M10 exact-source release gate

One exact substantive `SOURCE_SHA` must prove:

1. per-lane one-SYN same-association Reality-like real TLS 1.3 bootstrap;
2. no FIN/RST/reconnect/new WBD payload SYN inside a lane at bootstrap -> DTLS;
3. post-bootstrap no-HOL hole-bypass;
4. 1,2,3,4 active lanes accepted and fifth logical lane rejected while bounded replacement respects the 4+1 physical-incarnation ceiling;
5. normal desired=1, Game/weak desired=2..4;
6. Game first-arrival/dedup/out-of-order unique/no-cross-lane-HOL;
7. `A -> A+B -> B`, candidate failure preserving A, one-at-a-time multi-lane replacement, staggered 30..60m age rotation, current-process/LINK-liveness replacement, and local Windows path-change convergence;
8. distinct leases, source-spoof rejection, shared TUN + one NAT DNS/UDP/TCP;
9. lifecycle/functionality with FEC `off`, plus fixed `20:20` lane-local compatibility smoke; FEC parameter research is deferred;
10. Windows native/runtime and Linux raw/full-stack gates;
11. Windows portable and Linux amd64/arm64 artifacts from that exact `SOURCE_SHA`;
12. final clean physical Windows 11 + Npcap -> Ubuntu ARM64 DNS/UDP/TCP/lifecycle + deterministic cleanup.

A direct push CI-green result is necessary but not sufficient. The 16 successful ordinary push workflows at `fef0820d63736e037a790a87ef4de9e35ce39e26` do not by themselves establish V2-M10. The complete release-qualification matrix must intentionally finish on one exact final source, its Windows/Linux artifacts must be attributable to that source, and physical evidence must be fresh and exact-source before release-ready may be claimed.

## Deferred

- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps;
- FEC parameter research beyond fixed `20:20` compatibility smoke.
