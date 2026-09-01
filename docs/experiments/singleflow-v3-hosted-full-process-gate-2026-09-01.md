# Single-flow V3 hosted Windows full-process qualification — 2026-09-01

## Purpose

The previous hosted Windows Npcap ABI gate deliberately stopped after a real `wbd-faketcp.exe` had accepted the synthetic WBD SYNACK and emitted a real TLS ClientHello through the dynamically loaded `wpcap.dll` ABI. That was useful, but it left one automatable gap: the real Windows product process had not itself been exercised through Reality-like TLS authentication, the encrypted same-flow phase switch, and the post-switch datagram phase.

This document records the new hosted full-process gate that closes that gap **without changing the production TCP-like sender/receiver/recovery/FEC data plane**.

The gate is still not a substitute for a real Administrator Windows 11 x64 machine running Npcap 1.88 and a physical NIC. Every PASS marker therefore keeps `physical_driver=0` explicit.

## Test architecture

The Windows job still executes the real repository-built `wbd-faketcp.exe`.

A separate test-only `wpcap.dll` bridge implements the same dynamic symbols used by the production Windows backend:

- `pcap_open_live`
- `pcap_datalink`
- `pcap_setmintocopy`
- `pcap_setmode`
- `pcap_sendpacket`
- `pcap_next_ex`
- `pcap_geterr`
- `pcap_close`

Unlike the earlier one-shot ABI stub, the bridge does not implement the Reality-like protocol in C. It only converts the Ethernet boundary used by the real Windows executable into a localhost UDP transport carrying the original raw IPv4/TCP-shaped packet.

A test-only Go server helper then reuses repository production protocol packages:

- `internal/faketcp.ServerAssociation` for the raw TCP-like sequence/ACK state;
- `internal/realityfront.HandleServerConnSimpleSingleFlow` for real TLS 1.3 Reality-like marker/authentication and ticket creation;
- `internal/singleflow` for the encrypted switch request/ack contract;
- the same FakeTCP sender/retransmission APIs for server-to-client raw payload delivery.

Therefore the C stub is only a virtual Npcap/adapter boundary. It is not a second copy of TLS, authentication, ticket, switch, or TCP-like protocol logic.

## Full hosted path

The qualification now requires this real Windows process path:

```text
real wbd-faketcp.exe
  -> dynamically loaded test wpcap.dll
  -> raw SYN / WBD SYNACK / final ACK
  -> same raw FakeTCP sequence space
  -> real TLS 1.3 Reality-like ClientHello/SNI/handshake
  -> username/password bootstrap and 64-hex ticket
  -> encrypted SWITCH_REQ
  <- encrypted SWITCH_ACK
  -> WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only
  -> local UDP post-switch payload
  -> raw FakeTCP payload through the bridge
  -> Go server helper
  <- raw FakeTCP echo through the same flow
  <- exact local UDP echo
```

Required final hosted marker:

```text
WBD_WINDOWS_NPCAP_FULL_SINGLEFLOW_PASS real_exe=1 dynamic_wpcap=1 tls_auth=1 encrypted_switch=1 datagram_echo=1 same_flow=1 physical_driver=0
```

The cross-platform workflow additionally emits:

```text
WBD_SINGLEFLOW_V3_WINDOWS_PASS os=windows-latest npcap_demux_tests=1 npcap_abi_process_gate=1 npcap_full_process_gate=1 runtime_tests=1 physical_gate_scripts_parsed=1 physical_driver=0
```

## Files added/changed

### `tests/windows_npcap_abi/full_server/main.go`

Test-only Go bridge server. It owns no production listener and is never packaged. It performs the server side of the raw WBD association, real TLS/auth, encrypted phase switch, and post-switch echo using repository protocol packages.

Initial commit:

- `4a6cba9af5bcbda5596b871545a2b1d2b837e58b` — `test: add hosted single-flow protocol bridge server`

### `tests/windows_npcap_abi/wpcap_bridge_stub.c`

Test-only Windows DLL that turns the real product executable's Npcap Ethernet send/receive boundary into localhost UDP raw IPv4 exchange with the helper.

Commit:

- `d172c3cd523f91291974bf9d03f4e0d2ca3d7595` — `test: add hosted Npcap protocol bridge stub`

### `scripts/windows_npcap_full_singleflow_qualify.ps1`

Builds the bridge and helper on `windows-latest`, launches the **real** CI-built `wbd-faketcp.exe`, waits for the real same-flow switch/datagram markers, checks the ticket, and performs a post-switch UDP echo.

Initial commit:

- `ac60f4784d30276d98366d41d2cb9af723579fc6` — `test: add hosted Windows full single-flow qualifier`

### `.github/workflows/singleflow-v3-crossplatform.yml`

The Windows job now runs both the older adapter-noise/ABI gate and the new full process gate.

Commit:

- `c59c58c591a1671f8ba59c1ed6c71ab2df142364` — `test: run full Windows single-flow process gate`

## Qualification attempts and what failed

