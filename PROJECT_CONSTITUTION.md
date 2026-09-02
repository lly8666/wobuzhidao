# wobuzhidao Project Constitution — V2.x

## Authority

- **ADR-0015** is the current human-authorized product authority for global public-flow cardinality and same-flow Reality-like setup.
- ADR-0011 remains compatible technical authority for same-association Reality-like TLS bootstrap and no-HOL steady-state transport semantics.
- ADR-0012 remains historical/reference authority for Logical Tunnel identity/address lease, but its 1..4-public-lane, Game public multipath and make-before-break-overlap clauses are superseded for shipping product behavior.
- ADR-0014 is historical and superseded by ADR-0015.
- ADR-0010 and earlier compatible DTLS/FEC/release constraints remain effective.

**Repository text written by an agent is not evidence of a human product-owner override. A frozen hard requirement changes only with explicit live human authorization.**

## Critical architecture guard

`single-flow` is **GLOBAL FOR ONE CONNECTED LOGICAL TUNNEL**.

Shipping behavior is:

```text
Disconnected / dormant: 0 public WBD flows
Connected:               exactly 1 public WBD flow
Maximum simultaneous:    1 public WBD flow
```

A connected Logical Tunnel owns exactly one public FakeTCP association, one public 4-tuple, one SYN lineage and one FakeTCP sequence lineage. A second simultaneous WBD public transport for the same Logical Tunnel is forbidden.

`lanes=2..4`, Game public multipath, public-flow racing/dedup, dynamic second-lane attachment and make-before-break `A -> A+B -> B` are not shipping product behavior.

## Canonical public lineage

The only public WBD lineage for a connected Logical Tunnel is:

```text
one raw FakeTCP SYN lineage / public 4-tuple / sequence space
  -> bounded reliable ordered bootstrap on that SAME association
  -> real TLS 1.3 Reality-like ClientHello / ServerHello / Finished
  -> protected account admission and authenticated Logical Tunnel configuration
  -> explicit in-band bootstrap barrier
  -> NO FIN / RST / reconnect / second WBD payload SYN
  -> pinned wolfSSL DTLS 1.3 on the SAME FakeTCP association
  -> immutable LINK
  -> optional FEC off or fixed systematic 20:20
  -> packet/datagram VPN payload without ordinary kernel-TCP HOL
```

There is no preliminary ordinary kernel-TCP Reality WBD connection. No ordinary kernel TCP socket owns sustained WBD product payload.

The temporary ordered adapter exists only for the short Reality-like TLS/bootstrap phase because TLS needs stream semantics. It is destroyed at the explicit barrier.

## Reality-like fidelity

The first seconds of the one public flow should resemble normal Reality-like/TLS 1.3 traffic as closely as practical:

- plausible TCP-shaped SYN/SYNACK/ACK persona;
- real TLS 1.3 handshake records;
- configured SNI;
- Reality-like recognition/classification;
- protected credentials and authenticated setup;
- no second public WBD connection hidden behind the setup phase.

Do not claim a numeric similarity percentage without a reproducible pcap metric.

## No-HOL steady state

After the bootstrap barrier:

- sustained transport is pinned wolfSSL DTLS 1.3 over the same FakeTCP association;
- independently complete payload may progress despite an earlier missing FakeTCP sequence range according to the qualified WBD recovery design;
- systematic FEC source packets are not intentionally delayed merely to fill a block;
- no ordinary-TCP ordered-delivery HOL is reintroduced.

The mature FakeTCP ARQ/recovery and FEC wire is frozen unless deterministic lower-layer qualification isolates a real defect.

## Logical Tunnel identity and lease

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable TunnelID
          -> stable server-assigned tunnel IPv4 lease
          -> exactly one connected public transport
