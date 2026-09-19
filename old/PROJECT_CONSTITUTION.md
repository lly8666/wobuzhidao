# wobuzhidao Project Constitution — V2.x

## Authority

- **ADR-0012 is the current authoritative Logical Tunnel / lifecycle / multipath architecture.**
- ADR-0011 controls same-association Reality-like TLS bootstrap and no-HOL steady-state behavior inside each Transport Lane.
- ADR-0013, ADR-0014 and ADR-0015 are historical/withdrawn where they globalized the per-lane invariant or disabled product multipath.
- Compatible ADR-0010 DTLS/FEC/release boundaries remain effective.

A later explicit live human product-owner instruction outranks stale agent-authored repository text.

## Machine-readable architecture guard

**single-flow is PER TRANSPORT LANE**, not per Logical Tunnel lifetime.

**Logical Tunnel may own 1..4 independent complete WBD Transport Lanes.**

**Game Lane is a product multipath mechanism, not research-only.**

Planned healthy replacement is **A -> A+B -> B**. Candidate failure preserves healthy A.

The steady-state crypto authority remains **pinned wolfSSL DTLS 1.3**. Release FEC remains `off` or **fixed systematic `20:20`** and is always lane-local. The conservative release operating point remains **40 Mbit/s aggregate-inner** across the Logical Tunnel.

## Product identity

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> shared logical SessionID / PacketID race namespace
          -> 1..4 replaceable Transport Lanes while active
```

Logical Tunnel owns TunnelID, lease, installation identity, logical PacketID space, Wintun/routes/DNS state and product lifecycle. A lane owns its FakeTCP association/public tuple, generation, Reality-like bootstrap, DTLS, LINK, lane-local FEC, liveness/age and path binding.

## Per-lane public lineage

Every lane is complete and independent:

```text
one raw FakeTCP SYN lineage / 4-tuple / sequence space
  -> bounded reliable ordered Reality-like TLS 1.3 bootstrap on the SAME association
  -> configured SNI + protected account/tunnel admission
  -> explicit in-band barrier
  -> no FIN/RST/reconnect/new WBD payload SYN inside that lane
  -> pinned wolfSSL DTLS 1.3
  -> LINK
  -> lane-local FEC off or fixed systematic 20:20
  -> packet/datagram payload without ordinary kernel-TCP HOL
```

The sustained outer WBD payload has **no ordinary kernel-TCP HOL**.

Forbidden:

- ordinary kernel TCP as sustained outer WBD carrier;
- `ordinary Reality TCP -> close -> separate FakeTCP payload flow`;
- Reality/DTLS/payload split across different public transports within one lane;
- cross-lane FEC or a second migration ordering namespace.

The mature FakeTCP ACK/SACK/RTO/recovery wire is frozen unless deterministic lower-layer evidence proves a defect. `legacy` remains default; `sack-rack` remains experimental.

## Product lane cardinality and Game

```text
MinProductPublicTransportLanes = 1
MaxProductPublicTransportLanes = 4
```

Normal desired lanes = 1. Game/weak-network desired lanes = 2..4. Dormant/disconnected = 0. A fifth active product lane is rejected.

Game Lane races the same logical PacketID over independent complete WBD lanes. The first valid arrival wins, duplicates are suppressed, bounded out-of-order unique packets deliver without waiting for another lane, and there is no cross-lane HOL.

## Replacement, rotation and generation fencing

Planned healthy replacement is make-before-break:

```text
A ACTIVE
  -> build B completely
  -> authenticate/attach B to the same Logical Tunnel
  -> health gate
  -> A+B bounded race using the existing PacketID/dedup primitive
  -> drain A
  -> retire A
  -> B ACTIVE
