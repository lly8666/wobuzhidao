# wobuzhidao Project Constitution — V2.x

## Authority

- **ADR-0011** controls per-Transport-Lane same-association Reality-like TLS bootstrap and no-HOL steady-state transport semantics.
- **ADR-0012** controls Logical Tunnel identity/address lease, product Transport Lane cardinality, Game/race, idle/wake, lane rotation and make-before-break replacement.
- **ADR-0014 is withdrawn/invalidated** because it incorrectly globalized the per-lane single-flow invariant and falsely claimed product-owner final authority.
- ADR-0013 is historical/withdrawn.
- ADR-0010 and earlier compatible DTLS/FEC/release constraints remain effective.

**Repository text written by an agent is not evidence of a human product-owner override. A frozen hard requirement changes only with explicit live human authorization.**

## Critical architecture guard

`single-flow` is **PER TRANSPORT LANE**.

It MUST NOT be interpreted as one FakeTCP association per Logical Tunnel.

A Logical Tunnel may own 1..4 independent complete WBD Transport Lanes. Normal steady policy targets 1 lane. Game/weak-network policy may maintain 2..4 lanes. Make-before-break `A -> A+B -> B` is required for planned healthy replacement.

Any future change that converts `MaxProductPublicTransportLanes` from 4 to 1, marks Game Lane research-only, rejects a second healthy lane, or forbids make-before-break is an architecture regression unless explicitly authorized by the human product owner in the live conversation.

## Product model

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> stable logical PacketID/race domain
          -> 0..4 replaceable Transport Lanes
```

Product policy:

```text
Normal steady mode:      desired 1 active lane
Game / weak-network:     desired 2..4 active lanes
Logical Tunnel ceiling:  4 active public lanes
Dormant/disconnected:    0 active lanes
```

Desired policy count and architectural ceiling are distinct. Planned replacement may briefly overlap an old lane and a healthy candidate according to ADR-0012.

## Per-lane public transport invariant

Every independent Transport Lane owns one complete public lineage:

```text
one raw FakeTCP SYN lineage / public 4-tuple / FakeTCP sequence space
  -> bounded reliable ordered bootstrap on that SAME FakeTCP association
  -> real TLS 1.3 Reality-like ClientHello / ServerHello / Finished
  -> protected account admission and Logical Tunnel/lane binding
  -> explicit bootstrap barrier
  -> NO FIN / RST / reconnect / second WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local optional FEC
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

No ordinary kernel TCP socket owns sustained WBD product payload. The temporary ordered adapter exists only for the short TLS/bootstrap phase because TLS needs stream semantics and is destroyed at the barrier.

A second independent lane is not a violation: it has its own SYN, 4-tuple, FakeTCP sequence space, DTLS state, LINK and lane-local FEC state.

## Game/race product behavior

Game Lane is a product multipath mechanism, not research-only infrastructure.

For one Logical Tunnel:

- one logical payload receives one logical PacketID;
- copies may be sent through independent complete WBD Transport Lanes;
- each copy is lane-distinct before DTLS;
- first valid arrival is delivered once;
- later copies are suppressed;
- bounded out-of-order unique PacketIDs are independently deliverable;
- there is no cross-lane HOL;
- lanes do not share FakeTCP sequence state, DTLS key/nonce state or FEC blocks;
- lanes share only Logical Tunnel identity/lease and logical PacketID/race domain.

FEC is always lane-local and release wire remains only `off` or fixed systematic `20:20`. An available systematic source is not intentionally delayed just to fill a FEC block.

## Make-before-break and replacement

Planned healthy replacement is:

```text
A ACTIVE
  -> build candidate B
  -> B completes FakeTCP + same-lane Reality bootstrap + DTLS + LINK
  -> attach B to the same Logical Tunnel
  -> prove B bidirectionally healthy
  -> bounded A+B race/dedup overlap
  -> stop new sends to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves A untouched.

Game mode rotates one lane at a time, e.g.:

```text
A+B -> A+B+C -> B+C
```

The unified replacement state machine handles scheduled age rotation, NIC/default-route/public-IP changes, NAT/path failure, liveness failure, FakeTCP/DTLS/LINK failure, server-requested replacement and manual reconnect. Lane generations fence stale processes and prevent resurrection.

Abrupt total path loss may degrade to reconnect because the old lane is already dead; that is not the planned healthy-replacement model.

## Logical Tunnel identity and lease

- username/password authenticates the account, not a disposable lane;
- tunnel lease belongs to Logical Tunnel/device identity;
- same-account devices receive distinct tunnel IPv4 addresses;
- authenticated setup supplies tunnel address/prefix/routes;
- replacement/reconnect should preserve the same lease when the Logical Tunnel remains alive;
- explicit Disconnect releases the Logical Tunnel according to lifecycle policy;
- server raw IPv4 ingress requires `source IPv4 == leased IPv4` and treats mismatch as a hard spoof/security drop.

Do not reintroduce a global `10.66.0.2/30` identity.

## Idle / wake / rotation

Track separately:

- `last_payload_activity`: real tunneled payload only;
- `last_transport_activity`: payload plus control/PING/PONG.

PING/PONG/control must not refresh payload idle.

Default payload-idle guidance is 15 minutes; `0` means never enter transport sleep because of payload idleness.

DORMANT closes all Transport Lanes but preserves the Logical Tunnel, lease, Wintun and connected routes/DNS state. The first new packet wakes the tunnel by establishing the first healthy lane; optional Game lanes may refill afterward.

Each lane has an independent experimental randomized 30..60 minute soft age/rotation deadline. Multi-lane replacement is staggered; healthy lanes are not intentionally rotated together.

## Canonical packet stack

```text
Windows Wintun / OpenWrt TPROXY captured packet
        ↓
