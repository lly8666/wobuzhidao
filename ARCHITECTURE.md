# Architecture v2.3

> **Status: ACTIVE MAINLINE DESIGN.** ADR-0011 reopens only the former Reality/FakeTCP setup boundary. The product now requires one public TCP-shaped FakeTCP flow from the first SYN through Reality-like TLS bootstrap and the steady-state no-HOL DTLS/FEC data plane.

## Product intent

WBD is a personal weak-network VPN for privileged OpenWrt/Linux and Windows endpoints. The public carrier must look TCP-like, while sustained VPN payload stays packet/datagram-oriented and must not inherit ordinary TCP ordered-delivery HOL.

The defining public-wire invariant is now:

**one session = one public 4-tuple + one SYN lineage + one continuous FakeTCP sequence space.**

A separate ordinary TCP Reality connection followed by a fresh FakeTCP connection is not a valid product shape.

## Canonical stack

```text
TUN / TPROXY captured IP packet
        ↓
WBD packet/session layer
        ↓
optional fixed systematic FEC
        ↓
DTLS 1.3 application datagram
        ↓
WBD FakeTCP raw association
        ↓
public network
```

The same FakeTCP association also carries the short startup phase before DTLS:

```text
raw FakeTCP SYN / SYN-ACK / ACK
        ↓
temporary reliable ordered bootstrap stream
        ↓
real TLS 1.3 + Reality-like recognition/admission
        ↓
bootstrap drain + mode barrier
        ↓
SAME 4-tuple / SAME sequence space / NO new SYN
        ↓
DTLS 1.3 + immutable LINK + FEC/datagrams
```

## Why the bootstrap stream does not violate the no-HOL goal

TLS itself requires reliable ordered bytes. During the first few seconds WBD temporarily enables a bounded stream adapter inside its own FakeTCP association. Bootstrap chunks are retransmitted/ordered and may block on an earlier missing byte.

That behavior is deliberately limited to setup. Once authentication and the mode barrier complete, the stream adapter is destroyed. Steady-state delivery returns to packet semantics: later independent DTLS/FEC datagrams may complete while an earlier FakeTCP sequence range is missing.

The product must never route sustained VPN payload through an ordinary kernel TCP byte stream.

## Reality-like bootstrap

The bootstrap is no longer an independent public lane. It is the first payload phase of the FakeTCP association.

The target wire behavior is:

- plausible TCP-shaped SYN/SYN-ACK/ACK;
- real TLS 1.3 ClientHello/ServerHello/Finished on that same sequence space;
- configured SNI plus the WBD Reality-like recognition marker;
- username/password sent only inside TLS;
- one-time session ticket returned only inside TLS;
- no FIN/RST/new SYN between TLS bootstrap and DTLS data mode.

Product identification happens at the TLS ClientHello/marker layer, not by relying on a proprietary FakeTCP SYN-option fingerprint.

For active-probe resemblance, the target server behavior for an unrecognized ClientHello is to remain in the temporary stream mode and proxy to the configured fallback target. Until that fallback and fingerprint surface are measured, the implementation is described as real-TLS single-flow bootstrap rather than claiming browser-perfect REALITY equivalence.

## TCP-shaped outer semantics

FakeTCP owns the public 4-tuple, sequence/ACK state and packet emission from the first SYN onward. The local kernel does not own a WBD `ESTABLISHED` payload socket.

TCP-shaped means the outer packets maintain qualified TCP-like structure and state behavior, including SYN/SYN-ACK/ACK, sequence/ACK progression, plausible options/windows and selected shadow ACK/SACK/retransmission behavior. It does **not** mean that the kernel is allowed to impose stream delivery, congestion-control HOL or byte-stream ownership on VPN payload.

## DTLS and FEC ordering

Steady state remains:

```text
WBD/application datagram
   ↓
optional systematic FEC source/repair shard
   ↓
independent DTLS 1.3 application datagram
   ↓
FakeTCP raw carrier packet
```

