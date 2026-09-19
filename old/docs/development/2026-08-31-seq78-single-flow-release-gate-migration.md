# 2026-08-31 — Sequence 78 single-flow release-gate migration

## Scope

This entry records the recovery and release-qualification migration performed after chat/session interruption. Product transport semantics remain governed by ADR-0014: exactly one public FakeTCP-shaped connection / one SYN lineage per connected Logical Tunnel; Reality-like TLS bootstrap occurs on that same association; the in-band barrier then hands the same association to pinned wolfSSL DTLS 1.3 -> LINK -> FEC/raw-IP steady transport without an ordinary kernel-TCP data carrier.

The mature TCP-like recovery/FEC core is intentionally frozen in this work. The changes below are qualification topology and release-authority corrections, not a transport-algorithm rewrite.

## Recovered active state

- Product branch: `feat/single-flow-reality-faketcp`
- PR: #9, `[V2.6] Global single-flow Reality-like bootstrap + no-HOL steady transport`
- Recovered product HEAD before this migration: `92ddbe7cb982bf07aada3e1782a96712f74152de`
- Formal branch remains `dev/wbd-raw-fec-v2`; its handoff trail is not a substitute for live feature-branch refresh.
- Feature-branch recovery handoff before this migration was sequence 77.

## Deterministic release failure found on 92ddbe7c

The hosted Windows/Linux cross-platform qualification itself was green, but exact-head release qualification was not.

### `mux-load-100m` run 33393086899

Both RTT jobs failed in the first offered point. Build/unit setup completed; the deterministic failure was waiting for a **second** `WBD_SINGLE_FLOW_BOOTSTRAP_READY` in `scripts/bench_mux_two_session_single_flow_100m.py`.

The old benchmark still created:

1. FakeTCP public association A,
2. FakeTCP public association B,
3. independent Reality-like in-flow tickets,
4. independent DTLS clients,
5. independent LINK sessions.

That is a two-public-SYN / two-public-association topology. It contradicts ADR-0014 and therefore cannot be used as release authority for the current product.

### `game-lane-fullstack` run 33393084287

The Game/multipath qualification similarly represented the superseded model where a connected Logical Tunnel could have multiple simultaneously active public WBD associations. Under ADR-0014 this may remain useful research coverage, but it is not product release authority and must not force changes to the mature TCP-like core.

## Qualification topology correction

### 100 Mbit characterization

`scripts/bench_mux_two_session_single_flow_100m.py` was migrated to:

- **one** public FakeTCP association,
- one Reality-like TLS bootstrap and one ticket on that association,
- one DTLS client/server association,
- one LINK session,
- **two independent inner application streams** carried by that one LINK/public flow.

The historical aggregate offered points remain unchanged:

- 20 + 20 Mbit/s = 40 Mbit/s aggregate,
- 30 + 30 Mbit/s = 60 Mbit/s aggregate,
- 40 + 40 Mbit/s = 80 Mbit/s aggregate.

The existing 20 ms / 100 ms RTT, 20% random loss, FEC `off` / `20:20` matrix remains intact. Only public-transport cardinality changed.

Each result now records and the workflow asserts:

- `public_flow_model = one_public_faketcp_flow_two_inner_streams`
- `public_flow_count = 1`
- `inner_stream_count = 2`
- setup loss remains disabled until the one public association crosses bootstrap/LINK readiness;
- requested loss applies to the measured steady interval.

This preserves capacity characterization while enforcing the product invariant that capacity comes from multiplexing inner traffic over one public flow, not opening another public WBD connection.

### Release authority

`.github/workflows/release-qualification-kick.yml` now removes `game-lane-fullstack.yml` from the 13 dispatched release gates and replaces it with `single-flow-rawip-e2e.yml`.

The total release authority is unchanged:

- 13 explicitly dispatched gates,
- 8 exact-candidate push gates,
- 21 children total.

`single-flow-rawip-e2e` is the correct product replacement because it exercises one public FakeTCP flow through Reality-like bootstrap -> DTLS -> LINK -> raw-IP/TUN path, for both FEC `off` and `20:20`, and asserts the single-flow/no-second-public-transport wire invariants.

Game/multipath tests are not deleted; they are research-only under the current architecture and must not define release eligibility.

## Prequalification evidence on migration HEAD

The migration reached HEAD `93c000c3ca9f8b91e1df368796706f4e05c738bc` (`ci: replace multipath release gate with single-flow rawip`). A release-qualification kick on this moving development head is treated as **prequalification only**; it is not final authority if later documentation/handoff commits move the branch.

Confirmed so far on this head:

- `single-flow-rawip-e2e` run `33397885962`:
  - `rawip (off)` — success
  - `rawip (20:20)` — success
- `linux-server-firewall` run `33397883163` — success
- `windows-linux-single-flow` run `33397865418`:
  - Windows native wire producer — success
  - Linux consumes Windows wire — success
  - Linux raw/netns full-stack jobs were still running at the time this log entry was written.
- New `mux-load-100m` run `33397888968`:
  - both RTT jobs passed checkout/tool install/build of pinned DTLS and the one-public-flow multistream stack;
  - both entered the 40/60/80 Mbit sweep at log-write time.

No product artifact is qualified by this partial snapshot. Final authority requires a later frozen exact-head kick after development log + handoff are committed, with all 21 children successful and the branch still at the candidate SHA.

## Development rule going forward

If the new one-public-flow 100 Mbit gate fails, inspect the first deterministic failure and repair the harness/lifecycle layer first. Do **not** alter the mature FakeTCP recovery/FEC core merely to satisfy a stale or invalid multi-public-flow test model.

Only deterministic evidence that the one-public-flow product data plane itself is defective justifies changing TCP-like transport logic.
