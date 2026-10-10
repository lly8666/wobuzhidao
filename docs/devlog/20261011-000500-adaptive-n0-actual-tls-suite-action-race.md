# N0 actual outer TLS handshake suite: separate record policy and focused Actions race

## Exact-source state
- Branch `next/adaptive-fec-aes-tun-20261010`; verified parent/source `0124cc43f8b1cd2ac669e3cc40bf1d325f43b6e1`.
- Parent `next-foundation` [38066319972](https://github.com/lly8666/wobuzhidao/actions/runs/38066319972) PASS and `next-lifecycle` [38066320007](https://github.com/lly8666/wobuzhidao/actions/runs/38066320007) PASS. They verify parent three-client deterministic first S2C business + negotiated V3 policy in Go (among other tests), not native Linux 3 clients, independent focused -race or suite observer in this new candidate.
- Latest *one-client* native five-netns realpath PASS is still separate SOURCE `b52ed0edad655baaed0411c594f85fe3d82bb79a`, [run 38064016024](https://github.com/lly8666/wobuzhidao/actions/runs/38064016024), not qualification for this code.
- New candidate SOURCE is the commit containing this log; tests in newly scoped `next-adaptive-n0-negotiated` are PENDING until Action executes. New code must not be called PASS until that exact source qualifies.

## Source changes
- `internal/realityfront/admission.go`: copy the **actual negotiated outer TLS handshake CipherSuite** (`uTLS.ConnectionState().CipherSuite` client, `tls.Conn.ConnectionState().CipherSuite` server) into local-only `AdmissionResult.TLSCipherSuite` after protected admission and key exporter derivation. This metadata is not encoded, authenticated separately or included in the exporter; the TLS handshake itself authenticated the suite. It contains no exporter key, auth, ticket or business bytes.
- `internal/runtimeentry/lifecycle.go`: publish actual TLS suite under the already-used client/server locks after successful attached admission, alongside policy/version. Latest successfully attached lane wins on multipath/rotation, not a hard claim that every lane negotiated the same TLS suite.
- `internal/runtimeentry/diagnostic.go`: expose opt-in per-Tunnel `tls_cipher_suite` code alongside `admission_policy.cipher`. They are explicitly distinct: TLS handshake cipher vs independent-record AEAD; decoy alignment is not claimed, unsupported/unknown remains 0 until authenticated success.
- `internal/runtimeentry/adaptive_v3_mix_test.go`: assert server/client observed nonzero outer TLS CipherSuite and agreement in each of three independently authenticated installations, in addition to policy+lease+actual parity and first S2C before any client business.
- `.github/workflows/next-adaptive-n0-negotiated.yml`: branch/path-scoped **single functional job** for focused `go test -race ... TestV3ThreeClientsIndependentFirstBothDirectionsAndRotation -count=2`, and protected admission/record unit regression; no performance measurements or extra sample. Maintain existing foundation/lifecycle workflows and fixed performance one-sample rule.

## Scope and next work
- This is a partial N0 advance, not proof of 3 native real Linux client binaries simultaneously: existing in-memory three client simulation uses real TLS/FakeTCP code but an in-memory carrier, not real TPROXY and shared Linux TUN.
- S2C-before-business test allows transport health heartbeat; authenticated transport control is *not* user business. The test does not establish a real WAN first server payload before any client TPROXY traffic.
- No AES, auto FEC, performance/p99 credential, Win Wintun/native direct, P6 or physical test. Native 3-client same-port test, real per-client suite/profiles and same-Seq same-wire remain N0 gates.
- If focused race fails, preserve FAIL and diagnose with actual job log before fixing; do not rewrite current or previous PASS. No host work, secrets or large pcaps. User's **Actions before final unified physical** governance still applies.

## Follow-up acceptance
Verify `next-adaptive-n0-negotiated` focused -race Action plus automatic foundation/lifecycle on the same exact SOURCE. Only if successful advance the native 3-client real-netns functional harness; otherwise correct the failure first. `STATUS.json` remains sole authority.
