# 2026-08-31 — Rebase OpenWrt one-shot qualification onto current single-flow V2

## Context

After recovering the live feature branch from sequence-74 conversation loss, exact-head release qualification was kicked on candidate `e2f46e6f636331df8b5d6f63f82017a5fea3774d`.

The authoritative aggregator run `33388964347` successfully verified its own branch identity and dispatched all thirteen opt-in gates. The first deterministic child failure was:

- workflow: `openwrt-fullstack-one-shot`
- run: `33389037417`
- exact head: `e2f46e6f636331df8b5d6f63f82017a5fea3774d`
- job: `99478143851`
- failing step: `Clean OpenWrt TPROXY through frozen WBD DNS UDP TCP one-shot`

## Failure evidence

Builds and prerequisite Go tests passed. Pinned wolfSSL DTLS 1.3 also built successfully.

The runtime failure was:

```text
timeout waiting for WBD_LINK_READY role=client
WBD_LINK_PROXY_FAIL WBD demo preflight binding failed: expected matching DEMO_BIND_OK, got control.Error
```

Uploaded logs showed:

- the separate `wbd-reality-front` ordinary TCP setup connection authenticated successfully;
- raw FakeTCP client reached READY;
- pinned wolfSSL DTLS client/server reached READY;
- the raw mux advertised `single_flow_bootstrap=false ... logical_tunnel=false`;
- LINK server advertised the current Logical Tunnel contract and rejected the old demo ticket path with `session state failed; reconnect required`.

Therefore the red gate is not evidence of a FakeTCP recovery, DTLS, LINK wire, FEC or OpenWrt TPROXY data-plane regression. It is a stale qualification fixture.

## Exact architectural violation in the fixture

`scripts/openwrt_fullstack_one_shot.sh` still implements the retired topology:

```text
ordinary wbd-reality-front TCP :40443 -> ticket -> close
then
separate raw FakeTCP :40000 -> DTLS -> old demo LINK bind
```

This violates current ADR-0014, which requires exactly one WBD public TCP-shaped 4-tuple/SYN/sequence lineage. Reality-like TLS admission must be the first protected phase of the same FakeTCP association, followed by the in-band barrier and DTLS/LINK/FEC without a second WBD public connection.

## Fix boundary

Only the OpenWrt qualification fixture is to change.

Do not modify the mature FakeTCP/TCP-like recovery core, DTLS wire semantics, LINK wire semantics, FEC, platformproxy or OpenWrt TPROXY behavior.

Rebase the fixture admission path to the already-qualified current V2 pattern:

1. start one `wbd-faketcp-mux` public raw listener with single-flow front certificate/key, server name, route key, account credentials, ticket directory, bootstrap timeout and fallback target;
2. require `single_flow_bootstrap=true` and `logical_tunnel=true` server readiness;
3. start one `wbd-faketcp` client on the public tuple with current Reality-like V2 flags, installation ID, ticket output and authenticated Logical Tunnel configuration output;
4. require `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1 ... logical_tunnel=1` and validate the issued 64-hex ticket plus `/32` tunnel lease/routes;
5. keep that FakeTCP process alive and start pinned wolfSSL DTLS on its existing local UDP endpoint;
6. start LINK with the issued ticket and require Logical Tunnel session readiness;
7. leave the existing OpenWrt platform proxy, TPROXY, DNS, full-cone UDP, TCP, underlay escape and cleanup assertions unchanged.

The fixture must no longer start a product-path `wbd-reality-front` ordinary public connection.

## Qualification policy after the fix

The `e2f46e6f...` release aggregate is invalid as release authority because this deterministic child failed. After the targeted OpenWrt fixture is green and any remaining planned non-transport hardening is complete, create a new exact-head qualification kick and require the whole aggregator to pass again.
