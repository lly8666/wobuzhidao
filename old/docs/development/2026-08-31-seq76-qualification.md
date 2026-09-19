# 2026-08-31 — Sequence 76 exact-head qualification attempts

## Scope

This log records exact-head qualification after the ADR-0014 repository-authority recovery. No FakeTCP/TCP-like recovery/FEC product code is changed by the failures or fixes documented here.

## Candidate A — invalidated by missing durable devlog

- candidate SHA: `8a6041bd0b82f11ead14395504273086d5fe3352`
- qualification aggregator run: `33392037082`
- handoff-verify run: `33392037115`
- handoff job: `99487704225`

Failure:

```text
HANDOFF_VERIFY_FAIL: resume_read_set paths missing: ['docs/development/SINGLE_FLOW_DEVLOG.md']
```

Interpretation: repository recovery metadata referenced a durable development log that had not yet been created. This was not a transport/product failure. The missing log was created at commit `920e23b24b94bffb3e0af96e901b87e3dbb8803f`, invalidating candidate A because the branch moved.

## Sequence 76 handoff

- handoff commit: `5618ea929d8c73b6809c109e04e8a7b8ffac0685`
- handoff sequence: `76`
- checkpoint: `920e23b24b94bffb3e0af96e901b87e3dbb8803f`

The handoff explicitly marks candidate A as non-live/non-product evidence and requires a new immutable qualification kick.

## Candidate B — invalidated by over-exact handoff contract wording

- candidate SHA: `d16e1fbef0414a4fc36bea029052795d94f1e063`
- qualification aggregator run: `33392466254`
- handoff-verify run: `33392466328`
- handoff job: `99489096565`

The machine handoff verifier passed:

```text
HANDOFF_VERIFY_PASS
sequence=76 branch=feat/single-flow-reality-faketcp
```

The job then failed in `tests/test_handoff.py::test_adr0014_single_flow_architecture_invariants_are_persisted` because the test required the literal phrase:

```text
40 Mbit/s aggregate inner payload
```

inside `PROJECT_CONSTITUTION.md`, while the Constitution's canonical text is:

```text
40 Mbit/s aggregate-inner release operating point
```

This was a test wording mismatch. It did not establish a product, architecture or data-plane failure.

Fix:

- commit `fcd1d4be42eb7c135efc0fa5d6687883afae4df4`
- message `test: match ADR-0014 release wording`
- only `tests/test_handoff.py` was changed;
- the Constitution, Architecture and mature TCP-like transport were not changed.

Candidate B is invalid release evidence because the branch moved to apply the test fix. Any Windows/Linux child successes from candidate B are historical only and must not be mixed with the next candidate.

## Qualification rule after this fix

1. Write sequence 77 handoff recording candidate B as invalidated contract-test evidence.
2. Create a new `QUALIFICATION_KICK.md` generation and freeze the branch at the new exact candidate.
3. Require handoff-verify success in addition to the release aggregator's 21 exact-head child workflows.
4. On the first deterministic product red, inspect logs and fix only that layer.
5. Do not change the mature FakeTCP recovery/FEC core unless the failing gate isolates a core defect.
6. Do not deliver Windows/Linux artifacts until one exact candidate has both handoff and complete hosted Windows/Linux qualification green.
