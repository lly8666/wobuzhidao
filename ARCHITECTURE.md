# Architecture v2.4

> **Status: ACTIVE MAINLINE DESIGN.** ADR-0013 is authoritative for public transport count and replacement. ADR-0012 remains authoritative only for retained Logical Tunnel/address-lease/shared-TUN decisions that do not require multipath.

## Product intent

WBD is a personal weak-network VPN for privileged OpenWrt/Linux and Windows endpoints. Public WBD payload remains WBD-owned raw TCP-shaped FakeTCP, while sustained VPN payload stays packet/datagram-oriented and does not inherit ordinary kernel TCP ordered-delivery HOL.

The product separates a long-lived **Logical Tunnel** from one disposable **Public Transport Epoch**.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - TunnelID
          - server-assigned unique tunnel address lease
          - zero public transports while disconnected/dormant
          - exactly one active public transport while connected
```

One public transport epoch owns:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable bootstrap on that same association
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit barrier, no FIN and no second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> immutable LINK
  -> optional fixed FEC
  -> packet/datagram payload
```

## Global single-public-flow invariant

The invariant applies to the **whole connected Logical Tunnel**, not merely to an individual lane:

- a connected tunnel has exactly one usable public FakeTCP association;
- product startup has no separate ordinary kernel-TCP Reality connection;
- the same 4-tuple and FakeTCP sequence space carry Reality-like bootstrap and steady payload phases;
- no ordinary kernel TCP byte stream owns sustained WBD payload;
- the bootstrap stream adapter is bounded and destroyed before steady packet mode;
- release product configuration does not expose 2..4 public lanes;
- candidate/old public transport overlap is forbidden;
- normal replacement is break-before-make.

`internal/logicaltunnel.MaxProductPublicTransportLanes == 1` is a release invariant rather than a tuning parameter. The LINK server also rejects a second concurrently usable transport claim for the same TunnelID as defense in depth.

Historical `internal/gamelane` / race code remains research/history unless a future explicit ADR reopens multipath. It is not a current product path.

## Canonical packet stack

```text
Windows Wintun / OpenWrt TPROXY captured packet
        ↓
WBD raw packet envelope
        ↓
Logical Tunnel / address lease
        ↓
exactly one active public transport
        ↓
optional fixed systematic FEC
        ↓
DTLS 1.3 application datagram
        ↓
WBD FakeTCP raw association
        ↓
public network
```

FEC belongs to the current transport epoch. There is no cross-transport dependency or waiting.

## Logical Tunnel identity and server-assigned address lease

One shared username/password may authenticate several devices. Authentication creates a disposable transport credential/ticket, but tunnel address identity belongs to the Logical Tunnel/device rather than to LiveID or a FakeTCP epoch.

The server assigns each active Logical Tunnel a unique IPv4 lease from a configurable pool. The client must not globally hard-code `10.66.0.2/30`. Authenticated tunnel configuration supplies address/prefix/route parameters.

Same-account active devices receive different tunnel IPs. A short break-before-make reconnect may keep the same Logical Tunnel and lease.

Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

Future IPv6 applies the same binding to an assigned address.

## Linux Windows-raw-IP server data plane

The selected product direction is:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> exactly one active WBD transport for that tunnel
```

Upstream packets are authenticated to a Logical Tunnel, source-validated against its lease, then injected into the shared TUN. Downstream packets returning through Linux reverse NAT/routing are read from the shared TUN and demultiplexed by leased destination address to the owning Logical Tunnel, then emitted through that tunnel's one current transport.

The per-session Linux netns/veth/inner-NAT/host-NAT implementation is retained only as historical/correctness evidence. It is not the selected final product architecture. The earlier VRF/conntrack-zone prototype remains rejected.

## Idle sleep and long-lived VPN behavior

The logical VPN may stay enabled while its public transport sleeps.

Track two clocks:

- `last_payload_activity`: real tunneled IP payload only;
- `last_transport_activity`: payload plus PING/PONG/control.

PING/PONG maintains liveness/NAT state but does not reset the user's payload-idle timer.

A configurable non-zero idle timeout may close the one public transport while the Logical Tunnel, leased IP, Wintun and routing/DNS state remain. Public transport count is then zero. A new captured packet may wake the tunnel using a small bounded local buffer and one new public association.

Explicit Disconnect/Exit releases the Logical Tunnel and restores WBD-owned capture/routing/DNS/IPv6/firewall state.

## Transport age rotation and replacement

A transport epoch may have an age deadline, but replacement is **break-before-make**:

```text
old transport ACTIVE
  -> stop new payload admission
  -> bounded flush/close when possible
  -> old public association retired locally
  -> create one new FakeTCP association
  -> same-flow Reality-like TLS bootstrap
  -> DTLS + LINK ready
  -> resume payload
