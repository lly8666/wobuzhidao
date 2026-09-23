# 2026-09-23 lifecycle phase-isolation correction

Product candidate under test: `65ff2ef27dd763cba2f7293e6ef6274bca6632c3`.

## Exact-SHA result before this harness-only correction

At `65ff2ef27dd763cba2f7293e6ef6274bca6632c3`:
- `next-lifecycle` 35801110983 PASS.
- `next-foundation` 35801110955 PASS.
- `next-p4-steady-targeted` 35801110953 PASS.
- `next-tls-startup-padding` 35801111038 PASS.
- `next-realpath-calibration` 35801110919 PASS.
- `next-lifecycle-fullstack` 35801111026 produced 31 PASS sample jobs and 5 failed jobs; aggregate job 106993322768 correctly FAILed.

All L0, L1, L3, L4, L5, L6-partial and L7 groups passed. Both real L1 seeds prove that the new server PeerFIN commitment still allows ordinary no-business DORMANT convergence, sparse wake, and sustained pure downlink. Both L7 seeds pass the unchanged production 15s keepalive / 90s dead-after timing and 120s blackhole/recovery window.

## The five failures are harness-only

### L2 S2C seed101

The scenario-specific gates passed:
- neither endpoint falsely became DORMANT 10s into one-way total loss;
- retries remained bounded;
- after loss removal the final 30s S2C delivery ratio was 1.0.

After the traffic generator ended, the configured 4s auto-idle threshold elapsed before the final diagnostic sample, so the client correctly ended DORMANT with zero active lanes. The generic validator's “every non-L6 scenario must finish ACTIVE” rule was not an L2 requirement and made the result timing-dependent. Final ACTIVE is therefore retained only for scenarios whose configured auto-idle is disabled; positive-idle scenarios are judged at their explicit activity/fault/recovery windows.

### L6 race1/race4, both seeds

All four jobs reached and exercised the written blackhole condition. For race1/seed101, for example:
- 16/16 client-originated UDP events were accepted locally during total underlay blackhole;
- the client made bounded wake attempts, with bootstrap deadline failures rather than a tick storm;
- after the blackhole was removed, a wake succeeded about 1.2s after clear.
Race4/seed101 likewise recovered about 2.3s after clear.

The recovery target then received at least one new recovery-seed packet **and** 3-4 delayed packets from the prior blackhole phase on the same UDP port. Those delayed datagrams are not corruption and are bounded by the 16 packets injected during the fault, but the driver still had a global `unexpected==0` exit condition. It exited nonzero before the script could write manifest or start the separate strict 100-event phase.

The driver now takes an explicit `--max-unexpected`. Only the recovery probe permits up to 16 delayed old-phase datagrams. The strict 100-event clear-path phase uses a different UDP port and seed and keeps the hard requirement: 100/100 unique, corrupt=0, unexpected=0. The validator records and enforces both limits.

## Product boundary

This commit changes no production Go code, wire format, parameter/default, FEC setting, buffer, 4096 bound, repair horizon, ACK/HOL behavior, or AF_PACKET path. The product fix remains `65ff2ef27dd763cba2f7293e6ef6274bca6632c3`; the new SHA is an acceptance-harness rerun point only.

The strict 10 Mbps Normal / 3 Mbps Game performance matrix remains independently CAPACITY_LIMITED from `0882adb70ddb85d8d5dbd5e6be8618f3d7132fcd`. The user-requested AF_PACKET/uplink-capacity mainline remains HOLD.
