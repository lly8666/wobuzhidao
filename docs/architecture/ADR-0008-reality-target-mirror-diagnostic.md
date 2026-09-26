# ADR-0008: Reality-like same-entry front and one-time ticket join

## Status

Accepted for the personal product connection-establishment path. The older fixed-target mirror/witness experiment remains available only as a diagnostic compatibility path. WBD does not claim to implement Xray/XTLS REALITY and does not import VLESS/Vision stream semantics.

## Motivation

The useful REALITY-like property for WBD is not sustained VPN traffic inside an ordinary TLS/TCP stream. It is the ability to use **one TCP listener**, read the initial ClientHello, and then choose one of two branches:

- recognized WBD ClientHello: take over that same TCP socket locally with TLS 1.3;
- unrecognized ClientHello: forward the exact already-read bytes to the configured genuine fallback target.

This gives a much cleaner connection join than the earlier two-connection ClientHello witness design while preserving the main WBD architecture: sustained payload remains unordered DTLS/FEC/FakeTCP datagrams.

## Decision

The preferred front flow is:

```text
client TCP -> WBD :443
        -> ClientHello with WBD recognition marker

recognized:
        same socket -> local TLS 1.3 takeover
        -> one encrypted username/password request
        <- fresh one-time ticket
        close bootstrap TCP
        -> FakeTCP -> DTLS 1.3
        -> ticket bind
        -> LINK_INIT / LINK_ACCEPT
        -> WBD application datagrams

unrecognized:
        exact ClientHello bytes -> configured fallback target
        <-> ordinary target TLS session
```

There is deliberately **no sustained VPN payload inside the Reality-like TLS/TCP stream**. The unordered/no-HOL WBD data plane remains DTLS/FEC/FakeTCP.

## ClientHello recognition

The client lets its TLS implementation construct the ClientHello normally. WBD supplies the randomness used for the TLS 1.3 compatibility SessionID so the marker is part of the original TLS transcript rather than a post-build packet patch.

The current marker is derived from a shared route/classifier key, ClientHello random and configured SNI. The route key is only a cheap connection classifier; account authentication is still performed inside the recognized TLS branch.

An unrecognized ClientHello is not reserialized. The server replays the bytes it already read to the fixed fallback connection so fallback behavior stays byte-preserving at the join point.

## Simple shared-account admission

WBD is a personal deployment, not a multi-tenant identity service. Server recognition and admission should therefore stay deliberately small and fast.

After recognized TLS 1.3 establishment, the product path uses a **single encrypted username/password request**. TLS already provides confidentiality and integrity, so WBD does not add another application-layer nonce/HMAC challenge round trip.

Server behavior is intentionally simple:

1. read bounded username/password lengths and bytes inside TLS;
2. compare them against the one configured shared pair with constant-time equality checks;
3. on success generate a fresh random 32-byte one-time ticket;
4. persist the ticket with the account label and issue timestamp;
5. return the ticket immediately.

The **same username/password may authenticate multiple simultaneous devices/sessions**. Each successful request receives a different ticket. There is no required per-device credential database, password KDF layer, revocation table or username-based single-session lock.

The ticket is the session identity used to join the later DTLS/WBD association. The account name may be retained as log/metadata, but live transport state must not be keyed by username alone.

## Ticket join

The one-time ticket is short lived and consumed once by the DTLS/WBD startup path. A missing, expired or already-consumed ticket rejects that association.

A front-issued ticket means account admission already succeeded. The ticket path therefore does **not** require another normal device/account `AUTH` bearer exchange after DTLS. Legacy non-front and old witness compatibility modes may retain the older AUTH frame until they are retired.

The public path must not expose the ticket, username, password, WBDC framing or known application plaintext.

## Certificate and hostname policy

The personal client explicitly supports **certificate and hostname verification disabled**.

For the Reality-like TLS front, `verify-server=false` is the personal default. The configured SNI may therefore differ from the certificate name and the WBD server may present an arbitrary/self-signed local certificate.

For DTLS, the wolfSSL shim supports an explicit no-verification mode (`none`/`insecure`) that neither loads a trust anchor nor performs hostname checking.

This is an intentional product tradeoff: records remain encrypted, but the client does not authenticate the server certificate identity in this mode. The implementation must log the verification mode clearly so an operator cannot mistake it for verified TLS/DTLS.

## Legacy mirror diagnostic

`cmd/wbd-reality-mirror`, ClientHello witness files and `DEMO_BIND` remain useful for network-treatment experiments and historical regression. They are no longer the preferred product join.

The legacy mirror still has these constraints:

- fixed target/fixed SNI;
- bounded connection and byte limits;
- never an open relay;
- never carries sustained WBD payload;
- witness data is diagnostic correlation, not account authentication.

No future product work should depend on the old two-association witness merely because older tests reference it.

## Leakage and data-plane invariants

- Sustained VPN data never enters the front TCP/TLS stream.
- WBD never needs the fallback target private key.
- Public WBD control/data remains inside DTLS 1.3 after the front closes.
- Shared username/password are allowed for concurrent devices but only appear inside TLS records.
- Every successful login receives an independent one-time ticket.
- Ticket bytes are not repeated as plaintext on the public path.
- FEC and FakeTCP remain outside the front and keep first-complete datagram behavior.
- SACK/RACK shadow recovery must not make inner delivery wait on ordered TCP state.

## Qualification gate

The same-entry front is qualified only when tests prove:

1. a TLS-generated marker is recognized without post-build ClientHello mutation;
2. recognized traffic is taken over on the same TCP socket;
3. unrecognized traffic is forwarded to the genuine fallback without WBD TLS takeover;
4. an unrelated self-signed certificate succeeds when verification is explicitly disabled;
5. wrong username/password fails;
6. the same username/password can issue multiple distinct concurrent one-time tickets;
7. each ticket is consumable once and only once;
8. ticket mode reaches LINK_INIT/LINK_ACCEPT without a second bearer AUTH;
9. a public-side TCP+DTLS capture contains none of the username, password, raw/ascii ticket, WBDC magic or known application plaintext;
10. existing first-arrival, FEC and FakeTCP regressions remain green.

The front is considered connection setup only. Any design that makes sustained payload depend on an ordinary TLS/TCP byte stream is rejected even if its wire appearance is more browser-like.
