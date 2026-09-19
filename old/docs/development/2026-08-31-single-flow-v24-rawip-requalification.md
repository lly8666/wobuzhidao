# 2026-08-31 V2.4 single-flow Logical Tunnel raw-IP requalification

## Scope and frozen invariants

This log records the qualification/debugging sequence for PR #9 on `feat/single-flow-reality-faketcp` after the single-public-flow architecture pivot.

Frozen requirements for this work:

- One public TCP-shaped flow from SYN onward. Reality-like TLS 1.3 setup/admission runs over the bounded reliable bootstrap phase of that same FakeTCP association.
- No FIN, RST, second SYN, or second public 4-tuple between setup and sustained data.
- Sustained payload remains FakeTCP-owned datagram semantics -> pinned wolfSSL DTLS 1.3 -> LINK -> optional fixed 20:20 FEC. It must not fall back to ordinary kernel TCP HOL behavior.
- The mature FakeTCP sender/receiver/recovery/FEC core is not changed by this raw-IP fix.
- V2.4 identity model is Account -> Installation -> Logical Tunnel -> replaceable transport lanes. Raw-IP backend isolation is keyed by Logical Tunnel, not account text or disposable lane identity.
- No deliverable package is qualified until the exact same source HEAD passes Windows-native/build gates and Linux/raw-netns full-stack gates.

## Live refresh baseline

Feature branch baseline before this fix series:

- branch: `feat/single-flow-reality-faketcp`
- PR: #9, `[V2.4] Per-lane single-flow transport + logical tunnel multipath pivot`
- baseline HEAD: `38781d719f32c7d79dbfe2a8f56ae3fccb23fde0`
- canonical handoff branch: `dev/wbd-raw-fec-v2`
- canonical continuity sequence at refresh: 70

Exact-baseline failures included:

- `ci` run `33340963114`: Go tests passed; handoff tests failed because canonical recovery metadata lagged the feature branch.
- `single-flow-two-client` run `33340963089`: both `off` and `20:20` jobs failed.
- `single-flow-rawip-e2e` run `33340963180`: both `off` and `20:20` jobs failed.
- `single-flow-link-fullstack` run `33340963360`: failed because it reuses the raw-IP workflow.
- `linux-server-firewall` run `33340962339`: zero jobs; treated as workflow scheduling/parse evidence, not an executed product failure.

## Evidence: two-client qualification stopped before TUN

Downloaded artifact from run `33340963089` showed that both clients successfully completed:

1. same-flow Reality-like bootstrap (`same_flow=1 logical_tunnel=1`),
2. distinct Logical Tunnel leases,
3. FakeTCP READY,
4. pinned wolfSSL DTLS 1.3,
5. LINK client READY,
6. server-side `WBD_LINK_LOGICAL_TUNNEL_BIND`, and
7. server-side `WBD_LINK_MUX_SESSION_READY ... backend=pending`.

There was no `tun-a.log` or `tun-b.log`, and the gateway only showed its READY marker. The workflow had exited before starting the TUN clients.

Deterministic qualification bug:

```sh
test "$(grep -c 'WBD_LINK_MUX_SESSION_READY account=solo' "$ROOT/link-server.log")" -eq 2
```

V2.4 intentionally removed account text from this marker. The current product marker is:

```text
WBD_LINK_MUX_SESSION_READY tunnel_id_prefix=<8hex> ... backend=pending
```

Therefore the old grep always returned zero even when two sessions were ready. A later backend assertion also still expected the retired `sid=` form.

Fix: update the workflow to wait for/count the current `tunnel_id_prefix` markers and make evidence permissions robust.

## Evidence: single-client raw-IP probe was a real data-plane failure

Run `33340963180` reached TUN setup and then timed out on the first UDP echo to `203.0.113.2:53`. This was not explained by the stale marker assertion because the probe failed first.

The failure artifact was initially unavailable because `client.ticket` was root-owned and `upload-artifact` received `EACCES`. The workflow is now required to chmod evidence in an `if: always()` step before upload.

## Product mismatch #1: LINK emits TunnelMeta, gateway only consumed SessionMeta

V2.4 LINK raw-IP backend setup calls:

```go
rawipbackend.MarshalTunnelMeta(binding.Config.TunnelID, lease)
```

before forwarding the first application IP datagram.

`internal/rawipbackend/meta.go` defines:

- Version 1 `SessionMeta{SID}` for legacy qualification paths.
- Version 2 `TunnelMeta{TunnelID, Address4}` for V2.4 product traffic.

