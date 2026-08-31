# wobuzhidao Project Constitution

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- exactly one public TCP-shaped raw/FakeTCP flow per WBD session;
- real TLS 1.3 Reality-like setup/authentication during the first bounded phase of that same flow;
- no second public SYN/socket between admission and the data plane;
- UDP/datagram-like steady-state semantics with no ordinary-TCP retransmission/HOL dependency;
- optional WBD-owned release FEC: `off` or fixed systematic `20:20`;
- pinned standards-compliant DTLS 1.3 for steady-state encryption, integrity and anti-replay;
- OpenWrt transparent capture through TPROXY;
- Windows capture through a TUN/Wintun-class L3 adapter and Npcap raw carrier;
- one shared username/password account allowed to own multiple isolated sessions.

The current weak-network qualification ceiling is **100 Mbit/s physical link capacity**. The release operating point is 40 Mbit/s aggregate inner traffic on <=100 Mbit/s weak links. Higher-capacity optimization is not on the critical path.

## V3 one-public-flow law

This is the primary architectural law. A valid session is:

```text
raw SYN / SYN-ACK / ACK
  -> real TLS 1.3 Reality-like bootstrap on the same raw sequence space
  -> username/password admission + ticket
  -> encrypted TLS SWITCH_REQ / SWITCH_ACK
  -> same 4-tuple, no FIN/RST/close_notify/new SYN
  -> DTLS 1.3 + FEC + LINK datagrams
```

The following are forbidden in the V3 product path:

1. a separate ordinary TCP Reality bootstrap followed by a new FakeTCP connection;
2. two public listeners competing for the same WBD port (for example kernel `wbd-reality-front :443` plus raw mux `:443`);
3. moving an established kernel TCP socket into a sustained VPN payload role;
4. keeping an ordered TLS/TCP byte-stream assembler after the switch barrier;
5. claiming logical ticket correlation makes two public flows equivalent to one flow.

The **raw FakeTCP mux is the sole public owner** of `WBD_PORT`. Reality-like parsing/authentication is an in-flow phase inside that owner. A legacy/diagnostic `wbd-reality-front` binary may exist in source history, but an official V3 server bundle must not install or launch it as a competing public listener.

## Reality-like bootstrap requirements

The initial phase must be as close as practical to a normal Reality/browser-style TLS setup while preserving the one-flow law.

Mandatory properties:

- real TLS 1.3 record/handshake grammar;
- valid ClientHello and SNI;
- WBD Reality-like route marker/classification;
- certificate handshake and authenticated username/password admission;
- bounded ordered presentation only while TLS requires byte-stream semantics;
- switch control encrypted inside TLS 1.3 application data;
- no plaintext WBD switch magic on the public wire immediately after TLS;
- no TLS `close_notify`, FIN, RST or second SYN at the transition.

Current Go `crypto/tls` is not declared byte-for-byte equivalent to a particular browser/uTLS fingerprint. ClientHello extension ordering, TCP SYN options, timing and record sizing remain fidelity work. Such work may improve the first few seconds toward the 99% target, but it must never create a second public socket or put steady-state VPN payload into kernel TCP.

## No-HOL steady-state law

A short ordered assembler is allowed **only** for the TLS bootstrap. Once the client decrypts the server's encrypted switch ACK:

- client and server destroy ordered bootstrap assemblers (discarding all bootstrap-only ordered state);
- all sustained data is packet/datagram-preserving;
- later independent datagrams may complete while earlier units are lost/delayed;
- no missing earlier record may gate later DTLS/FEC/LINK delivery;
- ordinary TCP receive queues, congestion control or retransmission are not delivery authorities.

Qualification must include an explicit later-datagram-bypasses-earlier-loss test after the switch barrier.

## Steady-state stack

```text
TUN / captured IP packet
        ↓
LINK / packet-session layer
        ↓
FEC: off or fixed systematic 20:20
        ↓
DTLS 1.3
        ↓
first-arrival FakeTCP datagram carrier
        ↓
same public 4-tuple established at session start
```