### Attempt 1 — helper syntax failure

Head: `c59c58c591a1671f8ba59c1ed6c71ab2df142364`

Main CI run `33495335510` failed while compiling the new test helper. The first deterministic error was a missing closing brace in the helper's carrier reader. No production package or V3 protocol assertion had failed first.

Fix:

- `f147007db4dcd47bedd66e8ba4cab8772fb8037e` — `test: fix hosted single-flow bridge helper syntax`

### Attempt 2 — PowerShell log-poll startup race

Head: `f147007db4dcd47bedd66e8ba4cab8772fb8037e`

Main CI recovered and passed. In the Windows cross-platform job the **existing ABI gate passed first**, proving the real Windows executable still loaded the stub, ignored adapter noise, accepted the WBD SYNACK and emitted TLS payload.

The new full-process qualifier then failed before protocol evaluation because redirected `server.stdout.log` existed but was temporarily empty. PowerShell `Get-Content -Raw` returned `$null`, and the polling loop called `.Contains()` on it.

This was a harness lifecycle race, not a FakeTCP/TLS/switch failure.

Fix:

- `1d2d079e111604710360e5b09d2482c60c874a74` — `test: make hosted single-flow log polling race-safe`
- added a `ReadText()` helper that always returns a string for missing/empty logs.

## Final hosted result

Behavior/test head:

```text
1d2d079e111604710360e5b09d2482c60c874a74
```

Cross-platform run:

```text
33495868298
```

Results:

- Linux job `99817804719`: `success`.
- Windows job `99817804900`: `success`.
- old real-EXE Npcap ABI gate: PASS.
- new real-EXE full single-flow gate: PASS.

The Windows job log contains both final markers:

```text
WBD_WINDOWS_NPCAP_ABI_PASS real_exe=1 dynamic_wpcap=1 open_live=1 setmode=1 next_ex=1 sendpacket=1 adapter_noise=ignored synack=accepted tls_payload_tx=1 physical_driver=0
WBD_WINDOWS_NPCAP_FULL_SINGLEFLOW_PASS real_exe=1 dynamic_wpcap=1 tls_auth=1 encrypted_switch=1 datagram_echo=1 same_flow=1 physical_driver=0
WBD_SINGLEFLOW_V3_WINDOWS_PASS os=windows-latest npcap_demux_tests=1 npcap_abi_process_gate=1 npcap_full_process_gate=1 runtime_tests=1 physical_gate_scripts_parsed=1 physical_driver=0
```

## Same-head regression status

The same `1d2d079e...` head also completed the following important gates successfully:

- `ci` run `33495868295`: success.
- `singleflow-realitylike-e2e` run `33495868499`: success. This remains the strong Linux/NAT/public-pcap proof of one SYN/one tuple, real TLS, encrypted switch, pinned wolfSSL DTLS, and steady-state no-HOL.
- `faketcp-native` run `33495868238`: success.
- `faketcp-reconnect-stress` run `33495868270`: success.
- `faketcp-pcap-20loss` run `33495868289`: success.
- `faketcp-first-arrival` run `33495868353`: success.

This matters because the new Windows qualification work did not require a change to the frozen production TCP-like sender/receiver/recovery/FEC implementation.

## What this proves

It is now automatically demonstrated on a real hosted Windows OS process boundary that:

1. the production `wbd-faketcp.exe` can use its dynamic Npcap ABI;
2. unrelated adapter traffic does not kill the handshake parser;
3. the same raw public flow carries a real TLS 1.3 Reality-like bootstrap;
4. authentication produces a ticket;
5. the phase switch is acknowledged over encrypted TLS application data;
6. the process enters `public_flow=reused hol=bootstrap-only` datagram mode;
7. post-switch payload can make a complete Windows-product-process round trip over the same FakeTCP association.

## What this does NOT prove

The bridge is intentionally synthetic at the driver/NIC boundary. It does **not** prove:

- Npcap 1.88 NPF kernel driver capture/injection on a real Windows 11 machine;
- real NIC offload/filtering behavior;
- real LAN gateway/NAT/ISP traversal;
- administrator/firewall interactions of a physical machine.

Those remain the exact scope of `.github/workflows/windows-npcap-physical.yml`, which requires a self-hosted `wbd-npcap` Windows runner and must report `physical_driver=1` before final physical qualification is claimed.

## Next development step

Do not change the frozen TCP-like data plane based on this gate. The next useful work is qualification/lifecycle closure:

1. keep the hosted full-process gate mandatory on V3 pushes;
2. ensure the exact final substantive V3 head has all Linux/no-HOL/regression gates green;
3. run the self-hosted Windows 11 x64 + Npcap 1.88 physical workflow when a suitable runner is available;
4. only after that physical PASS, build/identify the final Windows portable and Linux ARM64 artifacts for user delivery;
5. update `.wbd/handoff/current.json` with exact live run IDs and the fact that hosted full-process is PASS while physical Npcap remains NOT_RUN until evidence exists.
