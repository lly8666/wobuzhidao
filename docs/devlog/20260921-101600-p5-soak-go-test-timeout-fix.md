# P5 soak Go test timeout fix

- Date: 2026-09-21
- Second candidate: `aeb02e2e385de2a5c08be96f11d34c0f642421ac`
- Actions: `35584644754`

The initial-business qualification fix worked: both soak jobs entered the actual sustained-loss measurement and ran well past the previous 10-second failure.

Both jobs then failed at the same framework boundary:

```
panic: test timed out after 10m0s
FAIL ... internal/runtimeentry 600.00s
```

This is the default `go test` timeout, not a product or soak-validator verdict. The other 26 jobs in the exact-SHA workflow passed, including the previously intermittent P4 lanes-4 race gate.

## Change

Only the dedicated soak workflow command changes:

```
go test ./internal/runtimeentry -count=1 -timeout=40m -run='^TestP5ThirtyMinuteSoakMeasurementHarness$' -v
```

The Actions job timeout remains 45 minutes.

No product code, soak duration, loss profile, rotation interval, flow-retirement check, memory-plateau rule, FEC/padding policy, or validator threshold changes.
