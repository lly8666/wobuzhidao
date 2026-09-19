# 2026-09-05 `2e44c407` Test Package Delivery

This record closes the hosted-build/package phase for runtime source `2e44c407eee677252897f2c75942407687ff8450` (`fix: fence Windows tunnel L3 identity`). It follows `docs/development/2026-09-05_windows-physical-retest-handoff.md` and does **not** authorize release.

## Exact-source fence

All producer evidence below is from the same immutable runtime source:

`2e44c407eee677252897f2c75942407687ff8450`

Later documentation/handoff commits are not runtime candidates and must not be substituted for this source.

## Hosted CI

GitHub Actions CI run `33941726034`:

- workflow: `ci`
- head SHA: `2e44c407eee677252897f2c75942407687ff8450`
- result: success
- `test` job: success
- `Go tests`: success
- `Handoff tests`: success
- title-gated `transport_prepare`, `transport_smoke`, `transport_bench`, and `transport_aggregate`: skipped by design on this push path; not failures and not substituted for physical qualification.

## Windows portable producer

GitHub Actions run `33941725966`:

- workflow: `windows-portable-bundle`
- head SHA: exact `2e44c407...`
- result: success
- job `bundle`: success
- WBD Windows child runtime build: success
- pinned wolfSSL DTLS shim build: success
- locked Wintun compatibility DLL: success
- child-runtime PE dependency allowlist: success
- manifest hash + embedded portable payload: success
- portable launcher build: success
- embedded portable runtime extraction qualification: success
- portable EXE dependency verification: success
- artifact upload: success

Artifact:

- GitHub artifact ID: `9962080813`
- name: `wbd-windows-portable-2e44c407eee677252897f2c75942407687ff8450`
- GitHub artifact digest / locally downloaded outer ZIP SHA256: `f799cac74d502a2b03b191e1a5d93b200b84cc9edbf60c5066a18c83a3b7e21c`
- outer ZIP contains exactly one `wbd.exe`, size `14,653,952` bytes
- extracted `wbd.exe` SHA256: `bccf596e2cb4b47ecb3e4eea81ef06cdda86037c70292ace12bc44a6d0c7386b`
- local string inspection independently sees `wbd-game-lane-client.exe`; the stronger producer-side embedded extraction qualification also passed.

## Linux server producer

GitHub Actions run `33941726028`:

- workflow: `linux-server-release`
- head SHA: exact `2e44c407...`
- result: success
- `settings`: success
- `arm64`: success
- `amd64`: success

ARM64 artifact:

- GitHub artifact ID: `9962069074`
- name: `wbd-linux-server-arm64-2e44c407eee677252897f2c75942407687ff8450`
- GitHub digest / outer ZIP SHA256: `b991b9b817cff58c9c04af5d5b753bfcd6fa15691a9ca29148f63455e0f93b14`
- inner `wbd-linux-server-arm64.tar.gz` SHA256: `471109e6260e5d258c41e5fababf50572e94dc6f433eec667ff8ab341c709db5`
- bundled `.sha256` matches the independently calculated inner hash
- bundled `wbd-server-arm64/SOURCE_SHA`: exact `2e44c407eee677252897f2c75942407687ff8450`

AMD64 backup artifact:

- GitHub artifact ID: `9962072737`
- name: `wbd-linux-server-amd64-2e44c407eee677252897f2c75942407687ff8450`
- GitHub digest / outer ZIP SHA256: `a314124ea3d2fe5fcad7117fc457920c2b87af68eef8f7dfdf960b2fc70a26eb`
- inner `wbd-linux-server-amd64.tar.gz` SHA256: `db75ff3524da4da4c537c4ea03b389e637e1723f472d9f1d50fcbfc732cbc034`
- bundled `.sha256` matches
- bundled `wbd-server-amd64/SOURCE_SHA`: exact `2e44c407eee677252897f2c75942407687ff8450`

## Qualification state

This source is now **HOSTED-GREEN + SAME-SOURCE TEST PACKAGES DELIVERED**.

It is still **NOT RELEASE-QUALIFIED**. The L3 identity fix itself still needs fresh physical Windows 11 + Npcap -> Ubuntu ARM64 application-path evidence.

The next physical run must confirm, on exact `2e44c407...`:

1. `WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ... address4=<lease> ... dhcp=disabled` and no non-lease/APIPA IPv4 remains on the WBD-owned Wintun;
2. locally generated non-IPv4 does not enter Game/raw-IP (`WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1` may appear and is not itself a failure);
3. Game and Windows TUN become ready;
4. server Game/LINK backend and shared-TUN session become ready;
5. shared raw IPv4 RX/TX uses the server-issued lease as source identity rather than APIPA;
6. route-fenced DNS, generic UDP, and TCP application probes succeed bidirectionally;
7. cleanup restores WBD-owned routes/DNS/IPv6 state without damaging host networking.

Do not weaken the server source anti-spoof boundary to pass this test. Do not modify FakeTCP/Reality-like TLS/DTLS/LINK/Game/FEC wire unless the fresh evidence identifies a deterministic lower-layer defect.
