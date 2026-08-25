# ADR-0009: Shared-account session fan-out

Status: **ACCEPTED FOR V2.2 DEVELOPMENT** (2026-08-26)

## Context

WBD is a personal weak-network VPN. The server intentionally uses one configured username/password pair that may be reused by several devices at the same time. Authentication must stay cheap and simple; username is an account label, not a sustained data-plane routing key.

The existing Reality-like front already authenticates one TLS-encrypted username/password request and issues an independent random 32-byte one-time ticket per successful login. The remaining product requirement is to let several such tickets become simultaneous transport sessions without collapsing state by username.

Current transport implementations were originally single-association test programs. The FakeTCP server selected one raw peer, and the wolfSSL DTLS server selected one UDP peer and one `WOLFSSL*`. Therefore account-level concurrent authentication alone is not sufficient for a real one-to-many server.

## Decision 1 — identity is ticket/session ID, never username

A successful front login issues a distinct random one-time ticket. At DTLS bind the ticket is atomically claimed and returns its account label.

Live identity is:

```text
(account metadata, LiveID/ticket, learned DTLS plaintext peer)
```

Only `LiveID` is the session identity key. The account string is used for accounting/caps/observability. The learned peer is a fast routing index. Sustained packets never perform a username/password lookup.

The same account may own many `LiveID` values simultaneously.

## Decision 2 — one-time ticket consumption is atomic

Product bind uses `ConsumeTicketForAccount`.

The ticket file is atomically renamed to a unique claim name before it is read and validated. The rename is the serialization point, so concurrent processes/goroutines cannot both consume the same ticket. A claimed invalid or expired ticket is not returned to circulation.

`TicketAccount` remains inspection/debug support and must not be used as a product bind primitive.

## Decision 3 — live data demux is peer/LiveID based

`internal/session.DataPlane` separates admission from data activation:

```text
Reserve(account, LiveID, peer)
  -> LINK_INIT / LINK_ACCEPT
Activate(LiveID, immutable LinkConfig)
  -> Inbound(peer, wire)
  -> Outbound(LiveID, packet)
```

Each LiveID owns an independent `linkdata.Path`, including its own FEC encoder/decoder block IDs, repair timers and decoder window. A second `Activate` is rejected; link parameter changes require a fresh association.

Peer collisions are rejected instead of overwriting another session. Removing one LiveID removes only that session and its peer index.

## Decision 4 — public FakeTCP listener fans out by raw 4-tuple

The server must expose one normal public FakeTCP listener while supporting several simultaneous associations. The fan-out key is:

```text
(client IPv4, client TCP-shaped source port,
 server IPv4, public TCP-shaped destination port)
```

Each raw association owns independent handshake state, sequence spaces, SACK/RTO state and first-arrival receiver state. Account identity is deliberately absent at this layer because the ticket is not available until after DTLS.

`internal/faketcp.ServerAssociationTable` and `ServerAssociation` are the reusable mux core. The next executable step is to replace the single-peer FakeTCP server loop with this table rather than duplicate ARQ code.

## Decision 5 — one DTLS worker per raw association for V2.2

The pinned wolfSSL shim is currently one-association-per-process: one UDP peer, one `WOLFSSL*`, one relay loop. For this personal server, V2.2 keeps that simple model instead of building a complex multi-peer wolfSSL event engine.

The public FakeTCP mux allocates one loopback UDP transport per raw association and launches/owns one DTLS worker for that association. All workers send plaintext to the shared WBD link/session server, where the source UDP peer becomes the hot demux index.

To avoid loopback port races, the DTLS shim can inherit an already-bound UDP fd through `WBD_DTLS_TRANSPORT_FD`. The parent binds loopback `:0`, learns the port, passes the fd to the child and routes only that FakeTCP association to it.

This process-per-association choice is intentional for a small personal device count. If measurements later show worker-process overhead matters, wolfSSL multi-association I/O can replace it without changing account/ticket/LiveID semantics.

## Decision 6 — loaded recovery policy favors new inner packets

The 100 Mbit/s, 65 Mbps inner-offered loaded A/B rejected SACK/RACK as the product default even after the repeated-retransmit storm was fixed. The remaining delivery/goodput gain was small while p50/p95/p99 latency increased under queue pressure.

Therefore:

- product FakeTCP default is `legacy` shadow recovery;
- `sack-rack` remains an explicit experimental mode and low-load research oracle;
- loaded A/B remains a diagnostic and must produce valid results for both modes, but release CI no longer requires the experimental mode to win;
- future advanced repair must be explicitly lower priority/bandwidth-budgeted before it can replace the latency-first default.

## Platform consequence

This fan-out work is protocol/server plumbing. It does not change final client capture targets:

- OpenWrt release path: TPROXY + policy routing + mandatory underlay escape;
- Windows release path: TUN/Wintun-class L3 + mandatory underlay escape.

Real platform packet-adapter integration starts after the transport/session protocol and multi-session server are frozen and all 100 Mbit weak-network gates are green.

## Next implementation order

1. Wire `ServerAssociationTable` into a real multi-association FakeTCP raw server loop.
2. Allocate one loopback transport and inherited-fd wolfSSL DTLS worker per raw association.
3. Convert the WBD link server to accept several DTLS plaintext peers and route them through `session.DataPlane`.
4. Bind each peer by atomically consuming a ticket, then activate its immutable LinkConfig after `LINK_ACCEPT`.
5. Qualify at least two simultaneous devices using the same username/password with independent traffic in both FEC-off and fixed-20:20 modes.
6. Freeze protocol, then perform final OpenWrt TPROXY and Windows TUN one-shot VPN qualifications.
