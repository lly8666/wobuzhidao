# Architecture v2.4

> **Status: ACTIVE MAINLINE DESIGN.** ADR-0012 supersedes the old assumption that one whole VPN session must remain one public 4-tuple until Disconnect and supersedes per-LiveID netns + double NAT as the final Windows raw-IP server architecture. The non-negotiable no-HOL/FakeTCP/DTLS/FEC security and release limits remain in force.

## Product intent

WBD is a personal weak-network VPN for privileged OpenWrt/Linux and Windows endpoints. Public WBD payload must remain WBD-owned raw TCP-shaped FakeTCP, while sustained VPN payload stays packet/datagram-oriented and does not inherit ordinary kernel TCP ordered-delivery HOL.

The product now separates a **long-lived Logical Tunnel** from its **replaceable Transport Lanes**.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - TunnelID
          - server-assigned unique tunnel address lease
          - race SessionID / PacketID space
          - desired lane count
          - active Transport Lanes
```

A Transport Lane is disposable and owns one complete WBD transport epoch:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable bootstrap on that same association
  -> real TLS 1.3 Reality-like recognition/admission
  -> same association, no second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> immutable LINK
  -> lane-local optional fixed FEC
  -> packet/datagram payload
```

## Per-lane single-flow invariant

The invariant is **per lane/transport epoch**, not per entire VPN lifetime:

- one lane has one public FakeTCP association from its SYN through Reality-like bootstrap and steady payload;
- no separate ordinary kernel-TCP WBD payload connection exists;
- no `Reality TCP -> close -> new FakeTCP payload connection` shortcut exists;
- no ordinary kernel TCP byte stream owns sustained WBD payload;
- the bootstrap stream adapter is bounded and destroyed before steady packet mode.

Normal mode targets one steady lane. Game/weak-network race mode may use 2..4 independent lanes. Controlled replacement may temporarily add a candidate lane before retiring an old lane.

This does **not** revive rejected PR #2. PR #2 used ordinary ordered kernel TCP lanes and therefore retained TCP HOL. The later Game Lane layer uses independent complete WBD associations with one logical PacketID, first-arrival delivery, duplicate suppression and bounded out-of-order acceptance; it has no cross-lane ordered-delivery dependency.

## Canonical packet stack

```text
Windows Wintun / OpenWrt TPROXY captured packet
        ↓
WBD raw packet envelope
        ↓
Logical Tunnel race/multipath layer
        ↓
1..N active lanes
        ↓ each lane independently
optional fixed systematic FEC
        ↓
DTLS 1.3 application datagram
        ↓
WBD FakeTCP raw association
        ↓
public network
```

`internal/gamelane` semantics are promoted as the general race foundation: one logical PacketID may be copied to independent lanes; first valid arrival is delivered; later copies are suppressed; unique packets may arrive out of order inside the replay window. FEC remains lane-local and a FEC block never spans lanes.

## Logical tunnel identity and server-assigned address lease

One shared username/password may authenticate several devices. Authentication creates transport credentials/tickets, but the tunnel address identity belongs to the Logical Tunnel/device, not to LiveID or a FakeTCP lane.

The server assigns each active logical tunnel a unique IPv4 lease from a configurable pool. The client must not globally hard-code `10.66.0.2/30`. Authenticated tunnel configuration supplies address/prefix/route parameters. `/32` plus explicit routes is preferred if Windows qualification proves it reliable; a shared-prefix fallback is allowed when Windows route/source-selection behavior requires it.

Same-account active devices receive different tunnel IPs. A short reconnect/lane replacement keeps the same logical tunnel and lease where possible.

Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

Future IPv6 applies the same binding to an assigned `/128`.

## Linux Windows-raw-IP server data plane

The final product direction is:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> race/multipath lane sets
```

Upstream packets are authenticated to a logical tunnel, source-validated against its lease, then injected into the shared TUN. Downstream packets returning through Linux reverse NAT/routing are read from the shared TUN and demultiplexed by leased destination address to the owning logical tunnel.

The per-session Linux netns/veth/inner-NAT/host-NAT implementation is retained only as historical/correctness evidence. It is not the selected final product architecture and must not be expanded. The earlier VRF/conntrack-zone prototype remains rejected.

## Idle sleep and long-lived VPN behavior

The logical VPN may stay enabled while its transport lanes sleep.

Track two clocks:

- `last_payload_activity`: real tunneled IP payload only;
- `last_transport_activity`: payload plus PING/PONG/control.

PING/PONG maintains liveness/NAT state but does not reset the user's payload-idle timer.

Client policy exposes `idle_transport_timeout`; initial product default is 15 minutes and `0` means **never sleep due to payload idleness**. On non-zero idle expiry, all active lanes close while the Logical Tunnel, leased IP, Wintun and routing/DNS state remain. The tunnel enters `DORMANT`.

A new captured packet wakes the tunnel. A small bounded wake buffer is allowed. The first healthy lane resumes traffic immediately; extra desired game lanes can be restored afterward.

Explicit Disconnect/Exit, not idle sleep, releases the logical tunnel and restores WBD-owned capture/routing/DNS/IPv6/firewall state.

## Transport age rotation and replacement

Every active lane has an independent age deadline, orthogonal to idle policy. Initial experimental/default guidance is a randomized 30..60 minute soft lifetime per lane; it is a product policy parameter, not a wire constant.

Continuous traffic therefore does not create an immortal FakeTCP association. Multi-lane mode staggers deadlines and never intentionally rotates all healthy lanes together.

Replacement is make-before-break:

```text
old lane ACTIVE
  -> build candidate FakeTCP -> Reality -> DTLS -> LINK
  -> attach candidate to the existing Logical Tunnel
  -> prove bidirectional health
  -> temporarily race old + candidate with the same logical PacketIDs
  -> first arrival delivers once; duplicate is suppressed
  -> old lane DRAINING
  -> retire old lane
