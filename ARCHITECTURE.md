# Architecture v2.x

> **Status: ACTIVE MAINLINE DESIGN. ADR-0011 controls each Transport Lane; ADR-0012 controls Logical Tunnel multipath/lifecycle. ADR-0014 is withdrawn/invalidated.**

## Product intent

WBD is a personal weak-network VPN whose public weak-network carrier is WBD-owned raw TCP-shaped FakeTCP. `single-flow` describes each independent Transport Lane, not the entire Logical Tunnel.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - stable server-assigned IPv4 lease
          - stable logical PacketID/race domain
          - 0..4 independent replaceable Transport Lanes
```

Policy:

- Normal steady state: 1 lane.
- Game / weak-network: 2..4 lanes.
- Architectural ceiling: 4 active public lanes.
- Dormant/disconnected: 0 lanes.

## Per-lane same-association lineage

Every Transport Lane independently owns exactly one complete public lineage:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered bootstrap on that SAME association
  -> real TLS 1.3 Reality-like recognition/admission
  -> explicit barrier; no FIN/RST/reconnect/new WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC off or fixed 20:20
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality product connection and no sustained WBD payload over an ordinary kernel TCP byte stream.

A second independent lane is valid because it has a different 4-tuple, SYN lineage, FakeTCP sequence space, DTLS state, LINK state and lane-local FEC state.

## Reality-like setup phase

FakeTCP owns each lane from the first SYN. Its bounded bootstrap adapter temporarily provides the reliable ordered byte-stream behavior TLS needs. Real TLS 1.3, configured SNI, Reality-like recognition and protected admission run on that same FakeTCP sequence space.

The bootstrap adapter is bounded in memory/time and destroyed at the explicit barrier. The transition does not send FIN/RST, reconnect or create another WBD payload SYN inside that lane.

Reality/browser fidelity is evidence-driven. Do not claim a numeric similarity percentage without a reproducible pcap metric.

## No-HOL steady data plane

After a lane crosses its bootstrap barrier:

- DTLS application datagrams are independently authenticated;
- later independently complete payload may progress while an earlier FakeTCP sequence range is missing;
- WBD shadow ACK/SACK/retransmission preserves TCP-shaped outer behavior without imposing ordinary TCP ordered delivery;
- systematic FEC sources are not delayed merely to fill a block;
- FEC is lane-local and never spans lanes.

The mature TCP-like/FakeTCP recovery and FEC wire remains frozen unless deterministic qualification proves a lower-layer defect.

## Logical Tunnel multipath / Game layer

Game/race operates above independent complete WBD lanes and below the one Logical Tunnel/Wintun/shared-TUN view.

```text
Logical Tunnel / one logical PacketID domain
├─ Lane A: FakeTCP -> Reality-like TLS -> DTLS -> LINK -> FEC
├─ Lane B: FakeTCP -> Reality-like TLS -> DTLS -> LINK -> FEC
├─ Lane C: FakeTCP -> Reality-like TLS -> DTLS -> LINK -> FEC
└─ Lane D: FakeTCP -> Reality-like TLS -> DTLS -> LINK -> FEC
```

For a raced logical payload:

- one PacketID is assigned;
- copies may be emitted through selected healthy lanes;
- first valid arrival is delivered once;
- later duplicates are suppressed;
- bounded out-of-order unique packets are independently deliverable;
- there is no cross-lane HOL.

Lanes do not share FakeTCP sequence spaces, DTLS key/nonce state or FEC blocks. They share Logical Tunnel identity/lease and the logical race domain only.

## Make-before-break lifecycle

Planned healthy replacement is:

```text
A ACTIVE
  -> build B
  -> B completes its own FakeTCP + same-lane Reality bootstrap + DTLS + LINK
  -> attach B to same Logical Tunnel
  -> prove health
  -> bounded A+B race/dedup overlap
  -> stop new sends to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves A untouched.

Game mode rotates one healthy lane at a time:

```text
A+B -> A+B+C -> B+C
```

One replacement state machine covers age rotation, NIC/default-route/public-IP change, NAT/path failure, liveness failure, FakeTCP/DTLS/LINK failure, server-requested replacement and manual reconnect. Generation fencing prevents stale lane resurrection.

## Logical Tunnel identity and lease

The server-assigned tunnel address belongs to the Logical Tunnel/device identity rather than disposable FakeTCP/DTLS/LINK state.

