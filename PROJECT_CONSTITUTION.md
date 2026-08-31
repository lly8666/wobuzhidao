# wobuzhidao Project Constitution — V2.4

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- WBD-owned raw TCP-shaped FakeTCP as the sole public transport;
- a short real-TLS Reality-like bootstrap carried inside that **same one FakeTCP association**;
- no FIN/RST/new WBD payload SYN between bootstrap and sustained payload;
- UDP/datagram-like sustained payload semantics with no ordinary-TCP retransmission/HOL dependency;
- pinned standards-compliant DTLS 1.3 for steady-state encryption, integrity and anti-replay;
- optional WBD-owned FEC, currently `off` or fixed systematic `20:20` on the release wire;
- a long-lived Logical Tunnel identity with a server-assigned unique tunnel address lease;
- exactly one usable public WBD transport for a connected Logical Tunnel;
- OpenWrt final transparent capture through **TPROXY**;
- Windows final client capture through a **TUN/Wintun-class L3 adapter**.

The current weak-network qualification ceiling remains **<=100 Mbit/s physical link capacity** and the conservative release operating point remains **40 Mbit/s aggregate inner payload**.

V1 (`dev/wbd-multilane-v1`, PR #2) remains rejected. Historical Game Lane/race code may remain as experimental evidence, but it is not a current release product path.

## Non-negotiable global public-flow invariant

1. A connected Logical Tunnel has **exactly one** usable public WBD FakeTCP association. Disconnected/dormant state may have zero.
2. That one association has one public client/server 4-tuple, one FakeTCP sequence space and one SYN lineage for the transport epoch.
3. Reality-like TLS is the first protected payload phase of that exact association.
4. Successful Reality-like admission must not be followed by a second ordinary kernel-TCP connection or a second FakeTCP SYN for WBD payload.
5. The transition from TLS bootstrap to DTLS data mode emits no FIN/RST/new WBD payload SYN and does not change the 4-tuple.
6. Product mode must not run a parallel kernel-TCP Reality listener as the WBD admission/payload owner on `WBD_PORT`.
7. FakeTCP owns public packet state from SYN onward. Kernel TCP state takeover is not a release dependency.
8. `internal/logicaltunnel.MaxProductPublicTransportLanes` is fixed at `1`; current release UI/config/lifecycle must not expose 2..4 active transport lanes.
9. Make-before-break/public candidate overlap is forbidden. Planned replacement is break-before-make.
10. The server rejects two concurrently usable LINK transports claiming the same TunnelID as defense in depth.

The controlling decision is **ADR-0013**. ADR-0012 remains authoritative only for retained Logical Tunnel/lease/shared-TUN decisions that do not require multipath.

## Non-negotiable no-HOL data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. **TCP-shaped does not mean kernel-TCP-owned.** WBD must not depend on a real kernel `ESTABLISHED` payload socket.
3. A temporary reliable ordered stream is permitted only during the bounded TLS/bootstrap phase because TLS requires stream semantics.
4. The ordered bootstrap adapter is destroyed at the bootstrap-to-DTLS barrier.
5. After that barrier, later independent authenticated datagrams must be able to complete while an earlier FakeTCP sequence range is missing.
6. Shadow ACK/SACK/retransmission may preserve TCP-like outer behavior but must not impose ordinary TCP ordered delivery/HOL on steady payload.
7. WBD FEC is systematic and optional. **Do not delay an available systematic source merely to fill a FEC block.**
8. FEC state is local to the one current transport epoch and must never require a previous/replacement transport to make progress.

## Logical Tunnel and address lease

The identity hierarchy is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> server-assigned tunnel address lease
          -> zero public transports while disconnected/dormant
          -> exactly one active public transport while connected
```

- username identifies the shared account, not transport identity;
- each replacement transport may use a fresh one-time ticket/LiveID and fresh FakeTCP/DTLS/LINK state;
- the tunnel address lease belongs to the Logical Tunnel/device, not to LiveID;
- same-account active devices receive distinct tunnel IPs;
- the server supplies authenticated address/prefix/route configuration; Windows must not globally hard-code `10.66.0.2/30`;
- the address pool is configurable because no private/CGNAT range is collision-free everywhere;
- server ingress drops raw packets whose source address does not match the tunnel's assigned lease;
- future IPv6 uses the same binding principle with an assigned `/128`.

## Canonical establishment sequence

```text
raw FakeTCP SYN / SYN-ACK / ACK
        -> temporary bounded reliable ordered bootstrap stream
        -> real TLS 1.3 ClientHello/ServerHello/Finished
        -> Reality-like marker recognition
        -> shared username/password admission inside TLS
        -> fresh one-time transport credential + Logical Tunnel config inside TLS
        -> bootstrap ACK drain + explicit mode barrier
        -> SAME 4-tuple / SAME FakeTCP association / NO new SYN
        -> DTLS 1.3 association
        -> LINK bind to the authenticated Logical Tunnel
        -> immutable FEC/LINK state for this transport epoch
        -> raw-IP payload
```

No separate product ordinary-TCP Reality connection is introduced.

## Reality-like bootstrap requirements

- real TLS 1.3 records on the FakeTCP sequence space;
- configured SNI and WBD recognition marker;
- username/password only inside TLS;
- one-time credential/tunnel configuration only inside TLS;
- bounded bootstrap duration/memory;
- bootstrap writes are ACK-gated for reliable ordered TLS bytes;
- the ordered bootstrap adapter is destroyed before steady packet delivery;
- unrecognized ClientHello traffic may remain in bounded stream mode and proxy to a configured fallback target when qualified.

Browser/REALITY resemblance is evidence-driven. Do not claim browser-perfect or a numeric `99%` resemblance unless a reproducible pcap analyzer defines and measures the claim. If a fidelity technique would require sustained kernel-TCP payload or a second public connection, the no-HOL/single-flow rules win unless a newer ADR explicitly changes them.

## Linux Windows-raw-IP product shape

Final product direction:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> exactly one active WBD transport for that tunnel
```

The per-LiveID netns + veth + inner NAT + host NAT implementation is historical/reference evidence, not the selected final design. The earlier VRF/conntrack-zone prototype remains rejected.

## Transport idle, rotation and reconnect policy

Logical Tunnel lifetime is not FakeTCP transport lifetime.

Track separately:

- `last_payload_activity` for real tunneled IP payload;
- `last_transport_activity` for payload plus PING/PONG/control.

PING/PONG/control does not reset the user's payload-idle timer.

A configurable non-zero idle timeout may close the one public transport while retaining Logical Tunnel identity, leased IP, Wintun and connected capture/routing/DNS state in `DORMANT`; public flow count is then zero. A new captured packet may wake it through a small bounded local buffer and one new public association.

Transport age rotation may exist, but release semantics are **break-before-make**:

```text
old transport ACTIVE
  -> stop new payload admission
  -> bounded close/flush when possible
  -> retire old public transport
  -> create one replacement public FakeTCP association
  -> same-flow Reality-like bootstrap -> DTLS -> LINK
  -> resume payload
```

Because the product forbids two simultaneous public connections, planned rotation/network migration may have a short packet pause. A bounded local reconnect buffer is allowed; a second public flow is not.

On abrupt path loss, a stale server-side record may briefly remain even though the client can no longer use that physical path. A newly authenticated replacement may supersede stale bookkeeping; this must not become two concurrently usable data transports.

Explicit user Disconnect/Exit releases the Logical Tunnel and restores WBD-owned network state. Server restart may destroy NAT/conntrack and is not guaranteed to preserve every application TCP flow.

## Historical Game Lane / race work

`internal/gamelane` and related race/multipath experiments are retained only as research/history unless a future explicit product decision reopens multipath.

Current release code must not:

- duplicate one logical payload onto multiple public WBD associations;
- maintain 2..4 public lane sets;
- create a candidate lane before the active public flow is retired;
- expose a Game/weak-network lane-count option above one.

This restriction is about public transport count. It does not invalidate earlier algorithmic evidence about first-arrival/dedup itself.

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
- one connected Logical Tunnel uses one public transport.

## Platform invariants

### OpenWrt

Final capture remains **TPROXY + policy routing** with mandatory public-underlay escape and WBD-owned cleanup.

### Windows

Final capture remains **Wintun/TUN raw L3**. Underlay escape is mandatory. Device-wide IPv6 remains fail-closed for the entire connected interval until a real IPv6 tunnel path is qualified.

Disconnect/Exit must restore WBD-owned routes, DNS/NRPT, IPv6 and firewall state. Npcap install/licensing constraints remain unchanged; WBD must not improperly redistribute the Free Edition.

### Linux server / firewall

The product server exposes one public `WBD_PORT`. Internal LINK/DTLS/raw-IP services remain private implementation details. Firewall helpers add/remove only WBD-owned state and must never flush or replace the user's host ruleset.

## Observability and secrecy

Keep non-secret tunnel/session correlation IDs, first-packet/boundary markers and counters. Do not enable per-packet INFO logging. Usernames/passwords, tickets, resume secrets and identity secrets must not be logged.

## Qualification gates

Before physical artifact delivery, exact-head automation must prove at least:

1. one SYN lineage preserves same-association Reality-like bootstrap -> DTLS -> LINK -> payload;
2. no ordinary kernel-TCP WBD bootstrap/payload connection exists in product startup;
3. no new SYN/4-tuple change occurs at the bootstrap barrier;
4. post-bootstrap no-HOL hole-bypass passes;
5. Windows Npcap ignores unrelated ARP/IPv6/UDP/TCP traffic before valid WBD frames;
6. same TunnelID cannot own two concurrently usable LINK transports;
7. Logical Tunnel lease survives a break-before-make transport reconnect;
8. two separate Logical Tunnels receive distinct leases and remain isolated;
9. DNS/UDP/TCP pass shared TUN + one host NAT;
10. lease source spoofing is rejected;
11. FEC `off` and `20:20` remain qualified;
12. Windows build/admin-smoke/capability tests and Linux release/firewall/full-stack tests pass from the exact source head;
13. exact-source physical Windows 11 -> Ubuntu ARM64 passes DNS/UDP/TCP and cleanup after automated qualification.

## Retired / non-product architectures

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as WBD sustained payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- release multipath / 2..4 Game Lane public transports;
- make-before-break public transport overlap;
- per-LiveID netns/veth/double-NAT Windows raw-IP final design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epochs/in-place FEC switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.

## Development discipline

- **ADR-0013 controls public transport count and replacement semantics.**
- ADR-0012 controls retained Logical Tunnel/lease/shared-TUN semantics only where it does not conflict with ADR-0013.
- TCP-like/FakeTCP core recovery should remain frozen unless a reproducible failing qualification demonstrates a defect that cannot be solved above it.
- New architecture rules override older conflicting dev logs/ADRs; conflicts must be corrected in current docs rather than silently coexisting.
- Detailed development decisions, failures, exact heads, qualification results and unresolved physical-only items must be written under `docs/development/` and summarized in `.wbd/handoff/current.json`.
