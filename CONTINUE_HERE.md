# Continue Here

This branch is the active development surface for wobuzhidao.

Do not infer the next task from old chat, local scratch state, old development logs or commit titles. Read, in order:

1. `docs/architecture/ADR-0012-logical-tunnel-address-lease-multipath-lifecycle.md`
2. `PROJECT_CONSTITUTION.md`
3. `ARCHITECTURE.md`
4. `ROADMAP.md`
5. `.wbd/handoff/current.json`
6. `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`
7. only the bounded `resume_read_set` named by the handoff

Before editing, refresh live HEAD/PR/Actions and reconcile commits after `checkpoint_based_on_head_sha`.

ADR-0012 is the current tunnel/lane lifecycle authority. In particular, do not continue per-LiveID netns/veth/double-NAT as final product work, do not restore `one entire VPN lifetime = one public flow`, and do not confuse the later Game Lane first-arrival/dedup design with the rejected V1 ordinary-kernel-TCP multilane architecture.

The handoff cursor is intentionally small: it says what just finished, what happens next, why now, what counts as done, and which GitHub development logs contain durable evidence/history needed to resume.

GitHub is the project recovery authority. Detailed development conclusions, failed experiments, physical-test evidence, qualification run IDs and architecture pivots must be recorded under `docs/development/` (or another repository path named by the handoff) before a task is considered complete. Chat history and external drive copies are convenience only and must never be required to reconstruct project state.
