# ADR-0008: Reality-style target mirror plus encrypted demo binding

## Status

Accepted as an **explicit demo-only experiment**. It is not the default WBD product mode and is not a claim that WBD implements Xray/XTLS REALITY.

The fixed-target mirror remains useful as a network-treatment oracle. The follow-on prototype adds a one-time binding from that genuine target preflight into the normal WBD DTLS 1.3 startup path without ever switching the mirror TCP/TLS byte stream into WBD application traffic.

## Motivation

A genuine public TLS target can answer the narrow measurement question of whether a flow to the WBD server IP is treated differently when its visible TLS exchange is supplied by that selected target. The user additionally requires that no WBD account data, link parameters or application payload leak while moving from the preflight into the VPN data association.

A certificate fingerprint alone cannot reproduce a target TLS identity: TLS 1.3 `CertificateVerify` requires the target private key. Conversely, blindly changing protocol after forwarding a third-party TLS handshake would either corrupt that TLS connection or require terminating/impersonating the target. WBD does neither.

## Decision

The demo is a two-association design:

```text
association A: genuine target preflight
client TCP -> WBD mirror -> fixed target:443
ClientHello ---------------------------> target
target TLS records --------------------> client
normal target certificate validation
close preflight

association B: WBD product data association
client -> WBD FakeTCP -> DTLS 1.3
       -> DEMO_BIND
       <- DEMO_BIND_OK
       -> LINK_INIT
       <- LINK_ACCEPT
       -> AUTH / AUTH_OK (mandatory in demo mode)
       -> WBD encrypted application datagrams
```

There is deliberately **no in-stream plaintext or protocol splice** from A to B. `DEMO_BIND`, `LINK_INIT`, `AUTH`, FEC metadata and application packets are DTLS application data and are therefore encrypted on the public path. The target mirror is never used as a container for sustained WBD payload.

## One-time witness

Both mirror endpoints derive the SHA-256 of the exact initial TLS ClientHello bytes. The hash is correlation metadata, not an authentication secret.

Server side:

1. `wbd-reality-mirror -witness-dir DIR` reads and validates the configured SNI.
2. It forwards that exact ClientHello to the fixed genuine target.
3. When the genuine target produces its **first downstream TLS bytes**, the mirror atomically records the ClientHello hash in a local `0700` witness directory **before those first bytes are forwarded to the client**.
4. Witness files are `0600`, target-name bound and short lived.

This ordering closes the preflight-to-WBD race without an arbitrary sleep: a client cannot complete the genuine TLS preflight before the server-side witness exists.

Client side:

1. `wbd-tls-diag -witness-out FILE` records the exact ClientHello flight it sent before the first server read.
2. The client performs ordinary certificate-chain/hostname validation of the genuine target.
3. It supplies the resulting 64-hex hash to `wbd-link-proxy -demo-reality-witness HEX`.

WBD server startup consumes the matching witness exactly once with a short TTL (default 15 seconds). A missing, expired, target-mismatched or already-consumed witness rejects the association before `LINK_INIT` is accepted.

The witness does not replace WBD account authentication. An observer can hash a public ClientHello too. Therefore demo constructors reject an empty client token or empty server expected-token: normal high-entropy device/account AUTH is mandatory and remains authoritative inside DTLS after the demo gate.

## Reliable startup semantics

Demo startup adds two WBDC frame types used only inside the already-established DTLS association:

- `DEMO_BIND`: 32-byte ClientHello witness;
- `DEMO_BIND_OK`: byte-identical witness echo from the server.

Loss handling follows the immutable startup rule:

- the client retries the exact `DEMO_BIND` until `DEMO_BIND_OK`;
- an exact duplicate bind is idempotent;
- changing the witness on the same association poisons startup and requires reconnect;
- `LINK_INIT` is not accepted before a valid demo bind;
- normal `LINK_INIT/LINK_ACCEPT/AUTH` behavior is unchanged after binding.

This avoids half-switched states when DTLS application datagrams are lost.

## Default mode

The demo is off unless explicit `-demo-reality-*` command-line arguments are supplied. With no demo arguments, `wbd-link-proxy` uses its normal immutable startup path and does not consult a mirror or witness directory.

Normal deployments may use an operator-controlled self-signed DTLS certificate. Self-signed means an explicit trust anchor or fixed SPKI on the client; it never means disabling certificate verification. The current wolfSSL shim already uses `WOLFSSL_VERIFY_PEER`, loads the configured trust file and calls `wolfSSL_check_domain_name`.

## Security and leakage invariants

- WBD never copies or presents a third-party private key.
- WBD never injects `DEMO_BIND`, AUTH tokens, WBDC frames or application data into the genuine target TLS connection.
- Public-path WBD control/data remains inside DTLS 1.3.
- Demo mode always requires normal WBD device/account authentication in addition to the witness.
- Mirror mode is fixed-target/fixed-SNI, bounded and explicit; it is not an open relay.
- Witnesses are non-secret, local, short-lived, target-bound and one-time.
- Witness publication is ordered before the first target TLS bytes reach the client.
- The unordered/no-HOL WBD data plane remains DTLS/FEC/FakeTCP and is never replaced by an ordinary TLS/TCP byte stream.

These are implementation requirements, but the no-leakage claim is not accepted from source inspection alone. `.github/workflows/demo-encryption.yml` must pass with the pinned wolfSSL DTLS 1.3 shim and a public-side pcap that contains none of the known `WBDC` magic, device-token bytes, or known application plaintext. A future refactor that cannot satisfy this capture gate is not an acceptable demo implementation.

## Non-goals

- no VLESS/Vision import;
- no transparent third-party identity takeover;
- no open fallback relay;
- no sustained VPN payload inside the mirror TCP stream;
- no automatic selection of public target sites;
- no claim that a target mirror will improve a given ISP path before paired real-network evidence exists.

## Qualification gate

Before this demo can influence product decisions, tests must prove:

1. exact ClientHello hash agreement between client capture and mirror observation;
2. one-time/TTL/target-name witness rejection behavior;
3. witness publication occurs before first target TLS bytes reach the client;
4. demo mode rejects missing normal device/account AUTH;
5. reliable `DEMO_BIND/DEMO_BIND_OK` retries under loss;
6. application forwarding begins only after demo bind + normal link setup + AUTH;
7. normal non-demo startup remains behavior compatible;
8. a real DTLS pcap contains no plaintext WBDC magic, device token or known application payload;
9. existing first-arrival and FakeTCP/FEC regressions remain green.

Only after real-network A/B evidence shows a repeatable benefit should WBD consider a more elaborate authenticated preflight. Any such work must preserve the same rule: the product data lane stays independently authenticated and encrypted, and unordered first-arrival semantics are not traded for a TLS/TCP stream disguise.
