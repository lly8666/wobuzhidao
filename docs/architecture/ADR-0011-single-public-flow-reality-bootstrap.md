# ADR-0011: One public FakeTCP flow owns Reality-like bootstrap and the no-HOL data plane

Status: **ACCEPTED / REOPENS ADR-0010 SETUP BOUNDARY** (2026-08-29)

## Context

The former V2.2 product shape used two logically bound but network-independent connections:

1. an ordinary kernel TCP/TLS Reality-like admission connection that issued a one-time ticket; then
2. a fresh raw FakeTCP association carrying DTLS/FEC/VPN payload.

That shape preserved no-HOL steady-state transport, but a public observer, NAT, firewall or conntrack could see two unrelated TCP flows. Sharing one numeric port did not make them one connection and also created kernel/raw SYN/SYN-ACK/RST interaction that complicated physical Windows/Linux qualification.

The product requirement is stricter: **from the first SYN until disconnect, a WBD session is one public TCP-shaped 4-tuple and one continuous TCP sequence space.** The first seconds should be as Reality/TLS-like as practical, while sustained VPN payload must never inherit ordinary TCP ordered-delivery HOL.

## Decision

### 1. One public association, one SYN

A WBD session has exactly one public carrier association:

```text
client-ip:client-port  <====================>  server-ip:WBD_PORT
          one raw TCP-shaped FakeTCP 4-tuple
```

The normal establishment sequence is:

```text
raw FakeTCP SYN / SYN-ACK / ACK
        -> bounded reliable ordered bootstrap mode on the same FakeTCP association
        -> real TLS 1.3 ClientHello/ServerHello/Finished
        -> Reality-like marker recognition + shared username/password admission
        -> one-time ticket/session identity returned inside TLS
        -> bootstrap ACK drain / mode barrier
        -> same 4-tuple and same sequence space, no FIN/RST/new SYN
        -> DTLS 1.3 datagrams
        -> immutable LINK
        -> optional fixed FEC
        -> VPN datagrams
```

No ordinary kernel TCP socket owns WBD product payload. Product mode must not create a separate public Reality TCP connection before FakeTCP.

### 2. Temporary stream semantics are allowed only during bootstrap

`crypto/tls` requires a reliable ordered byte stream. WBD therefore provides a **bounded bootstrap stream adapter inside FakeTCP**:

- out-of-order bootstrap segments may be retained until the missing sequence arrives;
- bootstrap writes are split into bounded TCP-shaped payload chunks;
- a bootstrap write chunk waits for its cumulative ACK before the next chunk is emitted;
- bootstrap has a short absolute deadline and bounded memory;
- the stream adapter is destroyed after admission and a mode barrier.

This HOL behavior is intentional and permitted only for the small setup exchange. It is not the steady-state product transport.

### 3. Steady-state no-HOL semantics remain unchanged

After the mode barrier, later independent DTLS/FEC/application datagrams may complete while an earlier FakeTCP payload is missing. FakeTCP ACK/SACK/shadow retransmission may preserve plausible TCP-shaped outer behavior but must not become an ordered delivery dependency for VPN payload.

The existing release choices remain frozen unless separately reopened:

- pinned wolfSSL DTLS 1.3;
- one raw lane;
- `legacy` FakeTCP shadow recovery default;
- FEC `off` or qualified fixed systematic `20:20`;
- immutable LINK parameters;
- 40 Mbit/s aggregate-inner release operating point on the <=100 Mbit/s weak-link target.

### 4. Reality-like recognition moves inside the raw association

The public raw listener, not a kernel TCP listener, owns the port from SYN onward. WBD identification must happen from the TLS ClientHello/Reality-like marker, not from a proprietary FakeTCP SYN fingerprint.

The product target is:

- WBD client: plausible TCP SYN -> real TLS 1.3 ClientHello with configured SNI and WBD Reality-like recognition marker -> authenticated WBD branch;
- unrecognized client: remain in stream mode and proxy the byte stream to the configured decoy/fallback target so ordinary probes do not merely see a dead/raw-only port.

Until fallback and fingerprint qualification are implemented, the single-flow branch is experimental and must not be called release-ready.

### 5. Mode switch is a protocol boundary

The TLS/bootstrap phase and DTLS/datagram phase share the same raw association but not the same delivery semantics. A switch is valid only when:

- TLS authentication succeeded;
- both peers agree to leave bootstrap mode;
- all bootstrap outbound chunks are cumulatively ACKed;
- no bootstrap byte remains waiting for ordered delivery;
- neither endpoint emits FIN/RST/new SYN as part of the switch.

A failed bootstrap closes the association; it must not silently fall into DTLS mode.

### 6. Platform ownership

Linux/OpenWrt product server:

- one raw FakeTCP listener on `WBD_PORT`;
- kernel RST suppression only for WBD-owned raw-port state;
- no parallel product `net.Listen("tcp", WBD_PORT)` Reality listener;
- per-association bootstrap/TLS state, then per-association wolfSSL DTLS worker.

Windows product client:

- Npcap/raw FakeTCP process owns the public flow from SYN onward;
- Reality-like TLS bootstrap runs inside that process/association;
- DTLS starts only after FakeTCP bootstrap readiness;
- LINK/TUN/routes remain readiness-gated and underlay escape remains mandatory.

## Reality-likeness qualification

"Reality-like" is an observed-wire requirement, not a label. Release qualification must capture the first seconds and verify at minimum:

- exactly one client SYN for the WBD session establishment lineage and no second post-auth SYN;
- continuous public 4-tuple and monotonic sequence-space continuity across the bootstrap-to-DTLS switch;
- real TLS 1.3 records and configured SNI during bootstrap;
- no plaintext username/password/ticket in public capture;
- no WBD-specific application bytes before TLS protection;
- a plausible SYN/TCP-option and TLS ClientHello fingerprint against the selected reference profile;
- unrecognized probe behavior reaches the configured fallback path once fallback is enabled.

The initial implementation may use Go TLS while the browser/REALITY fingerprint is brought closer in a later measured step. That temporary implementation must be described as "real TLS single-flow bootstrap", not "99% browser fingerprint qualified", until pcap evidence supports the latter claim.

## No-HOL qualification

A separate post-switch test must deliberately lose/delay an earlier DTLS/FEC FakeTCP payload while delivering a later independent datagram. The later datagram must be observable at the WBD plaintext/data layer before repair of the earlier sequence hole. Passing TLS bootstrap stream tests does not satisfy this gate.

## Superseded clauses

This ADR supersedes only the setup-boundary parts of ADR-0010 and older documents that state any of the following:

- Reality/TLS is an independent public connection from the FakeTCP data lane;
- product startup is `Reality TCP -> close -> new FakeTCP SYN`;
- a kernel TCP Reality listener and raw FakeTCP listener are the intended product shape on the same port;
- a proprietary SYN-option tuple is the product demultiplexing identity.

All non-conflicting ADR-0010 transport, FEC, DTLS, capacity and platform-capture decisions remain in force.
