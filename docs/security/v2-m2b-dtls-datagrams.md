# V2-M2B — DTLS 1.3 datagram independence gate

Status: **PASS** (local sandbox authority, 2026-08-25 JST)

## Purpose

M2A proved the pinned wolfSSL DTLS 1.3 implementation, native X.509 chain validation and hostname verification. M2B isolates the next structural risk: accidentally rebuilding ordered delivery above DTLS before UDPspeeder or FakeTCP are reintroduced.

The tested shim deliberately stays thin:

- one plaintext UDP datagram received by the shim causes one `wolfSSL_write()` call;
- one successful `wolfSSL_read()` application record causes one plaintext UDP datagram send;
- there is no retransmission queue, stream reassembly, sequence-gap wait, FEC, udp2raw or UDPspeeder in this gate;
- client authentication still uses the M2A `wbd.test` CA/hostname path.

The socket is switched to nonblocking mode only after the DTLS handshake and `wolfSSL_set_using_nonblock(ssl, 1)` is set at the same time. This is required so an empty UDP receive after consuming a record is reported as WANT_READ instead of a fatal socket error.

## Direct relay

Five numbered application datagrams were sent and all five returned. Both shims recorded 5 plaintext inputs, 5 DTLS application writes, 5 DTLS application records read and 5 plaintext outputs. Negotiated protocol/cipher remained `DTLSv1.3 / TLS_AES_256_GCM_SHA384`.

## Drop-one-encrypted-record gate

A deterministic UDP proxy was inserted between the two DTLS peers. The complete handshake was allowed to finish, both shims reached READY, the path was left idle for 500 ms, and only then was the proxy armed to drop the next client-to-server encrypted UDP datagram.

The accepted qualification run observed 12 forwarded DTLS packets before the arm point. Proxy event 13, a 28-byte client-to-server encrypted UDP datagram, was dropped. Plaintext sent: `msg-01`, `msg-02`, `msg-03`. Plaintext delivered: `msg-02`, `msg-03`.

`msg-01` was absent as expected. The server nevertheless authenticated/decrypted the later two DTLS application records and echoed both. Therefore loss of one earlier encrypted DTLS application datagram does **not** block later application records. No record-order HOL was introduced by the shim.

## Reproducibility

Run M2A first, then:

```text
python3 scripts/qualify_v2_m2b_dtls_datagrams.py <M2A_OUT> <M2B_OUT>
```

The M2B driver verifies the M2A receipt/source identity/build flags and checks the actual local `libwolfssl.a` SHA against that receipt before compiling the shim.

Accepted local identities:

- M2A `libwolfssl.a`: `22b132f66b74067507df8a891a2137e78d54309ec424c8bcc7ed9a3ac897c96a`;
- shim source SHA-256: `e45680a297645812ac5da2133d4357408e81776607ba641c192e1f874e4768ab`;
- shim binary SHA-256: `90cc3bf45d1b23cd1848159e730a6b80a2e16221177da5b83078d4db75b22e65`;
- driver SHA-256: `1a05f552e70a5da806b21e9d951f7d380dbe0073e748b463ae6ecf4f067b1cee`;
- receipt content SHA-256: `e6446e1da336e30c4e4e5bcdcdc50a914de4d07e274d43a2bb08a0d0f2be6bd1`.

The exact packet prefix and monotonic timestamp may differ on repeated runs because DTLS keys/nonces differ; the invariant is one armed post-handshake client-to-server datagram dropped and later numbered application datagrams still delivered.

## Next gate

M2B does not qualify the product carrier. M2C inserts this shim between the already-qualified UDPspeeder and udp2raw components, first as a clean 0% smoke and then under the M2 impairment family. Full M2 still requires latency/delivery/byte/CPU accounting with DTLS present.
