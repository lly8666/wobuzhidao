# V2-M3E fixed protection-mode negotiation

Status: **local qualification PASS**.

M3E adds one-time product-mode negotiation inside an already-Established WBD session. It does not reconfigure UDPspeeder, change DTLS/FakeTCP behavior, add a second lane, start TUN forwarding, or admit Auto.

## Wire extension

The M3A-D `WBDC` envelope remains unchanged. M3E reserves:

- type `9`: `CONFIG`, exactly one mode byte;
- type `10`: `CONFIG_OK`, exactly one echoed mode byte.

The carrier-agnostic `internal/control` package adds `MarshalExtended` / `UnmarshalExtended`: all M3A-D frames delegate to the already-qualified codec, while only types 9/10 are intercepted by the extension.

Admitted values are exactly the constitution-defined fixed modes:

- `1` = `normal`;
- `2` = `weak-1.5x` (UDPspeeder mode 0 `20:10` reference);
- `3` = `weak-2x` (UDPspeeder mode 0 `20:20` reference).

Value `4` is documented as the reserved `auto` value but is deliberately rejected. Zero and unknown values are rejected as unsupported. Auto remains deferred to V2-M9.

## Session semantics

`ConfigServerSession` is a thin wrapper around the already-qualified M3A-D `ServerSession`; it does not replace or mutate the existing auth/liveness/close state machine. CONFIG is accepted only when the base session is Established and is one-shot. Successful CONFIG returns CONFIG_OK and records `Configured=true` plus the selected `ProtectionMode` in combined session stats. Pre-HELLO, pre-auth and duplicate CONFIG are rejected deterministically.

PING/PONG, CLOSE, AUTH and version negotiation remain delegated to the M3A-D session implementation. No bearer token is stored by the config wrapper.

## Qualification

Unit tests cover all three fixed modes, zero/unknown/Auto rejection, malformed body lengths, string names, pre-HELLO/pre-auth ordering, one-shot success, duplicate rejection and combined encoded-wire counters.

Direct DTLS qualification uses the already-qualified quiet shim SHA-256 `63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a`. Both peers must report `DTLSv1.3 / TLS_AES_256_GCM_SHA384` READY before WBD control begins. The accepted sequence is:

`HELLO/ACCEPT -> AUTH/AUTH_OK -> CONFIG weak-1.5x/CONFIG_OK`

The server records `authenticated=true configured=true protection_mode=weak-1.5x`. The CLI also rejects `auto` before opening its UDP socket.

M3E forwards no application/L3 traffic and changes no UDPspeeder arguments. Those remain later milestone responsibilities.
