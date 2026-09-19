# 2026-08-29 Single-Flow Qualification Notes

This is an append-only development note for the V2.3 one-public-flow architecture. See `SINGLE_FLOW_V23_DEVLOG.md` for the full historical narrative.

## Single-flow 100 Mbit qualification is green

The legacy `mux-load-100m` setup was found to be architecturally invalid for V2.3 because it still acquired tickets through a standalone ordinary-TCP Reality front and then opened a second FakeTCP connection.

Commits `be13d6fd85b48e57107b09cac978931d1eb9784e` and `3da850c8cb151768d760bbb3e624aa9b4a9c4f49` replaced only the setup/admission portion of the benchmark with a single-flow core while preserving the mature load generator, qdisc, resource sampling, meter and result schema.

Authoritative run:

- workflow: `mux-load-100m`
- run: `33243884457`
- tested source head: `090a82e971eec46a913297efcc7a4eea5463fb19`
- result: SUCCESS
- `bench (20)` job `99077944286`: SUCCESS
- `bench (100)` job `99077944366`: SUCCESS
- `aggregate` job `99078425307`: SUCCESS
- both RTT jobs completed the full 40/60/80 Mbit aggregate-inner sweep.

This is the first 100 Mbit characterization run whose setup itself enforces the current product architecture: one SYN/one FakeTCP association per client, Reality-like TLS/admission inside that association, then DTLS/LINK on the same public flow. The prior dual-flow RTT100 setup timeout is retired as qualification evidence and must not be used to justify changes to mature FakeTCP recovery/FEC.

## No-HOL qualification remains green

Corrected `single-flow-no-hol` run `33243630006`, job `99077133630`, is SUCCESS:

- exactly one post-ready ACK|PSH payload dropped;
- later independent DTLS datagram arrived after 0.224 ms;
- intentionally earlier dropped datagram recovered after 1002.478 ms;
- later-before-repair = true.

Therefore the same TCP-shaped sequence lineage does not imply ordinary kernel-TCP head-of-line blocking after the setup -> DTLS switch.

## Reality-like TLS persona status

The public single-flow ClientHello is already pinned to `uTLS v1.6.5 / HelloFirefox_120`. The persona workflow compares the captured ClientHello against independently transcribed Firefox 120 expectations for cipher order, extension order, supported groups, signature algorithms, versions, ALPN, SNI, key shares, JA3 and JA3 MD5. Only the 32-byte TLS 1.3 compatibility SessionID contents are intentionally derived from the WBD route classifier; the field length remains Firefox-shaped.

Recent persona pcap evidence before TCP-profile work showed:

- client IPv4 TTL 64;
- TCP window 65535;
- MSS 1360 (WBD path-MTU-derived);
- SACK permitted;
- window scale 8;
- SYN option order `MSS,SACK,NOP,WS,NOP,NOP`;
- TLS ClientHello exactly matched the pinned Firefox 120 gate.

The TLS portion is therefore strongly qualified, while the client TCP/IP portion was still Linux-like even for the physical Windows product.

## Windows client TCP/IP persona correction

External reference basis:

- Microsoft Windows TCP feature documentation: initial unscaled SYN window semantics use 65535 where scaling is used, and Windows supports WS/SACK during the handshake: https://learn.microsoft.com/en-us/troubleshoot/windows-server/networking/description-tcp-features
- p0f Windows-family signatures use initial TTL 128 and option layout `mss,nop,ws,nop,nop,sok`; p0f also treats MSS as link-dependent for commodity links: https://github.com/p0f/p0f/blob/master/p0f.fp

WBD deliberately retains MSS 1360 because the product MTU is 1400; pretending to advertise 1460 while intentionally emitting smaller transport segments would be a less coherent disguise. The Windows-family characteristics being corrected are TTL and TCP option ordering, not path MTU.

Implementation commits:

- `26c404ada7e351662f0639e3964260727dc84dd6` add presentation-only packet persona wrapper;
- `185f5c827497c1f084c7282f8923c359119ee3ae` add byte/checksum tests;
- `0fafb3a4a6c148904ee6aa3c6794ed1232f300d9` apply platform-default persona while preserving a byte-identical legacy base marshal;
- `3901f720d4cb11d9717190dc7296a2fef6f4ee9f` add Windows-build default-persona assertion;
- `c902ef2cbefe8653bfae0b7cf99b90c0eccb6ac4` add a Windows hosted-runner persona gate.

Design:

- Windows builds default to `PacketPersonaWindows11`;
- Linux/OpenWrt builds default to `PacketPersonaLegacy`;
- the mature base marshal remains persona-free and byte-identical;
- Windows flow TTL is 128 for the whole raw association, including bootstrap and post-switch payload, avoiding a visible TTL jump at the TLS -> DTLS boundary;
- Windows SYN layout is exactly `MSS,NOP,WS8,NOP,NOP,SACK-permitted`;
- DF + incrementing nonzero IPv4 ID and window 65535 are retained from the mature builder/callers;
- sequence, ACK, SACK recovery, RTO, FEC and payload semantics are untouched.

The new `windows-faketcp-persona` workflow executes `internal/faketcp` on a real `windows-latest` runner and also builds `cmd/wbd-faketcp`. It must be green before calling the Windows TCP persona qualified.

## Deliberate non-change: server SYNACK persona

The Linux server currently retains the mature WBD SYNACK profile. Although a generic Linux server fingerprint often differs in window scale/options, those values are also part of the existing WBD handshake recognition semantics. Changing them merely for cosmetic matching would unnecessarily touch the mature TCP-like handshake core.

Policy for now:

1. make the physical Windows client coherent with a Windows-family TCP/IP profile;
2. keep the pinned Firefox 120 TLS ClientHello gate;
3. retain Linux server SYNACK semantics until a dedicated reference/compatibility experiment proves a safe presentation-only change;
4. never weaken no-HOL or steady-state qualification for persona polish.

## Next actions

1. Require `windows-faketcp-persona`, main CI, Windows portable, single-flow E2E, two-client, no-HOL and single-flow persona gates to pass on the packet-persona head.
2. Re-run the single-flow 100 Mbit qualification on the exact final packet-persona head; Linux path should be byte-identical, but exact-head evidence is required before release artifacts.
3. Inspect the resulting Windows portable artifact and Linux ARM64 release from the exact qualified head.
4. Update PR #9 development comments and canonical handoff with the exact final head and workflow runs.
