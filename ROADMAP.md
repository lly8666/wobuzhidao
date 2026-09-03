# Roadmap

> **Status: PER-LANE SAME-FLOW + LOGICAL TUNNEL MULTIPATH ACTIVE.** ADR-0011 controls each Transport Lane; ADR-0012 controls Logical Tunnel 1..4-lane lifecycle. ADR-0014 is withdrawn/invalidated.

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
| V2-M9E | make-before-break + unified lane replacement state machine | **AGE + PROCESS/LINK LIVENESS + LOCAL WINDOWS PATH + KERNEL ROUTE REBIND + LOCAL MANUAL + AUTOMATIC SERVER-IDLE CLOSE RECONNECT IMPLEMENTED; OPERATOR SERVER ACTUATOR + DIRECT PUBLIC NAT OBSERVABILITY ARE NOT IMPLEMENTED** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED UNTIL ONE EXACT-SOURCE MATRIX + ARTIFACTS + PHYSICAL EVIDENCE** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## 2026-09-03 lifecycle checkpoint

Substantive source `c42536fb30f4c2fddc17a0169513bc1ed16cf6ce` completes the explicit local manual transport reconnect phase and verifies existing authenticated server CLOSE reconnect semantics without adding a new LINK/control wire type. All **26** ordinary push workflows observed on the PR branch for that exact SHA completed successfully with queued=0, in_progress=0 and failure=0. This is exact-source lifecycle evidence, **not** release authorization.

Current code/tests establish:

- same logical LaneID old+candidate bounded Game race; candidate failure preserves the old transport;
- product logical-lane ceiling 1..4 with bounded 4+1 physical-incarnation capacity and a rotating spare slot across physical slots 1..5;
- real app/TUN payload activity is separate from control traffic; PING/PONG/control do not refresh payload idle;
- omitted product `idle_timeout` defaults to 15m; explicit `idle_timeout=0` disables payload-idle sleep only;
- automatic payload-idle DORMANT and payload-triggered wake, including first-READY publication;
- randomized per-incarnation 30..60m lane-age deadlines, staggered and serialized through the same replacement lifecycle;
- authoritative FakeTCP/DTLS/LINK child exit and LINK no-RX/missed-PONG converge through replacement while stale retired-incarnation exits are ignored;
- connected Windows path observation detects changes in source IPv4, Npcap packet device/NIC, source MAC, and next-hop MAC/default-route identity;
- physical-path discovery excludes WBD's own pinned server escape route so the observer does not self-lock to the old path;
- path convergence replaces current logical lanes one at a time with expected-generation make-before-break; discovery/candidate failure is non-destructive;
- after lane convergence, WBD-owned server `/32` escape and `RouteForeign` direct-prefix routes are rebound transactionally to the current physical interface/next hop without tearing down the shared Wintun/Logical Tunnel;
- route rebind adds/validates the new owned routes before removing stale owned routes; a re-observed path mismatch or mutation failure fails closed for that rebind attempt and leaves cleanup ownership intact for retry;
- DORMANT performs no lane replacement; Wake rediscovers the physical path and completes any needed route rebind before first-READY publication;
- Windows GUI `Reconnect transport` calls `Controller.RotateActiveLanes`, which snapshots authoritative LaneRefs and rotates active logical lanes sequentially through the existing generation-fenced make-before-break lifecycle while shared Wintun/routes/Game/TUN stay online;
- stale LaneRefs are fenced; a candidate failure stops the manual sweep and preserves the healthy old incarnation of the failed lane;
- existing DTLS-protected LINK `CloseIdleTimeout` and `CloseTransportTransient` reasons remain `ReconnectAllowed`; client receipt terminates LINK and the authoritative child-exit trigger converges on the same replacement lifecycle;
- production `wbd-link-server-mux` automatically emits the existing `CloseIdleTimeout` frame when an active session exceeds its server-side idle lease, then removes the peer;
- no FakeTCP/Reality/DTLS/LINK/FEC wire format changed for manual/server-CLOSE reconnect.

## 2026-09-03 server/NAT capability audit

The exact live-tree audit after sequence 95 closes two previously ambiguous claims without changing product code:

1. **Server actuator boundary.** `wbd-link-server-mux` has a real automatic server-originated actuator for idle leases: `expirePeers` sends the existing reconnect-capable `CloseIdleTimeout`. The production mux exposes no operator/admin `rotate now` input, no `CloseTransportTransient` emission site, and no rotate/admin flag or management endpoint. A future trusted local/admin actuator could reuse the existing `CloseTransportTransient` reason; a new LINK control Type is not justified by current evidence.
2. **Public NAT observation boundary.** `wbd-faketcp-mux` knows the real outer public FakeTCP `ServerFlow` (`ClientIP`, `ClientPort`, `ServerIP`, `ServerPort`) and uses the client endpoint as the Reality-like TLS bootstrap stream `RemoteAddr`. That outer tuple is not written into the Logical Tunnel ticket/binding, is not returned by the authenticated bootstrap reply, and is not passed as metadata when the server starts the DTLS worker. `wbd-link-server-mux` therefore sees only its local plaintext DTLS-worker UDP peer, not an authenticated reflection of the public FakeTCP/NAT mapping. The Windows client consequently has no existing authoritative public external-IP/NAT mapping observer.

