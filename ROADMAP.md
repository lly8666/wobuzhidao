# Roadmap

> **Status: V2.3 SINGLE-FLOW MAINLINE CANDIDATE.** ADR-0011 replaces the old two-public-connection Reality/FakeTCP setup with one continuous public FakeTCP-owned flow. Automated protocol/platform qualification is strong; final release acceptance still requires a same-source physical Windows + Linux/OpenWrt one-shot.

| Milestone | Scope | Status / exit gate |
| --- | --- | --- |
| V2-M0 | architecture restart / evidence preservation | **DONE** |
| V2-M1 | pinned one-lane udp2raw + UDPspeeder baseline | **DONE** |
| V2-M2 | native DTLS 1.3 security shim | **DONE** |
| V2-M3A-E | minimal session/control + immutable config foundation | **DONE AS FOUNDATION** |
| V2-M4 | kernel-anchor / real-return-packet experiment | **RETIRED** |
| V2-M5 | optional two raw lanes | **DEFERRED** |
| V2-M6A/B | packet-preserving Linux/TUN regression harness | **IMPLEMENTED** |
| V2-M6C | OpenWrt transparent capture | **FINAL SHAPE: TPROXY + POLICY ROUTING** |
| V2-M7A/B | Windows TUN + full/split routing | **IMPLEMENTED / PHYSICAL ACCEPTANCE PENDING** |
| V2-M8A-old | separate ordinary-TCP Reality-like front | **SUPERSEDED BY ADR-0011** |
| V2-M8A-SF1 | temporary reliable FakeTCP bootstrap stream | **IMPLEMENTED + QUALIFIED** |
| V2-M8A-SF2 | real TLS 1.3 / Reality-like auth over same FakeTCP association | **IMPLEMENTED + SINGLE-FLOW E2E QUALIFIED** |
| V2-M8A-SF3 | raw-listener fallback/decoy proxy + fingerprint qualification | **FALLBACK IMPLEMENTED/E2E GREEN; FINGERPRINT MEASUREMENT REMAINS** |
| V2-M8B-T1 | native FakeTCP + FEC first-arrival / pcap qualification | **RETAINED / GREEN** |
| V2-M8B-T2 | fixed FEC + immutable setup | **RETAINED** |
| V2-M8C | shared-account concurrent association/DTLS/LiveID fan-out | **SINGLE-FLOW TWO-CLIENT REQUALIFIED / GREEN** |
| V2-M9 | optional two-lane research | **POST-RELEASE ONLY IF JUSTIFIED** |
| V2-M10 | release qualification | **AUTOMATED GATES GREEN; SAME-SOURCE PHYSICAL WINDOWS + LINUX/OPENWRT ACCEPTANCE PENDING** |

## Frozen product invariant

A product WBD session is:

```text
one raw FakeTCP SYN lineage
  -> bounded reliable ordered bootstrap on that same association
  -> real TLS 1.3 / Reality-like marker + admission
  -> ACK drain + mode barrier
  -> SAME 4-tuple / SAME sequence space / NO new SYN
  -> pinned wolfSSL DTLS 1.3
  -> immutable LINK
  -> optional fixed FEC
  -> packet/datagram VPN payload without ordinary-TCP HOL
```

There is no separate product `Reality TCP -> close -> FakeTCP` sequence and no parallel kernel TCP Reality listener owning WBD admission on the public port.

## Current automated qualification evidence

The last fully qualified substantive feature head before release-manifest hardening was `fd0f1efeeb88e73f5bbf7034a7e1c7742c4f842b` on PR #9. Exact-head workflow evidence includes:

- `ci` — success, run `33293553761`;
- `single-flow-e2e` — success, run `33293553782`;
- `single-flow-no-hol` — success, run `33293553801`;
- `single-flow-two-client` — success, run `33293553757`;
- `single-flow-tcp-persona` — success, run `33293553825`;
- `single-flow-link-fullstack` — success, run `33293553790`;
- `single-flow-startup-stress` — success, run `33293553830`;
- `windows-faketcp-persona` — success, run `33293553765`;
- `windows-portable-bundle` — success, run `33293553733`;
- `windows-tun-build` — success, run `33293553796`;
- `windows-tun-admin-smoke` — success, run `33293553824`;
- `linux-server-release` — success, run `33293553809`;
- `linux-shared-port` — success, run `33293553808`;
- `mux-load-100m` — success, run `33293553734`;
- `faketcp-native` — success, run `33293553811`;
- `faketcp-first-arrival` — success, run `33293553826`;
- `faketcp-pcap-20loss` — success, run `33293553807`;
- `fullstack-first-arrival` — success, run `33293553836`.

These are automated qualification results, not physical release acceptance.

## What the dedicated single-flow gates prove

