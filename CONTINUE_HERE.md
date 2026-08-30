# Continue Here

This branch is the active development surface for wobuzhidao.

Do not infer the next task from old chat, local scratch state, or commit titles. Read, in order:

1. `PROJECT_CONSTITUTION.md`
2. `ARCHITECTURE.md`
3. `.wbd/handoff/current.json`
4. only the bounded `resume_read_set` named by the handoff

Before editing, refresh live HEAD and reconcile commits after `checkpoint_based_on_head_sha`.

The handoff cursor is intentionally small: it says what just finished, what happens next, why now, what counts as done, and which GitHub development logs contain the durable evidence/history needed to resume.

GitHub is the project recovery authority. Detailed development conclusions, failed experiments, physical-test evidence, qualification run IDs, and architecture pivots must be recorded under `docs/development/` (or another repository path named by the handoff) before a task is considered complete. Chat history and external drive copies are convenience only and must never be required to reconstruct the project state.
