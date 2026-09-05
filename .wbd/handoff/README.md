# WBD live handoff protocol

This directory is the machine-readable continuity boundary for long-running development sessions.

## Authority order

When resuming work, use this order of authority:

1. live GitHub branch `feat/single-flow-reality-faketcp` and open PR #9;
2. `.wbd/handoff/current.json` on that live branch;
3. commits between `checkpoint_based_on_head_sha` and the current branch HEAD;
4. current GitHub Actions status and artifacts for those commits;
5. `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, `ROADMAP.md`, active ADRs and pinned dependency lock files;
6. dated evidence/history under `docs/development/` that is named by the live handoff;
7. chat history only as secondary context.

Never ask the user to repeat project background that can be recovered from these sources.

## Takeover procedure

A new agent must:

1. refresh the live branch HEAD, PR #9 state and `.wbd/handoff/current.json` before making changes;
2. confirm `active_branch`, `active_issue`, `active_pr` and `continuity_sequence`;
3. compare `checkpoint_based_on_head_sha` to live HEAD and classify every intervening commit as known development, handoff-only, external/user change or unexpected divergence;
4. read every file in `resume_read_set` plus any file named by `continuation_cursor.next_atomic_action`;
5. restore the current architecture, product boundaries, dependency pins, qualified transport behavior, current CI state and benchmark/physical-evidence authority before editing code;
6. inspect the latest relevant Actions runs rather than assuming a previously queued run finished successfully;
7. continue directly from `next_atomic_action`, preserving the stop rules and explicit non-goals in the handoff;
8. keep substantive code/test/docs commits separate from the final handoff-only commit whenever practical;
9. at the end of every completed development phase, refresh live HEAD again, increment `continuity_sequence`, set `checkpoint_based_on_head_sha` to the latest substantive checkpoint, update the continuation cursor and run the repository handoff verifier if present/applicable;
10. if a verifier fails because repository truth changed, fix the stale contract/test rather than downgrading correct architecture documentation.

## Evidence authority

A benchmark or qualification result is authoritative only when its handoff/development record identifies the exact source SHA and the relevant execution/evidence boundary. Failed, partially uploaded, pre-fix, mixed-SHA or mixed-harness runs may be retained as diagnostics but must be labeled non-authoritative for release qualification.

For physical Windows qualification, keep these boundaries explicit:

- hosted CI/build success is not physical Windows 11 + Npcap -> Ubuntu ARM64 acceptance;
- a public `:443` capture can prove outer FakeTCP/Reality-like TLS/DTLS/LINK transport without proving Wintun/shared-TUN/application E2E;
- `connect_pass` or control-plane READY markers do not by themselves prove DNS/UDP/TCP application traffic;
- firewall RST-suppression counters are not substitutes for packet/application evidence;
- evidence from different source SHAs must not be stitched into one release candidate.

Older benchmark-specific parameters remain in their dated `docs/development/` records; they are not automatically the current task merely because they once appeared in this protocol file.

## Final handoff commit rule

The last repository write of a development session should normally be a handoff-only update to `.wbd/handoff/current.json`. Its `checkpoint_based_on_head_sha` points to the latest substantive code/test/docs commit immediately before it. The handoff-only commit cannot self-reference and must not be used as the substantive `current_head_sha`/checkpoint.

If a verifier fix is required after that handoff commit, make the verifier fix as a substantive commit, then issue a new incremented handoff commit.

`continuity_sequence` increments once per completed development/handoff phase, not once per ordinary intervening commit.

`data-plane.json` remains the bootstrap binary-asset manifest. Keep it empty unless a future task truly requires external binary state that cannot be reconstructed from the repository and pinned upstream sources.

Detailed development conclusions, failed experiments, physical-test evidence and architecture pivots belong under `docs/development/` (or another repository path explicitly named by the live handoff). This repository currently does not define a separate `.wbd/handoff/history/` subsystem; do not invent one without an explicit protocol change.
