# V2-M2A — pinned wolfSSL DTLS 1.3 handshake qualification

Date: 2026-08-25

## Decision

**PASS for the M2A security/correctness gate.** The exact architecture-pinned wolfSSL `v5.9.2-stable` source at commit `ac01707f552c611fbd135cc723b2682b3e7f80f2` was source-identity verified, built locally with DTLS 1.3 enabled, and used locally for a real UDP DTLS 1.3 client/server handshake with native certificate-chain and hostname verification.

This is **not yet the full V2-M2 carrier qualification**. UDPspeeder/FEC and udp2raw/FakeTCP were deliberately excluded from M2A so security correctness could be proven independently.

## Source identity

- Upstream: `wolfSSL/wolfssl`.
- Tag: `v5.9.2-stable`.
- Commit: `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
- The temporary Actions relay checked out the exact commit, explicitly fetched `v5.9.2-stable`, and asserted that the tag resolves to the same commit before creating the source archive.
- Relay workflow run: `32752897445`.
- Relay artifact: `9529710833` (`v2-m2-wolfssl-source`).
- Artifact digest: `sha256:303eb10638dbe79fa96aa167446cee47c5813c789a976a18640b9eac1a6e3259`.
- Exact relayed `git archive` SHA-256: `4a7ff40a32db0d7a262aaea2d2e674da6708250cba908441c737c981fc84f88b`.

Actions was a source-byte relay only. Build and protocol qualification were local.

## Local build

Configure flags:

```text
--enable-dtls13 --disable-shared --enable-static
```

Toolchain:

- GCC `14.2.0` (Debian 14.2.0-19)
- Autoconf `2.72`
- Automake `1.17`
- GNU libtool `2.5.4 Debian-2.5.4-4`

The generated options enable `WOLFSSL_DTLS13` and `WOLFSSL_TLS13`. They do **not** enable early data/0-RTT or `OPENSSL_EXTRA`.

Exact accepted local build hashes from the reproducibility run:

- `libwolfssl.a`: `22b132f66b74067507df8a891a2137e78d54309ec424c8bcc7ed9a3ac897c96a`
- `dtls13_probe`: `7bc0f2ab52710be65e0c69e71d0afe4d79e5c7a364d339d0965879388002dc5c`
- qualification receipt: `2a532a71ada9bedfa232c54faeac80910f23eea6e40a0b18b5d62c7211480471`

The static library exports the required `wolfDTLSv1_3_client_method`, `wolfDTLSv1_3_server_method`, and `wolfSSL_check_domain_name` APIs.

## Native verification path

The client uses:

- `wolfDTLSv1_3_client_method()` — DTLS 1.3-only method;
- `wolfSSL_CTX_set_verify(..., WOLFSSL_VERIFY_PEER, ...)`;
- `wolfSSL_CTX_load_verify_locations()` — trusted CA store;
- `wolfSSL_check_domain_name(ssl, "wbd.test")` before `wolfSSL_connect()`.

There is no manual `wolfSSL_X509_verify_cert()` / OpenSSL-compatibility verification step.

The server uses `wolfDTLSv1_3_server_method()`, a real X.509 certificate/private key, and DTLS 1.3 HRR-cookie support. The test certificate contains `subjectAltName=DNS:wbd.test`.

## Local cases

| Case | Expected | Result |
| --- | --- | --- |
| trusted CA + expected hostname `wbd.test` | accept | **PASS** — `DTLSv1.3`, `TLS_AES_256_GCM_SHA384`, application datagram round trip |
| trusted CA + expected hostname `wrong.test` | reject | **PASS** — rejected with `peer subject name mismatch` |
| wrong/untrusted CA + expected hostname `wbd.test` | reject | **PASS** — rejected with `ASN no signer` |

0-RTT is disabled by build and no early-data API is used.

## Reproducibility

- C probe: `native/dtls/dtls13_probe.c`.
- Local build/test driver: `scripts/qualify_v2_m2_dtls13.sh`.
- Machine-readable receipt: `docs/security/data/v2-m2a-dtls13-receipt.json`.
- Case summary: `docs/security/data/v2-m2a-dtls13-cases.csv`.

The qualification driver re-extracts the exact locked source archive, regenerates Autotools files, configures/builds locally, generates a fresh test CA/server certificate, compiles the probe against that local static library, and runs all three cases.

## Next gate

M2B should turn the verified DTLS association into a bidirectional UDP datagram shim. First prove on plain UDP that each input datagram maps to an independently readable DTLS application record and that dropping one encrypted application datagram does not block a later record. Only then put that shim between UDPspeeder and udp2raw for the full V2-M2 0/1/5/10/15% qualification matrix.
