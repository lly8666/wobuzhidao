# Architecture v2.x

> **Status: ACTIVE MAINLINE DESIGN. ADR-0012 is authoritative for Logical Tunnel identity, 1..4 Transport Lanes, Game/race and lifecycle. ADR-0011 controls same-association Reality-like setup inside each lane.**

## Product intent

WBD is a personal weak-network VPN whose sustained public carrier is WBD-owned raw TCP-shaped FakeTCP rather than an ordinary kernel-TCP byte stream.

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          - stable TunnelID
          - stable server-assigned IPv4 lease
          - shared logical PacketID / race namespace
          - 1..4 replaceable Transport Lanes while active
```

A Logical Tunnel is the user-visible long-lived VPN identity. A Transport Lane is disposable transport state.

## Per-lane same-association lineage

Every Transport Lane independently owns one complete public lineage:

```text
one raw FakeTCP SYN lineage / public 4-tuple / FakeTCP sequence space
  -> bounded reliable ordered bootstrap on that SAME association
  -> real TLS 1.3 Reality-like ClientHello / ServerHello / Finished
  -> configured SNI + protected admission + authenticated tunnel configuration
  -> explicit in-band bootstrap barrier
  -> no FIN / RST / reconnect / second WBD payload SYN inside the lane
  -> pinned wolfSSL DTLS 1.3 on that SAME association
  -> immutable LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality WBD connection and no ordinary kernel TCP socket owns sustained WBD payload.

`single-flow` is therefore a **per-Transport-Lane / per-Transport-Epoch invariant**, not a one-flow-per-Logical-Tunnel-lifetime invariant.

## Reality-like setup phase

FakeTCP owns each lane's public tuple from the first SYN. Its bounded bootstrap adapter temporarily provides the reliable ordered byte-stream behavior TLS needs. Real TLS 1.3, configured SNI, Reality-like recognition and protected account/tunnel admission run on the same FakeTCP sequence space.

The adapter is bounded in time/memory and destroyed at the explicit barrier. The transition does not emit FIN/RST, reconnect, or create a new WBD payload SYN inside that lane.

Reality-like fidelity is evidence-driven; use reproducible packet captures and handshake traces rather than unsupported similarity percentages.

## No-HOL steady data plane

After the bootstrap barrier:

- DTLS application datagrams are independently authenticated;
- independently complete payload may progress despite an earlier missing FakeTCP sequence range according to qualified WBD recovery semantics;
- WBD shadow ACK/SACK/retransmission preserves TCP-shaped behavior without ordinary kernel-TCP ordered-delivery HOL;
- FEC sources are not intentionally delayed merely to fill a block;
- FEC state never spans lanes.

The mature TCP-like/FakeTCP recovery and FEC wire remain frozen unless deterministic lower-layer qualification proves an actual defect.

## Logical Tunnel transport cardinality

```text
minimum active product lanes = 1
maximum active product lanes = 4
```

Policy:

- Normal steady mode: desired lanes = 1.
- Game / weak-network mode: desired lanes = 2..4.
- DORMANT/disconnected: active lanes = 0.
- A fifth active product lane is rejected.

Each lane has an independent FakeTCP tuple/sequence space, Reality-like bootstrap, DTLS state, LINK state, lane-local FEC and health/path state. All lanes share the Logical Tunnel identity/lease and logical PacketID race namespace.

## Game/race multipath

The current Game layer is not the rejected V1 ordinary-kernel-TCP lane pool.

One logical payload receives one logical PacketID. Copies may be emitted through 1..4 independent complete WBD lanes. The first valid arrival is delivered; later copies are deduplicated. Bounded out-of-order unique packets remain independently deliverable. There is no cross-lane HOL.

Do not create a second migration-specific sequence space when the existing SessionID/PacketID/LaneID race envelope already supplies the needed logical ordering/dedup semantics.

## Logical Tunnel identity and lease

TunnelID and the server-assigned tunnel IPv4 address belong to Logical Tunnel/device identity, not a disposable lane/LiveID/public tuple.

- same-account different installations receive distinct active leases;
- authenticated lane setup supplies the same Logical Tunnel configuration to lanes attached to that tunnel;
- short lane replacement/reconnect should preserve the lease where possible;
- explicit release permits later reassignment;
- prefer `/32` plus explicit routes where Windows qualification supports it.

Server raw IPv4 ingress requires:

```text
inner IPv4 source == leased tunnel IPv4
```

Mismatch is a hard security drop.

## Make-before-break replacement

Normal planned healthy replacement is:

```text
A ACTIVE
  -> build B completely
  -> B FakeTCP -> Reality-like bootstrap -> DTLS -> LINK
  -> authenticate/attach B to same Logical Tunnel
  -> health gate
  -> A+B bounded race using existing PacketID/dedup
  -> stop new sends to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves healthy A active.

Game mode rotates one lane at a time:

```text
A+B -> build C -> A+B+C -> retire A -> B+C
```

Do not intentionally rotate all healthy lanes together.

## Lane generation fencing and unified replacement

Each lane incarnation has a generation/epoch. Old goroutines, FakeTCP RX, DTLS/LINK callbacks, timers and candidates must verify their generation before mutating current state. Stale-generation events are ignored.

One replacement lifecycle handles scheduled age rotation, NIC/default-route/public-IP changes, NAT/path changes, missed PONG/no RX, FakeTCP/DTLS/LINK failures, server-requested replacement and manual reconnect:

```text
detect trigger
  -> request replacement
  -> build candidate
  -> fully authenticate / READY
  -> health gate
  -> overlap/race
  -> switch
  -> drain old
  -> retire old
