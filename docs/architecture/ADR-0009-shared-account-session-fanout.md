# ADR-0009: Shared-account transport-session fan-out

Status: **ACCEPTED / AMENDED BY ADR-0012** (original 2026-08-26; identity-scope amendment 2026-08-30)

> ADR-0012 preserves ticket/LiveID fan-out as transport plumbing but changes what `LiveID` means in the product model. `LiveID` identifies one transport lane/session epoch; it is **not** the long-lived Logical Tunnel identity and does not own the tunnel IP lease.

## Context

WBD is a personal weak-network VPN. The server intentionally uses one configured username/password pair that may be reused by several devices at the same time. Authentication must stay cheap and simple; username is an account label, not a sustained data-plane routing key.

The Reality-like bootstrap authenticates one TLS-encrypted username/password request and issues an independent random one-time credential/ticket per successful login. Several such credentials must be able to become simultaneous transport sessions without collapsing state by username.

Current transport implementations were originally single-association test programs. The FakeTCP server selected one raw peer, and the wolfSSL DTLS server selected one UDP peer and one `WOLFSSL*`. Therefore account-level concurrent authentication alone is not sufficient for a real one-to-many server.

ADR-0012 later adds a layer above these transport sessions: a Logical Tunnel/device with a stable TunnelID, server-assigned address lease and one or more replaceable transport lanes.

## Decision 1 — username is account identity; LiveID is transport-lane identity

A successful bootstrap issues a distinct random one-time ticket/credential. At DTLS/LINK bind the credential is atomically claimed and returns its account label.

Transport-lane live identity is:

```text
(account metadata, LiveID/ticket, learned DTLS plaintext peer)
```

Only `LiveID` is the hot identity key for that transport session/lane. The account string is used for accounting/caps/observability. The learned peer is a fast routing index. Sustained packets never perform a username/password lookup.

The same account may own many `LiveID` values simultaneously.

**ADR-0012 amendment:** a Logical Tunnel is a separate object above LiveID. Multiple successive or concurrent LiveIDs may attach to the same Logical Tunnel during game/race mode, reconnect or make-before-break replacement. The tunnel address lease must not be keyed by LiveID.

## Decision 2 — one-time ticket consumption is atomic

Product bind uses `ConsumeTicketForAccount` or its successor with equivalent one-shot semantics.

The ticket file is atomically renamed to a unique claim name before it is read and validated. The rename is the serialization point, so concurrent processes/goroutines cannot both consume the same ticket. A claimed invalid or expired ticket is not returned to circulation.

Any future tunnel-resume credential is a separate secret and must preserve explicit replay/ownership semantics; it must not weaken one-time lane-admission credentials by accident.

## Decision 3 — live transport demux is peer/LiveID based; tunnel routing is above it

The existing transport/session layer separates admission from data activation. Each LiveID owns independent transport state, including its LINK/FEC state for that lane epoch. A second activation with changed lane parameters is rejected; parameter changes require a fresh lane/association.

Peer collisions are rejected instead of overwriting another transport session. Removing one LiveID removes only that lane/session and its peer index.

ADR-0012 adds a higher-level tunnel manager:

```text
LiveID/lane -> authenticated Logical Tunnel attach
Logical Tunnel -> leased IP + race SessionID/PacketID + active lane set
```

Do not make raw-IP return routing depend on username or on one permanent LiveID. Downlink raw-IP routing is by the Logical Tunnel's leased address and then its active lane/race set.

## Decision 4 — one public FakeTCP listener fans out independent raw associations

The server exposes one normal public WBD FakeTCP port while supporting several simultaneous associations. The fan-out key is:

```text
(client IPv4, client TCP-shaped source port,
 server IPv4, public TCP-shaped destination port)
```

Each raw association owns independent handshake state, sequence spaces, SACK/RTO state and first-arrival receiver state. Account/tunnel identity is deliberately absent at the raw tuple layer because authenticated identity is not known until bootstrap/bind.

`internal/faketcp.ServerAssociationTable` and `ServerAssociation` remain reusable mux infrastructure. A new SYN allocates one independent WBD transport association.

ADR-0012 explicitly allows several such associations to belong to one Logical Tunnel when game/race mode or controlled migration requires it.

## Decision 5 — one DTLS worker per raw association remains acceptable

The pinned wolfSSL shim is one-association-per-process: one UDP peer, one `WOLFSSL*`, one relay loop. For the intended small personal device count, that simple model remains acceptable unless measurement proves otherwise.

The public FakeTCP mux may allocate one loopback UDP transport and one DTLS worker per raw association. Workers feed the shared LINK/tunnel layer where the authenticated lane is attached to a Logical Tunnel.

To avoid loopback port races, the DTLS shim can inherit an already-bound UDP fd through `WBD_DTLS_TRANSPORT_FD`. This implementation detail does not define Logical Tunnel identity.

## Decision 6 — loaded recovery policy favors new inner packets

The 100 Mbit/s loaded A/B evidence rejected SACK/RACK as the unconditional product default because the remaining delivery/goodput gain was small while queueing latency increased.

Therefore:

- product FakeTCP default remains `legacy` shadow recovery;
- `sack-rack` remains experimental;
- future advanced repair must be explicitly lower priority/bandwidth-budgeted before replacing the latency-first default.

## Platform consequence

This fan-out work is transport/server plumbing. It does not change final client capture targets:

- OpenWrt release path: TPROXY + policy routing + mandatory underlay escape;
- Windows release path: TUN/Wintun-class L3 + mandatory underlay escape.

ADR-0012 changes the Windows raw-IP server topology to unique Logical Tunnel leases + shared TUN + one host NAT and promotes later Game Lane race semantics as the multipath/replacement foundation.

## Superseded interpretation

Do not read this ADR as saying:

- `LiveID == Logical Tunnel`;
- `LiveID` permanently owns a tunnel IP;
- one whole user VPN session has exactly one LiveID/association until Disconnect;
- later Game Lane multipath is forbidden.

Those interpretations are superseded by ADR-0012. The atomic ticket, per-association isolation and concurrent shared-account fan-out decisions remain valid.
