# wobuzhidao Project Constitution — V2.5

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- WBD-owned raw TCP-shaped FakeTCP as the public weak-network transport;
- a short real-TLS Reality-like bootstrap carried inside the **same FakeTCP association for each lane**;
- no FIN/RST/new WBD payload SYN between a lane's bootstrap and sustained payload;
- UDP/datagram-like sustained payload semantics with no ordinary-kernel-TCP retransmission/HOL dependency;
- pinned standards-compliant wolfSSL DTLS 1.3 for steady-state encryption/integrity/anti-replay;
- optional lane-local FEC, release wire `off` or fixed systematic `20:20`;
- a long-lived Logical Tunnel with a stable server-assigned address lease;
- **1..4 independent WBD Transport Lanes** while active, with Game Lane first-arrival/dedup and make-before-break lifecycle;
- OpenWrt final transparent capture through TPROXY;
- Windows final capture through Wintun/TUN raw L3.

The <=100 Mbit/s weak-link qualification ceiling and conservative 40 Mbit/s aggregate-inner release operating point remain unchanged unless a later benchmark ADR explicitly changes them.

**ADR-0012 is authoritative. ADR-0013 is withdrawn.**

## Per-lane single-flow and no-HOL invariants

The **single-flow invariant applies to each lane**, not to the entire Logical Tunnel.

Every lane owns exactly one complete public lineage:

```text
raw FakeTCP SYN / SYN-ACK / ACK
  -> bounded reliable ordered bootstrap stream on that same sequence space
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit bootstrap barrier; no FIN/RST/new WBD payload SYN
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC
  -> packet/datagram payload
```

Non-negotiable properties:

1. There is no preliminary ordinary kernel-TCP Reality connection for a product lane.
2. Ordinary kernel TCP never owns sustained WBD payload.
3. The temporary reliable/ordered adapter exists only because TLS setup needs stream semantics and is destroyed at the bootstrap barrier.
4. After the barrier, later independently complete authenticated datagrams must be able to progress while an earlier FakeTCP sequence range is missing.
5. Shadow ACK/SACK/retransmission preserves plausible TCP-shaped behavior but must not reintroduce ordinary TCP ordered-delivery HOL.
6. Systematic FEC sources are not delayed merely to fill a block, and FEC state never spans lanes.
7. Mature TCP-like/FakeTCP recovery remains frozen unless a deterministic qualification proves a defect below the lifecycle layer.

## Logical Tunnel and lane ownership

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> stable logical PacketID/race space
          -> 1..4 disposable Transport Lanes while active
          -> zero lanes only while dormant/disconnected
```

- username/password authenticates the account, not lane identity;
- each lane has fresh disposable FakeTCP/DTLS/LINK/LiveID state;
- the tunnel lease belongs to Logical Tunnel/device identity, not to any lane;
- same-account devices receive distinct tunnel addresses;
- authenticated setup supplies address/prefix/route configuration;
- server ingress rejects raw IPv4 whose source does not equal the tunnel lease.

## Game Lane and multipath

Game Lane is a current product mechanism, not retired research.

- one logical payload receives one `PacketID`;
- policy may emit copies on 1..4 independent lanes;
- each lane has independent FakeTCP tuple/sequence space, DTLS keys/nonces, LINK and FEC state;
- first valid arrival is delivered immediately;
- later copies of that PacketID are suppressed;
- bounded out-of-order unique packets remain independently deliverable;
- there is no cross-lane HOL.

Normal mode targets one healthy steady lane. Game/weak-network policy may maintain 2..4 healthy lanes.

## Make-before-break lifecycle

**Make-before-break** is required for planned replacement while an old lane remains healthy:

```text
A ACTIVE
  -> establish candidate B completely
  -> attach B to same Logical Tunnel and prove bidirectional health
  -> A+B bounded race/overlap using the same logical PacketID dedup layer
  -> stop new sends to A
  -> drain/retire A
  -> B ACTIVE
