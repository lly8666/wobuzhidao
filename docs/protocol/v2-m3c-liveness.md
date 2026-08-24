# V2-M3C liveness and session accounting

Status: local qualification PASS.

M3C adds only liveness and observable session accounting after the M3A/M3B session is Established. It does not add a reliable stream, application-data retransmission, reconnect logic or any carrier/FEC behavior.

## Wire additions

- `PING` type 6: exactly 8 bytes, unsigned big-endian opaque nonce.
- `PONG` type 7: exactly 8 bytes and must echo the PING nonce.

The server accepts PING only in `Established`. Before HELLO it returns unexpected-state; while authentication is required it returns AUTH-required. PONG received by the server is not a valid state transition in M3C.

## Stats

`ServerSession` exposes a snapshot containing state, whether auth is required, whether the session is authenticated, control RX/TX message counts, encoded control RX/TX byte counts, PING/PONG counts and the caller-supplied last-activity tick. Bearer token contents or hashes are not exposed in stats.

Wire byte accounting occurs in `HandleWire`, around the same strict M3 control encoder/decoder used on the network path.

## Idle policy

`IdleExpired(now, idle)` is a pure helper over caller-supplied monotonic ticks. It has no goroutine/timer and retransmits nothing. Idle value zero disables expiry; clock regression does not expire a session; the exact `now-last_activity == idle` boundary is expired.

## Qualification

The direct-DTLS qualification uses the M2-qualified quiet shim and wbd.test certificate path. Session control starts only after both DTLS peers report READY. Auth-disabled and token-authenticated cases both establish successfully, then send one PING and receive a PONG with the exact same nonce. Server stats must show one PING and one PONG with the expected authentication flags and wire byte counts.

Reconnect, TUN/routing, data framing, FEC changes, extra lanes and Auto remain out of scope.