```

If B fails, A remains active.

Game rotation replaces one healthy lane at a time, e.g. `A+B -> A+B+C -> B+C`; do not rotate all lanes together.

Every lane incarnation has a generation/epoch. Old goroutines, FakeTCP RX, DTLS/LINK callbacks, timeouts and candidates must verify `generation == current expected generation` before mutating state. Stale generations are ignored.

Age, NIC/default-route/public-IP/NAT changes, missed PONG/no RX, FakeTCP/DTLS/LINK failure, server request and manual reconnect converge on one replacement lifecycle.

## Idle, liveness and DORMANT

Maintain separate `last_payload_activity` and `last_transport_activity`. Only real tunneled payload refreshes payload idle; PING/PONG/keepalive/health probes do not.

Initial payload-idle default is 15 minutes. `idle_timeout=0` means no idle-induced sleep; lane age rotation still applies.

DORMANT closes all Transport Lanes but preserves Logical Tunnel, TunnelID, leased IP, Wintun, routes, DNS/NRPT and relevant local state. First real payload wakes the tunnel; the first READY lane resumes traffic and optional Game lanes attach afterward.

Explicit Disconnect/Exit is not DORMANT: it deterministically removes WBD-owned routes, DNS/NRPT, IPv6 fail-closed state, firewall state and runtime state and releases the lease according to policy.

Each lane also has randomized soft age policy around 30..60 minutes; multi-lane ages are staggered.

## Tunnel lease and anti-spoof

The lease belongs to the Logical Tunnel/installation, not a lane. Same-account different installations receive distinct addresses; short lane replacement should preserve the address where possible. Prefer `/32` plus explicit routes where Windows qualification supports it.

Server raw IPv4 ingress must enforce:

```text
inner IPv4 source == leased tunnel IPv4
```

Mismatch is a hard security drop.

## Linux product architecture

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux
       <-> many independent lane tuples
       <-> Logical Tunnel manager + Game/race
       <-> one shared WBD TUN
       <-> Linux root routing
       <-> one WBD-owned host NAT/SNAT
```

One public server port does not mean one lane per tunnel. Per-session netns/veth/double NAT and VRF/conntrack-zone are historical/reference only. Firewall helpers touch WBD-owned state only and never flush unrelated user rules.

## Windows product architecture

One Wintun belongs to one Logical Tunnel. Product orchestration may own 1..4 complete lane groups, each with an independent FakeTCP source port/tuple and DTLS/LINK state, feeding the Game/race layer and one Wintun.

Windows IPv6 remains fail-closed during ACTIVE, DORMANT and replacement until real IPv6 tunneling is qualified. Disconnect/Exit cleans WBD-owned fail-closed rules.

## Frozen release boundaries

- FEC wire: `off` or fixed systematic `20:20` only; lane-local and immutable for one lane/epoch.
- Source packets do not wait merely to fill an FEC block.
- Weak-link qualification ceiling <=100 Mbit/s.
- Conservative operating point: 40 Mbit/s aggregate-inner, not per lane.
- Windows capture: Wintun/TUN raw L3. OpenWrt: TPROXY + policy routing.
- One public WBD server port.
- Secrets are never logged; no per-packet INFO spam.
- Npcap packaging/licensing constraints remain.
- Startup RTT redesign, DTLS HRR optimization, LINK bootstrap coalescing, Windows child slimming and new FEC ratios are deferred.

## Qualification before artifact delivery

One exact substantive `SOURCE_SHA` must prove: per-lane one-SYN same-association bootstrap/no-HOL; lane counts 1/2/3/4 accepted and fifth rejected; Normal=1 and Game=2..4; Game race/dedup/no-cross-lane-HOL; make-before-break with candidate-failure preservation; one-lane-at-a-time Game rotation; generation fencing; DORMANT/wake; unified triggers; distinct leases and source anti-spoof; shared-TUN DNS/UDP/TCP; FEC off/20:20; exact-head Windows/Linux gates and artifacts; and final same-source physical Windows 11 + Npcap -> Ubuntu ARM64 lifecycle/cleanup qualification.

Until those conditions pass, use status labels separately: IMPLEMENTED, AUTOMATED-GREEN, PHYSICAL-GREEN, RELEASE-QUALIFIED.
