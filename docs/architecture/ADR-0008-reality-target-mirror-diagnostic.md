# ADR-0008: Reality-style fixed-target mirror diagnostic

## Status

Accepted as an **isolated diagnostic experiment**. It is not the WBD product data plane and is not a claim that WBD implements Xray/XTLS REALITY.

## Motivation

The user observes strong time-of-day differences in international network quality and wants to test whether a flow to the same WBD server IP is treated differently when the visible TLS exchange is genuinely supplied by a selected public HTTPS target.

A certificate fingerprint alone cannot reproduce that property: TLS 1.3 `CertificateVerify` requires the target's private key. A scientifically cleaner first experiment is therefore to let the genuine target participate in the handshake.

## Relevant REALITY behavior

Current REALITY implementations have two distinct ideas that matter here:

1. the server opens a connection to a configured TLS target and observes/mirrors its handshake behavior;
2. unauthenticated/fallback traffic can be spliced to that target, while an authenticated REALITY client uses additional X25519/HKDF/AES-GCM material carried in ClientHello state and a custom authenticated certificate path.

WBD does **not** need the second mechanism to answer the first network-treatment question.

## Decision

Add `cmd/wbd-reality-mirror` plus `internal/realitymirror` as a bounded fixed-target oracle:

```text
client TCP
    -> WBD mirror listener
        -> fixed configured target:443

client ClientHello -------- exact bytes -------> target
target TLS records -------- exact bytes -------> client
then bounded bidirectional splice
```

The mirror parses enough ClientHello structure to enforce one configured SNI before dialing the target. It does not terminate TLS and does not forge, cache, replace or synthesize the target certificate. The client validates the genuine target certificate in the normal way.

The diagnostic is intentionally not an open proxy:

- one fixed target;
- one fixed allowed SNI;
- loopback listen by default;
- bounded ClientHello size;
- bounded concurrent sessions;
- bounded session lifetime;
- bounded transfer bytes by default.

`-max-bytes 0` is allowed only for a deliberately short throughput experiment so normal TCP `io.Copy` can use the platform fast path; the operator should retain a short session timeout and low concurrency and close the public listener after the test.

## Measurement method

Use the existing `wbd-tls-diag` plus `scripts/bench_reality_mirror.py` to alternate paired observations:

```text
A: client -> genuine target directly
B: client -> same WBD server IP -> mirror -> genuine target
```

A and B use the same TLS hostname/SNI and should expose the same real target certificate/SPKI. Pair order alternates A/B then B/A to reduce short-term network drift.

Useful next comparisons are:

```text
A direct genuine target
B target through mirror
C WBD's own TLS/Persona endpoint
D WBD FakeTCP/DTLS data plane
```

A vs B primarily changes destination IP/path while preserving a genuine target TLS endpoint. B vs C changes endpoint TLS identity/behavior. C vs D changes the WBD carrier.

## Non-goals

- no VLESS/Vision import;
- no open fallback relay;
- no copying or presenting a third-party private key;
- no claim that certificate/SPKI equality alone causes network treatment;
- no automatic selection of public target sites;
- no embedding of the WBD unordered data plane inside the mirror's ordinary TCP/TLS byte stream.

The final non-goal is a transport invariant: putting sustained WBD payload inside a normal TLS/TCP stream would reintroduce ordered-stream head-of-line behavior and invalidate the current first-arrival architecture.

## Admission to further work

Only if paired real-network tests show a repeatable material improvement for B over A/C/D should WBD spend time on a second prototype inspired by REALITY's authenticated path. Such a prototype must preserve the existing unordered DTLS/FEC/FakeTCP data semantics or explicitly remain a bootstrap-only mechanism; it must not silently replace the product data path with ordinary TLS/TCP.
