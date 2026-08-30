# Roadmap

> **Status: V2.4 LOGICAL-TUNNEL / MULTIPATH PIVOT ACTIVE.** ADR-0012 is the controlling decision. The previous per-LiveID netns + double-NAT raw-IP server path is no longer the final product direction. Preserve its evidence, but stop expanding it.

## Milestone map

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | raw FakeTCP + weak-network external baseline | **DONE** |
| V2-M2 | pinned wolfSSL DTLS 1.3 | **DONE** |
| V2-M3 | immutable LINK/session/control foundation | **DONE AS FOUNDATION** |
| V2-M4 | no-HOL FakeTCP/FEC first-arrival qualification | **DONE / MUST REMAIN GREEN** |
| V2-M5 | later Game Lane first-arrival race layer, 1..4 independent WBD associations | **IMPLEMENTED AS FOUNDATION / NOW PROMOTED** |
| V2-M6 | Reality-like TLS bootstrap on the same FakeTCP association per lane | **IMPLEMENTED / QUALIFIED FOUNDATION** |
| V2-M7 | Windows Wintun raw-L3 capture and routing | **IMPLEMENTED / PHYSICAL DATA PATH ACCEPTANCE PENDING** |
| V2-M8-old | per-LiveID raw-IP netns + veth + double NAT | **SUPERSEDED AS PRODUCT BY ADR-0012; REFERENCE ONLY** |
| V2-M9A | Logical Tunnel identity + server address lease | **NEXT** |
| V2-M9B | shared Linux TUN + one host NAT + lease demux | **NEXT** |
| V2-M9C | dynamic race-lane attach/detach/generation for raw-IP tunnel | **NEXT** |
| V2-M9D | idle DORMANT/wake lifecycle | **NEXT** |
| V2-M9E | make-before-break lane replacement + age rotation + path-change recovery | **NEXT** |
| V2-M10 | exact-source automated + physical Windows 11 -> Ubuntu ARM64 release qualification | **BLOCKED ON V2-M9** |
| V2-M11 | startup RTT / packaging/process simplification | **DEFERRED UNTIL FUNCTIONAL PIVOT IS GREEN** |

## Frozen per-lane transport invariant

For each independent Transport Lane:

```text
one raw FakeTCP SYN lineage
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 / Reality-like marker + admission
  -> SAME lane 4-tuple / SAME lane sequence space / NO second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local optional fixed FEC
  -> packet/datagram VPN payload without ordinary-TCP HOL
```

The whole Logical Tunnel may use multiple independent lanes in game/race mode or briefly during controlled replacement. Do not restore the old statement `one VPN session forever = one public flow`.

## Frozen weak-network/release limits

These remain release constraints unless a later explicit benchmark ADR reopens them:

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- `legacy` FakeTCP recovery default; `sack-rack` experimental;
- pinned wolfSSL DTLS 1.3;
- FEC `off` or fixed systematic `20:20` on the qualified release wire;
- FEC/LINK state is immutable and lane-local for a transport epoch;
- systematic source datagrams are not delayed merely to fill FEC blocks;
- no ordinary kernel-TCP sustained WBD payload path and no TCP-over-TCP HOL dependency.

Game/race redundancy does not authorize aggregate inner payload above 40 Mbit/s without separate qualification.

## V2-M9A — Logical Tunnel + server-assigned unique address

Implement a product identity hierarchy separate from transport association identity:

```text
Account -> Device/Installation -> Logical Tunnel -> Transport Lanes
```

A Logical Tunnel owns:

- stable TunnelID while the logical VPN is enabled;
- server-assigned unique IPv4 lease from a configurable pool;
- race SessionID / PacketID space;
- desired lane count;
- current active/candidate/draining lanes.

LiveID/FakeTCP/DTLS/LINK belong to lanes, not to the IP lease.

Exit gates:

1. two same-account logical tunnels receive different addresses;
2. Windows no longer assumes every client is `10.66.0.2/30`;
3. authenticated tunnel configuration carries address/prefix/route information;
4. ingress source address must equal the lease;
5. lease cleanup/reconnect/reuse is deterministic;
6. pool is configurable and does not claim universal collision freedom.

Test `/32` plus explicit Windows routes first; use a shared-prefix fallback if physical Windows source/routing semantics require it.

## V2-M9B — Shared server TUN + one NAT

Selected final Windows raw-IP server shape:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> shared WBD TUN
  <-> lease/tunnel demux
