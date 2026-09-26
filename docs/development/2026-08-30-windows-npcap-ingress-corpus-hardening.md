# Windows Npcap ingress corpus hardening follow-up

Date: 2026-08-30

## Recovery context

This record continues sequence 64 after the physical Windows 11/Npcap single-flow failure was fixed and the exact product source head `9bacaab9268f893349800316b93c4d692158d6ec` was fully requalified. The durable predecessor is `docs/development/2026-08-30-single-flow-post-physical-fix-qualification.md`.

The user has explicitly frozen the mature TCP-like recovery/FEC/data-plane work. The architecture remains one public FakeTCP-owned TCP-shaped flow from the only SYN through Reality-like TLS setup and the in-band switch to DTLS/LINK/FEC, with no second SYN and no ordinary-kernel-TCP sustained HOL.

## Sequence 64 handoff verification

Canonical handoff sequence 64 at canonical head `2211288634553e066de42cd7d9ad6214ad905526` was live-refreshed before this work. PR-triggered `handoff-verify` run `33263357866` completed `success`, together with the exact-head main CI and release/load gates recorded in sequence 64.

The sequence-64 `next_atomic_action` remains physical qualification with the already-qualified same-source Windows/Linux artifacts. No new physical marker was supplied after the exact-flow Npcap fix, so this follow-up deliberately avoids modifying FakeTCP ACK/SACK/recovery/FEC behavior.

## Audit finding

Current Windows Npcap receive handling already filters adapter traffic before the strict FakeTCP packet parser. Only the exact unfragmented reverse IPv4/TCP four-tuple is delivered to the protocol state machine. Outbound self-capture, UDP, ICMP, wrong peers/ports, fragments and malformed packets are ignored; exact local-kernel RST remains diagnostic-only. Ethernet extraction already supports stacked `0x8100`/`0x88a8` VLAN tags.

This means the earlier physical `faketcp: not ipv4/tcp` failure has a structurally correct fix. The useful remaining automated work is qualification hardening, not another product transport rewrite.

## Test-only hardening

Feature branch `feat/single-flow-reality-faketcp`, PR #9:

- commit `74bffc859c73befab2d26dce7a348c13162b3288` — `test: harden Windows Npcap noisy ingress corpus`
- only `cmd/wbd-faketcp/main_windows_test.go` changed;
- no product/wire/recovery/FEC source changed.

New Windows-only coverage adds:

1. stacked QinQ extraction (`0x88a8 -> 0x8100 -> IPv4`);
2. ARP, IPv6 and LLDP rejection before the IPv4/TCP parser;
3. truncated Ethernet/VLAN capture rejection;
4. exact reverse flow with IPv4 options;
5. malformed IHL rejection;
6. 4096 deterministic mutations spanning wrong source/destination IP, protocol, source/destination port, fragmentation, total-length corruption and IPv6-shaped noise;
7. every prefix truncation of an otherwise exact packet;
8. fuzz seed execution over arbitrary Ethernet frames, exercising `ethernetIPv4Payload`, exact-flow matching, local-RST detection and payload-boundary classification for panic safety.

`windows-faketcp-persona` run `33297977069` executed on the GitHub Windows Server hosted runner and completed `success`, including the new Windows backend tests. This proves the new corpus itself compiles and passes on Windows; it is still not claimed to be a physical Npcap-driver/NIC test.

## Repository contract drift found during the follow-up

The same test commit triggered `handoff-verify` run `33297977104`. `scripts/verify_handoff.py` itself printed `HANDOFF_VERIFY_PASS sequence=64`; the job failed later in `tests/test_handoff.py` because the ROADMAP machine-contract test still required the stable phrase `V2.3 SINGLE-FLOW CORRECTION ACTIVE`, while the document headline had evolved to `V2.3 SINGLE-FLOW MAINLINE CANDIDATE`.

This was documentation/test drift, not a handoff-schema failure and not a product regression.

Feature commit `ecb01c4eb620a9caaeeec7b04e50c5abc88ab520` restores the stable machine anchor as an explicit contract-anchor line while retaining `MAINLINE CANDIDATE` as the current delivery state. No architecture clause was reverted.

## Current status at log write

- product source/binary behavior remains the exact qualified single-flow implementation from sequence 64;
- TCP-like recovery/FEC/data plane remains untouched;
- Windows noisy-ingress qualification is stronger and has passed on a Windows hosted OS at `74bffc...`;
- the ROADMAP contract drift is fixed at `ecb01c4...`;
- current-head duplicate CI runs caused by the docs-only correction may still be queued/in progress and must be live-refreshed before being called green;
- no new artifact should be handed to the user merely because of this test/docs follow-up. The sequence-64 same-source product artifacts remain the qualified physical-test pair until a product-source change is made.

## Next action

Live-refresh `ecb01c4...` / PR #9. Require at minimum `handoff-verify` and main `ci` to close green after the ROADMAP contract correction. The Windows persona result on the substantive test commit is already green. If no new deterministic product failure appears, stop changing transport code and proceed with the exact same-source physical Windows 11/Npcap -> real NAT/ISP -> Ubuntu ARM64 qualification recorded in sequence 64. Any physical failure must be classified by its first deterministic marker and reproduced in sandbox before touching the mature TCP-like/FEC core.
