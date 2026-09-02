# 2026-08-31 — Logical Tunnel shared-TUN gateway migration

## Why this work exists

The single-public-flow Reality-like architecture itself is now substantially qualified. On feature head `05bc2c714500deadc4a10435cd6dd68f4da34168`, `single-flow-link-fullstack` succeeded through the intended chain:

`one raw FakeTCP SYN lineage -> Reality-like TLS bootstrap on that association -> DTLS 1.3 -> LINK -> client TUN -> raw-IP forwarding/echo`.

The remaining deterministic multi-client failure was not in FakeTCP recovery, Reality-like bootstrap, DTLS or LINK admission. It was in the Linux prototype raw-IP Internet gateway.

This matters because ADR-0012 explicitly supersedes the per-session netns/veth/double-NAT gateway prototype. The production target is one shared root-namespace TUN, one host NAT and userspace demultiplexing by the Logical Tunnel leased IPv4 address.

## Evidence from `single-flow-two-client` run 33347043906

Artifact: `single-flow-two-client-off` (Actions artifact id `9742337757`; downloaded evidence ZIP SHA-256 `517221501e5008cbc41aa39fb0a9abcb7863b1e6f9eae9621917d56841717fb4`).

Both same-account clients successfully completed:

- independent single-flow FakeTCP/Reality-like bootstrap;
- distinct one-time ticket issuance;
- distinct `tunnel_id` values;
- distinct leased IPv4 `/32` addresses;
- DTLS 1.3 readiness;
- LINK readiness;
- client TUN readiness.

Client A then completed UDP and TCP Internet probes. Client B timed out on its first UDP return.

Gateway evidence separated the fault from transport admission:

- A gateway session: `rawip_rx=8 rawip_tx=8 tun_tx=8 tun_rx=8`.
- B gateway session: `rawip_rx=1 tun_tx=1 tun_rx=0 rawip_tx=0`.

Therefore B's first raw IPv4 datagram passed the LINK lease/source check, reached the backend, and was written into B's per-session TUN. The failure occurred after that write, in the second per-session netns/veth/NAT return path.

`WBD_LINK_RAW_IP_SPOOF_DROP` also appeared in evidence, but it is not the primary cause of the B timeout because B's actual probe datagram reached the gateway and incremented its `rawip_rx/tun_tx` counters.

## Architectural decision

Do **not** repair or extend the netns prototype.

Per ADR-0012, final Linux forwarding must have:

- one shared root-namespace TUN for the whole lease pool;
- one host NAT/conntrack domain;
- no per-session network namespace;
- no per-session veth pair;
- no second NAT layer;
- upstream validation that a peer may emit only the source IPv4 it leased;
- downstream userspace demultiplexing by IPv4 destination lease;
- independent Logical Tunnel metadata so two clients may use identical inner UDP/TCP tuples without collision.

The mature FakeTCP/TCP-like recovery core, DTLS, FEC and LINK framing are intentionally unchanged by this migration.

## Implementation started on 2026-08-31

### `cmd/wbd-ip-gateway-shared`

A new Linux-only gateway command was added instead of rewriting the legacy gateway in place. The legacy command remains available as historical/regression evidence while the shared backend is qualified.

The shared gateway owns:

- one `tunnel.OpenTUN()` instance (default `wbdg0`);
- one route for the Logical Tunnel lease prefix (default `10.66.0.0/16`) to that TUN;
- registries `peer -> session`, `leased IPv4 -> session`, and `tunnel_id -> session`;
- a single TUN read loop.

Registration is driven by existing `rawipbackend.TunnelMeta`; no new wire metadata was introduced.

Upstream path:

1. LINK mux sends `TunnelMeta` to the backend peer.
2. shared gateway registers a unique `tunnel_id` and unique leased `/32`.
3. raw-IP packet is accepted only if IPv4 source exactly equals that session's lease.
4. packet is written to the single shared TUN.
5. kernel forwarding and the host NAT send it to the Internet.

