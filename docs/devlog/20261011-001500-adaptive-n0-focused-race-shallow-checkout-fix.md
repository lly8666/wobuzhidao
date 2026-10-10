# N0 focused race harness failure: shallow Git checkout, not a Go test result

## Exact source and original failed receipt
- Work branch next/adaptive-fec-aes-tun-20261010, parent SOURCE `62e66ddddc53e6cee6459578b7db353913419a33`.
- [next-adaptive-n0-negotiated run 38066640082](https://github.com/lly8666/wobuzhidao/actions/runs/38066640082) **FAIL**, job `114255508333`. Checkout@v4 default depth 1 did not fetch historical baseline object `b5c848f4e9afdffd15d1bc451560edf4e9390a35` required by `tools/check_repository.py`; exact stderr: `fatal: not a tree object`. No Go unit or focused race step executed (skipped), so this FAILURE is harness setup, NOT a proved source-code regression nor functional PASS.
- [next-lifecycle run 38066640027](https://github.com/lly8666/wobuzhidao/actions/runs/38066640027) **PASS** on `62e66ddd` for existing lifecycle + Go correctness. Parent `0124cc4` foundation+life both PASS. `62e66ddd` foundation still pending at fix decision; do not overstate.
- New fix commit SOURCE = containing commit. One workflow-only executable change, rest docs/receipt: add `with: fetch-depth: 0` to the existing scoped Actions workflow checkout so repository authority uses actual history without relaxing its checker. Functional Go test remains the SAME targeted test and 2 race repetitions, no workaround or skipped gate.

## Boundaries and next step
- No local build or test. Trigger corrected workflow once via the exact branch/path push and inspect job output. Preserve this FAIL in STATUS.actions/evidence, regardless of later PASS.
- N0 actual TLS suite is still not **targeted-race qualified** until that Actions step passes. Real three concurrent native client processes, independent leases+same server port, real first S2C/same-wire still NOT_RUN. Current partial in-memory Go and one older single-realpath PASS are separate evidence.
- Not a performance sample. No physical testing, user deployment, merge, credentials, packet captures or qualification-ref movement. All N0–N6 features/Actions/P6 must complete before the coordinated physical acceptance.
