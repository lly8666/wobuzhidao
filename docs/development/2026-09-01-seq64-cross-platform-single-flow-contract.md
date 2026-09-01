# Sequence 64 — cross-platform single-flow lineage qualification

Date: 2026-09-01
Experiment branch: `exp/single-flow-utls-firefox120`
Starting HEAD: `01eb331bc8c8ac161cd019a265d4bb325238e4fc`

## User acceptance rule

Do not deliver another Windows/Linux product pair merely because Linux namespace E2E is green or Windows binaries compile. The software side must first demonstrate the same single-public-flow contract on both Windows and Linux automation. A later physical Windows/Npcap + real NIC/NAT test is allowed to remain a final hardware qualification step, but it must not be used to discover ordinary state-machine bugs that deterministic CI can cover.

## Architecture frozen for this sequence

The mature TCP-like weak-network data plane is intentionally unchanged.

The public transport contract is:

1. FakeTCP owns the public TCP-shaped incarnation from the first SYN.
2. There is exactly one client SYN lineage and one public 4-tuple for a WBD session.
3. The first seconds use the temporary reliable `BootstrapStream` to carry the Firefox-120 Reality-like TLS 1.3 admission exchange.
4. The bootstrap adapter is discarded at a mode barrier without FIN, RST, redial, sequence reset, or a second SYN.
5. The same FakeTCP Sender/Receiver sequence space immediately carries pinned wolfSSL DTLS 1.3 and steady-state payload.
6. Steady state retains the existing earliest-complete-datagram/no-HOL semantics and existing FEC/LINK behavior.

## Evidence already green before this sequence

At starting HEAD `01eb331b...`:

- Main Go CI completed successfully.
- Linux `single-flow-e2e` completed successfully. Its public pcap qualification asserts one unique client SYN sequence, no second client 4-tuple to port 443, same-flow TLS bootstrap, pinned wolfSSL DTLS 1.3, and 20 bidirectional echo packets.
- Windows Server 2025 `windows_single_flow` completed successfully. It runs Windows FakeTCP adapter tests, repeats the physical-adapter-noise handshake regression 100 times, runs Windows runtime/single-flow tests, and builds the Windows FakeTCP/portable/GUI binaries.
- The Windows handshake regression ignores non-IPv4/non-TCP and unrelated exact-tuple traffic before the protocol parser, addressing the physical log that previously failed with `faketcp: not ipv4/tcp`.

The remaining automation gap was that the Windows job did not run one named protocol-lineage test that spans FakeTCP handshake, reliable bootstrap sends, the mode barrier, and post-bootstrap payload sequence continuity.

## Sequence 64 changes

### `33e971dc6e825d3f1356a9df1ebc9657aca8a104`

Added `cmd/wbd-faketcp/single_flow_lineage_test.go` with `TestSingleFlowLineageContract`.

The test deliberately uses the production common `endpoint` code rather than an independent model:

- `endpoint.handshakeClient()` emits the actual WBD SYN and final ACK.
- `endpoint.newBootstrapStream()` emits the actual TCP-shaped bootstrap payload through the production `Sender`.
- A deterministic raw peer only supplies the WBD SYNACK and cumulative ACKs.
- The bootstrap payload is larger than one `DefaultBootstrapChunk`, so multiple acknowledged bootstrap sends are exercised.
- The test clears/closes only the temporary bootstrap adapter, then sends a DTLS-looking payload through the same production Sender.
- Captured client packets are parsed with the production FakeTCP parser.

Hard assertions:

- exactly one SYN for the incarnation;
- every packet remains on the exact same IPv4 four-tuple;
- final ACK uses the expected SYN/SYNACK sequence lineage;
- every bootstrap payload segment is contiguous in the same sequence space;
- the first post-bootstrap payload sequence equals `client_isn + 1 + bootstrap_bytes`;
- bootstrap bytes followed by post-bootstrap bytes reconstruct exactly, with no reset/reconnect at the mode barrier;
- the post-bootstrap cumulative ACK is accepted by the same Sender.

### `d8e5b6eae8ae95ab895b5ba238e34a83152f41ee`

Added `.github/workflows/single-flow-cross-platform-contract.yml`.

The workflow runs the same contract on both:

- `ubuntu-24.04`
- `windows-2025`

Each OS runs:

1. `TestSingleFlowLineageContract` 100 times.
2. Bootstrap reliability + post-bootstrap no-HOL receiver tests 25 times.
3. Actual Firefox-120 Reality-like TLS admission/persona tests 20 times.

This matrix complements rather than replaces the Linux raw-network/pcap E2E. Linux E2E proves the real namespace/raw socket/inherited wolfSSL path; the cross-platform matrix proves that Windows and Linux compile and execute the identical common lineage/mode-barrier state machine.

## Qualification still required before product delivery

For the final tested HEAD of this sequence:

- `single-flow-cross-platform-contract`: both OS matrix jobs success.
- `single-flow-e2e`: success.
- `exp-single-flow-windows`: success.
- main `ci`: success.
- Windows portable bundle: success.
- Linux server release including ARM64: success.
- Existing FakeTCP recovery/first-arrival gates must show no deterministic regression.
- Update the handoff with the exact tested HEAD and workflow run IDs and make `handoff-verify` green.

If any gate fails, fix the first deterministic failure and append the result here before producing user-facing artifacts.

## Physical qualification boundary

GitHub-hosted Windows runners do not provide the user's Npcap installation, physical NIC, home NAT, or ISP path. Therefore a later physical Windows/Ubuntu test remains the final hardware/network qualification. It must be described accurately as physical qualification, not as something CI performed.
