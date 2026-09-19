# Continue Here

This branch is the active development surface for wobuzhidao.

Do not infer the next task from old chat, local scratch state, old development logs or commit titles. Read, in order:

1. `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`
2. `PROJECT_CONSTITUTION.md`
3. `ARCHITECTURE.md`
4. `ROADMAP.md`
5. `.wbd/handoff/current.json`
6. `docs/development/2026-09-05_windows-physical-retest-handoff.md`
7. `docs/development/2026-09-05_2e44-test-package-delivery.md`
8. `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`
9. only the bounded `resume_read_set` named by the live handoff

Before editing, refresh live HEAD/PR/Actions and reconcile commits after `checkpoint_based_on_head_sha`.

## Current recovery anchors

- Active branch: `feat/single-flow-reality-faketcp`
- Active PR: #9
- Runtime/test candidate source: `2e44c407eee677252897f2c75942407687ff8450` (`fix: fence Windows tunnel L3 identity`)
- Hosted state for that exact runtime source: **GREEN**
- Same-source Windows portable + Linux ARM64/amd64 test packages: **BUILT, HASH-VERIFIED AND DELIVERED**
- Qualification state: **NOT RELEASE-QUALIFIED**; fresh physical Windows 11 + Npcap -> Ubuntu ARM64 application-path evidence is still required.

The final documentation/handoff branch HEAD is newer than `2e44c407...`. Do not confuse it with the compiled runtime candidate. `.wbd/handoff/current.json` records the documentation checkpoint and runtime candidate separately.

## Recent physical/fix chain

### Physical round 1 — `be760709...`

Real public `:443` traffic proved raw FakeTCP SYN/SYNACK -> same-association Reality-like TLS -> DTLS 1.3 -> LINK. It did not prove Game/Wintun/shared-TUN/application E2E because the Windows portable package omitted required `wbd-game-lane-client.exe`.

### Packaging closure — `e042...` -> `4d391...` -> `cf8298...`

The Windows producer was fixed to build, manifest, embed and extraction-verify the Game child. At `cf8298...`, same-source CI/Windows/Linux builds were green.

### Physical round 2 — `cf8298...`

This crossed the prior break and physically reached Game, Windows TUN, server Game/LINK backend and shared-TUN session. The first broken layer moved to Windows L3 identity: non-IPv4 payloads and source `169.254.99.241` reached the IPv4 backend while the Logical Tunnel lease was `10.66.0.1`; strict server anti-spoof correctly rejected the mismatch and application probes timed out.

### Current fix — `2e44c407...`

The fix keeps server anti-spoof strict and fences only the WBD-owned Wintun:

- disable IPv4 DHCP on the WBD virtual adapter, not physical Wi-Fi/Ethernet;
- remove every non-lease IPv4 from that adapter;
- verify the authenticated server-issued Logical Tunnel lease is exclusive before routes proceed;
- drop non-IPv4 locally before Game/raw-IP.

The tunnel IPv4 is still automatically assigned by WBD from the authenticated server lease; Windows DHCP/APIPA is merely prevented from adding a competing identity on the WBD-owned virtual adapter.

Expected new markers include `WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ... address4=<lease> ... dhcp=disabled`. `WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1` may appear for local IPv6/control traffic and is not itself a failure.

## Exact-source hosted/package receipt for `2e44c407...`

- CI run `33941726034`: success; Go tests and Handoff tests success.
- Windows portable run `33941725966`: success; child runtime, wolfSSL, Wintun, manifest/embed, embedded extraction qualification and PE checks all success.
- Windows artifact ID `9962080813`, ZIP SHA256 `f799cac74d502a2b03b191e1a5d93b200b84cc9edbf60c5066a18c83a3b7e21c`.
- Linux server run `33941726028`: success; settings, amd64 and arm64 success.
- ARM64 artifact ID `9962069074`, ZIP SHA256 `b991b9b817cff58c9c04af5d5b753bfcd6fa15691a9ca29148f63455e0f93b14`; inner tar SHA256 `471109e6260e5d258c41e5fababf50572e94dc6f433eec667ff8ab341c709db5`; bundled SOURCE_SHA exact `2e44c407...`.
- AMD64 backup artifact ID `9962072737`, ZIP SHA256 `a314124ea3d2fe5fcad7117fc457920c2b87af68eef8f7dfdf960b2fc70a26eb`.

Full receipt: `docs/development/2026-09-05_2e44-test-package-delivery.md`.

## Architecture guardrails

- ADR-0012 remains the tunnel/lane lifecycle authority; ADR-0011 remains per-lane same-association bootstrap/no-HOL authority.
- `single-flow` is per Transport Lane / transport epoch, not per entire Logical Tunnel lifetime.
- One Logical Tunnel owns stable identity/lease and one SessionID/PacketID race namespace with 1..4 logical lanes.
- Normal desired lanes = 1; Game/weak-network policy may use 2..4.
- `wbd-game-lane-client` remains the internal race/dedup aggregator even when `Lanes=1`.
- Planned replacement is generation-fenced make-before-break `A -> A+B -> B`; candidate failure preserves healthy A.
- Four logical lanes map to bounded physical slots 1..5; slot 5 is replacement overlap only, never a fifth logical lane.
- Linux final path is one shared WBD TUN + root routing + one WBD-owned host NAT.
- Raw IPv4 ingress requires `source == leased IPv4`; never weaken this to accept APIPA or arbitrary client sources.
- FakeTCP/Reality-like TLS/pinned wolfSSL DTLS/LINK/Game/FEC wire remains frozen absent deterministic lower-layer evidence.
- FEC primary functional path remains OFF; fixed 20:20 is compatibility smoke only.
- Windows IPv6 remains fail-closed until real IPv6 qualification.
- SourceIP/default-route changes are not authoritative direct public NAT mapping reflection.

## Deployment/test fences

- Physical WBD public endpoint: port `443`.
- Internal LINK listen: `127.0.0.1:47010` because `47000` conflicts with frps on the test host.
- Do not reuse public `40443` without reconciling the host NAT redirect that previously sent it to old `10443`/v2ray.
- Normal physical retest: `Lanes=1`, FEC OFF.
- A public `:443` pcap proves only the outer transport. Full qualification requires shared-TUN/raw-IP and application-path evidence.

## Next atomic action

The exact `2e44c407...` test packages are already hosted-green and delivered. The next authoritative event is the user's fresh physical Windows 11 + Npcap -> Ubuntu ARM64 retest on those packages.

When evidence arrives:

1. verify the client/server logs identify runtime source `2e44c407...` or otherwise prove the exact delivered artifact identity;
2. inspect raw logs before trusting any test-AI summary;
3. require Wintun lease exclusivity/no APIPA leakage;
4. correlate Game/TUN/LINK/shared-TUN readiness with `WBD_SHARED_RAWIP_RX_FIRST` / `WBD_SHARED_RAWIP_TX_FIRST` where emitted;
5. verify route-fenced DNS, generic UDP and TCP application traffic plus return path;
6. if a pcap is supplied, analyze it independently and correlate timestamps/tuples with logs;
7. verify cleanup/host-network restoration;
8. only change the first deterministically broken layer. Do not redesign mature transport wire from symptoms above or below that boundary.

Do **not** call this source `RELEASE-QUALIFIED` until the physical application path passes on the exact runtime source.

GitHub is the project recovery authority; chat history and external copies are secondary convenience only.
