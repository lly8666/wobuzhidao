# V2-M3B optional bearer-token authentication

Status: local qualification PASS.

M3B adds an explicit authorization state after M3A version negotiation. It does not add another cryptographic layer: HELLO, ACCEPT, AUTH, AUTH_OK and ERROR are all ordinary WBD control datagrams carried only after the underlying DTLS 1.3 association has completed.

## Wire additions

- `AUTH` type 4: body is an opaque bearer token, 1..256 bytes.
- `AUTH_OK` type 5: empty body.
- ERROR code 4: AUTH required.
- ERROR code 5: authentication failed.
- ERROR code 6: unexpected session state.

The operator may configure no token. In that mode a successful HELLO/ACCEPT transition enters Established immediately. With a token configured, ACCEPT transitions the server to AwaitAuth and only a matching AUTH may enter Established.

## Server states

`AwaitHello -> AwaitAuth -> Established` when token authentication is enabled.

`AwaitHello -> Established` when authentication is disabled.

Unsupported protocol negotiation or a wrong token enters `Failed`. AUTH before HELLO, repeated HELLO while awaiting AUTH, and control messages after Established/Failed are rejected deterministically and do not open an application-data path.

## Token comparison

The configured and presented tokens are each SHA-256 hashed to a fixed 32-byte digest, then compared with `crypto/subtle.ConstantTimeCompare`. The SHA-256 operation here is only a fixed-length local comparison primitive; it is not a wire-protocol cipher, password KDF, challenge-response construction or replacement for DTLS. Server X.509/hostname verification from M2 remains mandatory.

## Qualification

The direct-DTLS qualification uses the M2-qualified quiet shim and `wbd.test` certificate path. Both DTLS peers must report `DTLSv1.3 / TLS_AES_256_GCM_SHA384` READY before the control client starts. Three cases are required: auth disabled success, correct-token AUTH_OK, and wrong-token ERROR/fail-closed.

M3B deliberately does not add username/password authentication, token rotation, keepalive, reconnect policy, TUN/routing, FEC changes, extra lanes or custom crypto.