At baseline, `cmd/wbd-ip-gateway-server/main_linux.go` only called `UnmarshalSessionMeta`. A V2 TunnelMeta was therefore passed to `dataplane.UnmarshalIP`, rejected as non-IP, and discarded. The first real IP packet then created a legacy-style backend session with no Logical Tunnel lease awareness.

## Product mismatch #2: fixed server /30 conflicts with /32 Logical Tunnel leases

Baseline raw-IP gateway configured every per-session netns TUN with:

```text
10.66.0.1/30
```

while the V2.4 Logical Tunnel manager issues client leases from the authenticated tunnel pool as `/32` addresses. The observed single-client qualification lease was `10.66.0.1/32`.

Consequences:

- For `.1`, the client source address was identical to the server netns TUN local address. Return traffic resolved locally instead of being emitted to the TUN fd.
- For other leases outside the fixed `/30`, there was no lease-specific return route to the TUN.
- Per-session SNAT matched the fixed `/30` instead of the authenticated Logical Tunnel lease.

This is an interface mismatch between V2.4 Logical Tunnel control metadata and the older raw-IP gateway, not a FakeTCP/recovery/FEC problem.

## Implemented product correction

Commit `0bc2bf82fa7a11e46d378f2643bd155b2fe1e343` (`rawip: bind gateway sessions to logical tunnel leases`) changes only the raw-IP backend side:

- consume Version 2 TunnelMeta before the first IP datagram;
- retain `{TunnelID, Address4}` per backend UDP peer;
- reject metadata changes on an active peer;
- validate the IPv4 source against the authenticated lease before/after session creation;
- use the tunnel ID prefix for V2 diagnostics;
- make V2 TUN interfaces unnumbered instead of assigning a client-pool address;
- install `route <lease>/32 dev <tun>` inside the session netns;
- pass exact `<lease>/32` as the per-session firewall/SNAT source scope;
- preserve Version 1 SessionMeta and the fixed `/30` behavior only for legacy/direct qualification compatibility.

The existing firewall helper already accepts `--inner-prefix`; no netfilter grammar change is required. Product V2 sessions now pass the lease `/32` for `session-add/session-del`, while legacy/global cleanup behavior remains intact.

## Unit-test corrections

Commits:

- `62d6a58b3c5ba167f2abce9c610c12a3ea089a8c` — verify Logical Tunnel sessions use a lease `/32` scope and `tunnel_id_prefix` marker while legacy sessions keep their configured prefix.
- `36089e88b897aa902526a2029ee87e774d900379` — directly verify `gateway.handleFrame` stores V2 TunnelMeta and rejects changed tunnel metadata on an active backend peer.

These tests do not require root, TUN, or network namespaces.

## Qualification workflow corrections

Commit `4f562d50c0dbd80bb567b72f5229a1a4ae3423ad`:

- replaces retired `account=solo` session marker assertions in `single-flow-two-client`;
- waits for two current `tunnel_id_prefix=... backend=pending` server sessions;
- uses current raw-IP backend/session markers;
- recursively makes evidence readable before upload.

Commit `8df35e4b2d233d7885c6a304b6b0e924c41be01d`:

- updates `single-flow-rawip-e2e` to correlate LINK and gateway using the same 8-hex `tunnel_id_prefix`;
- adds an unconditional evidence-permission step before artifact upload;
- removes stale `sid=` assumptions from the V2.4 qualification output.

## Required requalification after this log

The feature HEAD after this documentation commit is the next exact-head qualification target. Do not deliver artifacts merely because an earlier intermediate commit is green.

Minimum required Linux/raw-netns gates:

1. Go/CI product tests.
2. `single-flow-tcp-persona` (single public flow / TLS-like prefix wire invariant).
3. `single-flow-rawip-e2e`, both `off` and `20:20`.
4. `single-flow-link-fullstack` reusable wrapper.
5. `single-flow-two-client`, both `off` and `20:20`, including same inner TCP source port 40000 with isolated tunnel leases/netns.
6. Linux server/firewall qualification and Linux server release.

Minimum required Windows gates on the same source HEAD:

1. Windows single-flow ingress/parser qualification.
2. Windows child/runtime build and portable bundle.
3. Windows TUN/route/IPv6 build or admin-smoke gates that are applicable in hosted Actions.

Physical Npcap + real NIC/NAT/ISP testing remains a final environment-specific validation, but no physical retest should be requested while any reproducible upstream/downstream CI qualification is still red.

## Current next atomic action

Freeze the post-log feature HEAD, observe exact-head Go/raw-IP/two-client results, and inspect the first deterministic failure if any. If raw-IP passes, expand to the full Windows/Linux matrix. If it still fails, use the now-guaranteed gateway/link/tun/pcap artifacts to diagnose the first missing marker rather than changing FakeTCP/TCP-like recovery logic.