- same-account devices receive distinct leases;
- authenticated setup supplies address/prefix/route configuration;
- lane rotation/replacement preserves the Logical Tunnel lease while the tunnel remains alive;
- raw IPv4 ingress requires `source == leased IPv4`; mismatch is a hard spoof/security drop.

Do not reintroduce a global fixed `10.66.0.2/30` identity.

## Idle / wake / rotation

Track separately:

- `last_payload_activity` for real tunneled payload;
- `last_transport_activity` for payload + liveness/control.

PING/PONG/control does not reset payload-idle time.

Default payload idle guidance is 15m; `0` disables payload-idle sleep.

DORMANT closes all Transport Lanes while preserving Logical Tunnel, lease, Wintun/routes/DNS. First new packet establishes the first healthy lane; optional Game lanes refill afterward.

Each lane has an independent experimental randomized 30..60m soft age deadline. Multi-lane rotation is staggered.

## Canonical Windows stack

```text
one Wintun / raw L3
      ↓
Logical Tunnel / race layer
      ↓
1..4 independent LaneBootstrap instances
      ↓
per-lane LINK
      ↓
per-lane DTLS 1.3
      ↓
per-lane same-association Reality-like bootstrap + FakeTCP
      ↓
public network
```

Each lane owns an independent source port, FakeTCP child, Reality bootstrap state, DTLS and LINK. Normal mode establishes one lane; Game/weak-network may maintain 2..4. There is no preliminary ordinary kernel-TCP Reality WBD connection.

## Canonical Linux server stack

One public server port does not imply one lane per Logical Tunnel.

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux
        <-> many independent lane 4-tuples
        <-> per-lane DTLS + LINK
        <-> Game/race aggregation by Logical Tunnel
        <-> one shared WBD TUN
        <-> Linux root routing
        <-> one WBD-owned host NAT/SNAT
```

Per-LiveID netns/veth/double NAT remains historical/reference only. VRF/conntrack-zone remains rejected.

## Frozen weak-network/security boundaries

- no ordinary kernel-TCP sustained WBD payload and no TCP-over-TCP HOL;
- wolfSSL DTLS 1.3 remains pinned to `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP recovery remains default; `sack-rack` experimental;
- FEC release wire remains `off` or fixed systematic `20:20`, lane-local;
- <=100 Mbit/s weak-link qualification ceiling remains;
- 40 Mbit/s aggregate-inner remains the conservative release operating point;
- Windows capture remains Wintun/TUN raw L3 with mandatory underlay escape;
- Windows IPv6 remains fail-closed while connected until real IPv6 qualification;
- OpenWrt capture remains TPROXY + policy routing;
- Linux firewall manipulation remains WBD-owned/scoped;
- Disconnect/Exit restores WBD-owned routes/DNS/NRPT/IPv6/firewall state;
- secrets do not belong in logs and per-packet INFO spam is forbidden;
- Npcap packaging/licensing constraints remain;
- startup RTT optimization and Windows child slimming remain deferred;
- server reboot/conntrack loss is not promised to preserve arbitrary existing application TCP.

## Required qualification before artifact delivery

One exact substantive source HEAD must prove:

1. per-lane one-SYN same-association Reality-like TLS bootstrap -> barrier -> DTLS -> LINK -> payload;
2. no FIN/RST/reconnect/new WBD payload SYN inside a lane across the barrier;
3. post-bootstrap no-HOL hole-bypass;
4. lane counts 1,2,3,4 accepted; fifth rejected;
5. normal desired=1; Game/weak desired=2..4;
6. Game first-arrival, dedup, bounded out-of-order unique delivery and no cross-lane HOL;
7. make-before-break `A -> A+B -> B`, candidate failure preserving A, and staggered Game replacement;
8. distinct Logical Tunnels receive distinct leases and spoof mismatch is rejected;
9. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
10. FEC `off` and `20:20` remain lane-local/green;
11. exact-head Windows/Linux automated full-stack and release artifacts pass;
12. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

## Retired / invalid product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- global-one-lane ADR-0013/ADR-0014 policy;
- break-before-make as planned healthy replacement;
- per-LiveID netns/veth/double NAT as final raw-IP design;
- VRF/conntrack-zone raw-IP prototype;
- runtime FEC epoch switching;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.
