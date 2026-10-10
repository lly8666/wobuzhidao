# N0 Actions receipt contract correction; no qualification relabel

## SOURCE and observed run
Parent HEAD `da885bcee82151168fdcd6d4d719ce21994ddcab` (tree `23b75ac627e626d6eb3fffe652c532c8c5a651b4`). New exact SOURCE is the GitHub SHA of this commit including this devlog, sole STATUS, and evidence; it cannot contain its own SHA.

Actions next-foundation [38061204386](https://github.com/lly8666/wobuzhidao/actions/runs/38061204386), SOURCE da885bce: **FAIL** repository-contract job `114239659803`, before Go tests were unblocked. `python tools/check_repository.py` returns `Incomplete Actions receipt` twice: the two historical FAIL receipts added in the prior STATUS lacked mandatory `source_sha,url,scope,status` keys. Both historical first-SOURCE foundation and lifecycle FAIL remain FAIL; do not delete them or change any original observation. Other jobs of this source skipped because the guard failed, not product PASS.

This source corrects both receipt entries to the schema enforced by unchanged `tools/check_repository.py`, and appends the second run's contract failure as its own evidence row. No modification of product code or historical acceptance/analysis workflows. Inherited baseline product 7fb98fab; N0 only partial, N1..N6 not implemented/qualified.

No local Go/CPU/performance testing; all execution in Actions. Candidate SOURCE Go unit/race/Windows and realpath are NOT_RUN at authoring; next-foundation and next-lifecycle must be checked again. Physical deployment, P6, native Windows TUN/direct, CPU flags/quota/PSI/injection/drop remain NOT_RUN. Deferred ~80s S2C and multi-second late probes remain OPEN.

Next: inspect the new exact-source Actions logs, fix actual failures without widening product scope; then finish production per-client N0.