```

Candidate failure leaves the old healthy lane untouched.

Game mode example:

```text
A + B
  -> build C
  -> A + B + C briefly
  -> retire A
  -> B + C
```

Lane generation/epoch fencing prevents a stale old path from resurrecting as active after a newer generation commits.

## Unified reconnect/path-change model

One Transport Manager replacement state machine handles scheduled rotation, Windows network/default-route/public-IP change, NAT/path change, missed-PONG/no-valid-RX, FakeTCP/DTLS/LINK failure, server-requested replacement and manual reconnect.

Windows network-change notification may trigger replacement immediately; heartbeat liveness is the fallback detector.

Server restart may destroy host conntrack and is not guaranteed to preserve every application TCP connection. Continuity is targeted at short underlay/path changes and planned lane replacement while logical tunnel/NAT state remains available.

## Reality-like bootstrap

Reality-like TLS remains the first protected payload phase inside each FakeTCP lane. Required behavior remains:

- plausible TCP-shaped SYN/SYN-ACK/ACK;
- real TLS 1.3 ClientHello/ServerHello/Finished on the same lane sequence space;
- configured SNI and WBD recognition marker;
- username/password only inside TLS;
- one-time lane/session credential only inside TLS;
- no FIN/RST/new WBD payload SYN between bootstrap and DTLS mode.

Unrecognized ClientHello traffic may remain in bounded stream mode and proxy to the configured decoy/fallback target. WBD must not claim browser-perfect REALITY equivalence without packet-capture evidence.

## Frozen security and weak-network boundaries

ADR-0012 changes tunnel/lane lifetime and Windows raw-IP server topology. It does not relax:

- WBD-owned raw TCP-shaped FakeTCP as public payload carrier;
- no ordinary kernel-TCP sustained payload path and no TCP-over-TCP HOL dependency;
- post-bootstrap earliest-complete datagram behavior;
- pinned wolfSSL DTLS 1.3 as steady-state cryptographic authority;
- `legacy` FakeTCP shadow recovery default; `sack-rack` experimental;
- release FEC `off` or fixed systematic `20:20` unless a later ADR reopens profiles;
- systematic sources must not wait merely to fill a FEC block;
- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner conservative release operating point;
- immutable lane-local LINK/FEC configuration for a transport epoch.

## Platform requirements

### Windows

- final capture remains Wintun/TUN raw L3;
- public underlay escape is mandatory;
- IPv6 remains fail-closed during the connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state;
- Npcap release/licensing constraints remain unchanged;
- Windows child-process slimming/refactoring is a separate later workstream.

### Linux/OpenWrt

- Linux product server exposes one public `WBD_PORT`; internal LINK/DTLS/raw-IP services remain private implementation details;
- WBD firewall helpers add/remove only WBD-owned rules/state and never flush the user's host ruleset;
- OpenWrt final capture remains TPROXY + policy routing.

## Observability and secrecy

Retain non-secret session/tunnel correlation IDs, first-boundary markers and counters. Do not emit per-packet INFO logs. Usernames/passwords, tickets, resume secrets and identity secrets must not be logged.

## Required qualification

The active release path must prove:

1. two logical tunnels receive different leased IPs;
2. both can simultaneously use the same inner TCP source port `40000` to the same Internet target/port;
3. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
4. source spoofing across leases is rejected;
5. lease cleanup/reconnect/reuse is deterministic;
6. idle timeout closes lanes but keeps the logical tunnel and wake succeeds;
7. one-lane A->B make-before-break delivers no duplicate application payload;
8. game mode maintains desired healthy redundancy while one lane is replaced at a time;
9. candidate failure leaves the old lane set usable;
10. network-change and heartbeat-dead-path triggers use the same replacement state machine;
11. FEC `off` and `20:20` remain qualified per lane;
12. full stack passes FakeTCP -> Reality-like -> DTLS -> LINK -> race/raw-IP -> shared TUN -> Internet;
13. exact-source physical Windows 11 -> Ubuntu ARM64 DNS/UDP/TCP and cleanup pass.

Startup RTT optimization is explicitly deferred until this architecture is functionally qualified.

## Retired / superseded product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- one whole logical VPN session permanently tied to one public 4-tuple;
- per-LiveID Windows raw-IP netns + veth + double NAT as final product design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as the WBD data plane;
- WireGuard inner glue;
- Android/no-root.