```

## Idle, DORMANT and wake

Track separately:

- `last_payload_activity`: real tunneled payload only;
- `last_transport_activity`: payload + control/health traffic.

PING/PONG/keepalive/health probes do not refresh payload idle.

Initial payload-idle default is 15 minutes. `idle_timeout=0` disables idle-induced sleep only; lane age rotation continues.

When non-zero payload idle expires, close all lanes and enter DORMANT while preserving Logical Tunnel, TunnelID, lease, Wintun, routes, DNS/NRPT and relevant local product state. A first new real payload wakes the tunnel. The first healthy lane resumes traffic; optional Game lanes fill afterward.

Explicit Disconnect/Exit is different and deterministically cleans WBD-owned routes, DNS/NRPT, IPv6 fail-closed rules, firewall state and runtime state, and releases the lease according to product policy.

## Lane age policy

Every active lane has an independent maximum-age policy. Initial guidance is a randomized soft age around 30..60 minutes. Multi-lane mode staggers replacement. This is lifecycle policy, not a wire constant.

## Canonical Windows stack

```text
one Wintun / raw L3
      ↓
Logical Tunnel / stable lease / PacketID race namespace
      ↓
Game/race aggregation (1 lane Normal, 2..4 Game)
      ↓
Lane A: LINK -> DTLS -> same-association Reality-like bootstrap + FakeTCP
Lane B: LINK -> DTLS -> same-association Reality-like bootstrap + FakeTCP
optional Lane C/D
      ↓
one public WBD server port via independent lane 4-tuples
```

Every lane uses an independent dynamic source port and FakeTCP child/state. No lane uses ordinary kernel TCP as the sustained outer carrier.

Windows IPv6 remains fail-closed during ACTIVE/DORMANT/replacement operation until actual IPv6 support is qualified.

## Canonical Linux server stack

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux
        <-> many independent lane public tuples
        <-> per-lane DTLS + LINK
        <-> Logical Tunnel manager + Game/race
        <-> one shared WBD TUN
        <-> Linux root routing
        <-> one WBD-owned host NAT/SNAT
```

One server public port does not imply one lane per tunnel.

Per-LiveID netns/veth/double NAT remains historical/reference only. VRF/conntrack-zone remains rejected as the final product architecture.

Firewall helpers manipulate WBD-owned state only; never flush/replace unrelated user firewall state.

## Frozen weak-network/security/release boundaries

- sustained outer WBD payload never uses ordinary kernel TCP and never reintroduces TCP-over-TCP HOL;
- wolfSSL DTLS 1.3 remains pinned to `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`;
- `legacy` FakeTCP recovery remains default; `sack-rack` remains experimental;
- FEC release wire remains `off` or fixed systematic `20:20`, lane-local and immutable per lane/epoch;
- <=100 Mbit/s weak-link qualification ceiling remains;
- 40 Mbit/s aggregate-inner remains the conservative release operating point across the whole Logical Tunnel, not per lane;
- Windows capture remains Wintun/TUN raw L3; OpenWrt remains TPROXY + policy routing;
- Linux exposes one public WBD port and uses scoped WBD-owned firewall/NAT state;
- Windows IPv6 remains fail-closed until qualified;
- Disconnect/Exit cleanup is deterministic;
- secrets do not belong in logs and per-packet INFO spam is forbidden;
- Npcap packaging/licensing constraints remain unchanged;
- startup RTT optimization, DTLS HRR redesign, LINK bootstrap coalescing and Windows child slimming remain deferred;
- current lifecycle work does not reopen FEC tuning.

## Required qualification before artifact delivery

One exact substantive `SOURCE_SHA` must prove:

1. each lane has one SYN / one 4-tuple / one sequence lineage from Reality-like TLS bootstrap through DTLS, LINK and payload;
2. no preliminary ordinary kernel-TCP Reality WBD flow and no second payload SYN inside a lane;
3. post-bootstrap no-HOL hole-bypass;
4. 1, 2, 3 and 4 active product lanes are accepted and a fifth is rejected;
5. Normal desired=1 and Game desired=2..4;
6. Game first-arrival/dedup/out-of-order unique/no-cross-lane-HOL semantics;
7. make-before-break `A -> A+B -> B`, candidate-failure preservation and Game one-lane rotation;
8. lane generation fencing and unified replacement triggers;
9. DORMANT/wake retains Logical Tunnel local state and resumes after first lane READY;
10. distinct Logical Tunnels receive distinct leases and spoof mismatches are rejected;
11. DNS-style UDP, generic UDP and TCP pass shared TUN + one host NAT;
12. FEC `off` and `20:20` and mature FakeTCP recovery remain qualified;
13. exact-head Windows and Linux automated gates/artifacts pass;
14. same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes Normal/Game/lifecycle/network-change/cleanup qualification before release designation.

## Retired / invalid product shapes

- V1 ordinary kernel-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as sustained WBD payload carrier;
- `Reality TCP -> close -> new FakeTCP payload SYN`;
- global `MaxProductPublicTransportLanes = 1`;
- Game/multipath disabled for shipping;
- planned break-before-make replacement of a healthy lane;
- rejection of a legitimate second complete lane for the same Logical Tunnel;
- per-LiveID netns/veth/double NAT as final raw-IP design;
- VRF/conntrack-zone final raw-IP design;
- runtime FEC epoch switching or cross-lane FEC;
- VLESS/Xray/Vision stream semantics as WBD data plane;
- WireGuard inner glue;
- Android/no-root.
