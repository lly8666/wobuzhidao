# 2026-08-30 Windows ingress + single-flow qualification log

## Scope

This file is a durable recovery record for the physical Windows startup failure that was reported after the single-public-flow architecture had already been implemented. It intentionally records conclusions and test evidence without copying credentials or one-time tickets.

The transport core remains frozen. No change in this checkpoint alters FakeTCP sequence/recovery/FEC/no-HOL behavior.

## Product invariant under test

A product session has one public TCP-shaped FakeTCP four-tuple and one SYN/sequence lineage from connect until disconnect.

1. FakeTCP owns the public flow from the first SYN.
2. The first bounded phase of that same association carries Reality-like TLS 1.3 setup/admission.
3. After the bootstrap mode barrier, the same raw association carries pinned wolfSSL DTLS 1.3, LINK and optional fixed FEC.
4. There is no second public Reality TCP connection and no second SYN before DTLS.
5. Sustained payload must preserve WBD datagram/no-HOL semantics rather than inherit kernel TCP stream HOL.

## Physical evidence that triggered this pass

The physical Windows log showed the current single-flow startup sequence beginning correctly:

- dependency/Npcap preflight passed;
- underlay discovery passed;
- only `faketcp` was started first;
- Npcap reported `MODE_SENDTORX_CLEAR` ready;
- before the FakeTCP client reached READY it exited with `wbd-faketcp handshake: faketcp: not ipv4/tcp`;
- the runtime subsequently timed out waiting for the same-flow Reality ticket and rolled back.

The matching Linux server package was already in product single-flow mode (`public_single_flow=<ipv4>:443`) and the server mux advertised `single_flow_bootstrap=true`.

Separate physical attempts with the same single-flow server had previously progressed substantially further: server-side single-flow TLS bootstrap ready, DTLS server `PEEK/ACCEPT_PASS`, DTLS READY and LINK mux session ready were all observed. Therefore the architecture was not an unconditional Linux DTLS failure; the newly deterministic Windows failure occurred before bootstrap because adapter noise reached the packet parser.

## Root cause of `not ipv4/tcp`

The Windows Npcap handle captures the physical adapter, not only the WBD flow. During startup it can observe ARP, IPv6, IPv4 UDP/ICMP, unrelated TCP and self-captured outbound frames.

The old Windows backend extracted IPv4 from Ethernet and could pass unrelated IPv4 payloads into the generic FakeTCP `recvOne()` parser. The generic parser correctly rejects a non-TCP IPv4 packet with `faketcp: not ipv4/tcp`, but in handshake context that error was fatal.

Current `cmd/wbd-faketcp/main_windows.go` instead filters before `recvOne()`:

- ignore non-IPv4 Ethernet/VLAN traffic;
- ignore IPv4 fragments;
- require TCP;
- require the exact peer -> local WBD IPv4 four-tuple;
- ignore outbound self-capture and unrelated inbound TCP;
- continue observing an exact local-kernel RST separately for diagnostics.

Only an exact inbound WBD TCP packet is copied to the generic FakeTCP parser. This is a Windows/Npcap boundary fix, not a transport algorithm change.

## Coverage before this checkpoint

`cmd/wbd-faketcp/main_windows_test.go` already covered:

- Ethernet, VLAN and QinQ IPv4 extraction;
- ARP/IPv6/LLDP rejection;
- exact inbound flow acceptance;
- outbound self-capture rejection;
- UDP/ICMP/wrong-port/wrong-peer rejection;
- IPv4 fragment and malformed-total-length rejection;
- IPv4 options;
- a 4096-case mutation corpus;
- exact local-kernel RST recognition;
- exact payload-direction accounting;
- fuzzing of the ingress classifiers.

The `windows-faketcp-persona` workflow runs these tests natively on `windows-latest`, plus Reality-like single-flow tests, the platform-neutral single-flow virtual-wire TLS admission/no-HOL test, Windows runtime/diagnostics tests and a native Windows FakeTCP build.

## New coverage added in this checkpoint

Commit `27093ed35ebfd4fa8772b798b2f17f015ffb6ab9` adds `main_windows_ingress_sequence_test.go`.

The new Windows-only test feeds a realistic startup sequence:

1. truncated Ethernet;
2. ARP;
3. IPv6;
4. IPv4/UDP;
5. VLAN IPv4/ICMP;
6. Npcap self-capture of WBD outbound TCP;
7. unrelated inbound TCP;
8. finally an exact peer -> WBD SYN|ACK.

It applies the same two-stage classification used by production `ReadPacket` (`ethernetIPv4Payload` then `matchesInboundFlow`) and requires exactly one frame to be accepted: the final exact inbound WBD TCP frame. This specifically protects against recurrence of the physical `not ipv4/tcp` failure shape.

## First-arrival red gate review

At substantive head `ecb01c4eb620a9caaeeec7b04e50c5abc88ab520`, PR workflow run `33298039692` initially reported only `latency (100, 0)` failed while 20/0, 20/10 and 100/10 passed.

The failed job exited during the readiness window before probe statistics were emitted. It did not demonstrate a first-datagram latency threshold regression.

The individual failed job was rerun without code changes. The replacement jobs all completed successfully, including 100 ms / 0% loss. Therefore this evidence does not justify changing the already-qualified TCP-like sender/receiver/recovery logic.

## Existing same-flow qualification on the immediately preceding release-hardening head

`docs/development/2026-08-30-single-flow-release-hardening.md` records a prior exact substantive head (`fd0f1efe...`) where all major qualification gates were green together: single-flow E2E, no-HOL, two-client, TCP persona, LINK fullstack, startup stress, Windows persona/portable/TUN/admin smoke, Linux release/shared-port, mux-load-100m, FakeTCP native/first-arrival/pcap-loss and fullstack first-arrival.

The current release-hardening commits after that point are source-identity, packaging/security, Windows ingress-test and documentation work; they do not intentionally change the frozen TCP-like recovery core.

## Current qualification rule

Do not deliver a new physical candidate merely because this test file compiles.

For one exact substantive head, require at minimum:

- `ci` success;
- `windows-faketcp-persona` success on Windows runner;
- `windows-portable-bundle` success;
- `windows-tun-build` and admin smoke success;
- `single-flow-e2e`, `single-flow-no-hol`, `single-flow-link-fullstack`, `single-flow-two-client`, `single-flow-startup-stress` success;
- `linux-server-release` ARM64 and amd64 success;
- `linux-shared-port` success;
- `mux-load-100m` success including RTT100;
- `faketcp-native`, `faketcp-first-arrival`, `faketcp-pcap-20loss` success.

Windows hosted CI cannot legally/faithfully replace the final physical Npcap + NIC + home/ISP path, but it must execute the Windows protocol/runtime/backend tests natively rather than only cross-compile. Linux raw-socket/netns integration must also pass before a candidate is handed to the operator.

## Current state at time of writing

The new ingress-sequence test was committed as `27093ed35ebfd4fa8772b798b2f17f015ffb6ab9` and triggered the full PR qualification matrix. Those new-head runs were still queued when this record was first written.

No package should be labelled ready until the new-head Windows and Linux gates above complete. The next action is to inspect the first deterministic failed gate if any; otherwise verify same-source Windows/Linux artifacts and update the project handoff before physical delivery.
