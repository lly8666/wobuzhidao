# Native ARM profiling Actions YAML failed before jobs; exact command repair

- [Actions 38049682130](https://github.com/lly8666/wobuzhidao/actions/runs/38049682130) failed **before job creation**: JS replacement expanded `$'` in shell literal `-run '^$'` and made the YAML multiline command invalid. This is NOT an ARM64 or product test failure. Both native hotspot and checksum bench NOT_RUN.
- Repaired two exact commands to preserve shell `-run '^$'` and `-run '^TestTCPChecksumIndependentOracleExtended$'`. Scope only this new core workflow and docs/evidence; no product/FEC/headerMask/AEAD/MTU code changes. Full post-header-mask rollback Actions core38049393333/foundation38049393295/lifecycle38049393283 SUCCESS.
- Future native ARM profile is synthetic short/MTU only, not business. No ARM Poly1305 accelerator or checksum optimization before such evidence. Original weaknet Q2 probe FAIL and physical NOT_RUN unchanged.
