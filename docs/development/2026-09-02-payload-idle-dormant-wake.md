# 2026-09-02 automatic payload-idle DORMANT / wake

## Product contract

ADR-0012 keeps payload idleness separate from transport liveness. The Windows profile now accepts `idle_timeout` as a non-negative number of seconds. `0` is the compatibility default and disables automatic idle sleep.

A non-zero timeout observes only the shared Game process's real application/TUN payload sequence introduced in the previous slice. FakeTCP, DTLS, LINK PING/PONG and local control traffic cannot refresh this deadline.

## Controller behavior

- On successful CONNECTED entry, a controller-generation-scoped monitor starts only when `idle_timeout > 0`.
- The child-provided wall-clock timestamp is diagnostic. Idle policy uses the monotonic Game payload `Sequence` plus controller-local `time.Time` observations.
- While CONNECTED, a sequence change refreshes the local payload deadline. Activity-query failures fail open: the controller does not infer idle and tear down healthy lanes when payload observability is unavailable.
- At expiry, the controller re-queries the sequence immediately before calling the existing `Dormant()` lifecycle.
- `Dormant()` remains authoritative: Game first receives an empty lane target set, public Transport Lanes stop, and Logical Tunnel identity, lease, shared Game/TUN/routes/DNS state survive.
- While DORMANT, the first observed payload-sequence advance calls the existing `Wake()` lifecycle. The triggering packet may have been dropped by the empty-lane barrier; the first READY lane is published for subsequent traffic and optional lanes attach later.
- After a successful idle transition the monitor re-queries once more. If payload raced the final confirmation-to-Game-barrier window, it immediately wakes instead of leaving the recent payload asleep.
- Disconnect closes the generation-scoped monitor before runtime teardown, so an old monitor cannot act on a later reconnect.

## Frozen boundaries

This slice changes no FakeTCP recovery, Reality-like same-flow bootstrap, wolfSSL DTLS, LINK, FEC, public packet format, PacketID namespace or 1..4 lane product limit.

## Qualification

Focused tests cover strict `idle_timeout` parsing/defaults, negative rejection, monitor enable/disable, idle DORMANT without payload, real sequence-triggered wake, and sequence-over-child-timestamp authority. Exact-head GitHub Actions remain the qualification authority because this development environment does not have a usable local repository clone.