Every source/repair shard remains independently authenticated. Losing one DTLS record must not prevent a later complete datagram from becoming available.

## Security authority

DTLS 1.3 remains the steady-state cryptographic authority. Initial pin remains wolfSSL `v5.9.2-stable`, commit `ac01707f552c611fbd135cc723b2682b3e7f80f2`.

The Reality-like TLS phase protects setup/admission and provides traffic resemblance. It does not replace DTLS for sustained VPN payload and does not create a second post-handshake cipher.

## Session identity

One shared username/password may authenticate several devices. Every successful TLS bootstrap creates a fresh one-time ticket. Live identity is ticket/LiveID based, never username based.

The ticket belongs to the **same public FakeTCP association** that performed the TLS bootstrap. It is carried into DTLS-protected LINK setup to bind the authenticated admission result to the live session. No second public connection is created for that bind.

## Server fan-out

One public raw listener fans out many independent FakeTCP associations by client/server 4-tuple:

```text
public WBD_PORT raw listener
  -> association A: TLS bootstrap -> DTLS worker A -> LiveID A
  -> association B: TLS bootstrap -> DTLS worker B -> LiveID B
  -> association C: fallback stream -> configured decoy target
```

A product server must not run a parallel kernel TCP Reality listener on the same WBD port. Kernel RST suppression remains narrowly scoped to WBD-owned raw-port state.

## Windows client

The Windows Npcap/raw FakeTCP process owns the public session from SYN onward. It performs the TLS bootstrap on the raw association, writes/exports the resulting one-time ticket locally, then keeps the same association alive for the DTLS/FEC data phase.

Startup remains readiness-gated:

```text
FakeTCP single-flow TLS/auth ready
  -> DTLS ready
  -> LINK ready
  -> TUN ready
  -> IPv6 fail-closed rule
  -> capture routes
```

Underlay escape remains mandatory.

## OpenWrt/Linux client/server

Linux/OpenWrt use privileged raw packet I/O for the public FakeTCP association. OpenWrt final capture remains TPROXY + policy routing; Linux TUN remains a development/reference harness where useful.

## Frozen release operating point

ADR-0011 does not reopen the measured steady-state transport choices from ADR-0010:

- <=100 Mbit/s weak-link qualification ceiling;
- 40 Mbit/s aggregate inner release operating point;
- one raw lane;
- `legacy` FakeTCP shadow recovery default;
- FEC `off` or qualified fixed systematic `20:20`;
- immutable LINK parameters.

## Required qualification split

### Bootstrap/wire qualification

Capture the first seconds and prove:

- exactly one public WBD connection lineage; no second post-auth SYN;
- same 4-tuple before and after mode switch;
- continuous sequence space across the switch;
- real TLS 1.3 records and configured SNI;
- credentials/ticket absent from plaintext capture;
- plausible TCP/TLS fingerprint against the selected reference;
- fallback behavior for unrecognized probes once implemented.

### Steady-state no-HOL qualification

After switching to DTLS/datagram mode, deliberately lose or delay an earlier FakeTCP payload and deliver a later independent datagram. The later datagram must reach the authenticated WBD plaintext/data layer before the earlier sequence hole is repaired.

Bootstrap stream success is not evidence for this gate; the two delivery modes are intentionally different.

## Retired product shapes

The following are not valid V2.3 product architectures:

- ordinary kernel TCP as sustained VPN carrier;
- `Reality TCP connection -> close -> new FakeTCP SYN`;
- parallel kernel Reality listener + raw FakeTCP listener as the intended same-port product design;
- kernel TCP state takeover as a release dependency;
- VLESS/Xray/Vision stream semantics as the WBD data plane;
- runtime FEC epoch switching;
- mandatory multi-lane transport.

Historical experiments and diagnostic binaries may remain in-tree when clearly marked non-product.