```

The old per-LiveID netns/veth/inner-NAT/outer-NAT path is no longer a product milestone. It may remain as a historical correctness oracle only.

Exit gates:

- two different leased tunnel IPs simultaneously work;
- both clients may bind the same inner TCP source port `40000` to the same target/port;
- DNS-style UDP, generic UDP and TCP all pass;
- target observes host NAT identity, not WBD private addresses;
- spoofed source lease is rejected;
- WBD firewall helper modifies only WBD-owned rules and works with supported iptables/nft configurations.

## V2-M9C — Promote Game Lane race semantics to dynamic tunnel lanes

The later Game Lane design is valid architecture, not the rejected PR #2 multi-kernel-TCP design.

Preserve and reuse:

- one logical PacketID across copies;
- 1..4 independent complete WBD associations;
- first-arrival delivery;
- duplicate suppression;
- bounded out-of-order acceptance;
- no cross-lane HOL.

Add lifecycle, not a second packet-sequence protocol:

- attach candidate lane to an existing Logical Tunnel;
- lane id + generation/epoch;
- ACTIVE / CANDIDATE / DRAINING / RETIRED states;
- dynamic attach/detach while race SessionID/PacketID space remains stable;
- lane-local FEC state.

Normal mode target: 1 steady lane.

Game/weak-network mode: desired 2..4 lanes according to existing policy/controller limits.

## V2-M9D — Payload-idle sleep and wake

Client setting `idle_transport_timeout`:

- initial default 15 minutes;
- configurable;
- `0` means never enter DORMANT because of payload idleness.

Maintain separate clocks for real payload activity and transport liveness. PING/PONG/control never resets payload idle.

On idle expiry:

- close all lanes/FakeTCP/DTLS/LINK;
- keep Logical Tunnel lease, Wintun, capture routes/DNS and connected logical state;
- enter `DORMANT`.

On first new Wintun packet:

- wake with a bounded queue;
- one healthy lane is enough to resume traffic;
- restore optional desired game lanes afterward.

Exit gate: sleep/wake does not require user reconnect and does not leak unbounded buffered packets.

## V2-M9E — Make-before-break replacement and mobility

Every lane gets an independent age deadline. Initial experimental/default policy: random 30..60 minutes per lane. This applies even when idle timeout is non-zero but traffic is continuous and when idle timeout is `0`.

Multi-lane deadlines are staggered with a minimum replacement separation. Never intentionally rotate all healthy game lanes together.

Replacement sequence:

```text
A ACTIVE
  -> establish candidate B completely
  -> attach B to the existing Logical Tunnel
  -> prove bidirectional health
  -> temporarily race A+B using existing PacketID/dedup
  -> A DRAINING
  -> B ACTIVE
  -> A RETIRED
```

Candidate failure leaves A untouched.

Game example:

```text
A+B -> build C -> brief A+B+C -> retire A -> B+C
```

The same replacement state machine handles:

- scheduled rotation;
- Windows NIC/default-route/public-IP change;
- NAT/path change;
- missed-PONG/no-valid-RX;
- FakeTCP/DTLS/LINK child failure;
- server-requested replace;
- manual reconnect.

Lane generation fencing prevents stale transport resurrection.

## Platform requirements that must not regress

### Linux server

- one public `WBD_PORT` product surface;
- no ordinary kernel TCP WBD payload listener/path;
- WBD-owned firewall/RST suppression only;
- never flush or replace the user's host ruleset;
- internal LINK/DTLS/raw-IP listeners remain private implementation details.

### Windows

- Wintun/raw-L3 remains the product capture path;
- underlay escape remains mandatory;
- device-wide IPv6 remains fail-closed while connected until IPv6 tunneling is explicitly qualified;
- Disconnect/Exit restores routes, DNS/NRPT, IPv6 and WBD-owned firewall state;
- Npcap installation/licensing constraints remain unchanged;
- do not combine this pivot with child-process slimming.

## Observability requirements

Retain non-secret correlation IDs, first-boundary markers and counters. Add tunnel/lane lifecycle markers as needed, but do not emit per-packet INFO spam. Credentials, passwords, tickets, resume secrets and identity secrets never belong in logs.

## Current qualification interpretation

The latest pre-pivot raw-IP work implemented per-session netns isolation and added strong intended tests, but the decisive raw-IP qualification was not reached because legacy `wbd-link-server-mux` tests failed the newer strict application-frame classifier before the privileged raw-IP steps ran. Therefore do not treat the netns design as a proven product requirement and do not spend the pivot by first expanding it.

Existing single-flow/FakeTCP/DTLS/LINK/no-HOL/Game Lane evidence remains valuable and should be preserved, but any gate whose assumptions changed must be rerun on the new architecture.

## Final V2-M10 release gate

On one exact substantive `SOURCE_SHA`, all of the following must be true:

1. per-lane one-SYN Reality -> DTLS -> LINK continuity is green;
2. post-bootstrap no-HOL hole-bypass is green;
3. distinct tunnel leases + shared TUN + one NAT are green;
4. same source port `40000` / same Internet target works for two logical tunnels;
5. source spoofing is rejected;
6. idle DORMANT/wake is green;
7. one-lane A->B make-before-break has zero application duplicate delivery;
8. game/race mode replacement preserves desired healthy lane redundancy;
9. candidate failure does not damage current healthy lanes;
10. network-change/missed-PONG recovery uses the unified replacement path;
11. FEC `off` and `20:20` remain green per lane;
12. Windows and Linux artifacts report the same `SOURCE_SHA`;
13. clean physical Windows 11 -> Ubuntu ARM64 passes DNS + UDP + TCP;
14. Disconnect/Exit restores route, DNS/NRPT, IPv6 and WBD-owned firewall state without manual repair.

Until all changed gates pass together, do not call the package final/release-ready.

## Deferred until after V2-M10 functional architecture

- DTLS HRR-cookie startup RTT optimization;
- LINK bind/init coalescing;
- abbreviated resume/0-RTT work;
- Windows child-process/module slimming;
- native replacement of PowerShell underlay/network configuration;
- additional FEC profiles or higher release throughput caps.
