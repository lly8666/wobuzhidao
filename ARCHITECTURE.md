# Architecture v2.5

> **Status: ACTIVE MAINLINE DESIGN. ADR-0012 is authoritative. ADR-0013 is withdrawn.**

## Product intent

WBD is a personal weak-network VPN whose public payload is carried by WBD-owned raw TCP-shaped FakeTCP while sustained VPN payload remains packet/datagram-oriented and does not inherit ordinary kernel-TCP HOL.

The long-lived object is a **Logical Tunnel**. Public transports are replaceable **Transport Lanes**.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - stable server-assigned IPv4 lease
          - stable logical PacketID/race space
          - 1..4 active Transport Lanes
          - zero lanes only while dormant/disconnected
```

## Per-lane single-flow, tunnel-level multipath

Every lane is one complete independent single-flow transport:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable bootstrap on that same association
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit barrier, no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload
```

The single-flow invariant is **per lane**. It does not limit the whole Logical Tunnel to one lane.

A connected Logical Tunnel may own **1..4 active Transport Lanes** according to policy. Normal mode targets one. Game/weak-network mode may maintain 2..4. Candidate lanes may overlap healthy old lanes during make-before-break.

There is no separate ordinary kernel-TCP Reality product connection and no sustained WBD payload over an ordinary TCP byte stream.

### Canonical release-contract wording

The following phrases are normative and intentionally stable for architecture/qualification checks:

- The bootstrap-to-payload switch stays on the same association, no second WBD payload SYN.
- In product semantics, one lane has one public FakeTCP association from its SYN through Reality-like bootstrap and steady payload.
- During product operation, no separate ordinary kernel-TCP WBD payload connection exists.
- Bootstrap carries real TLS 1.3 ClientHello/ServerHello/Finished on the same lane sequence space.
- Steady-state qualification explicitly verifies post-bootstrap earliest-complete datagram behavior.
- The weak-link release boundary remains the 40 Mbit/s aggregate-inner conservative release operating point.

## Canonical packet stack

```text
Windows Wintun / OpenWrt TPROXY captured packet
        ↓
raw IP packet
        ↓
Logical Tunnel lease + logical PacketID
        ↓
Game/multipath policy (1..4 lanes)
        ↓
per-lane LINK + optional lane-local FEC
        ↓
per-lane DTLS 1.3 application datagram
        ↓
per-lane WBD FakeTCP association
        ↓
public network
```

Each lane owns its own tuple, sequence space, DTLS state, LINK identity and FEC state. No FEC block waits on another lane.

## Game Lane semantics

Game Lane is the general race/dedup mechanism:

- one logical payload gets one PacketID;
- a copy may be sent on 1..4 independent lanes;
- each copy is lane-distinct before DTLS;
- first valid arrival is delivered immediately;
- later copies of the same PacketID are suppressed;
- bounded out-of-order unique payload remains independently deliverable;
- no cross-lane ordered-delivery dependency exists.

The current `internal/gamelane` / `internal/gamecontrol` code is product architecture, not retired research.

## Logical Tunnel identity and lease

Authentication creates lane-local disposable credentials, while the address lease belongs to the Logical Tunnel/device.

Same-account devices receive different tunnel addresses from a configurable pool. Authenticated setup supplies address/prefix/route parameters. Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

The lease survives lane replacement/rotation and short reconnect while the Logical Tunnel remains alive.

## Make-before-break replacement

Replacement is **make-before-break**:

