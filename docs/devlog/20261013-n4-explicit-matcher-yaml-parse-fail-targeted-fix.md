# N4 explicit native matrix workflow YAML parse failure / targeted fix — 2026-10-13

Parent exact SOURCE `decb79c68d69f7b155899cedc4dde931edf66b3e`. GitHub [run 38358853661](https://github.com/lly8666/wobuzhidao/actions/runs/38358853661) completed/failure immediately with no jobs, workflow name displayed as its path rather than normal name. Inspection of exact remote YAML found new go-test selector line truncated before its closing single quote, causing workflow parse failure. This is a WORKFLOW/CI-INVALID failure, not a product test failure, and is preserved as FAIL without relabeling.

This candidate changes ONLY the broken line into a syntactically complete regex without end-of-line dollar anchor: `-run 'TestN4NativeTxnPartialGuardFinalizeOnlyAfterEmptyJournal|TestN4WFPPartialGuardDeletePlanAfterRouteCleanup' -count=3 -v`, still Windows2022/Windows2025 3 repetitions each. The WFP TEST-NET lab and prior software code remain unchanged. Await new SOURCE GitHub Actions, verify all jobs and selected cases; local Go testing forbidden.

N4 product still uses PowerShell and huge old CIDR routes; no Windows admin/real Wintun production WFP DNS/IPv6 and no unified network transaction. N2 C04/C05/C10/80s OPEN, N6 package NOT_STARTED, physical NOT_RUN. No deploy/merge.
