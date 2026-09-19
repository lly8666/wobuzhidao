# 2026-09-02 automatic payload-idle DORMANT / wake

## Product contract

ADR-0012 keeps payload idleness separate from transport liveness. The Windows product profile accepts `idle_timeout` as a non-negative number of seconds. If `idle_timeout` is omitted, the product default is **15 minutes (900 seconds)**. Only an explicitly configured `idle_timeout=0` disables automatic payload-idle sleep.

A non-zero timeout observes only the shared Game process's real application/TUN payload sequence introduced in the previous slice. FakeTCP, DTLS, LINK PING/PONG and local control traffic cannot refresh this deadline. The JSON configuration boundary preserves omission separately from explicit zero; runtime `Profile.IdleTimeoutSeconds == 0` therefore continues to mean disabled and is not normalized back to 900.

## Controller behavior

- On successful CONNECTED entry, a controller-generation-scoped monitor starts only when `idle_timeout > 0`.
- The child-provided wall-clock timestamp is diagnostic. Idle policy uses the monotonic Game payload `Sequence` plus controller-local `time.Time` observations.
- While CONNECTED, a sequence change refreshes the local payload deadline. Activity-query failures fail open: the controller does not infer idle and tear down healthy lanes when payload observability is unavailable.
- At expiry, the controller re-queries the sequence immediately before calling the existing `Dormant()` lifecycle.
- `Dormant()` remains authoritative: Game first receives an empty lane target set, public Transport Lanes stop, and Logical Tunnel identity, lease, shared Game/TUN/routes/DNS state survive.
- While DORMANT, the first observed payload-sequence advance calls the existing `Wake()` lifecycle. The triggering packet may have been dropped by the empty-lane barrier; the first READY lane is published for subsequent traffic and optional lanes attach later.
- After a successful idle transition the monitor re-queries once more. If payload raced the final confirmation-to-Game-barrier window, it immediately wakes instead of leaving the recent payload asleep.
- Disconnect closes the generation-scoped monitor before runtime teardown, so an old monitor cannot act on a later reconnect.

## Evidence at takeover baseline

The stale handoff checkpoint `c7a0622352889ff8906db940b3e1e2bb5df3d6b1` predates the current bounded-replacement/activity work. At takeover baseline `0d65698d1601951169a807d94c0eaa8c09c6531f`, code and tests already establish:

- app/TUN payload advances the Game payload activity sequence;
- local control snapshots do not advance it, so PING/PONG/control cannot refresh payload idle through the activity contract;
- the Windows runtime automatically enters DORMANT on payload-idle expiry and automatically wakes on a later payload-sequence advance;
- first-ready wake republishes the first healthy lane before optional Game-lane refill.

The drift fixed by this slice is only the omitted-vs-explicit-zero product configuration contract, not the activity source or DORMANT/wake state machine.

## Frozen boundaries

This slice changes no FakeTCP recovery, Reality-like same-flow bootstrap, wolfSSL DTLS, LINK, FEC, public packet format, PacketID namespace or 1..4 lane product limit.

## Qualification

Focused tests cover omitted `idle_timeout => 900`, explicit `idle_timeout=0 => disabled`, positive values, negative rejection, monitor enable/disable, idle DORMANT without payload, real sequence-triggered wake, and sequence-over-child-timestamp authority. Exact-source GitHub Actions remain the qualification authority because this development environment does not have a usable local repository clone.