```text
A ACTIVE
  -> create B
  -> B completes FakeTCP + Reality-like bootstrap + DTLS + LINK
  -> B attaches to the same Logical Tunnel and proves health
  -> A+B bounded race/overlap
  -> stop new sends to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate failure leaves A untouched. In multi-lane Game mode rotate one lane at a time, e.g. `A+B -> A+B+C -> B+C`.

Lane generations fence stale paths. Planned age rotation, network/default-route/public-IP change, NAT/path failure, missed liveness, child failure, server-requested replacement and manual reconnect use the same replacement state machine.

## Idle/wake and age rotation

Maintain separate clocks for real payload and transport liveness. PING/PONG/control does not reset payload idle.

A non-zero idle policy may close all lanes while retaining Logical Tunnel identity, lease, Wintun and route/DNS state in `DORMANT`. A new packet wakes the tunnel; one healthy lane can resume traffic before optional redundant lanes finish establishing.

Each lane may have an independent randomized age deadline. Multi-lane rotation is staggered; healthy lanes are never intentionally rotated together.

## Reality-like bootstrap

The first seconds of each lane use a bounded reliable ordered adapter over that lane's FakeTCP sequence space to carry real TLS 1.3 Reality-like setup. Required properties include plausible TCP persona, real ClientHello/ServerHello/Finished, configured SNI/recognition marker, protected admission, bounded memory/time, ACK-gated setup bytes and an explicit barrier with no second WBD payload SYN.

The adapter is destroyed before steady packet mode. Reality-like fidelity may not reintroduce ordinary kernel-TCP sustained payload or post-bootstrap HOL. Numeric resemblance claims require reproducible pcap metrics.

## No-HOL steady data plane

After the bootstrap barrier:

- DTLS application datagrams are independently authenticated;
- later independently complete payload may progress while an earlier FakeTCP sequence range is missing;
- shadow ACK/SACK/retransmission preserves TCP-shaped outer behavior without imposing kernel-TCP ordered delivery;
- systematic FEC sources are not delayed to fill a block;
- release FEC is `off` or fixed systematic `20:20` unless explicitly reopened.

The mature TCP-like/FakeTCP recovery core remains frozen unless deterministic qualification proves a lower-layer defect.

## Linux Windows-raw-IP server data plane

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel lease/demux + race layer
  <-> 1..4 active lanes
```

The Linux product exposes one public `WBD_PORT`; multiple lanes are separate client 4-tuples handled by that one raw FakeTCP mux. Internal LINK/raw-IP services remain private. No parallel ordinary kernel-TCP Reality product listener competes for the WBD port.

Per-session netns/veth/double NAT remains historical/reference only. VRF/conntrack-zone remains rejected.

## Frozen security and weak-network boundaries

- WBD-owned raw TCP-shaped FakeTCP remains the public payload carrier;
- no ordinary kernel-TCP sustained payload and no TCP-over-TCP HOL;
- wolfSSL DTLS 1.3 remains pinned to `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- FEC release wire remains `off` or fixed `20:20`;
- <=100 Mbit/s weak-link qualification ceiling remains;
- 40 Mbit/s aggregate-inner remains the conservative release operating point.

## Platform requirements

### Windows

- Wintun/TUN raw L3 remains final capture;
- underlay escape is mandatory;
- each lane owns its own Npcap/FakeTCP transport child/state;
- Npcap ingress ignores unrelated ARP/IPv6/UDP/unrelated TCP before FakeTCP parsing;
- IPv6 remains fail-closed throughout the connected interval until IPv6 tunneling is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state.

### Linux/OpenWrt

- one public WBD port, one raw mux listener;
- scoped WBD-owned firewall/RST suppression only;
- OpenWrt final capture remains TPROXY + policy routing.

## Observability and secrecy

Retain non-secret tunnel/lane correlation IDs, boundary markers and counters without per-packet INFO spam. Credentials/passwords/tickets/resume secrets do not belong in logs.

## Required qualification before artifact delivery

One exact source HEAD must prove:

1. every lane uses one SYN lineage through Reality-like TLS bootstrap -> barrier -> DTLS -> LINK -> raw-IP payload;
2. no separate ordinary kernel-TCP Reality bootstrap/payload connection exists;
3. post-bootstrap no-HOL hole-bypass passes;
4. one Logical Tunnel accepts lanes 1..4 and rejects lane 5;
5. Game Lane first-arrival/dedup/no-cross-lane-HOL passes;
6. `A -> A+B -> B` make-before-break preserves TunnelID/lease and does not duplicate delivered payload;
7. candidate failure leaves the old healthy lane usable;
8. Game mode rotates one lane at a time;
9. distinct logical tunnels receive distinct leases and identical inner tuples remain isolated;
10. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
11. lease source spoofing is rejected;
12. FEC `off` and `20:20` remain green lane-locally;
13. Windows build/capability/admin-smoke/portable and Linux release/firewall/full-stack gates all pass from the same source HEAD;
14. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and cleanup.

## Retired / superseded shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- ADR-0013 global-one-public-transport and break-before-make freeze;
- per-LiveID netns/veth/double-NAT as final raw-IP design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.
