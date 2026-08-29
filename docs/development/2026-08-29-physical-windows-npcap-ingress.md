# Physical Windows Npcap startup failure after single-flow qualification

Date: 2026-08-29

## Context

The feature line is `feat/single-flow-reality-faketcp`, PR #9. The previously qualified source head was `48e9fd45790a4c85d012aadb7a2ea50d3ad95479`.

The architecture remains unchanged and is still the required product model:

- exactly one public TCP-shaped 4-tuple and one SYN lineage;
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

The `windows-faketcp-persona` workflow previously did two things on `windows-latest`:

- run persona tests in `internal/faketcp`;
- build `cmd/wbd-faketcp`.

It did **not** execute the Windows-tagged tests in `cmd/wbd-faketcp/main_windows_test.go`. Consequently the Windows backend itself could regress while the Windows gate remained green.

Linux namespace single-flow E2E also does not model a noisy physical Ethernet adapter. Its raw socket input is much cleaner than a real Npcap capture handle.

## Fix line

The fix is intentionally outside the mature TCP-like core.

### Commit `e3bf03b18d407608771e2ced1177257b217733b1`

`ci: run Windows FakeTCP backend tests`

`windows-faketcp-persona` now executes `go test ./cmd/wbd-faketcp -count=1 -v` on `windows-latest` before building the child runtime.

### Commit `cc70119819d77d18cfa7441d37610f529f6e3b32`

`windows: filter Npcap ingress to exact FakeTCP flow`

Npcap still observes exact local-kernel RSTs for diagnostics, but only an unfragmented inbound IPv4/TCP packet matching:

`remote_ip:remote_port -> source_ip:source_port`

is delivered to the FakeTCP handshake/data receive state machine. Outbound self-capture, UDP, ICMP, unrelated TCP, wrong peers/ports, malformed lengths and fragments are ignored at the backend boundary.

Startup also reports `flow_filter=exact-inbound` in the Npcap-ready marker so physical logs identify the corrected backend.

### Commit `0d4301de1d1f6d3bcf9dfaf62cbe2e376c28dfb4`

`test: cover noisy Windows Npcap ingress`

Windows-tagged tests now cover:

- exact inbound WBD TCP accepted;
- outbound self-capture rejected;
- IPv4 UDP rejected;
- IPv4 ICMP rejected;
- unrelated TCP port rejected;
- different peer rejected;
- fragmented input rejected;
- malformed total length rejected;
- VLAN-encapsulated exact WBD TCP accepted after Ethernet decoding;
- VLAN-encapsulated UDP rejected.

## Qualification policy after this failure

No new physical package should be handed to the user merely because the previous Linux namespace single-flow E2E is green.

Before a replacement package is released, the exact new source head must satisfy at minimum:

1. `windows-faketcp-persona`, including the new Windows backend tests;
2. main `ci`;
3. `single-flow-e2e`;
4. `single-flow-tcp-persona`;
5. `single-flow-no-hol`;
6. `single-flow-two-client`;
7. `faketcp-native` and first-arrival/loss gates;
8. `windows-portable-bundle`;
9. `linux-server-release`;
10. `mux-load-100m`, with the 40 Mbit/s release operating point preserved.

A startup/reconnect stress gate should also be added so bootstrap is repeated many times under unrelated background traffic rather than accepted after one clean success.

## Next atomic actions

1. Observe the Windows backend tests on exact source head `0d4301de...` and fix any deterministic compile/test failure first.
2. Add repeated single-flow startup/reconnect stress with background UDP/ICMP/unrelated TCP traffic while preserving one public FakeTCP flow per session.
3. Re-run the full exact-head qualification matrix.
4. Only after exact-head qualification passes, build/download/verify Windows x64 and Linux ARM64 artifacts from that head.
5. Update canonical `.wbd/handoff/current.json` with the physical finding, fix commits, run IDs/artifact hashes, and next action; require `handoff-verify` success before ending the development turn.