These are deliberate **non-claims**: local SourceIP/default-route/NIC changes are not relabeled as public-NAT changes, and indirect age/liveness replacement is convergence rather than direct mapping detection. No wire change is introduced merely to satisfy a checklist.

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
- A fifth logical lane is rejected.
- Planned healthy replacement is `A -> build/qualify B -> A+B bounded race -> drain A -> B`.
- Candidate B failure leaves A alive.
- Physical transport slots are bounded to 1..5; only four are authoritative at once and the unused slot is the rotating make-before-break spare. Slot identity never creates a fifth logical LaneID or a second PacketID namespace.

The mature FakeTCP recovery, Reality-like bootstrap, DTLS, LINK and FEC wire are frozen unless deterministic lower-layer qualification isolates a real defect.

## Windows physical-path and route convergence

One Wintun belongs to one Logical Tunnel. The connected local observer compares:

```text
SourceIP + PacketDevice + SourceMAC + NextHopMAC/default-route identity
```

A changed lane baseline is not committed before candidate qualification. The candidate uses the newly observed underlay; only after bounded Game overlap and promotion does the controller commit that baseline. Candidate failure therefore preserves healthy A and its last-known-good baseline.

After all affected logical lanes have converged, WBD-owned kernel routes are rebound to the currently re-observed physical `InterfaceIndex + NextHop`. This includes the pinned server escape `/32` and `RouteForeign` direct prefixes. The rebind path verifies the current physical observation again before mutation, serializes route mutation through the Executor, and does not restart Wintun/Game/TUN.

This phase does **not** claim public external-IP/NAT mapping detection.

## Idle / wake / liveness policy

- Product default payload idle timeout: **15m (900s)** when `idle_timeout` is omitted.
- Explicit `idle_timeout=0`: never sleep due to payload idleness; lifecycle monitoring and lane-age rotation remain enabled.
- Only real application/TUN payload refreshes payload idle; PING/PONG/control does not.
- DORMANT closes all lanes but preserves Logical Tunnel, lease, Wintun/routes/DNS.
- First new payload wakes the tunnel; physical path and route ownership converge before the first READY lane is published; optional Game lanes refill afterward.
- Each active lane incarnation has an independent randomized 30..60m soft age deadline.
- Current authoritative FakeTCP/DTLS/LINK process exit and LINK no-RX/missed-PONG feed the same replacement lifecycle.
- Manual reconnect and reconnect-capable authenticated server CLOSE converge on the existing replacement lifecycle rather than creating a second state machine.

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

Distinct Logical Tunnels retain distinct leases; raw ingress remains source-lease checked.

## Frozen weak-network/release limits

- no ordinary kernel-TCP sustained WBD payload;
- no TCP-over-TCP HOL;
- pinned wolfSSL DTLS 1.3;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- FEC `off` or fixed systematic `20:20`, always lane-local;
- functional/lifecycle qualification prioritizes FEC `off`; `20:20` is compatibility smoke only and FEC parameter research remains deferred;
- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- Windows Wintun raw L3;
- OpenWrt TPROXY + policy routing;
- WBD-scoped Linux firewall ownership;
- Windows IPv6 connected interval fail-closed until IPv6 qualification;
- deterministic Disconnect/Exit cleanup;
- Npcap packaging/licensing constraints unchanged.

## V2-M10 exact-source release gate

One exact substantive `SOURCE_SHA` must prove:

1. per-lane one-SYN same-association Reality-like real TLS 1.3 bootstrap;
2. no FIN/RST/reconnect/new WBD payload SYN inside a lane at bootstrap -> DTLS;
3. post-bootstrap no-HOL hole-bypass;
4. 1,2,3,4 active lanes accepted and fifth logical lane rejected while bounded replacement respects the 4+1 physical-incarnation ceiling;
5. normal desired=1, Game/weak desired=2..4;
6. Game first-arrival/dedup/out-of-order unique/no-cross-lane-HOL;
7. `A -> A+B -> B`, candidate failure preserving A, one-at-a-time multi-lane replacement, staggered 30..60m age rotation, current-process/LINK-liveness replacement, local Windows path-change convergence, WBD-owned physical-route rebind, manual transport reconnect, and authenticated server-CLOSE reconnect semantics;
8. distinct leases, source-spoof rejection, shared TUN + one NAT DNS/UDP/TCP;
9. lifecycle/functionality with FEC `off`, plus fixed `20:20` lane-local compatibility smoke;
10. Windows native/runtime and Linux raw/full-stack gates;
11. Windows portable and Linux amd64/arm64 artifacts from that exact `SOURCE_SHA`;
12. final clean physical Windows 11 + Npcap -> Ubuntu ARM64 DNS/UDP/TCP/lifecycle + deterministic cleanup.

A direct push CI-green result is necessary but not sufficient. The 26 successful ordinary push workflows at `c42536fb30f4c2fddc17a0169513bc1ed16cf6ce` do not by themselves establish V2-M10. The complete release-qualification matrix must intentionally finish on one exact final source; same-source release artifacts and fresh physical evidence remain mandatory.

## Deferred / explicitly unresolved

- operator/admin server-side `rotate now` actuator; if added later, prefer a trusted local/admin surface that emits existing `CloseTransportTransient` rather than a new LINK frame;
- authoritative direct public external-IP/NAT mapping observation; current server public flow is not reflected to the client;
- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps;
- FEC parameter research beyond fixed `20:20` compatibility smoke.
