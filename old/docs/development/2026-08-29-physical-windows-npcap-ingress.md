# Physical Windows Npcap startup failure after single-flow qualification

Date: 2026-08-29

## Context

The feature line is `feat/single-flow-reality-faketcp`, PR #9. The previously qualified source head was `48e9fd45790a4c85d012aadb7a2ea50d3ad95479`.

The architecture remains unchanged and is still the required product model:

- exactly one public TCP-shaped 4-tuple and one SYN lineage per session;
- FakeTCP owns the public flow from SYN onward;
- the first bounded reliable phase carries the Reality-like TLS 1.3 bootstrap;
- the same association switches in-band to DTLS 1.3 -> LINK/FEC/VPN datagrams;
- no second public TCP connection and no ordinary-kernel-TCP steady-state HOL.

The mature TCP-like recovery/ACK/SACK/FEC core is frozen for this investigation.

## Physical evidence supplied by the user

A real Windows 11/Npcap client and Ubuntu ARM64 server were tested with the single-flow build.

One Ubuntu session demonstrated the complete intended server chain on the single public flow:

1. `WBD_SINGLE_FLOW_BOOTSTRAP_READY ... same_flow=1`;
2. inherited DTLS worker `BOUND`;
3. DTLS `PEEK`, peer set, HRR armed and `ACCEPT_START`;
4. `WBD_DTLS_SERVER_ACCEPT_PASS` / DTLSv1.3 READY;
5. `WBD_LINK_MUX_SESSION_READY`.

This proves that the single-flow architecture itself is viable on the real ARM64 server and that bootstrap -> DTLS -> LINK can complete without a second public SYN.

A later Windows self-test failed much earlier. The relevant sequence was:

1. profile/routing/dependency/underlay checks pass;
2. FakeTCP starts and Npcap mode is cleared successfully;
3. FakeTCP exits during handshake with `faketcp: not ipv4/tcp`;
4. the controller waits for the same-flow Reality ticket until timeout because the FakeTCP child that owns bootstrap has already exited;
5. cleanup attempts to stop the already-dead child, producing a secondary `TerminateProcess: Access is denied` diagnostic in the self-test runner.

The ticket timeout is downstream fallout. The first deterministic failure is the FakeTCP handshake parser receiving a non-TCP IPv4 packet from the physical Npcap adapter.

## Root cause

`cmd/wbd-faketcp/main_windows.go` previously converted every Ethernet IPv4 frame captured by Npcap into an IP packet and returned it to the protocol layer. It did **not** require:

- IPv4 protocol == TCP;
- peer/source IPs to match the WBD flow;
- TCP ports to match the WBD flow;
- inbound direction;
- non-fragmented input.

During steady state `rawLoop()` already ignores `ParseIPv4TCP` errors, but handshake uses `recvOne()`, where a parse error is fatal. A normal physical adapter can receive unrelated IPv4 UDP, ICMP, other TCP flows, and reflected outbound traffic while the WBD SYN handshake is in progress. Therefore the real adapter can terminate startup even though namespace CI is green.

This is a Windows/Npcap ingress-boundary bug, not a FakeTCP recovery/FEC algorithm bug.

## Test-gap finding

The original `windows-faketcp-persona` workflow only ran packet-persona tests under `internal/faketcp` and then built `cmd/wbd-faketcp`. It did **not** execute Windows-tagged backend tests under `cmd/wbd-faketcp`, nor the runtime/diagnostics tests that own physical startup readiness.

Linux namespace single-flow E2E also does not model a noisy physical Ethernet adapter. Its raw socket input is much cleaner than a real Npcap capture handle.

## Fix line

All fixes in this section deliberately stay outside the mature FakeTCP ARQ/SACK/FEC core.

### `e3bf03b18d407608771e2ced1177257b217733b1` — `ci: run Windows FakeTCP backend tests`

The Windows gate started executing `go test ./cmd/wbd-faketcp -count=1 -v` on `windows-latest`.

### `cc70119819d77d18cfa7441d37610f529f6e3b32` — `windows: filter Npcap ingress to exact FakeTCP flow`

Npcap still observes exact local-kernel RSTs for diagnostics, but only an unfragmented inbound IPv4/TCP packet matching:

`remote_ip:remote_port -> source_ip:source_port`

