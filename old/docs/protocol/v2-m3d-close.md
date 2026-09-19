# V2-M3D close and reconnect policy

Status: local qualification PASS.

M3D defines explicit session termination and caller-driven reconnect policy. It does not implement an automatic reconnect loop, replay application data, or change DTLS/FEC/FakeTCP behavior.

## CLOSE

`CLOSE` type 8 has a 2-byte reason followed by at most 256 bytes of UTF-8 detail. Defined reasons are normal, idle-timeout, auth-failure, policy, protocol-error, and transient-transport.

A peer CLOSE is accepted only after Established. The server echoes the CLOSE, transitions to Closed and records only the reason in session stats. Idle expiry can transition Established to Closed with the idle-timeout reason using the existing caller-supplied monotonic tick policy.

## Reconnect policy

`ReconnectAllowed(reason)` permits retries only for idle-timeout and transient-transport reasons. Normal shutdown, auth failure, policy rejection, protocol error and unknown reasons are non-retriable.

`Backoff(attempt,min,max)` is a pure saturating exponential helper. Attempt zero returns min; each attempt doubles until max; invalid ranges are rejected; very large attempt values and uint64 limits saturate without overflow. The helper sleeps nowhere and starts no goroutine.

## Qualification

Unit tests cover CLOSE encode/decode/malformed cases, peer and idle Close transitions, retry reason mapping and backoff boundaries/overflow. Direct DTLS qualification waits for both peers to be READY, then performs HELLO/ACCEPT, AUTH/AUTH_OK, PING/PONG, and CLOSE/CLOSE-echo. The server ends Closed with the recorded normal reason.

Application-data replay, automatic reconnect, TUN/routing, FEC changes, extra lanes and Auto remain out of scope.
