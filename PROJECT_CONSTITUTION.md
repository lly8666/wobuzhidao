# wobuzhidao Project Constitution — V2.4

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- WBD-owned raw TCP-shaped FakeTCP as the public payload carrier;
- a short real-TLS Reality-like bootstrap carried inside the **same FakeTCP association for each transport lane**;
- UDP/datagram-like sustained payload semantics with no ordinary-TCP retransmission/HOL dependency;
- pinned standards-compliant DTLS 1.3 for steady-state encryption, integrity and anti-replay;
- optional WBD-owned FEC, currently `off` or fixed systematic `20:20` on the release wire;
- a long-lived Logical Tunnel identity with a server-assigned unique tunnel address lease;
- 1..4 independent complete WBD transport lanes using the existing first-arrival/dedup race semantics when product policy requests multipath;
- OpenWrt final transparent capture through **TPROXY**;
- Windows final client capture through a **TUN/Wintun-class L3 adapter**.

The current weak-network qualification ceiling remains **<=100 Mbit/s physical link capacity** and the conservative release operating point remains **40 Mbit/s aggregate inner payload**.

V1 (`dev/wbd-multilane-v1`, PR #2) remains rejected. Its ordinary kernel-TCP lane architecture must not be confused with the later WBD Game Lane race design.

## Non-negotiable per-lane public-flow invariant

1. Each Transport Lane has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage for that transport epoch.
2. Reality-like TLS is the first protected payload phase of that same FakeTCP association.
3. Successful Reality-like admission for a lane must not be followed by a second ordinary kernel-TCP WBD payload connection.
4. The transition from TLS bootstrap to DTLS data mode emits no FIN/RST/new WBD payload SYN for that lane and does not change its 4-tuple.
5. Product mode must not run a parallel kernel TCP Reality listener as the owner of WBD admission/payload on `WBD_PORT`.
6. FakeTCP owns public packet state from SYN onward. Kernel TCP state takeover is not a release dependency.
7. A Logical Tunnel may intentionally own multiple independent lanes in game/race mode or temporarily during make-before-break replacement. Therefore `one entire VPN lifetime = one public flow` is no longer a valid invariant.

## Non-negotiable no-HOL data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. **TCP-shaped does not mean kernel-TCP-owned.** WBD must not depend on a real kernel `ESTABLISHED` payload socket.
3. A temporary reliable ordered stream is permitted only during bounded TLS/bootstrap for each lane because TLS requires stream semantics.
4. After the lane's bootstrap-to-DTLS barrier, later independent authenticated datagrams must be able to complete while an earlier FakeTCP sequence range is missing.
5. Shadow ACK/SACK/retransmission may preserve TCP-like outer behavior but must not impose ordinary TCP ordered delivery/HOL on steady payload.
6. WBD FEC is systematic and optional. **Do not delay an available systematic source merely to fill a FEC block.**
7. FEC is lane-local. A FEC block must not span multiple lanes or wait on another lane.
8. Normal mode targets one steady lane. Later Game Lane semantics may use 2..4 independent WBD lanes for first-arrival racing and duplicate suppression.

## Logical Tunnel and address lease

The identity hierarchy is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> server-assigned tunnel address lease
          -> stable race SessionID / PacketID space while active
          -> active Transport Lanes
```

- username identifies the shared account, not a transport session;
- each lane may use a fresh one-time ticket/LiveID and fresh FakeTCP/DTLS/LINK state;
- the tunnel address lease belongs to the Logical Tunnel/device, not to LiveID;
- same-account active devices receive distinct tunnel IPs;
- the server supplies authenticated address/prefix/route configuration; Windows must not globally hard-code `10.66.0.2/30`;
- the configured address pool must be configurable because no private/CGNAT range is collision-free everywhere;
- server ingress must drop raw packets whose source address does not match the tunnel's assigned lease;
- future IPv6 uses the same binding with an assigned `/128`.

## Game Lane / race foundation

The later `internal/gamelane` design is part of the accepted architecture and is not the rejected PR #2 design.

Its required semantics are:

- one logical payload gets one `PacketID`;
- the same logical payload may be copied onto 1..4 independent complete WBD associations;
- each lane has a distinct FakeTCP tuple/sequence space, DTLS association, LINK and lane-local FEC state;
- first valid arrival is delivered immediately;
- later copies of the same `PacketID` are suppressed;
- bounded out-of-order unique packets are allowed without waiting for a slower lane;
- there is no cross-lane HOL.

These semantics are promoted as the general multipath/replacement foundation. Do not invent a second migration-only packet-ID layer unless a later ADR demonstrates a real protocol gap.

## Canonical establishment sequence for one lane

```text
raw FakeTCP SYN / SYN-ACK / ACK
        -> temporary bounded reliable ordered bootstrap stream
        -> real TLS 1.3 ClientHello/ServerHello/Finished
        -> Reality-like marker recognition
        -> shared username/password admission inside TLS
        -> fresh one-time lane/session credential inside TLS
        -> bootstrap ACK drain + mode barrier
        -> SAME lane 4-tuple / SAME lane sequence space
        -> DTLS 1.3 association
        -> LINK/tunnel attach
        -> lane-local immutable FEC/LINK state
        -> ACTIVE lane
```

No separate product ordinary-TCP Reality connection is introduced.

## Linux Windows-raw-IP product shape

Final product direction:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> multipath/race lane sets
```

The current per-LiveID netns + veth + inner NAT + host NAT implementation is historical/reference evidence, not the final selected product design. Do not expand it as mainline. The earlier VRF/conntrack-zone prototype remains rejected.

## Transport idle, rotation and reconnect policy

Logical Tunnel lifetime is not FakeTCP lifetime.

Track separately:

- `last_payload_activity` for real tunneled IP payload;
- `last_transport_activity` for payload plus PING/PONG/control.

PING/PONG/control does not reset the user's payload-idle timer.

Client `idle_transport_timeout` is configurable; initial default is 15 minutes and `0` means never sleep **because of payload idleness**. Non-zero idle expiry closes all active lanes but retains Logical Tunnel identity, leased IP, Wintun and connected capture/routing/DNS state in `DORMANT`. A new captured packet wakes the tunnel using a bounded buffer; the first healthy lane resumes traffic before optional additional game lanes finish establishing.

Every active lane also has an independent age-rotation deadline, orthogonal to idle policy. Initial experimental/default guidance is randomized 30..60 minutes per lane. Multi-lane replacements are staggered and never intentionally rotate all healthy lanes together.

Replacement is make-before-break: establish and prove a candidate lane, briefly add it to the race set, use first-arrival/dedup during overlap, drain the old lane, then retire it. Candidate failure must leave the old healthy lane untouched.

One replacement state machine handles scheduled rotation, Windows network/default-route/public-IP change, NAT/path change, missed-PONG/no-valid-RX, FakeTCP/DTLS/LINK failure, server request and manual reconnect. Lane generation/epoch fencing prevents stale-path resurrection.

Explicit user Disconnect/Exit releases the Logical Tunnel and restores WBD-owned network state. Server restart may destroy NAT/conntrack and is not guaranteed to preserve every application TCP flow.

## Reality-like bootstrap requirements

Per lane:

- real TLS 1.3 records on the FakeTCP sequence space;
- configured SNI and WBD recognition marker;
- username/password only inside TLS;
- one-time credential only inside TLS;
- bounded bootstrap duration/memory;
- bootstrap writes are ACK-gated for reliable ordered TLS bytes;
- ordered bootstrap adapter is destroyed before steady packet delivery.

Unrecognized ClientHello traffic may remain in bounded stream mode and proxy to a configured fallback target. Browser/REALITY resemblance remains evidence-driven; do not claim browser-perfect/`99%` resemblance without pcap proof.

## DTLS and FEC security freeze

1. Steady-state WBD security remains **DTLS 1.3**.
2. Pinned implementation remains wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. 0-RTT remains disabled until replay/resume semantics are explicitly designed.
4. FEC source/repair datagrams are independently protected DTLS application datagrams.
5. Release FEC remains `off` or fixed systematic `20:20` unless a later ADR reopens profiles.
6. `legacy` FakeTCP shadow recovery remains product default; `sack-rack` remains experimental.

## Release operating point

For the current <=100 Mbit/s weak-link target:

- **40 Mbit/s aggregate inner offered payload** remains the conservative release operating point;
- do not promote 50/60/80 Mbit/s without a separate benchmark decision;
- normal mode uses one steady lane;
- game/weak-network lane counts are redundancy policy, not permission to exceed the 40 Mbit/s aggregate-inner release target without qualification.

## Platform invariants

### OpenWrt

Final capture remains **TPROXY + policy routing** with mandatory public-underlay escape and WBD-owned cleanup.

### Windows

Final capture remains **Wintun/TUN raw L3**. Underlay escape is mandatory. Device-wide IPv6 remains fail-closed for the entire connected interval until a real IPv6 tunnel path is qualified.

Disconnect/Exit must restore WBD-owned routes, DNS/NRPT, IPv6 and firewall state. Npcap install/licensing constraints remain unchanged; WBD must not improperly redistribute the Free Edition.

### Linux server / firewall

The product server exposes one public `WBD_PORT`. Internal LINK/DTLS/raw-IP services remain private implementation details. Firewall helpers must add/remove only WBD-owned state and must never flush or replace the user's host ruleset.

## Observability and secrecy

Keep non-secret tunnel/session correlation IDs, first-packet/boundary markers and counters. Do not enable per-packet INFO logging. Usernames/passwords, tickets, resume secrets and identity secrets must not be logged.

## Qualification gates

The release path must prove:

1. each lane preserves one-SYN/same-association Reality -> DTLS -> LINK lineage;
2. post-bootstrap no-HOL hole-bypass still passes;
3. two logical tunnels receive distinct leases and can simultaneously bind `source port 40000` to the same target;
4. DNS/UDP/TCP pass shared TUN + one host NAT;
5. lease source spoofing is rejected;
6. idle sleep/wake preserves logical tunnel semantics;
7. A->B make-before-break replacement causes no duplicate downstream delivery;
8. game mode replaces only one lane at a time while preserving desired redundancy;
9. candidate failure leaves old lanes usable;
10. network-change and heartbeat dead-path use the same replacement path;
11. FEC `off` and `20:20` remain qualified per lane;
12. exact-source physical Windows 11 -> Ubuntu ARM64 passes DNS/UDP/TCP and cleanup.

## Retired / non-product architectures

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as WBD sustained payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- one whole VPN lifetime permanently tied to one 4-tuple;
- per-LiveID netns/veth/double-NAT Windows raw-IP final design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epochs/in-place FEC switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.

## Development discipline

- ADR-0012 is the controlling tunnel/lane lifecycle decision;
- preserve qualified no-HOL, DTLS, FEC and 40 Mbit/s evidence unless a changed boundary invalidates a specific test;
- preserve later Game Lane race semantics and do not apply the PR #2 no-go conclusion to them;
- do not continue expanding netns/double NAT as final product;
- do not combine this pivot with Windows child-process slimming or startup-RTT optimization;
- do not call the product release-ready until the new shared-TUN/lease/multipath gates and final physical test pass.