Downstream path:

1. return packet is routed by the host to the shared TUN because its destination is in the lease pool;
2. userspace reads the raw IPv4 packet;
3. IPv4 destination selects exactly one live leased session;
4. the packet is M6A/raw-IP framed and returned only to that backend peer.

A duplicate live lease or duplicate live `tunnel_id` is rejected. Idle session cleanup removes only mappings; it does not destroy the shared TUN or NAT.

A `!linux` stub was added so Windows repository builds can enumerate the command package without inheriting Linux functionality.

### `scripts/linux_shared_tun_firewall.sh`

A separate WBD-owned firewall helper was added rather than overloading the old netns helper.

It owns only:

- one FORWARD rule from shared TUN / lease pool;
- one established/related return FORWARD rule to shared TUN / lease pool;
- one host `POSTROUTING MASQUERADE` for the lease pool outside the TUN;
- the saved/restored `net.ipv4.ip_forward` state;
- nftables equivalents with WBD-owned comments/table.

It has no session-add/session-del operation because NAT and conntrack are deliberately shared at host scope.

### Unit contracts

`cmd/wbd-ip-gateway-shared/main_linux_test.go` covers:

- two leases map to two different backend peers;
- duplicate lease rejection;
- duplicate `tunnel_id` rejection;
- out-of-pool lease rejection;
- strict IPv4 source/destination extraction;
- dropping one session does not damage the other lease mapping.

## New qualification workflow

`.github/workflows/shared-tun-two-client.yml` was added as an independent qualification gate rather than silently rewriting the older netns test.

For FEC `off` and fixed `20:20`, it constructs:

- two client network namespaces;
- one raw single-flow FakeTCP/Reality-like server association per client;
- pinned wolfSSL DTLS 1.3;
- LINK mux with Logical Tunnel metadata;
- the one shared root TUN gateway;
- one host NAT;
- an Internet namespace hosting two UDP echo ports and one TCP echo port.

Required outcomes for each matrix point:

- two distinct tunnel IDs;
- two distinct leased `/32` addresses;
- exactly one shared server TUN;
- zero `wbdgXX` per-session namespaces;
- both clients pass two UDP echoes;
- both clients pass TCP echo while deliberately binding the same inner TCP source port `40000`;
- both LINK backend sessions reach `backend=rawip`;
- shared host NAT rule exists.

Initial run created from commit `a8bd25d0f5b0004f4d4538a6b06b6152ca3e6367`: Actions run `33350165499`. At creation time both matrix jobs were queued. This document must be updated with the first deterministic outcome before the shared backend is promoted into the Linux product manager.

## Promotion gates before product default changes

Do not switch the Linux manager/release to shared gateway merely because the new unit tests compile.

Required sequence:

1. `shared-tun-two-client` green for `off` and `20:20`.
2. Re-run the existing single-flow LINK fullstack and no-HOL gates to prove no transport regression.
3. Add shared-TUN firewall owned-state/cleanup qualification for both iptables and nft where supported.
4. Switch the Linux manager from the superseded netns gateway to shared gateway.
5. Re-run Linux server release/shared-port/manager lifecycle tests.
6. Run Windows build/portable and the available Windows-hosted Logical Tunnel/TUN integration gates against the same source head.
7. Do not provide a user release package until the requested Windows + Linux chain is green; hosted Windows validation must not be misrepresented as a physical Npcap/NIC qualification.

## Frozen components during this migration

Unless a new deterministic test proves otherwise, do not modify:

- FakeTCP sender/receiver recovery and first-arrival semantics;
- legacy shadow recovery default;
- DTLS 1.3 wire behavior;
- FEC release modes (`off`, fixed `20:20`);
- LINK/FEC immutable per-lane parameters;
- the one-SYN, one-4-tuple Reality-like bootstrap/data-plane transition required by ADR-0011.
