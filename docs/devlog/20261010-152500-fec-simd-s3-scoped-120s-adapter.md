# FEC SIMD S3 — 120 秒阶段实际波形受限适配

- Parent: `89fcb5e99ffc6ae63354ea6628d367ada72d6bed`. S2 scoped core [38032963434](https://github.com/lly8666/wobuzhidao/actions/runs/38032963434) Linux amd64, Windows amd64, native Linux ARM64 jobs success; foundation [38032963574](https://github.com/lly8666/wobuzhidao/actions/runs/38032963574) and lifecycle [38032963427](https://github.com/lly8666/wobuzhidao/actions/runs/38032963427) success. Real CPU gain / ARM performance remain NOT_MEASURED.
- New narrow `--simd-ab` used only in copied generated five-netns harness, stage and analyzer. Supports 120s 30/60/30 or 300s 75/150/75 loss 5→20→5, with actual qdisc event counters and send timestamp phase metrics. Old FEC_POLICY and 36-case sweep do not opt in. CPU sampling/delivery/3s drain and 32ms/3s product policy are untouched.
- Sender's stage classification now derives from configured test duration, preserving old 300s boundaries. New parser recognizes every FEC fixed parity mode without expanding non-SIMD approved tuples.
- This helper edit is not a new product SOURCE and not a performance PASS. Next: commit standalone strict A/B runner/workflow, static tests, then pilot; evidence and failures must be persisted.
- Historical RTT/Game4/300ms TCP-off/80s S2C FAIL/OPEN and physical NOT_RUN unchanged.
