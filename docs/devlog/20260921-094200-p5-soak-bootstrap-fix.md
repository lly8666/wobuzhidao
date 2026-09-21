# P5 soak initial-business qualification fix

- Date: 2026-09-21
- First candidate: `35c240e8af27145cfb378e9366b9da3e8b71ee49`
- Actions: `35584372982`

Both new soak jobs failed before the 31 minute measurement window started. The failure was deterministic:

```
p5_soak_measurement_test.go:240: timed out waiting for lifecycle state
```

The harness waited for `LifecycleServer.TunnelQualified` immediately after protected admission. That is not the lifecycle contract: initial lanes exist after admission, but the server marks the tunnel qualified only after the first real steady business packet.

## Fix

Before `recorder.markSteady()` and before the sustained-loss network is armed, the harness now:

1. sends one real IPv4 business packet through `TunnelClient.SendPacket`,
2. verifies the packet arrives at the server shared-TUN writer,
3. then waits for both endpoints to report two active/two physical lanes and no retiring lane.

This is only bootstrap qualification. It is outside the 31 minute measurement window and outside loss injection.

The actual soak contract is unchanged:

- 31 minutes,
- 300 ms one-way,
- sustained 5% deterministic loss both directions,
- Game=2,
- automatic same-ID rotation every 60 seconds,
- real TLS1.3/HTTPS close flow every 10 seconds,
- per-flow TCP/BusinessFlow retirement hard checks,
- final lane convergence,
- minute-floor HeapInuse plateau gate,
- FEC off, padding off.

The same first-candidate workflow also hit the previously-seen unrelated P4 `lanes-4` race signature (`GameDelivered=1, GameDuplicates=2`). This P5 fix does not modify P4.
