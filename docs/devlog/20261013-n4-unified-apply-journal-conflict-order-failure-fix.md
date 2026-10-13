# N4 unified network WAL concurrent Apply check-order regression — 2026-10-13

## Original failure (must remain FAIL)
Exact code SOURCE `0ddcfefcf4be6b22a69457828a4c205345368278`, N4 native matrix [38362537980](https://github.com/lly8666/wobuzhidao/actions/runs/38362537980). Linux -race job115179896427 **failure** in `TestN4NativeTxnUnifiedExclusiveCreateCollision` on four count repeats: concurrent Apply on the same path yielded `ErrN4ForeignRoute`, not the required journal-conflict refusal. Cause is deterministic: after the first full successful Apply, later waiters entered `Stage.Snapshot` and saw current owned OS state before the code tried journal O_EXCL. This was a genuine test FAIL; do not reclassify or reuse its limited passing subtests to claim whole source PASS.

## Narrow code fix
Inside `N4NativeUnifiedTxn.Apply`, after the mandatory cross-process exclusion Gate and live owner verification, check existing journal via `os.Stat` **before any OS stage Snapshot** and return `ErrN4JournalConflict`; return non-ENOENT Stat errors instead of continuing. Still use O_EXCL journal creation after bounded snapshots as the final authoritative create race check, no unsafe takeover. Add `TestN4NativeTxnUnifiedExistingJournalRejectedBeforeSnapshots` to prove repeated Apply neither mutates nor queries stage baselines. Prior suite's 12-concurrent collision test remains strict.

## Qualification & limitations
At commit time this targeted new SOURCE is ACTIONS_PENDING. Need full N4 Windows2022/2025 (x3), Linux race (x4) plus same-source Foundation and Lifecycle final jobs/artifact digests. No actual NIC/DNS/MTU/lease/WFP mutation, still generic software stage scaffold. No new configuration or performance sample. Missing native cross-process lock, owner generation fencing, typed Win32 stage backends and administrator driver tests; product still PowerShell/CN CIDR complements. No physical, N6 packaging or deploy/merge. N2 historical C04/C05/C10/80s remain OPEN.
