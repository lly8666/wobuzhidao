# V2-M8C shared-account full-stack mux qualification

## Scope

This gate closes the gap between the already-qualified per-layer fan-out pieces and the product requirement that one personal account may own several simultaneous data sessions behind one public WBD server entry.

The sustained public data path remains:

```text
client application UDP
  -> WBD immutable link path (FEC off or fixed 20:20)
  -> wolfSSL DTLS 1.3
  -> WBD FakeTCP raw association
  -> one shared public FakeTCP listener
  -> per-4-tuple FakeTCP association
  -> per-association inherited-fd wolfSSL DTLS worker
  -> shared multi-peer WBD link server
  -> per-LiveID service socket
  -> UDP echo/service
```

Reality-like TLS is used only before the data associations to authenticate the shared username/password and issue independent one-time tickets. Username is never a sustained data routing key.

## Evidence already closed

GitHub Actions `faketcp-mux-two-client` run `32896555995` passed on commit `b539d7ace2afc5e47fb4d8671ae7fc6f39dab70e`.

That run established two client network namespaces against the same public FakeTCP address/port and proved:

- two independent raw 4-tuples coexist behind one listener;
- two wolfSSL DTLS 1.3 server workers are created;
- each server worker receives an already-bound loopback UDP transport through inherited fd 3 (`inherited=yes`);
- both clients complete DTLS 1.3 with explicit `verify=none` personal mode;
- distinct client markers complete UDP echo without cross-association corruption;
- the mux product default is legacy shadow recovery.

This qualifies transport fan-out only. It does not by itself prove shared-account ticket admission, independent LINK_INIT/LINK_ACCEPT state, or independent FEC state.

## Full-stack two-client gate

Workflow: `.github/workflows/fullstack-mux-two-client.yml`.

Each matrix case creates fresh server/client namespaces and runs from clean process/network state. Cases are:

- FEC `off`;
- fixed systematic tail-RS `20:20`.

For each case:

1. One Reality-like front is started with account `solo` and one shared password.
2. Client A and client B authenticate with the same credentials and must receive different 32-byte tickets.
3. Exactly two ticket files must exist before data association bind.
4. One `wbd-link-server-mux` listens for plaintext from all DTLS workers.
5. One `wbd-faketcp-mux` owns the single public raw FakeTCP address/port.
6. Two ordinary `wbd-faketcp client` processes create different public raw 4-tuples to that same server port.
7. Two DTLS clients connect through their local FakeTCP plaintext ports.
8. The server mux must create two inherited-fd wolfSSL DTLS workers.
9. Two `wbd-link-proxy` clients bind their distinct one-time tickets and propose the matrix FEC profile.
10. The link server must report exactly two established `account=solo` sessions.
11. Server-side ticket files must be gone after atomic bind, proving both one-time tickets were consumed.
12. Both clients concurrently send 20 unique application markers through their own WBD link endpoints and receive only the matching echo.
13. The FakeTCP mux and shared link server must still be alive after both exchanges.

Any missing session, ticket reuse, username-keyed collapse, raw-association overwrite, DTLS worker collision, LINK state collision, FEC state collision, or cross-session packet routing makes the workflow fail.

## Release interpretation

Passing both matrix cases qualifies the clean-path shared-account multi-session protocol shape. It does not yet freeze the protocol.

Before protocol freeze, M8C still requires a bounded 100 Mbit/s two-session regression that records first-complete latency, delivery, aggregate CPU/RSS, retransmission bytes and FEC wire cost. Platform packet capture work (OpenWrt TPROXY and Windows TUN) remains after this protocol/session gate.
