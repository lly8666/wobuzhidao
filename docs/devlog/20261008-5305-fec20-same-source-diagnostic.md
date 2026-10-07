# 5305 FEC20:20 matched-source counterfactual (Actions only)

Date: 2026-10-08. Branch: `diagnostic/normal-5305-fec20-counterfactual-20261008` (not main). User requested causal differentiation before physical testing. No product sources or releases are modified.

## Frozen identities

- Product SOURCE: `c853935e5356a9bc99d380befb5a8ac8e0c08d97` and manifest qualifications remain pinned to this commit.
- Historical matched baseline: Normal one lane, 10Mbps each direction, 5305 seed1508, FEC20:12, Actions run 37692499392. Qdisc loss 29.857% c2s / 29.935% s2c; business residual loss 6.308% / 6.498%; RTT probe stress 56/60 received, returned-only p99 2178.685ms; socket/link drop zero.
- Counterfactual: exact same SOURCE, seed, topology, load and network stage; change only FEC parity to 20. This is ONE separately identified formal20 sample, not a global 70-sample/physical claim.
- Independent IID k20 r12 theoretical source erasure reference at 30%: 5.7724% (not finite-3s recovery oracle); business's 6.3–6.5% is compatible, but must not be represented as exact causation for any single lost packet.

## Diagnostic changes

- Adds branch-scoped trigger to the preexisting one-sample Actions relay and forwards optional exact product_source_sha, validated as 40 hex. Ref must equal the triggering branch; rerun is required to dispatch one strict sample.
- Updates `.github/perf-dispatch-request.json` for matched Normal/5305/seed1508/10Mbps/one lane.
- No code, protocol, MTU, FEC algorithm, sysctl, physical machine, or release qualification changes.

## Decision rule (not pre-judged)

- Compare injected actual loss, unique app loss/goodput, FEC parity delivery, ARQ counts, true RTT probe sample count and p95/p99, optional gaps, and resource/CPU against the baseline. Do not use 5xPASS_SCREEN as proof of recovery or compare returned-only p99 without reporting timeouts.
- If 20:20 suppresses app residual loss and slow probes under matched conditions, support FEC profile reserve explanation. If anomalous long tails persist despite sufficient parity, pursue code/ARQ/queue path; do not proceed physical until interpretation.
- Existing real ARM native AF_PACKET drops and MTU9000/UDP8973 late tail remain OPEN and independent. Physical tests NOT_RUN.
