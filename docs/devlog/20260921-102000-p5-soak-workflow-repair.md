# P5 soak workflow text repair

- Date: 2026-09-21
- Bad workflow commit: `b4695ab5ab4161238ddb1d85d6fa8e50208256a1`
- Actions: `35587800818`
- Result: workflow-level immediate failure, zero jobs

The previous timeout-only edit accidentally damaged the YAML shell block while doing a multiline text replacement. The committed workflow blob was `f25885952ce3fbd1929edd28837755761f8da00e`.

The workflow is rebuilt from the last structurally valid version and only this command token is changed:

`-count=1` -> `-count=1 -timeout=40m`

The rebuilt workflow blob is `7af2cad5ff13a216b7a33b6451a62ff09dcfd640`.

No product code, soak scenario, validator threshold, or acceptance criterion changes.
