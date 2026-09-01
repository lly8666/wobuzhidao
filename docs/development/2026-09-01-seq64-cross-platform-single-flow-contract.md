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

### `45364c3139b1f7f672c0e603a3308b2c8bfa4908`

Added an experiment-only dispatcher for the canonical Windows portable and Linux server release workflows. This commit does not change product source, wire semantics, or data-plane behavior compared with `ff26100c4d6bf86bc496e46511ba5bf4c2ff259a`; it only dispatches the repository's canonical release workflows on the experiment ref.

## Final qualification snapshot

Qualification/head used for the releasable artifact pair: `45364c3139b1f7f672c0e603a3308b2c8bfa4908`.

Protocol and platform gates on this exact HEAD:

- `single-flow-cross-platform-contract` run `33503717470`: Windows Server 2025 success and Ubuntu 24.04 success. Each OS ran the single-flow lineage contract 100 times, temporary-bootstrap/no-HOL tests 25 times, and Firefox-120 Reality-like TLS admission/persona tests 20 times.
- `single-flow-e2e` run `33503725059`: success. Real Linux raw socket/network namespace/pinned wolfSSL test proved one SYN lineage carries TLS bootstrap then DTLS data; wrong Reality marker stayed on the same flow and reached the TLS decoy.
- Windows portable release run `33503726887`: success. Windows child runtime, static-CRT pinned wolfSSL DTLS shim, locked official Wintun, portable manifest/embed, PE dependency verification, and artifact upload all passed.
- Linux server release run `33503729024`: settings, amd64, and arm64 jobs all success.

The product-code-equivalent preceding HEAD `ff26100c4d6bf86bc496e46511ba5bf4c2ff259a` additionally had the main `ci`, `exp-single-flow-windows`, `exp-single-flow-utls`, `faketcp-native`, `faketcp-pcap-20loss`, `faketcp-first-arrival`, and `fullstack-first-arrival` gates green. The final `45364c...` push also reran the protocol contracts because the only delta was CI dispatch plumbing.

### Release artifact verification

Windows portable Actions artifact:

- workflow run: `33503726887`
- artifact id: `9798808451`
- Actions ZIP SHA-256: `0a5556d117df8bb1ea0212499d68657e779d9d624cf922c363e5bf605592bab3`
- extracted `wbd.exe` SHA-256: `67829bc5edbe22c5d8e8b26047b9ea3047db316745f8159348a9a37f144d16ef`
- extracted executable verified as PE x86-64.

Linux ARM64 Actions artifact:

- workflow run: `33503729024`
- artifact id: `9798791484`
- Actions ZIP SHA-256: `3812de83ec7dd61e71448779342875aca6e8e3503b928bdadfd7e067da813593`
- extracted `wbd-linux-server-arm64.tar.gz` SHA-256: `9fb150ecfd47314dced71fcdd2d3a3998e9b5531df4c15a719a92f50d9caa5ab`
- artifact-provided `.sha256` matches the extracted tarball.
- representative native binaries (`wbd_dtls_shim`, `wbd-link-server-mux`, `wbd-faketcp-mux`, `wbd-platform-proxy-server`) all have ELF `e_machine=183` (AArch64).

Linux AMD64 was also built successfully by the same release run; the user's physical server target remains ARM64.

## Workflow anomaly kept separate from product qualification

Experimental pushes continue to create a `.github/workflows/linux-server-firewall.yml` run that fails before scheduling any job (`0 jobs`). No firewall test step executes in that record. This must not be misreported as a successful product gate, but it is also not evidence of a runtime firewall regression. Keep it as a workflow scheduling/parsing follow-up rather than changing the frozen data plane to chase a zero-job record.

## Physical qualification boundary

GitHub-hosted Windows runners do not provide the user's Npcap installation, physical NIC, home NAT, or ISP path. Therefore a later physical Windows/Ubuntu test remains the final hardware/network qualification. It must be described accurately as physical qualification, not as something CI performed.

## Sequence 64 completion decision

Software-side single-flow qualification is complete for the artifact pair above. The next physical test should use the matching Windows and Linux ARM64 products from `45364c...` and verify the expected log chain:

`FakeTCP single-flow handshake -> Firefox-120 Reality-like bootstrap on same flow -> ticket ready -> mode barrier without reconnect -> DTLS 1.3 -> LINK ready -> TUN/routes/probes`.

If the physical path fails, analyze the first missing marker against this exact tested artifact pair; do not re-open the architectural decision or modify the mature TCP-like steady-state data plane without new deterministic evidence.
