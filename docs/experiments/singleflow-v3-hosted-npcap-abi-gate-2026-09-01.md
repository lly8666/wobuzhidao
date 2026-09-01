# Single-flow V3 hosted Windows Npcap ABI gate — 2026-09-01

## Context

The V3 release contract remains unchanged: one public TCP-shaped 4-tuple and one SYN; a bounded Reality-like TLS 1.3 bootstrap on the same raw FakeTCP sequence space; an encrypted in-flow switch; then the existing no-HOL FakeTCP datagram transport carrying pinned wolfSSL DTLS 1.3, LINK and optional FEC. The existing sender/receiver/recovery/FEC algorithms remain frozen.

A real Administrator Windows 11 x64 machine with Npcap 1.88 is still required before release delivery. Hosted CI must never claim that a synthetic DLL is equivalent to the physical NPF driver, NIC, capture/injection path, NAT or ISP.

The reason for adding this gate is narrower: an earlier real Windows run failed during initial FakeTCP handshake with `faketcp: not ipv4/tcp`. The defect was adapter-wide Npcap noise escaping into the strict FakeTCP parser. Unit tests now cover exact four-tuple demultiplexing, but they did not exercise the real executable's dynamic `wpcap.dll` ABI boundary.

## Goal

Add a hosted-Windows process-level qualification between unit tests and the physical Npcap release gate. It must execute the real `wbd-faketcp.exe` and exercise:

- `syscall.LoadDLL("wpcap.dll")`;
- `FindProc` for the Npcap exports used by WBD;
- `pcap_open_live`;
- Ethernet datalink validation;
- `pcap_setmintocopy` when available;
- mandatory `pcap_setmode(..., MODE_SENDTORX_CLEAR=0x0200)`;
- repeated `pcap_next_ex` capture reads;
- adapter-noise demultiplexing before the strict FakeTCP parser;
- `pcap_sendpacket` for the actual WBD SYN/ACK and bootstrap data path;
- generation of a real Go `crypto/tls` ClientHello payload by the single-flow client after FakeTCP handshake.

It is intentionally not required to emulate the TLS server, DTLS, LINK or TUN. Those are already covered by the strong Linux/NAT single-flow E2E, while the real Npcap driver/NIC boundary remains covered only by the physical gate.

## Implementation

### `tests/windows_npcap_abi/wpcap_stub.c`

A minimal x64 `wpcap.dll` ABI implementation compiled on `windows-latest` with the installed Visual C++ toolchain.

On the first real WBD SYN sent by `wbd-faketcp.exe`, the stub queues capture frames in this order:

1. unrelated IPv4/UDP adapter noise;
2. inbound IPv4/TCP with the wrong server port;
3. a self-captured copy of the client's own outbound WBD SYN;
4. a valid WBD SYN/ACK using the existing wire profile: MSS 1360, SACK permitted, window scale 8, and ACK=`client_isn+1`.

The Go executable must discard the first three frames at the Npcap boundary, accept the fourth, send the final ACK, initialize the V3 single-flow client, and then send its actual TLS ClientHello bytes as TCP-shaped payload through `pcap_sendpacket`.

The stub writes deterministic markers to a path named by `WBD_NPCAP_STUB_MARKER`. It does not contain WBD TLS/authentication logic and therefore cannot accidentally duplicate the V3 protocol implementation under test.

### `scripts/windows_npcap_abi_qualify.ps1`

The hosted qualifier:

- refuses to run if a real `%SystemRoot%\System32\Npcap\wpcap.dll` exists, so the test cannot silently use a machine-installed driver;
- locates the Visual C++ x64 toolchain with `vswhere` and builds the stub DLL;
- copies the real CI-built `wbd-faketcp.exe` next to that DLL;
- starts the executable with a V3 single-flow configuration and a short bootstrap deadline;
- waits only until the stub observes the first real TLS bootstrap payload, then terminates the client because no synthetic TLS server is intentionally implemented;
- requires evidence for DLL open, `MODE_SENDTORX_CLEAR`, WBD SYN, queued noise, accepted SYN/ACK and TLS payload transmission;
- requires `WBD_FAKETCP_WINDOWS_NPCAP_MODE_READY sendtorx=cleared` from the real executable;
- rejects any `faketcp: not ipv4/tcp` escape.

Successful qualification emits:

`WBD_WINDOWS_NPCAP_ABI_PASS real_exe=1 dynamic_wpcap=1 open_live=1 setmode=1 next_ex=1 sendpacket=1 adapter_noise=ignored synack=accepted tls_payload_tx=1 physical_driver=0`

The explicit `physical_driver=0` field is mandatory: this gate must never be confused with the separate physical release qualification.

### `.github/workflows/singleflow-v3-crossplatform.yml`

The Windows hosted job now runs the ABI qualifier after Go tests and after building the real FakeTCP/portable executables. The job's final marker now includes `npcap_abi_process_gate=1` only after the process-level gate succeeds.

## Relationship to the release gate

This gate strengthens automated coverage but does not relax the release policy.

A Windows/Linux pair remains non-deliverable until all of the following are true for the same behavior-equivalent checkpoint:

1. strong Linux/NAT single-flow Reality-like E2E is green, including one SYN/one tuple/encrypted switch/DTLS/no-HOL assertions;
2. hosted Windows V3 tests and this real-executable Npcap ABI gate are green;
3. the real Administrator Windows 11 x64 + Npcap 1.88 physical qualifier emits `WBD_WINDOWS_NPCAP_PHYSICAL_PASS` with capture/injection, DTLS, LINK, TUN, Internet probes and cleanup all proven;
4. Linux ARM64 release is rebuilt/reconfirmed for the final behavior checkpoint;
5. artifact hashes/run identities and handoff verification are recorded.

## Current checkpoint

The ABI gate was wired into branch `exp/singleflow-realitylike-v3` at commit `8701671e3257fbf33102dad7a6400e24f9273246`. Actions were triggered immediately after that commit. At the time this note is created, those runs are not yet treated as passing until their completed conclusions and, on failure, the first deterministic job log are inspected.
