# P5 sparse docs-only closure retry

- Date: 2026-09-21
- Branch authority: `next/tlslike-dataplane`
- Product SOURCE_SHA: `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e`
- Product Actions: `35524525129` / **8/8 PASS**
- Failed docs-only closure SHA: `190e3d03c26b5710077050536576871c06285aba`
- Failed docs-only closure Actions: `35524734259`
- Retry SOURCE_SHA: pending this commit
- Product qualification SHA must remain `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e`.

## Failure classification

The first sparse docs-only closure run completed 7/8 PASS.

The only failed job was the unchanged P2 kernel fallback gate:

`TestKernelTLSFallbackVerifiedHTTPAndNormalClose`

which ran for about 10 seconds and ended at `kernel_fallback_linux_test.go:318` with:

`unexpected EOF`.

Failed P2 artifact:

- ID `10609435991`
- size `5087` bytes
- digest `sha256:5cc80ca1267079627e86ed5014d47edf9e7b878eeece3c5a4dbd1de16b21b641`.

The docs-only closure SHA changed only `docs/STATUS.json` and its closure devlog. It did not change P2, the sparse harness, runtime transport, workflow, or production code.

The exact sparse product SHA `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e` had already passed the same P2 kernel fallback job in Actions `35524525129`, together with all seven other gates.

Therefore this retry does **not** modify P2 or reopen the sparse product atom. It only records the failed closure run and creates a fresh docs-only SHA for the complete workflow to rerun.

## Governance

- `last_tested_source_sha` remains the product qualification SHA `d9627cc9faf4ea66119f82a1c544ac0bb73bbe5e`.
- This retry SHA is docs-only and must never replace it.
- No `old/` code is reused; `docs/REUSE_LEDGER.json` is unchanged.
- Windows/Npcap physical remains P7 `NOT_RUN`.
- Production padding remains `0/off`.

## After a green closure retry

Proceed to P5 natural-concurrency as a separate atom. Do not fold natural concurrency code into this retry.
