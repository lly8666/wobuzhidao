# WBD live handoff protocol

This directory is the machine-readable continuity boundary for long-running development sessions.

## Authority order

When resuming work, use this order of authority:

1. live GitHub branch `dev/wbd-raw-fec-v2` and open PR #3;
2. `.wbd/handoff/current.json` on that live branch;
3. commits between `checkpoint_based_on_head_sha` and the current branch HEAD;
4. current GitHub Actions status and artifacts for those commits;
5. `PROJECT_CONSTITUTION.md`, `ARCHITECTURE.md`, `ROADMAP.md`, active ADRs and pinned dependency lock files;
6. chat history only as secondary context.

Never ask the user to repeat project background that can be recovered from these sources.

## Takeover procedure

A new agent must:

1. refresh the live branch HEAD, PR #3 state and `.wbd/handoff/current.json` before making changes;
2. confirm `active_branch`, `active_issue`, `active_pr` and `continuity_sequence`;
3. compare `checkpoint_based_on_head_sha` to live HEAD and classify every intervening commit as known development, handoff-only, external/user change or unexpected divergence;
4. read every file in `resume_read_set` plus any file named by `continuation_cursor.next_atomic_action`;
5. restore the current architecture, product boundaries, dependency pins, qualified transport behavior, current CI state and benchmark authority before editing code;
6. inspect the latest relevant Actions runs rather than assuming a previously queued run finished successfully;
7. continue directly from `next_atomic_action`, preserving the stop rules and explicit non-goals in the handoff;
8. keep substantive code/test/docs commits separate from the final handoff-only commit whenever practical;
9. at the end of every development turn, refresh live HEAD again, increment `continuity_sequence`, set `checkpoint_based_on_head_sha` to the latest substantive checkpoint, update the continuation cursor and run `handoff-verify`;
10. if `handoff-verify` fails because repository truth changed, fix the stale contract/test rather than downgrading correct architecture documentation.

## Benchmark authority

A benchmark result is authoritative only when its handoff entry records the exact Actions run ID, setup/measurement impairment boundary and relevant commit. Failed, partially uploaded, pre-fix or mixed-harness runs may be retained as diagnostics but must be labeled non-authoritative.

For the current two-session 100 Mbit/s ceiling work:

- session setup uses RTT/rate shaping with `setup_loss_pct=0`;
- random `measurement_loss_pct=20` is enabled only after both LINK sessions are ready and before the offered interval;
- `off` and fixed systematic `20:20` use the same Reality/FakeTCP/DTLS/LINK stack;
- load ceiling sweeps compare aggregate inner 40/60/80 Mbit/s (20/30/40 per session) at RTT 20/100 ms;
- do not retune FEC or FakeTCP recovery during the ceiling sweep;
- stop raising load when persistent queue/tail collapse or material cross-session unfairness appears.

## Final handoff commit rule

The last repository write of a development session should normally be a handoff-only update to `.wbd/handoff/current.json`. Its `checkpoint_based_on_head_sha` points to the latest substantive code/test/docs commit immediately before it. If a verifier fix is required after that handoff commit, make the verifier fix as a substantive commit, then issue a new incremented handoff commit.

`data-plane.json` remains the bootstrap binary-asset manifest. Keep it empty unless a future task truly requires external binary state that cannot be reconstructed from the repository and pinned upstream sources.
