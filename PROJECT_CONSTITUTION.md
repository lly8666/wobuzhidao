# wobuzhidao Project Constitution — V2.x

## Authority

- **ADR-0012 is the current authoritative Logical Tunnel / lifecycle / multipath architecture.**
- ADR-0011 is the compatible technical authority for same-association Reality-like TLS bootstrap and no-HOL steady-state semantics **inside each Transport Lane**.
- ADR-0013, ADR-0014 and ADR-0015 are historical/withdrawn where they globalized the per-lane single-flow invariant or disabled product multipath.
- ADR-0010 and earlier compatible DTLS/FEC/release constraints remain effective.

A repository document written by an agent cannot override a later explicit live human product-owner instruction.

## Product identity model

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> shared logical PacketID / race namespace
          -> 1..4 replaceable Transport Lanes while active
```

Logical Tunnel state owns TunnelID, leased tunnel address, installation identity, logical PacketID space and Windows Wintun/routing/DNS product state.

A Transport Lane owns its FakeTCP association/public tuple, lane generation, Reality-like TLS bootstrap, DTLS 1.3, LINK, lane-local FEC, liveness/age and path/NIC/NAT binding. Lanes are disposable; Logical Tunnel identity is not.

## Single-flow invariant — per Transport Lane

`single-flow` is **per Transport Lane / per Transport Epoch**, not per Logical Tunnel lifetime.

Every lane independently owns one complete public lineage:

```text
one raw FakeTCP SYN lineage / public 4-tuple / FakeTCP sequence space
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap on that SAME association
  -> configured SNI + Reality-like recognition
  -> protected account admission + authenticated Logical Tunnel configuration
  -> explicit in-band bootstrap barrier
  -> no FIN / RST / reconnect / second WBD payload SYN inside the lane
  -> pinned wolfSSL DTLS 1.3 on that SAME FakeTCP association
  -> immutable LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

Forbidden:

- ordinary kernel TCP for sustained outer WBD payload;
- `ordinary Reality TCP -> close -> separate FakeTCP payload flow`;
- splitting one lane's Reality/DTLS/payload across different public transports.

Allowed and required by product policy: multiple independent complete lanes under one Logical Tunnel.

## Product transport cardinality and Game Lane

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 4
```

Policy:

- Normal steady mode: desired lanes = 1.
- Game / weak-network mode: desired lanes = 2..4.
- DORMANT/disconnected: active lanes = 0.
- A fifth active product lane is rejected.

Game/race is current product architecture, not the rejected V1 ordered-kernel-TCP multilane design.

One logical payload receives one logical PacketID. Copies may race over independent complete lanes. The first valid arrival delivers immediately, later copies are suppressed, and bounded out-of-order unique packets remain independently deliverable. There is no cross-lane HOL.

FEC remains lane-local and never spans lanes.

## Replacement and generation fencing

Planned healthy replacement is **make-before-break** and reuses the existing PacketID/race/dedup primitive:

```text
A ACTIVE
  -> build candidate B completely
  -> B FakeTCP + same-lane Reality-like TLS + DTLS + LINK
  -> authenticate/attach B to existing Logical Tunnel
  -> health gate
  -> bounded A+B overlap / race
  -> stop assigning new payload to A
  -> drain A
  -> retire A
  -> B ACTIVE
```

Candidate B failure leaves healthy A active.

Game mode rotates one lane at a time, for example:

```text
A+B -> A+B+C -> B+C
```

Do not rotate every healthy Game lane simultaneously.

Every replacement incarnation carries a lane generation/epoch. Any old goroutine, FakeTCP RX, DTLS/LINK callback, timeout or candidate must verify its generation before mutating current tunnel/lane state. Stale generations are ignored/dropped.

## Unified replacement lifecycle

Scheduled lane age, NIC/default-route/public-IP changes, NAT/path changes, missed PONG/no RX, FakeTCP/DTLS/LINK failures, server request and manual reconnect feed one replacement state machine:

```text
detect trigger
  -> request replacement
  -> build candidate
  -> candidate fully authenticated/ready
  -> health gate
  -> overlap/race
  -> switch
  -> drain old
  -> retire old
