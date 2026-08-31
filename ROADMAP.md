# Roadmap

> **Status: V2.5 ADR-0012 LOGICAL-TUNNEL MULTIPATH LIFECYCLE ACTIVE.** ADR-0012 controls 1..4 lanes, Game Lane and make-before-break. ADR-0013 is withdrawn.
>
> Compatibility contract: **V2.4 LOGICAL-TUNNEL / MULTIPATH PIVOT ACTIVE** remains the stable milestone phrase used by automated handoff/architecture checks; V2.5 is the current refinement under ADR-0012.

## Milestone map

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | raw FakeTCP + weak-network external baseline | **DONE** |
| V2-M2 | pinned wolfSSL DTLS 1.3 | **DONE** |
| V2-M3 | immutable LINK/control foundation | **DONE AS FOUNDATION** |
| V2-M4 | no-HOL FakeTCP/FEC qualification | **DONE / MUST REMAIN GREEN** |
| V2-M5 | Game Lane first-arrival/race layer | **IMPLEMENTED FOUNDATION / CURRENT PRODUCT MECHANISM** |
| V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association | **IMPLEMENTED / PER-LANE FOUNDATION** |
| V2-M7 | Windows Wintun raw-L3 + Npcap underlay | **IMPLEMENTED FOUNDATION / MULTILANE LIFECYCLE INTEGRATION ACTIVE** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED / REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **IMPLEMENTED FOUNDATION / REQUALIFY** |
| V2-M9C | ADR-0012 bounded 1..4 lane admission + Game integration | **ACTIVE** |
| V2-M9D | payload-idle DORMANT/wake with 0 active lanes and 1..4 wake target | **NEXT** |
| V2-M9E | make-before-break age/path replacement | **ACTIVE AFTER M9C CORE GREEN** |
| V2-M10 | exact-source Windows/Linux qualification + physical Windows 11 -> Ubuntu ARM64 | **BLOCKED ON MULTILANE LIFECYCLE QUALIFICATION** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED** |

## Frozen transport model

One active Logical Tunnel owns **1..4 independent WBD Transport Lanes**. Each lane separately satisfies:

```text
one raw FakeTCP SYN lineage
  -> bounded reliable Reality-like real TLS 1.3 bootstrap on that same association
  -> SAME 4-tuple / SAME sequence space / NO second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local optional fixed FEC
  -> packet/datagram VPN payload without ordinary-TCP HOL
```

Normal mode targets one lane. Game/weak-network mode may maintain 2..4 lanes. A candidate lane may temporarily overlap an old lane during make-before-break. All lanes terminate at the same Linux public `WBD_PORT` but use independent client 4-tuples and independent transport state.

## Frozen weak-network/release limits

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- pinned wolfSSL DTLS 1.3;
- FEC `off` or fixed systematic `20:20` per lane;
- no ordinary kernel-TCP sustained WBD payload and no TCP-over-TCP HOL.

The mature TCP-like recovery core is frozen unless deterministic evidence proves a defect below the new lifecycle layer.

## V2-M9A — Logical Tunnel + server-assigned address

```text
Account -> Device/Installation -> Logical Tunnel -> 1..4 disposable Transport Lanes
```

A Logical Tunnel owns a stable TunnelID, unique IPv4 lease and authenticated route/address configuration. LiveID/FakeTCP/DTLS/LINK belong to lanes.

Exit gates include distinct same-account leases, no global `10.66.0.2/30` identity, source anti-spoof and deterministic lease reuse/cleanup.

## V2-M9B — Shared server TUN + one NAT

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> shared WBD TUN
  <-> lease/tunnel demux + Game/race layer
  <-> 1..4 active lanes
