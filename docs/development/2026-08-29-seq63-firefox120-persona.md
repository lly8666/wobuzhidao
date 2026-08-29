# Sequence 63 — Firefox 120 single-flow TLS persona experiment

Date: 2026-08-29
Experiment branch: `exp/single-flow-utls-firefox120`
Base feature branch: `feat/single-flow-reality-faketcp`

## Frozen requirements

- One public TCP-shaped flow only: one SYN lineage and one public 4-tuple per WBD session.
- FakeTCP owns the public sequence space from SYN through teardown.
- The first seconds are a Reality-like/TLS-like setup phase on that same FakeTCP association.
- The mode switch must not FIN, RST, redial, or create a second public TCP flow.
- Sustained payload remains pinned wolfSSL DTLS 1.3 + immutable LINK/FEC and must not inherit kernel-TCP HOL.
- The mature FakeTCP sender/receiver/recovery/FEC data plane is frozen for this experiment.

Sequence 62 already qualified the single-flow architecture with one SYN/one tuple, same-flow TLS admission, inherited DTLS 1.3, 20 bidirectional echoes, and the 20% loss gate. This sequence changes only the setup TLS persona.

## Why this experiment exists

The sequence-62 client used Go `crypto/tls` for the setup ClientHello. It is valid TLS 1.3, but its cipher/extension/GREASE/order fingerprint is recognizably Go rather than a browser. The user requirement is that the first seconds look as close to normal Reality/browser TLS as technically practical while keeping the same raw FakeTCP association and preserving the no-HOL steady-state design.

## Chosen experiment

Pin `github.com/refraction-networking/utls v1.6.5` and use `utls.HelloFirefox_120` only for the setup ClientHello.

Construction rule:

1. Build the pinned Firefox 120 ClientHello on the already-established FakeTCP bootstrap stream.
2. Keep the preset-generated TLS random.
3. Compute the existing WBD route marker from that random and server name.
4. Replace only the normal 32-byte TLS 1.3 compatibility SessionID with the WBD marker.
5. Re-marshal the same preset and perform TLS 1.3/auth on the same FakeTCP connection.
6. After admission, leave the public association intact and continue into the existing DTLS/FEC data plane.

No FakeTCP sender/receiver/recovery files are changed by the persona experiment.

## Commits and findings

- `70588b869a3e9508878e7bffdd8b39bfafb65e9d` — pin uTLS v1.6.5.
- `f05b83c9be065863b09a69680a2a86a4ea0ffb43` — use Firefox 120 persona for the single-flow TLS client.
- `da4caa7a3ac053f8708fd40eab03e36dbc9eb104` — test that the WBD SessionID marker does not change Firefox cipher suites, supported versions/groups/signatures, ALPN, or extension type/order.
- First normal CI attempt failed before compilation because the repository previously had no `go.sum`; Go's readonly module mode correctly rejected the missing pinned dependency checksums.
- A focused runner generated the exact Go 1.23 module graph and checksums. They were then committed rather than guessed:
  - `f500aca27a76416b91eff0646991c40bc5f0bb1b` — lock module graph.
  - `01c6675d2239a5514222b22ea2ded5a2625ecaee` — add `go.sum`.
- The first focused TLS admission test then failed with TLS `handshake_failure`.
- Root cause was deterministic and test-only: the legacy helper generated an Ed25519 server certificate, while the pinned Firefox 120 signature-algorithm list does not advertise Ed25519. The browser persona therefore had no compatible server signature scheme.
- `27c682e6c58cd3aab88afc45f356a1309a6286df` — use ECDSA P-256 for the browser-persona admission test while preserving the old Ed25519 tests for the legacy front.
- Focused workflow `exp-single-flow-utls` run `33240826775` completed `success`: module resolution, `internal/realityfront` tests, and `wbd-faketcp`/mux builds passed.
- `4b03de1e93a560ac6fd37a78527f10bb70666d36` — convert the temporary dependency resolver into a permanent `go mod tidy` + zero-diff module-graph gate.
- `291ed571c15f3f02072dbdcae7773d3cc047c059` — enable the existing full `single-flow-e2e` workflow on this experiment branch so the browser persona is tested over the real namespace/raw-FakeTCP/inherited-wolfSSL path and a public pcap is captured.

## Current qualification plan

The experiment is not accepted on source-level tests alone. Acceptance requires all of the following on one experiment HEAD:

1. `exp-single-flow-utls`: pinned module graph zero-diff, persona unit tests, FakeTCP/mux builds.
2. `single-flow-e2e`: one SYN / one public 4-tuple, same-flow TLS bootstrap, inherited DTLS 1.3, 20 bidirectional echo packets, wrong-marker same-flow TLS fallback.
3. Analyze the captured public pcap with `scripts/analyze_single_flow_persona_pcap.py`.
4. Compare the normalized ClientHello ciphers/extensions/groups/signatures/versions/ALPN and JA3 against the pinned Firefox 120 source, not a dynamically generated self-reference.
5. `faketcp-pcap-20loss`, `faketcp-native`, main `ci`, Windows cross-build/portable, and Linux release must remain green before promotion.

The only intentional TLS-persona difference from the pinned Firefox preset is the content of the normal 32-byte compatibility SessionID used as WBD's route classifier. SessionID content is not part of JA3.

## Current status

Focused Firefox 120 TLS admission is green. Full single-flow E2E run `33240969496` has been triggered for experiment HEAD `291ed571c15f3f02072dbdcae7773d3cc047c059`; persona pcap evidence is pending at the time of this checkpoint.

Do not request a physical Windows/Ubuntu test from the user until the sandbox/CI persona and cross-platform qualification above are complete.