```

There is no old+candidate race interval and no second simultaneously usable public transport.

This deliberately trades zero-gap planned migration for the stronger global one-public-flow invariant. A small bounded local reconnect buffer is allowed above the transport. A second public connection is not.

On abrupt underlay loss, the server may temporarily retain a stale peer record even though the client can no longer send on the old physical path. A new authenticated replacement may supersede stale bookkeeping; the implementation must not turn this into two concurrently usable data transports.

## Reality-like bootstrap

Reality-like TLS is the first protected payload phase inside the one FakeTCP association. Required behavior:

- plausible TCP-shaped SYN/SYN-ACK/ACK persona;
- real TLS 1.3 ClientHello/ServerHello/Finished on the same FakeTCP sequence space;
- configured SNI and WBD recognition marker;
- username/password only inside TLS;
- one-time transport/tunnel credential only inside TLS;
- bounded ACK-gated ordered bootstrap stream;
- no FIN/RST/new WBD payload SYN between bootstrap and DTLS mode;
- explicit bootstrap barrier destroys ordered stream semantics before steady payload.

Unrecognized ClientHello traffic may remain in bounded stream mode and proxy to the configured decoy/fallback target when qualified.

Browser/REALITY resemblance is evidence-driven. Do not claim browser-perfect or a numeric `99%` similarity without a reproducible packet-capture metric. A fidelity technique may not reintroduce ordinary kernel-TCP sustained payload, a second public connection, or post-bootstrap ordered-delivery HOL without a newer explicit ADR.

## No-HOL steady data plane

After the bootstrap barrier:

- each DTLS application datagram is independently authenticated;
- later independently complete payload may be delivered while an earlier FakeTCP sequence range is missing;
- shadow ACK/SACK/retransmission exists to preserve the TCP-shaped outer behavior, not to impose kernel-TCP ordered delivery;
- systematic FEC sources are not delayed merely to fill a block;
- release FEC is `off` or fixed systematic `20:20` unless explicitly reopened.

The existing TCP-like/FakeTCP recovery core is considered mature and should remain frozen unless a deterministic qualification demonstrates a defect that cannot be fixed above it.

## Frozen security and weak-network boundaries

- WBD-owned raw TCP-shaped FakeTCP remains the public payload carrier;
- no ordinary kernel-TCP sustained payload path and no TCP-over-TCP HOL dependency;
- pinned wolfSSL DTLS 1.3 remains steady-state cryptographic authority;
- wolfSSL pin remains `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP shadow recovery remains release default; `sack-rack` remains experimental;
- release FEC remains `off` or fixed `20:20`;
- <=100 Mbit/s weak-link qualification ceiling remains in force;
- 40 Mbit/s aggregate-inner remains the conservative release operating point.

## Platform requirements

### Windows

- final capture remains Wintun/TUN raw L3;
- public underlay escape is mandatory;
- one `Connect()` owns one `wbd-faketcp` public child; another Connect is rejected until lifecycle teardown;
- Npcap ingress filters unrelated ARP/IPv6/UDP/unrelated TCP before FakeTCP parsing;
- IPv6 remains fail-closed during the connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state;
- Npcap release/licensing constraints remain unchanged.

### Linux/OpenWrt

- Linux product server exposes one public `WBD_PORT`; internal LINK/DTLS/raw-IP services remain private implementation details;
- there is no parallel kernel-TCP Reality product listener competing with the raw single-flow listener;
- WBD firewall helpers add/remove only WBD-owned rules/state and never flush the user's host ruleset;
- OpenWrt final capture remains TPROXY + policy routing.

## Observability and secrecy

Retain non-secret session/tunnel correlation IDs, first-boundary markers and counters. Do not emit per-packet INFO logs. Usernames/passwords, tickets, resume secrets and identity secrets must not be logged.

## Required qualification before artifact delivery

The exact source head must prove:

1. one SYN lineage carries Reality-like TLS bootstrap -> DTLS -> LINK -> raw-IP payload;
2. no separate ordinary kernel-TCP Reality bootstrap/payload connection exists in product startup;
3. no new SYN or 4-tuple change occurs at the bootstrap barrier;
4. post-bootstrap no-HOL hole-bypass passes;
5. Windows Npcap ingress noise filtering passes sequence/mutation/fuzz coverage;
6. same TunnelID cannot own two concurrently usable LINK transports;
7. Logical Tunnel lease survives a break-before-make reconnect;
8. distinct logical tunnels receive distinct leases and remain isolated;
9. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
10. source spoofing across leases is rejected;
11. FEC `off` and `20:20` remain qualified;
12. same-flow startup stress and TCP/TLS persona capture gates pass;
13. Windows build/admin-smoke/capability and Linux release/firewall/full-stack gates all pass from the same exact source head;
14. exact-source physical Windows 11 -> Ubuntu ARM64 DNS/UDP/TCP and cleanup passes after automated qualification.

## Retired / superseded product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- release multipath / 2..4 public Game Lane transports;
- make-before-break public candidate overlap;
- per-LiveID Windows raw-IP netns + veth + double NAT as final product design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as the WBD data plane;
- WireGuard inner glue;
- Android/no-root.

Startup RTT optimization remains separate from proving the global single-flow/no-HOL architecture.
