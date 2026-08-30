# 2026-08-30 Single-flow release hardening checkpoint

## Purpose

This log is the durable GitHub record for the release-hardening work performed after the V2.3 single-public-flow architecture and the Windows lifecycle fixes were already implemented. Chat history is not authoritative; resume from the repository handoff and the development logs named there.

## Frozen product requirement

The transport core is intentionally unchanged in this checkpoint.

A product session must have exactly one public TCP-shaped FakeTCP 4-tuple and one SYN/sequence lineage from establishment until disconnect. The first bounded phase of that same association carries real TLS 1.3 / Reality-like recognition and admission. After an ACK-drained mode barrier, the same raw association carries pinned wolfSSL DTLS 1.3, immutable LINK and optional fixed FEC. Sustained VPN payload must not inherit ordinary kernel-TCP stream HOL.

The old `ordinary Reality TCP -> close -> second FakeTCP SYN` shape is retired and must not return through orchestration, documentation or packaging.

## Starting point refreshed before edits

Feature branch: `feat/single-flow-reality-faketcp` / PR #9.

The last fully qualified substantive head before this release-hardening pass was:

`fd0f1efeeb88e73f5bbf7034a7e1c7742c4f842b`

Exact-head automated evidence included:

- CI `33293553761` success;
- single-flow E2E `33293553782` success;
- single-flow no-HOL `33293553801` success;
- single-flow two-client `33293553757` success;
- single-flow TCP persona `33293553825` success;
- single-flow LINK fullstack `33293553790` success;
- single-flow startup stress `33293553830` success;
- Windows FakeTCP persona `33293553765` success;
- Windows portable `33293553733` success;
- Windows TUN build `33293553796` success;
- Windows admin smoke `33293553824` success;
- Linux server release `33293553809` success;
- Linux shared-port `33293553808` success;
- mux-load-100m `33293553734` success;
- FakeTCP native `33293553811` success;
- FakeTCP first-arrival `33293553826` success;
- FakeTCP pcap 20-loss `33293553807` success;
- fullstack first-arrival `33293553836` success.

These results are automated qualification, not final physical Windows/Linux acceptance.

## Audit results

### Architecture authority

`ARCHITECTURE.md`, `PROJECT_CONSTITUTION.md` and `docs/architecture/ADR-0011-single-public-flow-reality-bootstrap.md` are aligned on the one-flow design. They explicitly reject a second public Reality connection and a parallel product kernel TCP Reality listener.

`README.md` is also aligned.

### Recovery authority

`CONTINUE_HERE.md` still referred to local/Drive state as if it might be required for recovery. That conflicts with the operator requirement that detailed project history remain in GitHub. It was changed so GitHub handoff + repository development logs are the recovery authority; external copies are convenience only.

### Roadmap drift

`ROADMAP.md` still cited an older `fd2fbc10...` checkpoint and described concurrent two-client requalification as pending. That was stale relative to the exact-head successful runs above. The roadmap was updated to record the completed two-client/startup/no-HOL/fullstack/load qualification and to keep only physical platform acceptance and measured fingerprint evidence open.

### Linux bundle operator text

`scripts/build_linux_server_bundle.sh` generated README text saying the public port was shared by a Reality-like setup/admission listener and raw FakeTCP. That wording could be read as the retired dual-listener architecture even though `linux_server_manager.sh` now starts only `wbd-faketcp-mux` for the public flow.

The bundle README now states explicitly:

- one raw `wbd-faketcp-mux` public listener owns `WBD_PORT`;
- Reality-like TLS setup is the first phase of that same association;
- no parallel kernel TCP Reality front exists in product mode;
- no second public SYN occurs before DTLS;
- bundled `wbd-reality-front` is diagnostic/reference only.

## Same-source artifact hardening

A new release rule was introduced: physical Windows/Linux qualification is invalid if the artifacts were built from different substantive source SHAs.

### Linux

Commit `6c4f9f1a0c6b72c31c3827e13bf2bbb47c31bd58` changed the Linux bundle builder to write `SOURCE_SHA` inside the extracted server bundle and to report `public_single_flow=1` plus the build SHA in the release marker.

### Windows

Commit `4e7cceee7a16b0ec3a285ffbcd45c38528452499` changed the Windows portable workflow to:

- write `SOURCE_SHA` into the embedded payload;
- include `source_sha` in `manifest.json`;
- upload a top-level `SOURCE_SHA` sidecar beside `wbd.exe`;
- fail if the sidecar does not match `GITHUB_SHA`.

The single-executable product shape is unchanged; the sidecar is artifact evidence for qualification/hand-off.

### Release contract gate

Commit `803e9af1790215633c6b1041fa5e5375f7c7b72a` added `internal/releasecontract/single_flow_release_test.go` so ordinary `go test ./...` fails if:

- Linux product `run_server()` starts a parallel `wbd-reality-front` public listener;
- Windows product controller reintroduces legacy `reality-bootstrap` / `BuildBootstrap`;
- Linux bundle stops carrying `SOURCE_SHA` or regresses to dual-listener operator wording;
- Windows portable stops carrying embedded/artifact source-SHA evidence.

This is intentionally static release-contract coverage. It complements, rather than replaces, the protocol/fullstack tests.

## Existing reconnect/lifecycle coverage checked before adding tests

No duplicate reconnect workflow was added. `.github/workflows/single-flow-startup-stress.yml` already performs twenty rapid end-to-end cycles through a NAT namespace while keeping the server stack alive. Each round uses the real single-flow TLS bootstrap, pinned wolfSSL DTLS, LINK and UDP echo; the client processes are then killed to model dirty termination before the next flow. This is the durable repeated-startup regression.

Windows controller tests already contain a one-public-flow orchestration assertion and reject a legacy `reality-bootstrap` event. The release-contract gate therefore protects the repository/product surface rather than duplicating the controller behavior test.

## Known security hardening still open

Linux product startup currently sources the account configuration in the manager and passes the Reality-like route key / username / password to `wbd-faketcp-mux` through command-line flags. The single-flow data path is unaffected, but privileged process listings/systemd status can expose these values.

This must be removed before release. Preferred direction: a root-only runtime bootstrap-secrets file (or another non-argv protected channel) consumed by the mux, while retaining explicit CLI flags only for isolated tests/diagnostics. Do not mix this security change with FakeTCP recovery/FEC/sequence logic.

## Physical acceptance state

Do not deliver a final package yet.

The user requires Windows and Linux to be actually proven together before final delivery. Previous physical evidence reached the single-flow bootstrap, DTLS, LINK and TUN layers, and later deterministic Windows lifecycle failures were addressed, but the current exact source pair has not yet completed a new physical one-shot after all current fixes.

Before the next physical attempt:

1. wait for the final release-hardening HEAD automated gates to complete;
2. require Windows portable and Linux ARM64 release success on that same substantive HEAD;
3. extract/compare their `SOURCE_SHA` values and reject any mismatch;
4. only then provide clearly labelled physical-test candidates;
5. require physical server startup + Windows connect + DNS/UDP/TCP probes + disconnect cleanup;
6. only after that evidence may the pair be called final/release-ready.

## Handoff requirement

Before ending this development task, update canonical `.wbd/handoff/current.json` to the next sequence with:

- live refresh required;
- PR #9 and exact tested feature HEAD;
- exact workflow run IDs/conclusions for required green gates;
- `docs/development/SINGLE_FLOW_V23_DEVLOG.md` and this file in the bounded resume set;
- physical acceptance explicitly pending;
- next atomic action based on the first remaining deterministic blocker, not old chat.