```

Exit gates: two Logical Tunnels can use the same inner source port to the same target, DNS/UDP/TCP pass, spoofing is rejected, and firewall changes remain WBD-scoped.

## V2-M9C — Bounded multipath + Game Lane product integration

Required work:

- release invariant `1 <= active lanes <= 4`;
- LINK server accepts up to four lane peers for one TunnelID and rejects the fifth;
- Game Lane PacketID/first-arrival/dedup is the tunnel multipath layer;
- each lane independently uses per-lane same-association Reality-like bootstrap -> DTLS -> LINK;
- normal mode requests one lane; Game/weak policy may request 2..4;
- Windows lane lifecycle owns independent per-lane FakeTCP/DTLS/LINK child state without changing the TCP-like recovery core;
- Linux still exposes one raw public listener, not one listener per lane;
- release-contract tests make regression to global-one-lane semantics a CI failure.

Exit gate: unit tests plus Game Lane, per-lane same-flow, no-HOL, Windows and Linux exact-head gates are green together.

## V2-M9D — Payload-idle sleep and wake

A Logical Tunnel may retain lease/Wintun/routes while active lane count becomes zero. PING/PONG/control never resets real payload idle.

On wake, establish one lane first so queued payload can resume; establish optional extra Game lanes afterward. Wake must use bounded buffering.

## V2-M9E — Make-before-break replacement and mobility

```text
A ACTIVE
  -> establish B completely
  -> attach/prove B
  -> A+B bounded race/dedup overlap
  -> drain/retire A
  -> B ACTIVE
```

Candidate failure leaves A usable. In Game mode rotate only one lane at a time: `A+B -> A+B+C -> B+C`.

One replacement state machine handles scheduled age, Windows NIC/default-route/public-IP change, NAT/path failure, missed liveness, child failure, server request and manual reconnect. Lane generations fence stale paths.

## Reality-like fidelity work

For every lane, packet captures must qualify TCP persona, TLS 1.3 ClientHello/persona/SNI/record progression, same 4-tuple through the bootstrap barrier and absence of a second WBD payload SYN. Do not claim numeric `99%` similarity without a defined reproducible metric.

## Platform requirements

### Linux server

- one public `WBD_PORT` and one raw mux listener;
- no parallel ordinary kernel-TCP Reality product listener;
- multiple lanes are independent 4-tuples handled by that mux;
- WBD-owned firewall/RST suppression only;
- internal LINK/DTLS/raw-IP listeners remain private.

### Windows

- Wintun/raw-L3 capture remains;
- underlay escape remains mandatory;
- lane manager may own 1..4 independent per-lane FakeTCP/DTLS/LINK states;
- Npcap ingress must ignore unrelated traffic;
- IPv6 remains fail-closed while connected;
- Disconnect/Exit restores routes, DNS/NRPT, IPv6 and WBD-owned firewall state.

## Final V2-M10 release gate

On one exact substantive `SOURCE_SHA`:

1. every lane one-SYN Reality-like -> DTLS -> LINK -> raw-IP continuity is green;
2. no ordinary kernel-TCP Reality product bootstrap exists;
3. post-bootstrap no-HOL is green;
4. one TunnelID admits lanes 1..4 and rejects 5;
5. Game first-arrival/dedup/no-cross-lane-HOL is green;
6. `A -> A+B -> B` make-before-break preserves lease and delivers once;
7. candidate failure leaves A healthy;
8. one-at-a-time Game rotation preserves redundancy;
9. distinct leases + shared TUN + one NAT are green;
10. identical inner tuples from two Logical Tunnels remain isolated;
11. source spoofing is rejected;
12. FEC `off` and `20:20` remain green;
13. Windows build/admin-smoke/capability/portable and Linux release/firewall/full-stack are green;
14. Windows/Linux artifacts report the same `SOURCE_SHA`;
15. clean physical Windows 11 -> Ubuntu ARM64 passes DNS + UDP + TCP and deterministic cleanup.

Until changed automated gates pass together, do not hand a new artifact to the physical tester.

## Deferred

- DTLS HRR startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT;
- Windows child-process/module slimming;
- additional FEC profiles or higher release throughput caps.