is delivered to the FakeTCP handshake/data receive state machine. Outbound self-capture, UDP, ICMP, unrelated TCP, wrong peers/ports, malformed lengths and fragments are ignored at the backend boundary.

Startup reports `flow_filter=exact-inbound` in the Npcap-ready marker so physical logs identify the corrected backend.

### `0d4301de1d1f6d3bcf9dfaf62cbe2e376c28dfb4` — `test: cover noisy Windows Npcap ingress`

Windows-tagged tests cover exact inbound WBD TCP acceptance and rejection of outbound self-capture, UDP, ICMP, unrelated TCP, wrong peers/ports, fragments and malformed lengths. VLAN-encapsulated exact WBD TCP is accepted after Ethernet decoding while VLAN UDP is rejected.

### `d2710aa51bfde939634614db6264fd5f750ae911` — `windows: expose process-aware bootstrap readiness`

`internal/windowsruntime` gained a generic process marker wait. Readiness is now a property of the child process and its output, not a side-effect file.

### `223b4aa5b04defa0540c50efa41fce6d9dc549e7` — `windows: fail fast on single-flow bootstrap exit`

The product controller now starts the sole public FakeTCP process and waits for `WBD_SINGLE_FLOW_BOOTSTRAP_READY` **before** reading the ticket file. `wbd-faketcp` writes the ticket before this marker. If the child exits, the real process failure returns immediately instead of being hidden behind a 15-second ticket-file timeout.

The subsequent order remains one-flow safe:

`FakeTCP same-flow bootstrap READY -> read ticket -> FakeTCP steady READY -> DTLS READY -> LINK READY -> TUN READY -> IPv6/routes`.

### `e472ce2248d805104dc0d0ba8b9bd045eaca8f5b` and `b0921ee943890a4680dcfa71282c1c0f96fd959c` — readiness regression tests

Controller/executor tests now distinguish the in-flow Reality bootstrap marker from the later FakeTCP steady-state READY marker and require bootstrap failure to occur before ticket polling or higher-layer startup.

### `00c19a756946d0d69457676a5fb3f54ef419b4af` — `windows: make diagnostics readiness process-aware`

The self-test runner had a second readiness implementation that only rescanned JSONL and could not see an exited child. It now uses a real Windows process handle plus `WaitForSingleObject` / `GetExitCodeProcess`, so an early child exit is reported immediately.

### `33d0fa67cfd138b7aa2f977112b835dff5e79a93` — `test: cover diagnostics child early exit`

A Windows test starts `cmd.exe /c exit 23` and requires diagnostics readiness to return the child exit promptly rather than wait for its marker timeout.

### `7dae5adf43cfd0ee88349a715b41ca3208bf94fc` — `windows: reap diagnostics child on readiness exit`

When diagnostics observes an already-exited child it now calls `Wait`, flushes output, and clears the command handle before returning the first failure. Later cleanup is therefore idempotent and does not add a misleading `TerminateProcess: Access is denied` on top of the real startup error.

### `2ce5825dc45223738ed42da1f87e57ba1cd3db91` and `f603da0f0785327eace9ce2b4a89a925d6b07565` — browser-like per-Connect source ports

The old Windows product path always used raw TCP source port `41001`. That is less Reality-like than a normal browser connection and increases rapid-reconnect 4-tuple reuse.

Product `Controller.Connect` now assigns one source port in the normal Windows dynamic TCP range `49152..65535`. A random per-process starting offset plus an atomic sequence avoids immediate reuse for a full 16384-port cycle. The selected port is then frozen for that single public flow from SYN through Reality-like bootstrap and DTLS/LINK data.

This changes connection metadata only; it does not change FakeTCP recovery, ACK/SACK, FEC, DTLS or LINK semantics.

### `136d8ff635f78107400b780d6936ed5566e95a3c` — source-port tests

Tests require dynamic-range allocation, no duplicate among 512 consecutive allocations, use of the assigned port in the FakeTCP command, and rejection of an explicit non-dynamic product source port.

### `8f00bc74e65cd63b931ac50d0f42b0de9babcd0f` — `ci: gate Windows runtime and diagnostics readiness`

`windows-faketcp-persona` now runs all of the following on `windows-latest`:

