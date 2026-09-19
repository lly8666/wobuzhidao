# ADR-0008: Reality-like same-entry recognition and one-time ticket join

## Status

**AMENDED BY ADR-0011.** The useful same-entry recognition/fallback and shared-account ticket semantics remain accepted. The former V2.2 implementation boundary — a kernel TCP/TLS connection that closed before a fresh FakeTCP association — is superseded.

WBD does not claim to implement Xray/XTLS REALITY and does not import VLESS/Vision stream semantics.

## Motivation

The useful REALITY-like properties for WBD are:

- a realistic TLS ClientHello at the beginning of the public session;
- recognition of WBD traffic from that ClientHello without plaintext credentials;
- unrecognized ClientHello forwarding to a configured genuine fallback target;
- simple encrypted shared username/password admission and a fresh one-time ticket.

The additional V2.3 requirement is that the recognized WBD path must stay on **one public TCP-shaped FakeTCP 4-tuple and one continuous sequence space** from the first SYN through sustained VPN data.

## V2.3 decision

The preferred public flow is now:

```text
client raw FakeTCP -> WBD :443
        -> SYN / SYN-ACK / ACK
        -> temporary reliable ordered bootstrap stream
        -> real TLS 1.3 ClientHello with WBD recognition marker

recognized:
        same raw association -> local TLS 1.3 takeover
        -> one encrypted username/password request
        <- fresh one-time ticket
        -> bootstrap ACK drain + mode barrier
        -> SAME 4-tuple / SAME sequence space / NO new SYN
        -> DTLS 1.3
        -> ticket bind
        -> LINK_INIT / LINK_ACCEPT
        -> WBD/FEC application datagrams

unrecognized:
        exact already-read ClientHello bytes -> configured fallback target
        <-> ordinary target TLS session
        over the same client-facing raw FakeTCP stream
```

There is deliberately **no sustained WBD VPN payload inside an ordinary kernel TLS/TCP byte stream**. The temporary ordered stream exists only because TLS requires stream semantics. The post-barrier WBD data plane remains no-HOL DTLS/FEC/FakeTCP datagrams.

## ClientHello recognition

The client lets `crypto/tls` construct a real TLS 1.3 ClientHello. WBD supplies the randomness used for the TLS 1.3 compatibility SessionID so the marker is part of the original handshake transcript rather than a post-build packet patch.

The marker is derived from a shared route/classifier key, ClientHello random and configured SNI. The route key is only a connection classifier; account authentication still happens inside the recognized TLS branch.

An unrecognized ClientHello is not reserialized. The raw mux replays the exact bytes it already consumed to the configured fallback connection, then proxies the remaining stream bidirectionally. This decoy path may use an ordinary outbound TCP socket because it is not WBD VPN payload.

## Simple shared-account admission

WBD is a personal deployment, not a multi-tenant identity service.

After recognized TLS 1.3 establishment, the product uses a **single encrypted username/password request**. TLS already provides confidentiality/integrity for setup, so WBD does not add another application-layer bearer password exchange.

On success the server generates a fresh random 32-byte one-time ticket. The **same username/password may authenticate multiple simultaneous devices/sessions**, each receiving an independent ticket.

Username identifies the shared account, not live transport state. Live WBD state is ticket/LiveID based.

## Ticket join

The ticket is short lived and consumed once by the DTLS/WBD startup path. In V2.3 this bind occurs later on the **same public FakeTCP association** that issued the ticket; it does not mean a second public connection.

A front-issued ticket means account admission already succeeded, so ticket mode does **not** require another normal device/account `AUTH` bearer exchange after DTLS.

The public path must not expose the ticket, username, password, WBDC framing or known application plaintext.

## Certificate and hostname policy

The personal client supports explicit certificate/hostname verification disablement for the bootstrap and explicit no-verification mode for DTLS. This is an intentional personal-deployment tradeoff and must be visible in configuration/logging.

DTLS 1.3 remains the steady-state cryptographic authority.

## Legacy diagnostics

`cmd/wbd-reality-front`, `cmd/wbd-reality-mirror`, witness files and earlier same-socket kernel-TCP tests may remain as diagnostic/reference material, but the product manager must not start a parallel public kernel TCP Reality listener.

No future product work should depend on the old `Reality TCP -> close -> new FakeTCP SYN` shape merely because historical tests reference it.

## Leakage and data-plane invariants

- One recognized WBD session has one public SYN lineage and one 4-tuple.
- Shared username/password appear only inside TLS records.
- Every successful login receives an independent one-time ticket.
- Ticket bytes are not repeated as plaintext on the public path.
- The bootstrap stream is bounded and temporary.
- After the mode barrier, later independent authenticated datagrams may complete across an earlier FakeTCP sequence hole.
- SACK/RACK/shadow recovery must not make inner delivery wait on ordered TCP state.

## Qualification gate

The single-flow Reality-like path is release-qualified only when tests prove:

1. a TLS-generated marker is recognized without post-build ClientHello mutation;
2. exactly one public WBD SYN lineage is used from bootstrap into data mode;
3. recognized traffic is taken over on the same raw FakeTCP association;
4. unrecognized ClientHello bytes are forwarded byte-for-byte to the configured fallback;
5. real TLS 1.3 and configured SNI are visible during setup;
6. wrong username/password fails;
7. the same username/password can issue multiple distinct concurrent one-time tickets;
8. each ticket is consumable once and only once;
9. ticket mode reaches LINK_INIT/LINK_ACCEPT without a second bearer AUTH or second public SYN;
10. a public capture contains none of the username, password, raw/ascii ticket, WBDC magic or known application plaintext;
11. after mode switch a later DTLS/FEC datagram can bypass an earlier missing FakeTCP payload;
12. the selected TCP/TLS fingerprint is measured against the chosen Reality/browser reference before any "99%" resemblance claim.
