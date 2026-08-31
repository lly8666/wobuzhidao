# Single-flow Reality-like V3 — 2026-08-31 closure note

This note supplements `docs/experiments/singleflow-v3-development-log.md` and corrects one stale item in that long-running log.

## 1. Correction: `windowsdiag` child-exit lifecycle is already fixed

The older development log section 10 says the diagnostic `loggingRunner` still waits the full readiness timeout when a child exits early. That statement is stale.

Commit `52b4515111e38f2e646efd953489a57d80edd82f` (`windowsdiag: fail fast when child exits before readiness`) already closed this gap. Current `internal/windowsdiag/readiness_windows.go` maintains a process `done` channel and an idempotent `wait()` path. `WaitReady()` selects between readiness-marker progress, process exit, and timeout. If the child exits before the marker, it reports `command ... exited before readiness marker ...` immediately rather than waiting out the configured readiness deadline. `Stop()` treats an already-exited process as success and also tolerates a Kill/access-denied race when the child has already completed.

`internal/windowsdiag/readiness_windows_test.go` contains a Windows-native regression test that starts a helper child which exits after about 150 ms, configures a 5 s readiness deadline, requires the failure to be reported in under 2 s, and then requires a subsequent `Stop()` to succeed. Therefore no new transport or diagnostic lifecycle patch is needed here.

## 2. Substantive V3 checkpoint

The substantive product/test checkpoint remains:

`6a2e41a2eeb50ce9d4ff565fa11a3879a777519b`

It does not rewrite the existing TCP-like sender/receiver/recovery/FEC implementation. It adds an explicit cross-platform proof of the V3 phase boundary:

1. real TLS 1.3 Reality-like marker/authentication;
2. one-time ticket agreement;
3. encrypted TLS application-data `SWITCH_REQ / SWITCH_ACK`;
4. no plaintext switch frame on the caller-owned public carrier;
5. after encrypted switch ACK, the ordered bootstrap authority is considered destroyed;
6. the existing FakeTCP first-arrival receiver is used for steady state;
7. an earlier sequence range is intentionally missing while a later independent datagram is submitted;
8. the later datagram must deliver immediately while the shadow ACK frontier remains before the hole and SACK/out-of-order state is retained.

This proves the V3 requirement that TCP-shaped recovery metadata can coexist with no steady-state HOL.

## 3. Windows hosted qualification

`singleflow-v3-crossplatform` run `33410098583` passed on substantive checkpoint `6a2e41a2...`. The Windows job ran the real Windows Go test binaries, not a Linux cross-compile. It executed Reality-like TLS/auth/encrypted-switch/no-HOL tests, Windows Npcap demux/parser regressions, repeated adapter-noise handshake tests, and built the Windows FakeTCP/portable binaries.

After documentation-contract alignment, latest audited head `c3d07a628adcb494c41916db061892607de83c8b` re-ran `singleflow-v3-crossplatform` as run `33411152261` and completed **success**. The later commits between `6a2...` and `c3d07a...` are documentation/handoff contract alignment only; no TCP-like or V3 runtime behavior changed.

Boundary: hosted Windows qualification does not emulate the real Npcap kernel driver + physical NIC + home NAT/ISP. That remains the final physical platform gate.

## 4. Linux raw/NAT strong E2E

`singleflow-realitylike-e2e` run `33410098425` passed on the substantive checkpoint and uses client/server/router namespaces, NAT, raw FakeTCP, public pcap, temporary CA/certificate, and pinned wolfSSL DTLS 1.3.

The captured public association proved:

- exactly one SYN;
- no late SYN;
- no FIN or RST at the phase switch;
- no plaintext switch control on the wire;
- TLS ClientHello occurs before DTLS ClientHello;
- the same public flow proceeds into DTLS 1.3;
- bidirectional payload succeeds;
- deliberate loss of an earlier steady-state datagram does not block a later independent datagram.

After documentation-contract alignment, latest audited head `c3d07a628adcb494c41916db061892607de83c8b` re-ran the strong Linux E2E as run `33411152335` and completed **success**.

## 5. Latest-head deterministic gate closure

At audited head `c3d07a628adcb494c41916db061892607de83c8b`:

- main `ci` run `33411152318`: **success**;
- `handoff-verify` run `33411152198`: **success**;
- `singleflow-v3-crossplatform` run `33411152261`: **success**;
- `singleflow-realitylike-e2e` run `33411152335`: **success**;
- `faketcp-reconnect-stress` run `33411152268`: **success**;
- `faketcp-native` run `33411152193`: **success**;
- `faketcp-pcap-20loss` run `33411152226`: **success**;
- `faketcp-first-arrival` run `33411152330`: **success**;
- `fullstack-first-arrival` run `33411152274`: **success**;
- `openwrt-tcp-tproxy` run `33411152204`: **success**.

The earlier main-CI/handoff reds on `6a2...` were not product failures: Go tests passed, while machine-persisted architecture phrases in `PROJECT_CONSTITUTION.md` / `ARCHITECTURE.md` lagged the implemented V3 design. Documentation-only commits `d3d359d2...`, `6c0f5d4a...`, `7da6d66c...`, and `c3d07a62...` aligned those contracts. The latest-head success above closes that mismatch.

## 6. Delivery rule after this checkpoint

The automated V3 criteria requested before another physical handoff are now met: Windows hosted runtime/protocol qualification and Linux raw/NAT strong E2E both pass, and the existing TCP-like regression gates remain green.

The next action is artifact hygiene, not another speculative transport patch:

1. use the already-qualified substantive code checkpoint artifacts (later branch commits are docs/handoff only), or rebuild the same code if a release workflow is explicitly re-run;
2. verify Windows portable and Linux ARM64 artifact integrity and contents locally;
3. persist the artifact SHA-256 values in the handoff/development record;
4. only then provide the pair for the final physical Windows 11/Npcap/NIC/NAT/ISP -> Linux ARM64 test.

Do not claim the final physical platform gate has passed until that real test is actually run.