DTLS 1.3 remains the steady-state cryptographic authority. The current lock is wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`, until an explicit security-lock qualification replaces it.

## FakeTCP invariants

1. FakeTCP owns the public packet shape and sequence space; kernel TCP does not own sustained WBD payload.
2. Product default recovery remains the latency-first legacy path. SACK/RACK is experimental unless separately re-qualified under load.
3. One raw lane is the release baseline.
4. Shadow ACK/retransmission may maintain TCP-shaped behavior, but it must not serialize independent steady-state datagrams behind missing earlier data.
5. Half-open/reconnect cleanup must not allow stale associations to delete or block a new incarnation reusing a 4-tuple.
6. Windows/Linux kernel RST suppression, where required, must be narrowly WBD-owned and cleaned on teardown.

## FEC invariants

1. WBD FEC is systematic and optional.
2. Do not delay an available systematic source merely to fill a FEC block.
3. Release wire modes are only `off` and fixed systematic `20:20`.
4. One-lane proactive expansion must remain within the qualified release budget; no hidden adaptive/learning FEC is allowed in V3.
5. FEC must preserve packet boundaries and must not reconstruct an ordered aggregate byte stream.

## LINK/session invariants

- one-time ticket/session identity binds Reality-like admission to the in-flow data session;
- ticket/LiveID/session isolation must remain per logical connection;
- LINK may multiplex sessions on loopback, but public transport remains one raw association per client session;
- a failed child or expired session must not be masked by an unrelated long-lived supervisor child.

## Linux V3 server composition

Official release runtime:

```text
public WBD_PORT
   ↓ sole owner
wbd-faketcp-mux
   ├─ bounded Reality-like TLS 1.3 phase
   └─ DTLS worker per admitted session
        ↓
127.0.0.1 LINK mux
        ↓
127.0.0.1 platform proxy
```

The manager/firewall must expose and protect one public port. No independent Reality kernel listener is part of the product composition. If a future decoy/fallback site is added, it must be implemented under the same raw-front ownership model rather than by restoring a competing public TCP listener.

Secrets such as route keys/passwords must not be intentionally exposed in status/log output or command-line diagnostics. Packaging work should prefer protected files/environment/credential channels over visible argv where practical.

## Windows V3 client composition

The Windows product path must not execute a separate public `reality-bootstrap` connection. Underlay discovery occurs before the single raw association. The required readiness order is:

```text
single FakeTCP association
  -> in-flow Reality-like TLS/auth/switch complete
  -> DTLS READY
  -> LINK READY
  -> TUN READY
  -> IPv6 fail-closed policy and routes
  -> connected
```

`connect_pass`/connected state must not be reported merely because processes were spawned. Route mutation happens only after the data path is actually ready.

## Routing/platform invariants

- Linux public binding may resolve `0.0.0.0` to the concrete IPv4 required by the raw carrier.
- Linux manager lifecycle must support install/uninstall/start/stop/pause/resume/restart/status/logs/config/set/regen-certs/doctor/show-config.
- Firewall cleanup removes only WBD-owned rules/state.
- Windows route modes remain Full/Foreign/China; DNS modes remain Auto/System/Cloudflare/Custom.
- Device-wide IPv6 fail-closed behavior while connected is retained, with route cleanup before tunnel teardown.
- Npcap remains the privileged Windows raw path and Wintun-class L3 capture remains the tunnel interface path.

## Qualification gates

A V3 candidate is not release-qualified until automated tests prove, on one captured association:

1. exactly one public SYN/session 4-tuple;
2. Reality-like real TLS 1.3 occurs before DTLS;
3. switch request/ack plaintext is absent from public capture;
4. no second SYN, FIN, RST or TLS close-notify occurs at the mode switch;
5. DTLS 1.3 succeeds on the same raw association;
6. bidirectional packet payload succeeds;
7. deliberate earlier steady-state loss/reordering does not block a later datagram;
8. repeated dirty reconnects do not require a second listener or stale public flow;
9. Linux release bundle contains one public owner and no product `wbd-reality-front` listener;
10. Windows portable and Linux amd64/arm64 builds pass their existing dependency/static/manifest gates.

The weak-network matrix remains centered on 20/100 ms RTT and loss/load points required by current CI, with the 40 Mbit/s release operating point mandatory before interpreting 60/80 Mbit/s headroom.

Physical Windows 11/Npcap + real NIC/NAT/ISP + Linux ARM64 qualification remains the final platform gate. CI evidence must never be presented as a substitute for that physical test.

## Deferred work

- ClientHello/TCP fingerprint fidelity improvements toward browser/REALITY appearance.
- Additional FEC profiles or adaptive FEC.
- Multiple raw lanes.
- Higher-than-100-Mbit/s optimization.
- Android/unprivileged endpoints.

None of those may weaken the one-public-flow or no-HOL laws.