# ADR-0008: Reality-like same-entry front and one-time ticket join

> **V3 SUPERSESSION NOTICE (2026-08-31):** ADR-0011 supersedes the product flow below where it closes an ordinary Reality-like TCP socket and then opens a distinct FakeTCP association. V3 keeps the useful TLS recognition/authentication/ticket concepts, but executes them inside the bounded first phase of the **same raw FakeTCP public flow**, followed by an encrypted in-flow switch to DTLS/FEC/LINK. This file is retained as V2 historical evidence and diagnostic background.

## Status

**SUPERSEDED IN PART BY ADR-0011 FOR V3.** The older fixed-target mirror/witness experiment remains diagnostic compatibility material. WBD does not claim to implement Xray/XTLS REALITY and does not import VLESS/Vision stream semantics.

## Historical motivation

V2 sought a Reality-like front that could inspect a TLS ClientHello, authenticate a personal shared account and issue a one-time ticket without carrying sustained VPN traffic in ordinary TLS/TCP. That work established useful TLS parsing, marker, credential and ticket primitives.

The V2 implementation used a kernel TCP listener and then joined a later FakeTCP association with a ticket. Physical/NAT qualification subsequently showed that two public flows violate the product's one-connection requirement and can create competing kernel/raw TCP state. ADR-0011 therefore moved these primitives into the first bounded phase of one FakeTCP-owned association.

## Historical V2 flow (not the V3 product path)

```text
client TCP -> WBD :443
        -> ClientHello with WBD recognition marker
        -> local TLS 1.3 takeover
        -> encrypted username/password
        <- fresh one-time ticket
        -> close bootstrap TCP
        -> separate FakeTCP / DTLS / ticket bind
```

**Do not implement or package this sequence for V3.** The current V3 sequence is defined by ADR-0011 and PROJECT_CONSTITUTION.md: one raw SYN/4-tuple, in-flow TLS 1.3 bootstrap, encrypted switch, then same-association DTLS/FEC/LINK datagrams.

## ClientHello recognition retained

The recognition work remains useful. A TLS implementation constructs a valid ClientHello; WBD uses a classifier marker tied to configured routing identity/SNI. Browser/uTLS fidelity can evolve, but recognition must happen without a second public socket in V3.

Unrecognized/fallback behavior, if retained in a future V3 product, must live under the sole raw-front ownership model. It must not restore a competing kernel TCP listener on `WBD_PORT`.

## Shared-account admission retained

WBD remains a personal deployment rather than a multi-tenant identity service.

- one configured username/password pair may authenticate multiple devices/sessions;
- credentials are sent only inside encrypted TLS application data;
- each successful admission receives fresh per-session identity/ticket material;
- live transport/session state is not keyed by username alone;
- ticket/session identity remains useful for internal LINK/session isolation.

In V3 this identity does **not** join two public flows. It is issued and consumed within lifecycle of the same FakeTCP association.

## Certificate and hostname policy

Historical V2 supported an explicit personal no-verification mode for TLS and DTLS. Current V3 release policy is governed by PROJECT_CONSTITUTION.md, ADR-0011 and the V3 qualification workflows. This historical file must not override newer security/fidelity decisions.

## Legacy mirror diagnostic

`cmd/wbd-reality-mirror`, older ClientHello witness material and the standalone `wbd-reality-front` implementation may remain useful for regression/reference work, but they are not authoritative V3 public-server composition.

No official V3 Linux bundle may install or launch `wbd-reality-front` as a second WBD public listener.

## V3-retained invariants

- sustained VPN data never depends on an ordinary kernel TCP byte stream;
- credentials/ticket/switch control are not plaintext on the public wire;
- DTLS 1.3 remains the steady-state cryptographic authority;
- FEC/FakeTCP preserve datagram/first-complete behavior after switch;
- same shared account may own concurrent isolated sessions;
- browser/Reality-like setup must not weaken the one-public-flow law.

## Superseding qualification

The active V3 qualification is in ADR-0011. In particular, captures must prove:

1. exactly one SYN and one public 4-tuple per session;
2. real TLS 1.3 Reality-like setup before DTLS on that same association;
3. encrypted switch control;
4. no FIN/RST/close_notify/new SYN at the boundary;
5. later post-switch datagrams can bypass an earlier loss;
6. official Linux composition has one raw WBD public owner.