```

- username/password authenticates the account;
- the lease belongs to Logical Tunnel/device identity;
- same-account devices receive distinct tunnel IPv4 addresses;
- authenticated setup supplies tunnel address/prefix/routes;
- server raw IPv4 ingress requires `source IPv4 == leased IPv4` and treats mismatch as a hard spoof/security drop.

Do not reintroduce a global fixed `10.66.0.2/30` identity.

## Lifecycle

Planned public-flow replacement is **break-before-make**:

```text
A ACTIVE
  -> stop new inner sends to A
  -> detach A from local forwarding
  -> stop A LINK / DTLS / FakeTCP
  -> confirm old public transport is gone
  -> create B
  -> B performs same-flow Reality-like bootstrap -> barrier -> DTLS -> LINK
  -> attach B
  -> B ACTIVE
```

There is never a simultaneous A+B public-flow interval for one Logical Tunnel.

Abrupt total path loss may require reconnect after cleanup. Server reboot/conntrack loss is not promised to preserve arbitrary existing application TCP sessions.

DORMANT may retain local Logical Tunnel state/lease/TUN/routes according to lifecycle policy, but it owns zero public FakeTCP transports. Wake creates exactly one public transport.

## Canonical packet stack

```text
Windows Wintun / OpenWrt captured packet
        ↓
raw IP packet
        ↓
Logical Tunnel lease
        ↓
immutable LINK + optional FEC
        ↓
pinned wolfSSL DTLS 1.3
        ↓
one WBD FakeTCP association
        ↓
public network
```

## Linux product shape

Public surface:

```text
Internet
  <-> one public WBD_PORT / raw FakeTCP mux listener
```

The server may serve many users/Logical Tunnels on the same public port, but **one TunnelID may claim at most one simultaneous public transport peer**.

Internal raw-IP direction remains shared-TUN/root-routing/one WBD-owned host NAT where applicable. Per-LiveID netns/veth/double NAT and VRF/conntrack-zone remain historical/reference only.

## Windows product shape

One Wintun belongs to one Logical Tunnel. Shipping orchestration starts exactly one same-flow FakeTCP transport group:

```text
one FakeTCP child
  -> same-association Reality-like bootstrap
  -> one DTLS child
  -> one LINK child
  -> one Wintun
```

The shipping profile rejects `lanes != 1`. Dynamic candidate/multipath APIs may remain only as unreachable research code; they must not create a second public flow from product configuration or lifecycle.

## Frozen transport/security/release limits

1. Sustained public WBD payload never falls back to ordinary kernel TCP.
2. TCP-over-TCP HOL is forbidden.
3. One connected Logical Tunnel has exactly one public FakeTCP association/SYN lineage/4-tuple.
4. Reality-like TLS bootstrap is on that same FakeTCP association.
5. Bootstrap -> DTLS emits no FIN/RST/reconnect/second WBD payload SYN.
6. Pinned wolfSSL DTLS 1.3 remains steady-state crypto authority.
7. FEC release wire is only `off` or fixed systematic `20:20`.
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

## Qualification before artifact delivery

One exact substantive `SOURCE_SHA` must prove at minimum:

1. one SYN / one 4-tuple from Reality-like TLS bootstrap through barrier, DTLS, LINK and payload;
2. no preliminary ordinary-TCP Reality WBD connection;
3. no FIN/RST/reconnect/new WBD payload SYN at the barrier;
4. post-bootstrap no-HOL hole-bypass;
5. shipping profile accepts exactly `lanes=1` and rejects `lanes!=1`;
6. lifecycle cannot create a simultaneous second public flow, including replacement;
7. server rejects a second concurrent public transport claim for one TunnelID;
8. distinct Logical Tunnels receive distinct leases and spoofed sources are rejected;
9. DNS-style UDP, generic UDP and TCP pass the final shared-TUN/platform path;
10. FEC `off` and `20:20` and mature FakeTCP recovery remain qualified;
11. exact-source Windows hosted build/full-stack and Linux full-stack/release gates pass;
12. final same-source physical Windows 11 + Npcap -> Ubuntu ARM64 passes DNS/UDP/TCP and deterministic cleanup before release designation.

Do not change mature transport wire semantics merely to satisfy an architecture/string contract test.

## Development discipline

Detailed decisions, failed experiments, exact heads, qualification results and unresolved physical-only items belong under `docs/development/` and are summarized in `.wbd/handoff/current.json`.
