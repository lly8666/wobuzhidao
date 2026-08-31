# ADR-0012: Stable logical tunnel and server-assigned address lease

Status: **PARTIALLY SUPERSEDED BY ADR-0013** (original decision 2026-08-30; release-policy correction 2026-08-31)

## Why this ADR changed

The original ADR-0012 combined two separate ideas:

1. a stable Logical Tunnel identity/address lease above disposable transport state;
2. 1..4 simultaneous Transport Lanes with Game Lane racing and make-before-break replacement.

The first idea remains useful and accepted. The second conflicts with the newer hard product rule that one connected WBD VPN exposes exactly one public TCP-shaped WBD flow at any instant.

**ADR-0013 is authoritative for transport count and replacement.** Multipath, race lanes and make-before-break are no longer current release architecture.

## Retained decisions

### 1. Logical Tunnel identity is separate from transport identity

The retained model is:

```text
Account
  -> Device / Installation
      -> Logical Tunnel
          -> stable server-assigned tunnel address lease
          -> zero public transports while disconnected/dormant
          -> exactly one public transport while connected
```

The transport owns disposable state such as the FakeTCP 4-tuple/sequence space, DTLS state, LINK LiveID and lane-local FEC state. Those disposable values do not own the tunnel address lease.

### 2. Server assigns a unique tunnel address lease

The server allocates each active Logical Tunnel/device a unique IPv4 tunnel address from a configurable private pool. Same-account devices receive distinct addresses.

The client must not use one globally hard-coded tunnel address as device identity. Authenticated tunnel configuration supplies address/prefix/routes.

Server ingress enforces:

```text
raw IPv4 source == leased tunnel IPv4
```

Future IPv6 applies the same principle to an assigned address.

### 3. Shared Linux TUN + one host NAT remains the selected raw-IP direction

The retained server data-plane direction is:

```text
Internet
  <-> one WBD-owned host NAT/SNAT
  <-> Linux root routing
  <-> one shared WBD TUN
  <-> Logical Tunnel manager
  <-> the tunnel's one active WBD public transport
```

The per-session Linux netns/veth/double-NAT prototype remains historical/correctness evidence, not the selected final product architecture. The rejected VRF/conntrack-zone prototype remains rejected.

### 4. Payload-idle policy may keep the logical tunnel while closing transport

A Logical Tunnel may remain user-visible while its public transport is absent because of an idle policy. In that state public transport count is zero. A new payload can wake the tunnel by establishing one new public transport.

PING/PONG/control activity does not redefine real payload activity.

### 5. Logical identity may survive transport reconnect

A public transport is disposable. When it fails or is intentionally rotated, the Logical Tunnel and leased address may remain stable.

Replacement now follows ADR-0013 break-before-make semantics. There is no old+candidate overlap in the release product.

### 6. Server restart is not transparent application-flow mobility

A server restart can destroy host conntrack/NAT state. WBD does not promise transparent preservation of arbitrary application TCP flows across server restart. Logical identity/lease continuity is a separate concept from impossible kernel-state preservation.

## Superseded original decisions

The following original ADR-0012 decisions are **not current product architecture**:

- a connected tunnel owning `zero or more replaceable Transport Lanes`;
- 2..4 Game/weak-network race lanes;
- promotion of Game Lane first-arrival/dedup as the general release transport layer;
- make-before-break replacement;
- temporary candidate + old-lane overlap;
- multi-lane staggered rotation;
- wording that the single-flow invariant applies only per lane and not to the whole connected VPN.

The corresponding `internal/gamelane` / `internal/gamecontrol` code may remain as experimental/history code, but release product configuration and lifecycle must not use it to create another public WBD flow.

## Preserved hard release constraints

- The one public flow is WBD-owned raw TCP-shaped FakeTCP from the first SYN.
- The Reality-like TLS 1.3 bootstrap runs over a bounded reliable stream inside that same FakeTCP sequence space.
- Bootstrap ends at an explicit barrier without FIN/new SYN.
- Sustained payload is pinned wolfSSL DTLS 1.3 -> LINK/FEC -> raw-IP and must retain no-HOL behavior.
- Ordinary kernel TCP never owns sustained WBD payload.
- FEC release wire remains `off` or fixed systematic `20:20` unless a later ADR changes it.
- `legacy` FakeTCP recovery remains the release default unless separately requalified.
- Windows capture remains raw L3 through Wintun/TUN; Linux product uses one public WBD port.
- WBD-owned firewall/route/DNS/IPv6 cleanup remains scoped and reversible.
- Credentials, passwords, tickets and other secrets must not be logged.

## Qualification carried forward

Qualification still needs to prove:

1. distinct installations receive distinct leases;
2. same inner tuples from distinct tunnels remain isolated;
3. raw-IP UDP/TCP/DNS paths pass through shared TUN + one host NAT;
4. source spoofing is rejected;
5. lease reuse/cleanup is deterministic;
6. one-flow Reality-like bootstrap -> DTLS -> LINK -> raw-IP works end to end;
7. post-bootstrap no-HOL qualification remains green;
8. Windows and Linux exact-head build/full-stack gates pass before physical artifact delivery.

For transport-count, replacement and overlap rules, see **ADR-0013**.
