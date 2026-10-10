# Q1 config-only trigger aborted in helper static phase (no business)

- [Actions 38048479344](https://github.com/lly8666/wobuzhidao/actions/runs/38048479344) job114202679537: exact three-file guard PASS, repository/policy PASS, **new own helper test FAIL** only: test_crypto_packet_abba.TestCryptoPacketABBA expected `preflight` even when config phase explicitly `q1screen120`. All product binaries, network and 120s legs NOT_RUN. Preserve this result, do not claim actual product loss or latency.
- Change only this unit's assertion to membership in the explicitly allowed phase set, leaving runner `conf()` exact schema/source/phase checking intact; no business gate relaxation. Product A and B frozen untouched, original FEC and 80s issue unchanged.
- Next exact config-only push with incremented nonce to retry Q1 A→B→B→A. This commit does NOT trigger measurement.
