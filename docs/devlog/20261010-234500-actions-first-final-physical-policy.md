# Development governance update — all feature validation in Actions first, one final physical acceptance

## Authority / exact source
- User instruction on 2026-10-10: **all functions are developed and tested in Actions first; only after every function is finished and all corresponding Actions work is complete should physical-machine tests be done, uniformly at the end.**
- Verified target branch `next/adaptive-fec-aes-tun-20261010` parent HEAD: `9d597cdce6d32128365340e0f9d95a384444e7b1`, Git tree `5d47f95602745c6171a79157ca371dc26b54afeb`. Tested product SOURCE remains `b52ed0edad655baaed0411c594f85fe3d82bb79a` (N0 single-client realpath scoped PASS), not this policy-only commit.
- The **new policy document SOURCE** is the Git SHA of the commit containing this devlog and the sole STATUS update; it cannot self-contain its own commit hash. It is not a new product acceptance claim.
- Parent docs-only SOURCE `9d597cdce6d32128365340e0f9d95a384444e7b1` next-foundation [38064279578](https://github.com/lly8666/wobuzhidao/actions/runs/38064279578): **SUCCESS** (scope foundation only). No physical trial and no new functional acceptance represented here.

## Mandatory sequence, applied to N0–N6
1. Complete feature code increment/stage and run corresponding compile/unit/race/fuzz/functional/integration/performance as appropriate **on GitHub Actions**, preserving exact SOURCE, configuration, seed and artifacts. Fix failures and re-run without overwriting older FAIL. Do not skip planned Actions or treat an isolated unit PASS as a product PASS.
2. Repeat in STATUS order N0→N1→N2→N3→N4→N5→N6, finish all cross-feature Actions, performance/capacity receipts and three-platform same-SOURCE P6 packages/manifest hashes. Maintain the per-performance-Action **one source/config/seed/scenario and one measurement job** rule; the historical SIMD serial exception is not available.
3. Before this full Actions and packaging closure: **no intermediate physical tests, no test deployment or automatic update on users' existing machines, no mainline merge or qualification-ref movement**. Linux/mock cannot substitute for real Windows TUN evidence; record runner-driver restrictions as UNSUPPORTED/NOT_RUN and enumerate those checks for the final physical acceptance, not as PASS.
4. Only after complete functions/Actions/P6 acceptance: set **ACTIONS_READY_FOR_PHYSICAL**, keep `physical=NOT_RUN`, and hand off to the **original chat** for one coordinated final physical-machine testing stage. Physical tests never replace the preceding Actions work.

## Source files updated atomically with this rule
- `AGENTS.md` and `PROJECT_CHARTER.md`: permanent collaboration / safety boundary.
- `docs/ADAPTIVE_NETWORK_PLAN.md`: section 9.1 stage gates and final two-gate workflow.
- `docs/AGENT_CONTINUITY.md` and `docs/templates/ADAPTIVE_NETWORK_AGENT_PROMPT.md`: durable handoff instructions for future agents.
- Sole `docs/STATUS.json`: acceptance_sequence machine-readable policy; preserves N0 active_work stage, the real next_task, prior exact-SOURCE Actions statuses and all historical failures; latest_log points here.
- `docs/evidence/adaptive-actions-first-final-physical-rule-20261010.json`: rule-decision receipt, not test PASS.

## Explicitly unchanged
- N0 remains **IN_PROGRESS**: 3 simultaneous real-netns clients, independently observed effective profile/cipher and first downlink-before-uplink still needed. N1–N6 not started. Do not schedule physical trial just because N0 single-client Action passed.
- Existing PASS/FAIL results, native Windows driver limitations, and the previously deferred ~80-second S2C plus multi-second late probe remain unchanged.
- This is a governance/docs-only edit, not compiled or tested locally. Actions may run due to repository policy hooks; report them separately if triggered. No deployment, secrets, packet captures or binaries uploaded.
