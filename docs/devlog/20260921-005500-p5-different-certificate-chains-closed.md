# P5 different certificate chains atom closed

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `e2c1d6685f678df8fa4157f97a29de54ef84b803`
- GitHub Actions: `35524123958`
- Result: **8/8 PASS**
- Previous product qualification: `1fb65c90b272454e09a624eaa23e6a432de7955a`
- This closure commit is docs-only and must not replace the product qualification SHA.

## Closed atom

The fourth P5 atom is closed: controlled real inner-HTTPS measurement across two independent certificate trust chains while preserving the previously-qualified full/resumed and sequential-close behavior.

The exact qualified flow sequence is:

1. chain A, TLS 1.3 full handshake;
2. normal HTTP/TLS close and both platformflow TCP registries return to zero;
3. chain A, TLS 1.3 resumed handshake with a real session-cache hit;
4. normal close and retirement;
5. chain B, TLS 1.3 full handshake with an independent cache and no resume hit;
6. normal close and retirement.

All three business flows use one already-established outer FakeTCP + protected admission Normal lane. The client initial outer SYN count remains exactly one.

## Certificate construction and verification

The two controlled chains are independently generated with system cryptographic RNG:

- `chain-a`: Root A -> Intermediate A -> leaf A for `target.test`;
- `chain-b`: Root B -> Intermediate B -> leaf B for `target.test`.

Each HTTPS server sends leaf + intermediate. Its corresponding client trust store contains only that chain's root.

Every TLS handshake requires Go's normal certificate verification to produce exactly one verified path of length 3. The harness then checks the verified leaf, intermediate, and root DER SHA-256 fingerprints against the scenario identities recorded in the manifest. The validator independently cross-checks those same fingerprints against each flow event.

The two scenarios are required to have different root, intermediate, and leaf identities. This is a real independent trust-chain test, not two labels applied to one self-signed certificate.

## Preserved handshake regression

The atom preserves the third P5 atom rather than replacing it:

- flow 1 / chain A: `DidResume=false` and real session-state Put;
- flow 2 / chain A: `DidResume=true` and real cache Hit;
- flow 3 / chain B: `DidResume=false`, own session-state Put, zero resume hits.

Therefore flow 1 versus flow 3 are the controlled full-handshake certificate-chain comparison, while flow 1/2 continue to guard full/resumed behavior.

## Exact-SHA evidence

The dedicated P5 gate emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_CAPTURED source_sha=e2c1d6685f678df8fa4157f97a29de54ef84b803 flows=3 outer_connections=1 sequential_close=pass handshakes=full,resumed,full certificate_chains=2 fec=off padding=off records=65`

The independent validator emitted:

`WBD_P5_HTTPS_MEASUREMENT_BASE_PASS source_sha=e2c1d6685f678df8fa4157f97a29de54ef84b803 flows=3 outer_connections=1 sequential_close=pass handshakes=full,resumed,full certificate_chains=2 fec=off padding=off`

P5 artifact:

- ID `10609303454`
- size `30758` bytes
- digest `sha256:029ce15d396bf5400be547e669b7efb1c6416f2e51a256d8a107f4ebef379fc6`
- raw PCAP / JSONL events / manifest / summary / test log uploaded.

The full `next-foundation` workflow also passed repository-contract, Windows active Go tests, Ubuntu unit/build/race/directed fuzz/reference, P2 kernel fallback, OpenWrt privileged TPROXY/SocketTunnel, and Linux shared-TUN privileged iptables/nft on the same SHA.

## Non-claims

- These controlled test chains do not claim to reproduce a target website's certificate chain or complete server TLS fingerprint.
- No sparse or natural-concurrency qualification yet.
- No weak-network, load, or soak result.
- No FEC-on result.
- No classifier or indistinguishability claim.
- Production padding remains `0/off`.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- OpenWrt IPv6 TPROXY/capture remains `NOT_IMPLEMENTED`.
- Existing runtimeowner cumulative ACK / 4096 metadata / 1s default RTO / 3s absolute repair horizon remains unchanged.
- No `old/` implementation was reused; `docs/REUSE_LEDGER.json` is unchanged.

## Next atom

Cover only the controlled **sparse single-connection** workload dimension.

The workload should keep one inner TLS/HTTP connection alive, issue a small number of sequential requests with an explicit recorded application-arrival schedule, and measure the resulting request gaps, outer packet/record intervals, bursts, business latency, and wire bytes.

Any intentional request spacing belongs only to the measurement workload generator. It must not become transport batching, a production fixed sleep, or waiting for other business traffic. Keep one certificate chain, TLS 1.3, FEC off, padding 0/off, and no network injection. Do not mix in natural concurrency, FEC-on, weak-network matrices, classifier work, or performance conclusions.
