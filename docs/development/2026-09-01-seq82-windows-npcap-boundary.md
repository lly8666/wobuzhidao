# Sequence 82 — Windows Npcap boundary review and permanent handshake regression

Date: 2026-09-01
Branch: `feat/single-flow-reality-faketcp`
PR: #9
Starting live head: `b8e84ad76daecbb7f281e4fefc46dec8c2e542b4`

## Scope

This checkpoint follows canonical handoff sequence 81. The mature TCP-like DATA/ACK/SACK/recovery/FEC core remains frozen. The only area under review is the Windows/Npcap capture boundary before and during the single-flow SYN/SYNACK bootstrap.

The product invariant remains:

- one public TCP-shaped four-tuple and one client SYN lineage;
- FakeTCP owns the public flow from the first SYN;
- Reality-like TLS 1.3 setup/admission is carried in the bounded reliable bootstrap phase of that same association;
- after SFOK the same association switches to DTLS 1.3 -> LINK/FEC/VPN datagrams;
- no second public connection and no ordinary-kernel-TCP steady-state HOL.

## Live refresh findings

Canonical `dev/wbd-raw-fec-v2` is at handoff sequence 81 and its `handoff-verify` run is green.

The active PR #9 branch advanced beyond the SHA stored in sequence 81. The live head at the start of this checkpoint was `b8e84ad7...` (`ci: kick seq80 exact-head product qualification`). Its exact-head Actions set contained 30 completed workflows with no failure and no in-progress run.

The separate `exp/single-flow-utls-firefox120` line is not a simple successor of PR #9; it diverged from an older merge base. It remains useful as qualification/research evidence but is not the active implementation baseline.

## Physical failure chronology correction

The real Windows failure containing:

`wbd-faketcp handshake: faketcp: not ipv4/tcp`

was recorded at 2026-08-29 13:08Z.

The production ingress fix `cc70119819d77d18cfa7441d37610f529f6e3b32` (`windows: filter Npcap ingress to exact FakeTCP flow`) was committed at 2026-08-29 13:17Z, about nine minutes later. Therefore that physical log exercised the pre-fix Windows backend and is not evidence that current PR #9 still has the same bug.

## Current production Npcap semantics reviewed

`cmd/wbd-faketcp/main_windows.go` at the live PR head was reviewed directly.

`ReadPacket` currently has the correct `pcap_next_ex` status handling:

- `1`: inspect one captured packet;
- `0`: return the WBD raw read-timeout sentinel;
- `-1`: return an explicit `pcap_next_ex` error using `pcap_geterr`;
- `-2`: return EOF/break-loop;
- every other status: explicit unexpected-status error.

The captured-frame path also filters before the generic FakeTCP parser:

1. Ethernet/VLAN extraction must produce IPv4;
2. exact local-kernel RST is observed only for diagnostics;
3. only unfragmented IPv4/TCP matching the exact inbound `server_ip:server_port -> local_ip:local_port` WBD tuple is delivered;
4. UDP/ICMP, unrelated TCP, self-captured outbound WBD frames, wrong ports/peers, malformed lengths and fragments are ignored.

This means the historical `not ipv4/tcp` startup failure is already corrected in production code without changing the FakeTCP transport state machine.

## Remaining test gap and change in this checkpoint

Existing PR #9 Windows tests already cover Ethernet/VLAN/QinQ decoding, exact tuple filtering, a 4096-mutation corpus, RST recognition, payload direction accounting, fuzzing, and a realistic ordered ingress-noise classifier test.

However, the active branch did not retain the qualification experiment's higher-level regression that runs the actual shared `handshakeClient()` state machine while the simulated physical adapter presents noise before the valid SYNACK.

Commit `357f94af90d19398a34cec3cbfb3a20da9859873` adds `cmd/wbd-faketcp/main_windows_handshake_test.go` to PR #9. The test presents, in order:

1. same-address IPv4/UDP noise;
2. inbound TCP from the wrong server port;
3. Npcap self-capture of the client's own WBD SYN;
4. the real WBD SYNACK.

It requires the actual FakeTCP client handshake to succeed, initialize the steady sender/receiver, and emit exactly two raw writes: one WBD SYN and one final ACK on the same four-tuple and sequence lineage.

This is deliberately a Windows capture-boundary regression. No DATA/FEC/recovery code was modified.

## Qualification policy for this checkpoint

The new test commit is not a deliverable by itself. The updated PR head must again satisfy the Windows + Linux/single-flow qualification matrix. At minimum inspect:

- main `ci`;
- native Windows FakeTCP/persona/backend tests;
- Windows portable bundle and TUN build/admin smoke;
- single-flow E2E, mutational, shape, SYN-wire, loss, data-plane and startup stress gates;
- Linux server release and shared-port gates;
- FakeTCP native/first-arrival/pcap-loss;
- `mux-load-100m` including RTT100.

If the new handshake regression fails to compile on the active implementation, fix the test or the first deterministic capture-boundary defect only. Do not weaken the qualified Reality-like shape and do not modify post-SFOK TCP-like byte behavior without new failing evidence.

## Next atomic action

Observe the exact Actions set produced by the new PR head after `357f94af...`. If all relevant gates remain green, record the tested source head and update canonical handoff to sequence 82. If any gate fails, inspect the first deterministic job log and keep the fix constrained to the Windows capture/readiness boundary unless evidence proves otherwise.
