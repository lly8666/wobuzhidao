# ADR-0013: Global single-public-flow release freeze

Status: **ACCEPTED / SUPERSEDES ADR-0012 MULTIPATH AND MAKE-BEFORE-BREAK CLAUSES** (2026-08-31)

## Context

The single-flow Reality-like transport work proved the important property WBD originally wanted: one WBD-owned raw TCP-shaped association can own the public 4-tuple from the first SYN, carry a bounded reliable Reality-like TLS 1.3 bootstrap, cross an explicit barrier, then carry DTLS 1.3 + LINK/FEC/raw-IP payload without falling back to an ordinary kernel-TCP payload stream.

ADR-0012 later generalized the Logical Tunnel above this transport and proposed 1..4 replaceable Transport Lanes, Game Lane racing, and make-before-break replacement. That proposal preserved no-HOL *inside each lane*, but it weakened a newer and now explicit product requirement: the entire connected WBD VPN must expose only one public TCP-shaped WBD flow at any instant.

The product requirement takes precedence over the older multipath proposal.

## Decision

### 1. One public transport is a global product invariant

For one connected Logical Tunnel:

```text
public WBD transport count = 1
```

While disconnected or transport-dormant, the count may be zero.

The release product must never intentionally run two simultaneously usable public WBD transports for one Logical Tunnel. This includes candidate lanes, race lanes, migration lanes and age-rotation overlap.

`internal/logicaltunnel.MaxProductPublicTransportLanes` is fixed at `1`. It is a release invariant, not a user or tuning option.

### 2. The one flow owns the entire public lineage

The one public transport owns:

```text
raw FakeTCP SYN / SYNACK / ACK
  -> bounded reliable bootstrap stream on the same sequence space
  -> Reality-like real TLS 1.3 setup/admission on that stream
  -> explicit bootstrap barrier; no FIN and no new SYN
  -> DTLS 1.3 on the same FakeTCP association
  -> LINK
  -> lane-local FEC (release: off or fixed 20:20)
  -> raw-IP payload
```

There is no preliminary ordinary kernel-TCP Reality connection and no second public FakeTCP association after bootstrap.

The temporary ordered/reliable adapter exists only for setup bytes. It is destroyed at the bootstrap barrier. Sustained payload remains datagram/packet oriented and must continue to pass the no-HOL qualification gate.

### 3. Logical Tunnel identity and address lease remain

The useful parts of ADR-0012 remain accepted:

- stable Logical Tunnel identity above disposable transport state;
- per-installation server-assigned IPv4 lease;
- same-account devices receive distinct leases;
- server anti-spoofing validates raw-IP source against the lease;
- shared Linux TUN + one WBD-owned host NAT is the selected raw-IP server direction;
- tunnel identity/address may survive a transport reconnect.

These properties do not require multipath.

### 4. Multipath/Game Lane is not a release product path

The existing `internal/gamelane` / race research may remain in the repository as historical or experimental evidence. It must not be wired into current Windows/Linux product configuration in a way that opens more than one public WBD transport for a Logical Tunnel.

Current release UI/configuration must not expose a desired lane count greater than one.

### 5. Replacement is break-before-make

Normal replacement sequence is:

```text
old transport ACTIVE
  -> stop new payload admission to old transport
  -> bounded flush/close where the path still exists
  -> old public transport retired locally
  -> create one new FakeTCP association
  -> same-flow Reality-like bootstrap
  -> DTLS/LINK ready
  -> resume payload
```

There is no candidate overlap and no `A+B` race interval.

This has a plain tradeoff: without a second simultaneous public connection, planned rotation/network migration cannot be zero-gap make-before-break. A short packet pause is acceptable; the implementation may use a small bounded local wake/reconnect buffer above the transport, but it must not create a second public flow to hide the pause.

### 6. Abrupt path failure and stale server records

A disappeared client path may leave a server-side peer/session record until timeout. That stale record is not considered a second *usable public transport* if the client can no longer send on the old path.

A newly authenticated replacement may supersede stale bookkeeping for the same Logical Tunnel, but the release client still initiates only one public association. The server must reject two concurrently usable LINK transports for the same TunnelID as defense in depth.

### 7. Reality-like fidelity does not justify kernel-TCP HOL

Reality-like setup should match normal TLS/TCP behavior as closely as technically practical during the first seconds, including TCP persona, TLS 1.3 ClientHello/persona, timing and ordinary TLS record behavior where qualified.

However, fidelity must not be achieved by handing sustained WBD payload to an ordinary kernel TCP byte stream. Any proposal that reintroduces TCP-over-TCP HOL, a second public connection, or hidden ordered delivery after the bootstrap barrier requires a new ADR and explicit product decision.

Do not claim a numeric browser/REALITY similarity percentage unless a reproducible capture analyzer defines and measures it. Release claims must be evidence-based.

## Superseded ADR-0012 clauses

ADR-0013 supersedes these ADR-0012 product decisions:

- `zero or more replaceable Transport Lanes` as a connected release topology;
- 2..4 Game/weak-network race lanes;
- general multipath promotion of Game Lane semantics;
- make-before-break transport replacement;
- temporary old+candidate overlap;
- one-lane-at-a-time rotation of a multi-lane active set;
- any statement that the single-flow invariant applies only per lane rather than to the whole connected product session.

ADR-0012 sections about Logical Tunnel identity, leases, anti-spoofing, shared TUN/one host NAT, payload-idle policy and separation of logical identity from disposable transport state remain valid where they do not require multiple simultaneous public transports.

## Release qualification

Before Windows/Linux artifacts are handed to a physical tester, exact-head automation must prove at least:

1. product startup creates one public FakeTCP association and no ordinary kernel-TCP Reality bootstrap connection;
2. one SYN lineage carries Reality-like TLS bootstrap and the later DTLS/LINK/payload phase;
3. bootstrap barrier occurs without FIN/new SYN;
4. post-bootstrap no-HOL test still passes;
5. Windows Npcap ingress ignores unrelated ARP/IPv6/UDP/TCP traffic without poisoning FakeTCP state;
6. LINK server rejects a second concurrently usable transport claim for the same TunnelID;
7. Logical Tunnel lease remains stable across a break-before-make reconnect;
8. raw-IP shared-TUN downstream/upstream paths and anti-spoofing pass;
9. Windows build/admin-smoke/capability tests pass;
10. Linux server release/firewall/full-stack tests pass;
11. same-flow startup stress and TCP/TLS persona capture gates pass;
12. exact-head source SHA evidence matches both Windows and Linux artifacts.

Physical Windows 11 + Npcap + real NIC/NAT/ISP remains the final hardware/environment qualification after these automated gates, not a substitute for them.