```

Do not build separate incompatible reconnect protocols for each trigger.

## Idle, liveness and DORMANT

Track at least:

- `last_payload_activity`: real tunneled IP payload only;
- `last_transport_activity`: payload plus health/control traffic.

PING/PONG/keepalive/health probes must not refresh payload idle.

Initial payload-idle default is 15 minutes. `idle_timeout = 0` means never sleep because of payload idleness; it does not disable lane age rotation.

When non-zero payload idle expires:

```text
ACTIVE -> DORMANT
```

Close all Transport Lanes while preserving Logical Tunnel, TunnelID, leased IP, Wintun, routes, DNS/NRPT and relevant product state. The first real payload wakes the tunnel. Traffic resumes after the first lane becomes READY; optional Game lanes attach afterward.

Explicit Disconnect/Exit is different: stop lanes, release lease as policy requires, remove WBD-owned routes/DNS/NRPT/IPv6/firewall state and stop product runtime deterministically.

## Lane age policy

A lane cannot live forever. Initial policy is a randomized soft age around 30..60 minutes, independent of payload-idle configuration. Multi-lane deadlines must be staggered.

This is lifecycle policy, not a wire-protocol constant.

## Logical Tunnel lease and anti-spoof

The server assigns each Logical Tunnel/installation a unique IPv4 lease from a configurable pool.

- same account + different installation => distinct tunnel addresses;
- short reconnect/replacement should retain the same lease where possible;
- lane replacement does not change tunnel IP;
- explicit release permits later reassignment;
- prefer `/32` plus explicit routes where Windows qualification supports it.

Server raw IPv4 ingress must enforce:

```text
inner IPv4 source == leased tunnel IPv4
```

Mismatch is a hard security drop. TunnelID/lease identity must not be confused with lane LiveID/public tuple.

## Linux product shape

Public surface remains one WBD public port:

```text
Internet
  <-> one WBD_PORT / raw FakeTCP mux
       <-> many independent Transport Lane tuples
       <-> Logical Tunnel manager + Game/race
       <-> one shared WBD TUN
       <-> Linux root routing
       <-> one WBD-owned host NAT/SNAT
```

One public server port does not mean one lane per Logical Tunnel.

Per-session netns/veth/double NAT and VRF/conntrack-zone are historical qualification/reference paths, not final product architecture.

Firewall helpers may manipulate only WBD-owned chains/marks/rules and must never flush the user's ruleset.

## Windows product shape

One Wintun belongs to one Logical Tunnel. Product orchestration may own 1..4 lane groups:

```text
Logical Tunnel
  -> Lane A: FakeTCP -> Reality-like TLS -> DTLS -> LINK
  -> Lane B: FakeTCP -> Reality-like TLS -> DTLS -> LINK
  -> optional Lane C/D
  -> Game/race aggregation
  -> one Wintun
```

Each lane has its own source port/public tuple and transport processes but shares the authenticated Logical Tunnel identity/lease and logical PacketID race namespace.

Windows IPv6 remains fail-closed throughout connected/DORMANT/replacement operation until actual IPv6 qualification. Disconnect/Exit cleans the WBD-owned fail-closed rules.

## Frozen transport/security/release boundaries

1. Sustained outer WBD payload never falls back to ordinary kernel TCP.
2. TCP-over-TCP HOL is forbidden.
3. Reality-like TLS bootstrap stays on each lane's own FakeTCP association.
4. Pinned wolfSSL DTLS 1.3 remains steady-state crypto authority.
5. Mature FakeTCP recovery stays frozen; `legacy` remains default and `sack-rack` experimental.
6. FEC release wire is only `off` or fixed systematic `20:20`; FEC is lane-local and immutable for one lane/epoch.
7. Systematic source packets are not intentionally delayed merely to fill an FEC block.
8. Weak-link qualification ceiling remains <=100 Mbit/s.
9. Conservative release operating point remains 40 Mbit/s aggregate inner, not per lane.
10. Windows capture remains Wintun/TUN raw L3; OpenWrt remains TPROXY + policy routing.
11. Linux exposes one public WBD port.
12. Windows IPv6 remains fail-closed until qualified.
13. Disconnect/Exit cleanup is deterministic and scoped to WBD-owned state.
14. Passwords/tickets/resume/identity secrets are never logged; no per-packet INFO spam.
15. Npcap packaging/licensing constraints remain unchanged.
16. Startup RTT optimization and Windows child-process slimming remain deferred.
17. Current lifecycle work must not reopen FEC profile tuning or startup-handshake redesign.

## Required qualification before artifact delivery

One exact substantive `SOURCE_SHA` must prove at minimum:

1. each lane has one SYN/4-tuple/sequence lineage from Reality-like TLS bootstrap through DTLS, LINK and payload with no second WBD payload SYN inside the lane;
2. no preliminary ordinary kernel-TCP Reality WBD connection;
3. post-bootstrap no-HOL hole-bypass;
4. active lane counts 1, 2, 3 and 4 are accepted and a fifth is rejected;
5. Normal desired=1 and Game desired=2..4;
6. Game first-arrival/dedup/out-of-order unique delivery has no cross-lane HOL;
7. scheduled `A -> A+B -> B` replacement works and candidate failure preserves A;
8. Game one-lane rotation `A+B -> A+B+C -> B+C` works;
9. generation fencing rejects stale lane events;
10. DORMANT/wake preserves Logical Tunnel local state and resumes after first READY lane;
11. distinct Logical Tunnels receive distinct leases and source spoofing is rejected;
12. shared TUN + one host NAT passes DNS-style UDP, generic UDP and TCP;
13. FEC `off` and `20:20` remain lane-local and qualified;
14. exact-head Windows/Linux automated qualification passes;
15. final same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes functional/lifecycle/cleanup qualification before release designation.

Do not change mature transport wire semantics merely to satisfy an architecture/string contract test. Detailed decisions, failures, exact heads and qualification evidence belong under `docs/development/` and the current handoff.
