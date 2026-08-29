# wobuzhidao Project Constitution — V2.3

## Product goal

Build a personal-use weak-network VPN for **OpenWrt/Linux ↔ Linux or Windows** with:

- exactly one public TCP-shaped raw/FakeTCP association per WBD session from the first SYN until disconnect;
- a short real-TLS Reality-like bootstrap carried inside that same FakeTCP association;
- UDP/datagram-like sustained payload semantics with no ordinary-TCP retransmission/HOL dependency;
- optional WBD-owned FEC, currently `off` or fixed systematic `20:20` on the release wire;
- standards-compliant DTLS 1.3 for steady-state encryption, integrity and anti-replay;
- simple shared username/password admission issuing independent one-time ticket/LiveID session identities;
- OpenWrt final transparent capture through **TPROXY**;
- Windows final client capture through a **TUN/Wintun-class L3 adapter**.

The current weak-network qualification ceiling remains **100 Mbit/s physical link capacity** and the conservative release operating point remains **40 Mbit/s aggregate inner payload**.

V1 (`dev/wbd-multilane-v1`, PR #2) remains rejected.

## Non-negotiable public-flow invariant

1. A WBD session has one public client/server 4-tuple and one FakeTCP sequence space.
2. Product establishment emits one FakeTCP SYN lineage. Successful Reality-like admission must not be followed by a second public SYN for the data plane.
3. The Reality-like TLS exchange is the first payload phase of the FakeTCP association, not an independent ordinary TCP connection.
4. The transition from TLS bootstrap to DTLS data mode emits no FIN/RST/new SYN and does not change the public 4-tuple.
5. Product mode must not run a parallel kernel TCP Reality listener as the owner of WBD admission traffic on `WBD_PORT`.
6. Kernel TCP state takeover is not a release dependency. FakeTCP owns public packet state from SYN onward.

## Non-negotiable no-HOL data-plane invariants

1. Product packets and FEC symbols must not traverse an ordinary kernel TCP byte stream on the public weak-network path.
2. The required public carrier is WBD-owned udp2raw-compatible FakeTCP: TCP-shaped raw packets with WBD-controlled state.
3. **TCP-shaped does not mean kernel-TCP-owned.** WBD does not require a real kernel `ESTABLISHED` payload socket.
4. A temporary reliable ordered stream is permitted only during the bounded TLS/bootstrap phase because TLS requires stream semantics.
5. After the bootstrap-to-DTLS mode barrier, later independent authenticated datagrams must be able to complete while an earlier FakeTCP sequence range is missing.
6. Shadow ACK/SACK/retransmission may preserve TCP-like outer behavior, but ordinary TCP ordered delivery, congestion-control HOL and kernel byte-stream ownership must not become dependencies of steady-state payload delivery.
7. WBD FEC is systematic and optional. **Do not delay an available systematic source merely to fill a FEC block.**
8. One raw lane is the product baseline. Extra lanes remain optional post-release research.

## Canonical establishment sequence

The product startup sequence is:

```text
raw FakeTCP SYN / SYN-ACK / ACK
        -> temporary bounded reliable ordered FakeTCP bootstrap stream
        -> real TLS 1.3 ClientHello/ServerHello/Finished
        -> Reality-like marker recognition
        -> shared username/password admission inside TLS
        -> fresh one-time ticket/session identity inside TLS
        -> bootstrap ACK drain + mode barrier
        -> SAME public 4-tuple / SAME sequence space
        -> DTLS 1.3 association
        -> one-time ticket bind
        -> WBD LINK_INIT proposal
        -> WBD LINK_ACCEPT
        -> Established immutable data association
```

The ticket path does not add a second public connection. Legacy/non-product test modes may retain older AUTH/witness paths only when clearly marked compatibility/diagnostic.

## Reality-like bootstrap requirements

The setup phase should be as close to ordinary Reality/TLS traffic as practical and is judged by packet capture rather than naming.

Required properties:

- real TLS 1.3 records on the public FakeTCP sequence space;
- configured SNI and WBD Reality-like recognition marker in/derived from the ClientHello path;
- username/password sent once only inside TLS;
- one-time ticket returned only inside TLS;
- no plaintext WBD credential, ticket or control frame before TLS protection;
- bounded bootstrap duration and memory;
- bootstrap writes are ACK-gated so `crypto/tls` observes reliable ordered byte semantics;
- the ordered bootstrap adapter is destroyed before steady-state data delivery begins.

The selected TCP SYN/options and TLS ClientHello should be measured against a realistic reference profile. Go TLS is acceptable for the first single-flow integration checkpoint, but the project must not claim a browser-perfect/"99%" fingerprint until pcap comparison supports that claim.

### Active-probe / fallback target

Product target behavior for an unrecognized TLS ClientHello is to remain in the temporary stream mode and proxy the connection to the configured fallback target. This fallback connection may use an ordinary outbound TCP socket because it is decoy traffic, not WBD VPN payload.

A proprietary FakeTCP SYN-option tuple must not be the final WBD recognition mechanism. Recognition belongs at the TLS/Reality-like layer.

## Bootstrap stream boundary

During bootstrap only:

- out-of-order payload may be retained until contiguous;
- writes are split into bounded chunks;
- the next chunk may wait for cumulative ACK of the current chunk;
- retransmission/HOL is accepted because this phase is only setup traffic.

The switch to datagram mode is valid only after authentication succeeds and bootstrap outbound bytes are ACK-drained. A failed or timed-out bootstrap closes the raw association; it must not silently enter DTLS mode.

## DTLS security

1. Steady-state WBD security remains **DTLS 1.3**.
2. The pinned initial implementation remains wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.
3. In personal product mode, encryption is required but certificate-chain/hostname verification may be explicitly disabled by configuration.
4. 0-RTT remains disabled until replay semantics are explicitly designed.
5. After DTLS Finished, product traffic remains standards-compliant DTLS application data; WBD does not invent a second post-handshake AEAD/key schedule.
6. FEC source/repair datagrams are independently protected DTLS application datagrams.

## Fixed FEC policy

The current release wire admits:

- `off` — no proactive repair; FakeTCP shadow recovery and DTLS remain active;
- `20:20` — fixed systematic tail-RS profile qualified for the current release surface.

The intended future fixed family remains `off`, `20:4`, `20:8`, `20:12`, `20:16`, `20:20`; intermediate profiles must not be advertised until implemented and qualified.

Any FEC/profile change takes effect only on a fresh WBD association. There is no in-place runtime FEC epoch switch.

## Release operating point

For the current <=100 Mbit/s weak-link target:

- release qualification uses **40 Mbit/s aggregate inner offered payload**;
- `legacy` FakeTCP shadow recovery is the product default;
- `sack-rack` remains experimental;
- fixed `20:20` is the strongest currently qualified proactive FEC profile;
- one raw lane is the release baseline.

ADR-0011 reopens the setup boundary only; it does not discard the measured steady-state transport evidence behind ADR-0010.

## Account/session model

WBD is a personal shared-account server, not a multi-tenant control plane.

- one configured username/password pair may be reused by several devices simultaneously;
- username identifies the shared account, not a transport session;
- each successful TLS bootstrap produces a fresh random one-time ticket;
- ticket consumption is atomically claimed before validation so one ticket establishes at most one live session;
- live state is keyed by ticket/`LiveID`, never username alone;
- each LiveID owns independent immutable FEC/link state;
- simultaneous-session count may be bounded by simple process/resource limits such as `max-sessions`.

## Server fan-out

One public raw FakeTCP listener fans out associations by raw 4-tuple. Product WBD associations progress through two per-association phases:

```text
bootstrap phase: FakeTCP stream adapter -> TLS/Reality-like admission
steady phase:    FakeTCP datagrams -> wolfSSL DTLS worker -> shared LiveID/LINK demux
```

The process-per-association DTLS model remains acceptable for the intended small personal device count.

Unrecognized probe sessions may remain in stream mode and proxy to the decoy target; they do not create WBD LiveID/DTLS state.

## Client capture / routing modes

Supported policy targets remain:

- `off` / manual routing;
- `global` full capture;
- `only-cn`;
- `only-non-cn`.

Every mode has a mandatory **underlay escape invariant**: the public WBD server endpoint must continue through the original physical/default route and must never recursively enter the tunnel.

### OpenWrt release shape

OpenWrt final capture uses **TPROXY**, not a TUN device. nftables/iptables TPROXY plus policy routing redirects selected TCP/UDP traffic to the local WBD adapter while explicit marks/routes exempt the WBD public association.

The existing Linux TUN bridge remains useful for regression and development but is not the final OpenWrt release gate.

### Windows release shape

Windows final capture uses a **TUN/Wintun-class L3 adapter**. Full-tunnel mode uses broad routes plus explicit underlay endpoint escapes. Split mode must use compact prefix classification rather than thousands of persistent firewall rules.

Device-wide IPv6 remains fail-closed while connected until the IPv6 product path is explicitly qualified.

## Product stack

```text
OpenWrt TPROXY adapter                 Windows TUN/Wintun adapter
             \                           /
              -> WBD packet/session layer
                     ↓
              optional fixed FEC
                     ↓
                 DTLS 1.3
                     ↓
         same single public FakeTCP flow
                     ↑
      initial TLS/Reality bootstrap occurred here
```

## Qualification gates

### Gate A — bootstrap resemblance and single-flow continuity

Capture establishment and prove:

1. one public WBD SYN lineage and no second post-auth SYN;
2. one public 4-tuple before and after bootstrap;
3. sequence/ACK continuity across the mode switch;
4. real TLS 1.3 + configured SNI during the first seconds;
5. credentials/ticket absent from plaintext capture;
6. selected SYN/TLS fingerprint is plausibly close to the reference profile;
7. fallback probes reach the configured target once fallback support is enabled.

### Gate B — post-switch no-HOL

After mode switch, deliberately lose/delay an earlier FakeTCP payload while delivering a later independent DTLS/FEC datagram. The later authenticated datagram must become available before repair of the earlier sequence hole.

### Gate C — transport/load

Retain existing first-arrival, pcap, 20/100 ms and <=100 Mbit/s release/load regressions, including CPU/RSS/wire accounting.

### Gate D — platform one-shot

From clean state on Windows and OpenWrt/Linux:

- establish the single public flow;
- complete TLS bootstrap, DTLS, LINK and capture readiness;
- pass real DNS/TCP/UDP traffic;
- verify underlay escape;
- cleanly restore routing/firewall state.

## Retired / non-product architectures

- V1 ordinary-TCP lane pools/RBC/reinjection/rescue lanes;
- ordinary kernel TCP as WBD sustained payload carrier;
- `Reality TCP -> close -> new FakeTCP SYN`;
- parallel kernel Reality listener + raw FakeTCP listener as the intended product same-port design;
- kernel TCP state takeover as a release dependency;
- runtime config epochs / in-place FEC switching;
- VLESS/Xray/Vision stream semantics as the WBD data plane;
- WireGuard inner glue;
- Android/no-root;
- mandatory two-lane transport.

## Development discipline

- preserve exact upstream pins and already-qualified steady-state DTLS/FEC/FakeTCP evidence unless the changed setup boundary invalidates a specific test;
- optimize from first-arrival + delivery + resource + wire measurements, not intuition;
- do not delay systematic source datagrams merely to fill a FEC block;
- do not call the single-flow implementation release-ready until both the bootstrap single-flow gate and the post-switch no-HOL gate pass;
- do not reintroduce a second public connection as a shortcut for Reality-like setup.