`single-flow-e2e` proves one public SYN lineage, one public 4-tuple, real TLS 1.3 Reality-like bootstrap, same-process mode switch, pinned wolfSSL DTLS 1.3, and bidirectional post-switch payload.

`single-flow-no-hol` separately proves the key delivery boundary: after the mode switch, a later independent authenticated datagram can be delivered before an intentionally missing earlier FakeTCP payload is repaired. Bootstrap stream ordering is therefore not inherited by the sustained data plane.

`single-flow-two-client` requalifies concurrent shared-account sessions on independent raw 4-tuples/LiveIDs.

`single-flow-startup-stress` keeps one server stack alive while running repeated full TLS-bootstrap -> DTLS -> LINK -> echo cycles through a NAT namespace with dirty client exits and fresh source ports. This is the durable reconnect/lifecycle regression rather than relying on a one-shot happy path.

## Reality-like resemblance / fallback

Single-flow is necessary but not sufficient to claim browser-perfect or "99% Reality" resemblance.

Current requirements remain:

- one plausible TCP-shaped SYN/SYN-ACK/ACK lineage;
- real TLS 1.3 records and configured SNI on the same FakeTCP sequence space;
- WBD recognition/authentication only inside TLS;
- no credentials/ticket in plaintext capture;
- unrecognized ClientHello remains in bootstrap stream mode and reaches the configured decoy target;
- selected SYN/TCP-option and TLS ClientHello fingerprint is measured against the chosen reference profile.

Fallback transport is implemented and has dedicated single-flow E2E coverage. Browser/REALITY fingerprint wording remains evidence-driven; do not claim "99%" until pcap comparison supports it.

## Frozen steady-state transport

ADR-0011 reopened only establishment. The qualified steady-state choices remain frozen unless deterministic evidence requires reopening them:

- pinned wolfSSL DTLS 1.3;
- one raw lane;
- `legacy` FakeTCP shadow recovery default;
- FEC `off` or fixed systematic `20:20`;
- immutable LINK parameters;
- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate-inner release operating point.

## Platform release paths

### Linux/OpenWrt server

Product mode owns `WBD_PORT` with one raw `wbd-faketcp-mux` listener. Reality-like TLS setup is the first phase of each raw association. `wbd-reality-front` may remain in a bundle only as a diagnostic/reference binary and must never be started by the product server run path.

OpenWrt final capture remains TPROXY + policy routing with explicit underlay escape and complete WBD-owned cleanup.

### Windows

Product `Connect()` discovers the underlay first, assigns a fresh dynamic raw source port, starts the sole FakeTCP process, waits for `WBD_SINGLE_FLOW_BOOTSTRAP_READY`, then starts DTLS/LINK/TUN through readiness gates. It must not run a preliminary ordinary-TCP Reality bootstrap.

Device-wide IPv6 remains fail-closed while connected until an IPv6 product path is separately qualified.

## Same-source artifact rule

Physical acceptance must never mix artifacts from different substantive commits.

Windows portable and Linux server bundles must carry an explicit `SOURCE_SHA`. Before any physical Windows <-> Linux/OpenWrt qualification, compare the two values. A mismatch invalidates the test before networking begins.

This rule exists because independently green Windows and Linux workflows are insufficient evidence that the exact pair handed to an operator was built from identical source.

## V2-M10 final release gate

A candidate becomes release-acceptable only when all of the following are true on one exact substantive source SHA:

1. unit/protocol/pcap regressions green;
2. exactly one public WBD SYN lineage from setup through data mode;
3. real TLS bootstrap and authenticated admission on that flow;
4. post-switch no-HOL hole-bypass test green;
5. shared-account two-client single-flow fan-out green;
6. <=100 Mbit/s / 40 Mbit release-load surface green;
7. Windows and Linux/OpenWrt artifacts both report the same `SOURCE_SHA`;
8. clean physical Linux/OpenWrt server start passes;
9. clean physical Windows TUN one-shot passes DNS + UDP + TCP application probes over that server;
10. disconnect/failure cleanup restores routes, NRPT/DNS, IPv6 and firewall state without manual repair.

Until items 7-10 are observed together, do not call a package final or release-ready.

## Removed / rejected work

- ordinary kernel TCP as product data carrier;
- `Reality TCP -> close -> second FakeTCP connection`;
- parallel kernel Reality listener + raw FakeTCP listener as the intended product same-port design;
- kernel TCP state takeover as a release dependency;
- runtime FEC epochs;
- SACK/RACK as unconditional product default;
- high-frequency continuously learning Auto FEC on the current critical path;
- VLESS/Xray/Vision stream semantics as the data plane;
- WireGuard inner glue;
- Android/no-root;
- mandatory multi-lane duplication.