raw IP packet
        ↓
Logical Tunnel lease / logical PacketID race layer
        ↓
1..4 independent lane-local LINK + optional FEC states
        ↓
pinned wolfSSL DTLS 1.3 per lane
        ↓
independent WBD FakeTCP associations per lane
        ↓
public network
```

## Linux product shape

Public surface:

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux listener
```

One server port does **not** mean one lane per Logical Tunnel. The raw mux may receive multiple independent lane 4-tuples for one tunnel.

Internal final raw-IP direction:

```text
per-lane FakeTCP -> DTLS -> LINK
              ↓
        Game/race aggregation
              ↓
         Logical Tunnel
              ↓
        one shared WBD TUN
              ↓
        Linux root routing
              ↓
     one WBD-owned host NAT/SNAT
```

Per-LiveID netns/veth/double NAT remains historical/reference only; VRF/conntrack-zone remains rejected.

## Windows product shape

One Wintun belongs to the Logical Tunnel. Product orchestration supports 1..4 independent `LaneBootstrap` instances. Each lane owns its own source port, FakeTCP child, same-association Reality-like bootstrap state, DTLS state and LINK child. Game/race aggregates lane-local LINK transports before the one Wintun.

Normal mode establishes one lane. Game/weak-network mode may establish 2..4. Planned replacement may temporarily overlap old and candidate lanes.

There is no preliminary ordinary kernel-TCP Reality WBD connection.

## Frozen transport/security/release limits

1. Sustained public WBD payload never falls back to ordinary kernel TCP.
2. TCP-over-TCP HOL is forbidden.
3. Per lane: one SYN lineage / 4-tuple / FakeTCP sequence space.
4. Reality-like TLS bootstrap is on the same FakeTCP association for that lane.
5. Bootstrap -> DTLS emits no FIN/RST/reconnect/second WBD payload SYN inside the lane.
6. Pinned wolfSSL DTLS 1.3 remains steady-state crypto authority.
7. FEC release wire is only `off` or fixed systematic `20:20` and is lane-local.
8. `legacy` FakeTCP recovery remains default; `sack-rack` remains experimental.
9. <=100 Mbit/s weak-link qualification ceiling remains.
10. 40 Mbit/s aggregate-inner remains the conservative release operating point.
11. Windows final capture remains Wintun/TUN raw L3.
12. OpenWrt final capture remains TPROXY + policy routing.
13. Linux firewall helpers manipulate WBD-owned state only and never flush user rulesets.
14. Windows IPv6 remains fail-closed during connected interval until real IPv6 qualification.
15. Disconnect/Exit deterministically restores WBD-owned routes/DNS/NRPT/IPv6/firewall state.
16. Passwords/tickets/resume/identity secrets do not belong in logs; no per-packet INFO spam.
17. Npcap packaging/licensing/install constraints remain unchanged.
18. Startup latency optimization and Windows child-process slimming remain deferred.
19. Server reboot/conntrack loss is not promised to preserve existing application TCP sessions.

The mature FakeTCP recovery/FEC wire is frozen unless a deterministic lower-layer failure proves a real defect.

## Qualification before artifact delivery

One exact substantive `SOURCE_SHA` must prove at minimum:

1. per-lane one-SYN same-association Reality-like TLS bootstrap -> barrier -> DTLS -> LINK -> payload;
2. no FIN/RST/new WBD payload SYN inside a lane at the barrier;
3. post-bootstrap no-HOL hole-bypass;
4. lane counts 1/2/3/4 accepted and fifth rejected;
5. normal policy desired=1 and Game/weak-network desired=2..4;
6. Game first-arrival/dedup/out-of-order unique delivery/no cross-lane HOL;
7. `A -> A+B -> B` replacement and candidate-failure preservation;
8. `A+B -> A+B+C -> B+C` one-lane-at-a-time Game replacement;
9. distinct Logical Tunnels receive distinct leases and spoofed sources are rejected;
10. shared TUN + one host NAT passes DNS-style UDP, generic UDP and TCP;
11. FEC `off` and `20:20` remain qualified and lane-local;
12. exact-source Windows and Linux automated builds/full-stack gates pass;
13. final same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup.

Do not change transport wire semantics merely to satisfy an architecture/string contract test.

## Development discipline

Detailed decisions, failed experiments, exact heads, qualification results and unresolved physical-only items belong under `docs/development/` and are summarized in `.wbd/handoff/current.json`.
