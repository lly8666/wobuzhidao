# 2026-08-30 Windows route-state recovery and runtime logging

## Context

This log is durable project history for the physical Windows 11 x64 -> real NAT/ISP -> Ubuntu ARM64 qualification of the single-public-flow design. Chat history is not authoritative because long sessions may be truncated.

Qualified architecture remains unchanged:

- one public TCP-shaped 4-tuple and one SYN/sequence lineage per WBD session;
- FakeTCP owns the public flow from SYN;
- the first bounded reliable phase carries Firefox-like TLS 1.3 / Reality-like admission;
- the same association switches in-band, without FIN/RST/new SYN, to pinned wolfSSL DTLS 1.3 -> LINK/FEC/VPN datagrams;
- mature FakeTCP recovery/ACK/SACK/FEC behavior remains frozen.

## Physical evidence received 2026-08-30

Windows self-test reached all transport readiness boundaries on one public flow:

1. Npcap mode ready with exact inbound flow filtering.
2. First raw payload TX and RX observed.
3. `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`.
4. FakeTCP client READY.
5. `WBD_DTLS_CLIENT_CONNECT_PASS` / DTLSv1.3 READY.
6. `WBD_LINK_READY role=client`.
7. `WBD_TUN_READY mode=client`.
8. IPv6 kill-switch apply succeeded.

The first deterministic failure was **not transport**. `route-apply` failed because an existing `C:\ProgramData\WBD\diagnostics\route-state.json` was treated as a manual-cleanup prerequisite. Cleanup then tried to remove a saved NRPT GUID that no longer existed and PowerShell surfaced WIN32 1168 / ObjectNotFound. The runtime therefore rolled back.

Ubuntu evidence independently showed successful single-flow bootstrap, DTLS server `ACCEPT_PASS`, and LINK mux session creation for multiple physical sessions. This further confirms that the current blocker is Windows lifecycle/state cleanup rather than FakeTCP/DTLS/LINK.

## Root causes

### 1. Stale route-state was not crash-recoverable

`windows_tun_route.ps1` previously aborted Apply whenever the state file already existed:

`state already exists; run Cleanup first`

That turns an ordinary prior crash/interrupted cleanup into a permanent startup blocker until manual intervention.

### 2. Missing saved NRPT object was treated as fatal

`Remove-Owned-State` attempted `Remove-DnsClientNrptRule` before route/address cleanup. A saved WBD-owned GUID may already be absent because Windows or a prior partial cleanup removed it. On the physical machine this produced ObjectNotFound / WIN32 1168 and prevented the remainder of WBD-owned cleanup.

### 3. Route-state JSON was written with Windows PowerShell UTF-8 BOM

PowerShell 5.1 `Set-Content -Encoding UTF8` emits a BOM. Earlier diagnostics showed Go JSON decoding failing with `invalid character 'ï' looking for beginning of value`. The route script now writes state with `System.Text.UTF8Encoding(false)`.

### 4. FakeTCP process ownership could be released twice on failed continuation

`Controller.Connect` starts the sole public FakeTCP child, then passes it to `Executor.StartAfterFakeTCP`. On route/readiness failure the Executor rollback already stops the child, but the Controller defer still considered itself owner. Diagnostics could then issue a second Stop/Wait and log `exec: Wait was already called`.

Ownership is now transferred to Executor **before** invoking `StartAfterFakeTCP`, so every rollback path has exactly one owner.

### 5. Portable Windows runtime did not retain default logs beside `wbd.exe`

Self-test defaulted to TEMP and normal GUI runs inherited effectively invisible Windows-GUI stdout/stderr. The portable outer client now creates a `logs` directory beside `wbd.exe`:

- self-test: `logs/self-test-YYYYMMDD-HHMMSS.mmm.jsonl`;
- normal GUI/runtime: `logs/runtime-YYYYMMDD-HHMMSS.mmm.log`.

The GUI process inherits the runtime log file as stdout/stderr, so existing child process output from FakeTCP, DTLS, LINK, TUN and PowerShell route/IPv6 operations is persisted without changing transport semantics.

## Implemented commits on `feat/single-flow-reality-faketcp`

- `0fba4f4197a87b7187a011c39a21171924f33f23` — transfer FakeTCP ownership before runtime continuation.
- `5867e2ff0b8cf09c37961b6c7f02e93529ee9a42` — persist portable runtime/self-test logs beside `wbd.exe`.
- `e1eed301f788a732023a54a23a9c9cfe0048c400` — make Windows route-state recovery idempotent, tolerate missing WBD-owned NRPT, and write UTF-8 without BOM.
- `eb8885fadfb32aed0539d61ed386606bddf7d457` — regression test that transferred FakeTCP is stopped exactly once on route-apply rollback.
- `018d18394d1495ab65f19c0d344d64c5ac5a2893` — Windows portable log-path test.
- `54f304aaafc2de4c8de1e22211287658166456c8` — Windows hosted Wintun smoke now reproduces stale state with a nonexistent NRPT GUID and requires automatic recovery plus BOM-free state.

Current substantive head for this qualification round: `54f304aaafc2de4c8de1e22211287658166456c8`.

## Required automated proof before another physical artifact is delivered

The new Windows admin smoke must prove on a real hosted Windows Wintun adapter:

1. write a stale WBD route-state file containing a deliberately nonexistent NRPT GUID;
2. call normal route Apply without manual cleanup;
3. observe `WBD_WINDOWS_TUN_NRPT_ALREADY_ABSENT`;
4. observe `WBD_WINDOWS_TUN_STALE_STATE_RECOVERED`;
5. verify newly persisted state has no UTF-8 BOM;
6. verify Wintun address, underlay escape route and split capture route;
7. run the existing real bidirectional Wintun dataplane ping/probe;
8. cleanup and prove state, address and capture route are gone.

In addition, the exact head must pass the existing single-flow E2E/no-HOL/persona/two-client/startup-stress/link-fullstack gates, Windows portable build, Windows runtime tests, Linux release, loss/recovery gates and mux-load-100m. No new physical artifact should be delivered until relevant current-head red gates are resolved.

## Interpretation rule

Do not modify the mature TCP-like recovery/FEC core because of this physical failure. The physical trace already proves the one-flow bootstrap, DTLS, LINK and TUN layers became ready. Any further failure should be classified from the first deterministic marker and reproduced in CI/sandbox before transport-core changes.
