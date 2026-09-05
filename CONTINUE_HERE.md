# Continue Here

This branch is the active development surface for wobuzhidao.

Do not infer the next task from old chat, local scratch state, old development logs or commit titles. Read, in order:

1. `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`
2. `PROJECT_CONSTITUTION.md`
3. `ARCHITECTURE.md`
4. `ROADMAP.md`
5. `.wbd/handoff/current.json`
6. `docs/development/2026-09-05_windows-physical-retest-handoff.md`
7. `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`
8. only the bounded `resume_read_set` named by the live handoff

Before editing, refresh live HEAD/PR/Actions and reconcile commits after `checkpoint_based_on_head_sha`.

## Current recovery anchors

- Active branch: `feat/single-flow-reality-faketcp`
- Active PR: #9
- Current runtime/test candidate source: `2e44c407eee677252897f2c75942407687ff8450` (`fix: fence Windows tunnel L3 identity`)
- Last same-source Windows portable + Linux amd64/arm64 build candidate known to have completed successfully before the second physical run: `cf8298f9cb30c7aa9d60ca00611a783c401ba735`
- Qualification state: **NOT RELEASE-QUALIFIED**. The current `2e44c407...` L3 fix still requires exact-source hosted build/artifact collection and a fresh Windows 11 + Npcap -> Ubuntu ARM64 application-path retest.

The final handoff-only commit may be newer than `2e44c407...`. Do not confuse a documentation/handoff HEAD with the runtime candidate that has actually been compiled/tested. `.wbd/handoff/current.json` records both the latest substantive documentation checkpoint and the runtime candidate separately.

## What changed since the stale `0e0bf686...` checkpoint

The old checkpoint said physical qualification had not yet proved real raw SYN/TLS/DTLS/LINK. That is no longer current.

### Physical round 1 — `be7607093b3065bddce024048ba376f2e5d21cdd`

Real Windows 11 + Npcap -> Ubuntu ARM64 traffic on public `:443` proved the outer path through:

- raw FakeTCP SYN/SYNACK;
- same-flow Reality-like TLS bootstrap;
- DTLS 1.3;
- LINK.

It did **not** prove Game/Wintun/shared-TUN/application E2E. The Windows portable package omitted `wbd-game-lane-client.exe`, so the controller failed before TUN startup. This was a packaging/runtime-closure defect, not evidence that Game must be bypassed for `Lanes=1`.

### Packaging closure — `e042...` -> `4d391...` -> `cf8298...`

The Windows producer was fixed to build, manifest, embed and extraction-verify `wbd-game-lane-client.exe`; the physical workflow contract was updated accordingly. Two follow-up commits repaired only mistakes in the new static contract test. At `cf8298...`, CI and same-source Windows portable plus Linux amd64/arm64 builds completed successfully, and the Windows runner verified that the Game child was actually present in the embedded runtime.

### Physical round 2 — `cf8298f9cb30c7aa9d60ca00611a783c401ba735`

This run crossed the previous break and reached:

- `WBD_GAME_LANE_CLIENT_READY`;
- `WBD_TUN_READY` / client `connect_pass`;
- server `WBD_LINK_MUX_BACKEND_READY`;
- server `WBD_GAME_LANE_SESSION_OPEN`;
- server `WBD_SHARED_TUN_SESSION_READY`.

The next failure was at the Windows L3 boundary: the server observed non-IPv4 payloads and IPv4 source `169.254.99.241` while the Logical Tunnel lease was `10.66.0.1`. Strict server source anti-spoof correctly rejected the mismatch; DNS/UDP/TCP application probes timed out.

### Current fix — `2e44c407eee677252897f2c75942407687ff8450`

The fix keeps the server anti-spoof boundary strict and instead fences the WBD-owned Wintun identity:

- disable IPv4 DHCP only on the WBD-owned Wintun interface;
- remove all IPv4 addresses on that Wintun except the server-issued Logical Tunnel lease;
- ensure/verify the lease address is exclusive before routes proceed;
- forward only valid IPv4 from the Windows TUN into Game/raw-IP;
- drop non-IPv4 locally/fail-closed.

This does **not** disable DHCP on physical Wi-Fi/Ethernet. The tunnel IPv4 is still assigned automatically from the server Logical Tunnel lease; only Windows DHCP/APIPA behavior on the WBD virtual adapter is fenced out.

Expected evidence includes `WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ... address4=<lease> ... dhcp=disabled`; `WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1` may appear for locally generated non-IPv4 traffic and is not itself a failure.

## Architecture guardrails

ADR-0012 is the current tunnel/lane lifecycle authority. In particular:

- `single-flow` is per Transport Lane / transport epoch, not per entire Logical Tunnel lifetime;
- one Logical Tunnel owns stable identity/lease and one SessionID/PacketID race namespace with 1..4 logical lanes;
- Normal desired lanes = 1; Game/weak-network policy may use 2..4;
- `wbd-game-lane-client` remains the internal race/dedupe aggregator even when `Lanes=1`;
- planned replacement is generation-fenced make-before-break `A -> A+B -> B`; candidate failure preserves healthy A;
- four logical lanes map to a bounded 1..5 physical-slot pool, where slot 5 is replacement overlap only, never a fifth logical lane;
- Linux final path is one shared WBD TUN + root routing + one WBD-owned host NAT;
- raw IPv4 ingress requires `source == leased IPv4`; do not weaken this to accept APIPA or arbitrary sources;
- FakeTCP/Reality-like TLS/pinned wolfSSL DTLS/LINK/FEC wire remains frozen absent deterministic lower-layer evidence;
- FEC primary functional path remains OFF; fixed 20:20 is compatibility smoke only;
- Windows IPv6 remains fail-closed until real IPv6 qualification;
- SourceIP/default-route changes are not authoritative direct public NAT mapping reflection.

## Deployment/test fences learned from the physical runs

- Final public endpoint used for WBD physical qualification is port `443`.
- Internal LINK listen moved to `127.0.0.1:47010` because the host already used `47000` for frps.
- Do not reuse the earlier `40443` physical endpoint without first reconciling the host's existing NAT redirect; that trial was redirected to the old `10443` service and did not hit the intended WBD listener.
- A public `:443` pcap proves the outer transport only. Full qualification also needs internal/shared-TUN markers and route-fenced application probes.

## Next atomic action

Continue the user's requested delivery flow from the current runtime candidate `2e44c407eee677252897f2c75942407687ff8450`:

1. live-refresh Actions for the exact runtime candidate and, if the handoff/docs commits caused a newer source requirement, deliberately decide whether a rebuild at the newer source is necessary rather than mixing SHAs;
2. run/collect the normal hosted CI plus real Windows portable and Linux ARM64 producer results for one exact source;
3. deliver the compiled Windows x64 portable client and Ubuntu ARM64 server test candidate with SHA256/source fencing once those hosted gates are green;
4. the user then performs the next physical Windows 11 + Npcap -> Ubuntu ARM64 retest;
5. require the retest to prove Wintun address exclusivity/no APIPA leakage, Game/TUN/shared backend continuity, shared raw-IP RX/TX and route-fenced DNS/generic UDP/TCP application traffic before considering release qualification.

Do **not** wait for the next physical run merely to hand over a hosted-green test candidate; equally, do **not** call that candidate `RELEASE-QUALIFIED` before the physical application path passes.

GitHub is the project recovery authority. Detailed physical-test conclusions and the recent fix chronology are recorded in `docs/development/2026-09-05_windows-physical-retest-handoff.md`; chat history and external drive copies are secondary convenience only.