```

Candidate failure leaves A untouched. In Game mode only one healthy lane is intentionally rotated at a time, e.g. `A+B -> A+B+C -> B+C`.

The same replacement state machine handles age rotation, Windows network/default-route/public-IP change, NAT/path failure, missed-liveness, child failure, server-requested replacement and manual reconnect. Lane generations fence stale paths.

## Idle/wake behavior

Track `last_payload_activity` separately from transport liveness. PING/PONG/control does not reset the user-visible payload-idle timer.

A configurable non-zero idle timeout may close all lanes while retaining Logical Tunnel identity, lease, Wintun and capture/routing/DNS state in `DORMANT`. A new packet wakes the tunnel. The first healthy lane may resume traffic before optional redundant lanes finish establishing. Explicit Disconnect/Exit releases the Logical Tunnel and restores WBD-owned network state.

## Reality-like bootstrap fidelity

The first seconds of **each lane** should resemble ordinary Reality-like/TLS behavior as closely as reproducibly practical while preserving the no-HOL architecture:

- plausible TCP-shaped SYN/options/ACK progression;
- real TLS 1.3 ClientHello/ServerHello/Finished;
- configured SNI and WBD recognition marker;
- credentials only inside TLS;
- bounded memory/time and ACK-gated setup stream;
- same public 4-tuple and FakeTCP sequence space across the bootstrap barrier;
- no second WBD payload SYN.

Do not claim a numeric `99%`/browser-perfect similarity without a reproducible packet-capture metric. Fidelity may not be achieved by putting sustained payload on ordinary kernel TCP.

## Linux raw-IP server shape

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel lease/demux
  <-> 1..4 active lanes
```

The public product surface remains one `WBD_PORT`; different lanes are independent client 4-tuples to that same raw FakeTCP listener. There is no parallel ordinary kernel-TCP Reality product listener. Per-LiveID netns/veth/double NAT remains historical/reference only; VRF/conntrack-zone remains rejected.

## Security and release freeze

- pinned wolfSSL remains `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- DTLS 1.3 remains steady-state cryptographic authority;
- 0-RTT remains disabled until replay/resume semantics are explicitly designed;
- FEC release wire remains `off` or fixed `20:20`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- secrets/tickets/passwords must not be logged;
- non-secret boundary markers/counters remain without per-packet INFO spam.

## Platform invariants

### Windows

- Wintun/TUN raw L3 remains the capture path;
- underlay escape is mandatory;
- Npcap handles per-lane raw FakeTCP packet I/O and must ignore unrelated ARP/IPv6/UDP/unrelated TCP noise;
- IPv6 remains fail-closed for the connected interval until a real IPv6 tunnel path is qualified;
- Disconnect/Exit restores WBD-owned routes, DNS/NRPT, IPv6 and firewall state;
- Npcap licensing/install constraints remain unchanged.

### Linux/OpenWrt

- Linux server exposes one public WBD port and private internal LINK/raw-IP services;
- WBD firewall helpers add/remove only WBD-owned state and never flush the user's ruleset;
- OpenWrt final capture remains TPROXY + policy routing.

## Release qualification

Before physical artifact delivery, one exact substantive source HEAD must prove at least:

1. every lane uses one SYN lineage through Reality-like TLS bootstrap -> barrier -> DTLS -> LINK -> payload;
2. a Logical Tunnel admits 1..4 active lanes and rejects a fifth;
3. Game Lane first-arrival/dedup/no-cross-lane-HOL passes;
4. `A -> A+B -> B` make-before-break passes without duplicate delivery;
5. candidate failure leaves the old lane usable;
6. Game mode rotates one lane at a time while preserving redundancy;
7. distinct Logical Tunnels receive distinct leases and can use identical inner tuples independently;
8. shared TUN + one NAT passes DNS-style UDP, generic UDP and TCP;
9. source spoofing across leases is rejected;
10. FEC `off` and `20:20` remain qualified lane-locally;
11. Windows build/capability/admin-smoke/portable and Linux release/firewall/full-stack gates pass on that same source HEAD;
12. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and cleanup.

## Development discipline

- **ADR-0012 controls current transport count, Game Lane and make-before-break semantics.**
- ADR-0013 is historical/withdrawn and must not be used to re-freeze the product to one global lane.
- New deterministic failures are fixed at the first broken layer; do not reopen mature FakeTCP recovery without evidence.
- Detailed decisions, failed experiments, exact heads and qualification results belong under `docs/development/` and are summarized in `.wbd/handoff/current.json`.
