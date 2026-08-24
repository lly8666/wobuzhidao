# Fresh Agent Bootstrap

This file is the stable zero-context entry point for `lly8666/wobuzhidao`.

## Fixed takeover phrase

> 接管 wobuzhidao（lly8666/wobuzhidao）：按仓库实时 handoff 恢复项目边界、active branch、当前架构、上游锁定版本、本地测试状态和 continuation cursor；从 next_atomic_action 继续，不要让我重复背景。

## Cold-start route

1. Open GitHub issue **#1 — wobuzhidao live handoff / session bootstrap**.
2. Resolve the active development branch from that rendezvous; never guess it from old chat.
3. On the active branch, read `CONTINUE_HERE.md`.
4. Read `PROJECT_CONSTITUTION.md` and `ARCHITECTURE.md`.
5. Read `.wbd/handoff/current.json`.
6. Refresh the live branch HEAD. Compare it with `checkpoint_based_on_head_sha` and classify every intervening commit before changing code.
7. Read only the bounded paths in `resume_read_set` plus files directly required by the current atomic task.
8. Inspect `.wbd/handoff/data-plane.json`; restore only assets with `required_for_current_task=true`, and verify size/SHA-256 before execution.
9. Execute `continuation_cursor.next_atomic_action` until its `done_when` condition is met.
10. Before returning control, run local tests, commit substantive work, update receipts/data-plane if needed, then update `current.json` in a handoff-only commit.

## Authority rules

- Repository state beats chat recollection.
- Live branch/HEAD beats stale handoff snapshots, but unclassified substantive deltas must be reconciled before continuing.
- Local sandbox qualification is the final test authority. GitHub Actions are build/network/CI assistance only.
- Git stores source, text state, hashes and receipts. Durable binaries belong in Google Drive and are referenced by exact Drive file ID + SHA-256.
- Do not ask the user to repeat context already recoverable from the repository.
