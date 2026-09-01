# Single-flow V3 physical Npcap qualification gate — 2026-09-01

## Purpose

This note is an authoritative development log for the final Windows physical qualification layer of `exp/singleflow-realitylike-v3` / PR #10.

The user acceptance rule is now strict: **do not hand out a new Windows/Linux release pair until the same V3 behavior is green on Linux and on a real Administrator Windows 11 x64 machine using the real Npcap capture/injection driver path.** Hosted Windows compilation and user-mode packet-demux tests are necessary but are not a substitute for a physical Npcap pass.

The existing TCP-like steady-state data plane is frozen. This work must not redesign sender/receiver/recovery/FEC behavior. The public transport invariant remains:

1. one public TCP-shaped 4-tuple;
2. one SYN / one SYN-ACK;
3. ordered/reliable bootstrap only for the short Reality-like TLS setup;
4. encrypted switch request/ack inside the same raw sequence space;
5. no FIN, no close-notify, no second SYN and no new public tuple at switch;
6. post-switch datagram phase reuses the existing first-arrival TCP-like transport and carries pinned wolfSSL DTLS 1.3 -> LINK -> optional FEC without steady-state HOL.

## Why a new physical gate is required

Earlier physical Windows V3 evidence exposed `wbd-faketcp handshake: faketcp: not ipv4/tcp` even though Linux V3 had reached the same-flow bootstrap, DTLS server READY and LINK session READY. The cause was Windows Npcap adapter noise being passed to the strict FakeTCP parser. That defect was fixed in V3 by exact Ethernet/IPv4/TCP/4-tuple demultiplexing before the strict parser, with repeated hosted-Windows adapter-noise tests.

That fix is now covered automatically, but only a machine with the Npcap driver attached to a real Windows adapter can prove the final `pcap_next_ex` / `pcap_sendpacket` capture/injection boundary together with actual route, DTLS, LINK, TUN and Internet probes.

## Npcap license / lifecycle boundary

WBD remains pinned to Npcap 1.88:

- installer SHA-256: `a2f4ec1e5ea353ff67efd24b2ebf081ba44532410fae8d5e146af0310aa4f56b`
- expected Authenticode signer: `Nmap Software LLC`
- WBD never redistributes the Free Edition installer.

Official Npcap documentation distinguishes installation and removal:

- The public/free installer is intentionally launched in normal graphical mode by WBD; WBD does **not** use OEM-only silent-install entitlement.
- Silent uninstall `/S` is documented as available for every edition. The physical qualifier therefore supports normal operator installation before the run and verified silent removal in `finally` after the run.

The qualifier must never claim success if cleanup fails.

## Commits added for the physical gate

### `1c6a2e5754f30cb9a15ecef29bffd12233e9bd5e` — verified Npcap uninstall

`scripts/windows_npcap_prepare.ps1` now supports `-Action Uninstall`.

The uninstall path:

- locates the installed Npcap uninstaller under Program Files;
- validates the uninstaller Authenticode signer as `Nmap Software LLC`;
- invokes `/S /no_kill=no`;
- accepts normal success/reboot-required exit codes;
- verifies that `wpcap.dll`, `Packet.dll` and the `npcap` service are gone;
- emits `WBD_WINDOWS_NPCAP_UNINSTALL_PASS` only after verification.

The existing `Install` action remains graphical and operator-driven.

### `162177b90590bf84cd6de2e6207d134035934ccc` — headless portable presentation

`cmd/wbd-windows-portable/main_windows.go` now honors `WBD_HEADLESS=1` in `showMessage`.

This changes presentation only. The exact same portable EXE still performs elevation, embedded-runtime extraction, single-flow transport startup, readiness gates, route mutation, probes and cleanup. In headless mode modal MessageBox calls become stdout/stderr output so a self-hosted Administrator runner cannot hang on UI after `--self-test`.

### `8aa563e7aa770ebdb744d3e8533896cc1482bd55` — physical qualification script

New `scripts/windows_npcap_physical_qualify.ps1`:

- requires Administrator rights;
- requires a real portable `wbd.exe` and a runner-local profile path;
- verifies installed Npcap with `windows_npcap_prepare.ps1 -Action Status`;
- runs the exact portable EXE with `WBD_HEADLESS=1 --self-test`;
- parses the resulting JSONL rather than accepting process startup as success;
- can call verified Npcap uninstall in `finally`.

Required top-level self-test events:

- `dependency_preflight_pass`
- `underlay_pass`
- `connect_pass`
- `probe_system_dns_pass`
- `probe_udp_pass`
- `probe_tcp_pass`
- `cleanup_pass`
- `self_test_pass`

Required child evidence:

- FakeTCP: `WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared`
- FakeTCP: `READY role=client ...`
- FakeTCP: `WBD_SINGLEFLOW_TLS_SWITCH_ACK_RECEIVED`
- FakeTCP: `WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only`
- DTLS: `READY role=client ...`
- LINK: `WBD_LINK_READY role=client ...`
- TUN: `WBD_TUN_READY mode=client ...`

Any failure event rejects the qualification. A pass emits:

`WBD_WINDOWS_NPCAP_PHYSICAL_PASS ... npcap_driver=1 capture_injection=1 single_public_flow=1 dtls=1 link=1 tun=1 probes=1 cleanup=1`

### `508ab8a94dd8119e690803299522cd43ce9c382c` — self-hosted physical workflow

New `.github/workflows/windows-npcap-physical.yml` is a manual physical gate targeting labels:

`self-hosted`, `windows`, `x64`, `wbd-npcap`

Inputs identify the exact successful Windows portable artifact run, expected source SHA, a runner-local profile path and whether to uninstall Npcap after the run.

The workflow refuses an artifact whose Actions run HEAD differs from the requested source SHA. It verifies the Actions ZIP digest when supplied by GitHub, logs the portable EXE hash, runs the physical qualifier and uploads the JSONL evidence.

Credentials are not stored in the repository; `profile_path` points to a profile already present on the physical runner.

### `34906fc6997b1440a8dc1c4709691a8f44a3a875` — hosted parser/build guard

`.github/workflows/singleflow-v3-crossplatform.yml` now makes Windows hosted CI:

- run the V3 Go tests;
- build Windows FakeTCP and portable entrypoint;
- parse both Npcap PowerShell qualification scripts with PowerShell's parser;
- assert that the uninstall and physical PASS contracts are present.

This catches syntax/build regressions before a physical runner is used. It does **not** pretend that hosted Windows has performed Npcap capture/injection.

## Automated result snapshot for `34906fc6997b1440a8dc1c4709691a8f44a3a875`

Authority: historical snapshot, **not live**. Always refresh Actions before resuming.

Confirmed completed/success at the time of this note:

- `singleflow-v3-crossplatform` run `33467225228`
- `singleflow-realitylike-e2e` run `33467224779`
- `ci` run `33467225428`
- `faketcp-native` run `33467224666`
- `faketcp-first-arrival` run `33467225128`

At the time this note was written, `fullstack-first-arrival` run `33467224620` was still in progress and therefore must not be treated as green until refreshed.

Other current-head regressions such as reconnect stress / pcap loss must likewise be refreshed before closure.

## What is and is not proved

Already proved automatically by V3 before this physical-gate work and rechecked by current-head gates:

- the public flow is single-tuple/single-SYN in the strong Linux/NAT E2E;
- Reality-like TLS bootstrap and encrypted switch occur before datagram mode;
- the switch reuses the same raw sequence space;
- DTLS 1.3, LINK and payload echo work after switch;
- an explicit post-switch drop/reorder test demonstrates no steady-state HOL;
- Windows V3 compiles and repeatedly ignores unrelated adapter traffic before strict packet parsing;
- the new physical qualification scripts parse on hosted Windows.

**Not yet proved for this new final gate:** a successful run of the exact final portable artifact on a real Administrator Windows 11 x64 adapter with Npcap 1.88 doing real capture/injection, followed by verified Npcap removal. Until that evidence exists, no new artifact pair is release-qualified for user delivery.

## Next atomic actions

1. Refresh all `34906fc...` current-head Actions and investigate the first deterministic red if any.
2. Ensure a Windows portable artifact is built from the exact behavior checkpoint used by physical qualification.
3. Run `windows-npcap-physical` on an Administrator self-hosted Windows 11 x64 runner with Npcap 1.88 already installed normally; use `uninstall_after=true`.
4. Require the physical JSONL to contain every marker/event listed above and no failure event; require verified Npcap uninstall success.
5. Run/reconfirm the strong Linux single-flow NAT E2E and build the ARM64 server from the same behavior checkpoint.
6. Only after both physical Windows and Linux gates are green, hash and deliver the Windows x64 portable + Linux ARM64 pair.
7. If physical Windows fails, inspect the first missing marker/boundary and fix that layer before producing any user artifact.