1. Windows FakeTCP packet-persona tests;
2. `cmd/wbd-faketcp` Windows backend tests;
3. `internal/windowsruntime` tests;
4. `internal/windowsdiag` tests;
5. Windows FakeTCP executable build.

The workflow triggers on changes to the Windows backend, runtime and diagnostics paths.

## New full-stack startup stress gate

### `b4673c86c43601097ad602f4372a0fc0c353d787` — `ci: stress repeated single-flow startup through NAT`

A new `single-flow-startup-stress` workflow builds the pinned wolfSSL DTLS 1.3 stack and creates:

`client namespace -> real iptables MASQUERADE NAT router namespace -> persistent server namespace`.

Persistent server components are:

- FakeTCP single-flow mux;
- Reality-like TLS bootstrap with temporary certificate;
- DTLS worker factory;
- LINK server mux with one-time ticket consumption;
- UDP echo service.

The client performs **20 complete rapid sessions** without restarting the server stack. Every round requires:

1. one FakeTCP public SYN lineage with a fresh dynamic-range source port;
2. same-flow Reality-like TLS/auth bootstrap READY;
3. a 64-hex one-time ticket;
4. wolfSSL DTLS 1.3 client/server readiness;
5. LINK client/server session readiness using that ticket;
6. three UDP application echoes through the whole stack;
7. hard termination of the actual namespace client children;
8. immediate next connection through the same NAT and persistent server.

The final gate requires exactly 20 server bootstrap completions, 20 DTLS accepts and 20 LINK session-ready events, while the persistent mux/link server remain alive. This is designed specifically to catch the physical pattern "one startup works, subsequent startups become unstable" and stale session/resource problems.

The stress harness itself is considered untrusted until its first real Actions execution succeeds; a script/PID failure must be diagnosed as harness vs product before interpreting the result.

## Same-4-tuple reconnect note

The server mux is keyed by the public 4-tuple. A dirty-disconnected established association would not currently reinterpret a new SYN on the **identical** old tuple as a new incarnation. That is a real theoretical lifecycle gap if a NAT forcibly reuses an exact tuple.

It has deliberately **not** been patched yet because:

- the new product client now uses a fresh dynamic source port per Connect;
- changing server incarnation semantics without evidence would touch a stable association lifecycle unnecessarily;
- the TCP-like recovery/FEC core is frozen.

If the new NAT stress or later physical evidence shows exact-tuple reuse despite source-port rotation, the narrow server-side fix is: same old client ISN SYN = retransmission; same tuple plus a new client ISN = retire the old session and create a new association. That change belongs to mux session lifecycle, not ARQ/FEC.

## Exact-head qualification policy after this failure

No replacement physical package is released merely because one namespace E2E succeeds. The exact source head must satisfy at minimum:

1. `windows-faketcp-persona` with backend + runtime + diagnostics tests;
2. main `ci`;
3. `single-flow-startup-stress` 20/20;
4. `single-flow-e2e`;
5. `single-flow-tcp-persona`;
6. `single-flow-no-hol`;
7. `single-flow-two-client`;
8. `faketcp-native`, first-arrival and loss gates;
9. `windows-portable-bundle`;
10. `linux-server-release`;
11. `mux-load-100m`, preserving the 40 Mbit/s release operating point.

Only after the exact head passes the matrix should Windows x64 and Linux ARM64 artifacts be downloaded, checksummed and handed to the physical tester.

## Current checkpoint

At the time this section was written, the feature branch exact source head was:

`7dae5adf43cfd0ee88349a715b41ca3208bf94fc`

The new Windows and startup-stress workflows had been successfully scheduled but were still queued behind older duplicate matrices. Therefore this head is **not yet qualified and no package should be emitted from it yet**.

## Next atomic actions

1. Keep `7dae5adf...` frozen while Actions drain.
2. Inspect `windows-faketcp-persona` first; fix the first deterministic Windows compile/test failure if any.
3. Inspect `single-flow-startup-stress`; distinguish harness failure from product failure before changing code.
4. If both gates pass, verify the rest of the exact-head qualification matrix including `mux-load-100m`.
5. Download and independently checksum Windows x64 + Linux ARM64 artifacts only after the exact head is fully green.
6. Update canonical `.wbd/handoff/current.json` with this physical failure, all fix commits, exact run IDs/artifact hashes, and the next action; require `handoff-verify` success before ending the development turn.
