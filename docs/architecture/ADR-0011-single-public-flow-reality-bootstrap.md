# ADR-0011: One public FakeTCP flow owns Reality-like bootstrap and the no-HOL data plane for each transport lane

Status: **ACCEPTED / AMENDED BY ADR-0012** (original 2026-08-29; lifecycle amendment 2026-08-30)

> ADR-0012 preserves the core single-flow result but changes its scope. The invariant now applies to each independent Transport Lane/epoch, not to the entire lifetime of one Logical Tunnel. Game/race mode and make-before-break replacement may intentionally create multiple independent WBD lanes, each of which must satisfy this ADR on its own.

## Context

The former V2.2 product shape used two logically bound but network-independent connections:

1. an ordinary kernel TCP/TLS Reality-like admission connection that issued a one-time ticket; then
2. a fresh raw FakeTCP association carrying DTLS/FEC/VPN payload.

That shape preserved no-HOL steady-state transport, but a public observer, NAT, firewall or conntrack could see two unrelated TCP flows. Sharing one numeric port did not make them one connection and also created kernel/raw SYN/SYN-ACK/RST interaction that complicated physical Windows/Linux qualification.

The correction introduced by this ADR remains non-negotiable **inside one WBD transport lane**: from that lane's first SYN through Reality-like TLS bootstrap and its steady DTLS/LINK/FEC payload, one raw FakeTCP association owns the lane. There is no second ordinary kernel-TCP WBD payload connection.

ADR-0012 later separated Logical Tunnel lifetime from lane lifetime. A long-lived tunnel may rotate lanes, recover onto a new lane, or intentionally use 2..4 independent race lanes. That does not weaken the per-lane single-association rule.

## Decision

For each independent Transport Lane/epoch, the normative invariant is **one public TCP-shaped 4-tuple and one continuous TCP sequence space** from the lane's first SYN through Reality-like bootstrap, the mode barrier, and steady DTLS/FEC payload. Reality-like recognition moves inside the raw association. The mode switch preserves the **same 4-tuple and same sequence space, no FIN/RST/new SYN**. Reality-likeness qualification therefore requires **exactly one client SYN** for each lane/epoch; additional independent game/migration lanes are separate lanes with their own single SYN lineage.

### 1. One public association per lane/epoch

A WBD Transport Lane has exactly one public carrier association:

```text
client-ip:client-port  <====================>  server-ip:WBD_PORT
          one raw TCP-shaped FakeTCP 4-tuple
```

The normal lane establishment sequence is:

```text
raw FakeTCP SYN / SYN-ACK / ACK
        -> bounded reliable ordered bootstrap mode on the same FakeTCP association
        -> real TLS 1.3 ClientHello/ServerHello/Finished
        -> Reality-like marker recognition + shared username/password admission
        -> one-time lane/session credential returned inside TLS
        -> bootstrap ACK drain / mode barrier
        -> same lane 4-tuple and same lane sequence space, no FIN/RST/new WBD payload SYN
        -> DTLS 1.3 datagrams
        -> LINK / Logical Tunnel attach
        -> lane-local optional fixed FEC
        -> VPN datagrams
```

No ordinary kernel TCP socket owns WBD product payload. Product mode must not create a separate public Reality TCP connection before the FakeTCP payload association for that lane.

### 2. Temporary stream semantics are allowed only during bootstrap

`crypto/tls` requires a reliable ordered byte stream. WBD therefore provides a **bounded bootstrap stream adapter inside FakeTCP**:

- out-of-order bootstrap segments may be retained until the missing sequence arrives;
- bootstrap writes are split into bounded TCP-shaped payload chunks;
- a bootstrap write chunk waits for cumulative ACK before the next chunk is emitted;
- bootstrap has a short absolute deadline and bounded memory;
- the stream adapter is destroyed after admission and the mode barrier.

This HOL behavior is intentional and permitted only for the small setup exchange. It is not the steady-state product transport.

### 3. Steady-state no-HOL semantics remain unchanged

After the mode barrier, later independent DTLS/FEC/application datagrams may complete while an earlier FakeTCP payload is missing. FakeTCP ACK/SACK/shadow retransmission may preserve plausible TCP-shaped outer behavior but must not become an ordered delivery dependency for VPN payload.

The following measured boundaries remain frozen unless separately reopened:

- pinned wolfSSL DTLS 1.3;
- `legacy` FakeTCP shadow recovery default;
- FEC `off` or qualified fixed systematic `20:20`;
- immutable lane-local LINK/FEC parameters;
- 40 Mbit/s aggregate-inner release operating point on the <=100 Mbit/s weak-link target.

ADR-0012 reopens only the old permanent one-lane/lifetime assumption: normal mode remains one steady lane, while later Game Lane/race mode and controlled replacement may use multiple independent lanes.

### 4. Reality-like recognition stays inside each raw association

The public raw listener, not a kernel TCP listener, owns the WBD port from SYN onward. WBD identification must happen from the TLS ClientHello/Reality-like marker, not from a proprietary FakeTCP SYN fingerprint.

For each candidate lane:

- WBD client: plausible TCP SYN -> real TLS 1.3 ClientHello with configured SNI and WBD Reality-like recognition marker -> authenticated WBD branch;
- unrecognized client: remain in stream mode and proxy the byte stream to the configured decoy/fallback target so ordinary probes do not merely see a dead/raw-only port.

### 5. Mode switch is a per-lane protocol boundary

The TLS/bootstrap phase and DTLS/datagram phase share the same lane association but not the same delivery semantics. A switch is valid only when:

- TLS authentication succeeded;
- both peers agree to leave bootstrap mode;
- all bootstrap outbound chunks are cumulatively ACKed;
- no bootstrap byte remains waiting for ordered delivery;
- neither endpoint emits FIN/RST/new WBD payload SYN as part of the switch.

A failed bootstrap closes that candidate lane; it must not silently fall into DTLS mode.

### 6. Relationship to Logical Tunnel multipath

ADR-0012 defines a Logical Tunnel above these lanes.

Examples:

```text
normal steady state:
  Tunnel T -> Lane A

game/race steady state:
  Tunnel T -> Lane A + Lane B

make-before-break replacement:
  Tunnel T -> Lane A
            -> build Lane B
            -> brief A+B race
            -> retire A
            -> Lane B
```

Every A/B/C lane independently satisfies this ADR. The race layer may copy one logical PacketID across lanes and suppress duplicate arrivals, but it never merges FakeTCP sequence spaces or FEC blocks across lanes.

This is not rejected V1 PR #2. V1 placed redundancy/FEC above ordinary kernel TCP lanes and inherited their HOL. Later Game Lane uses independent WBD FakeTCP/DTLS/LINK associations and first-arrival/dedup semantics.

## Platform ownership

Linux/OpenWrt product server:

- one public raw FakeTCP listener on `WBD_PORT` fans out independent WBD lane associations by raw tuple;
- kernel RST suppression is narrowly scoped to WBD-owned raw-port state;
- no parallel product `net.Listen("tcp", WBD_PORT)` Reality listener owns WBD admission/payload;
- each lane has independent bootstrap/TLS and DTLS/LINK state before attaching to a Logical Tunnel.

Windows product client:

- Npcap/raw FakeTCP owns each lane from SYN onward;
- Reality-like TLS bootstrap runs inside that lane's raw association;
- DTLS/LINK starts only after lane bootstrap readiness;
- Wintun/Logical Tunnel may remain alive while lanes are replaced or temporarily dormant;
- underlay escape remains mandatory.

## Reality-likeness qualification

For every lane establishment under test, capture the first seconds and verify at minimum:

- one client SYN lineage for that lane and no second post-auth WBD payload SYN for the same lane;
- continuous lane 4-tuple and sequence-space continuity across bootstrap-to-DTLS switch;
- real TLS 1.3 records and configured SNI;
- no plaintext username/password/ticket/credential;
- no WBD-specific application bytes before TLS protection;
- plausible SYN/TCP-option and TLS ClientHello fingerprint against the selected reference profile;
- fallback behavior for unrecognized probes.

A test must not interpret a legitimate second independent game/migration lane as a violation of this ADR.

## No-HOL qualification

A separate post-switch test must deliberately lose/delay an earlier DTLS/FEC FakeTCP payload on a lane while delivering a later independent datagram. The later datagram must be observable at the WBD plaintext/data layer before repair of the earlier sequence hole. Passing TLS bootstrap stream tests does not satisfy this gate.

## Superseded clauses

ADR-0011 still supersedes older text that states:

- Reality/TLS is an independent public ordinary-TCP connection from the FakeTCP payload lane;
- product lane startup is `Reality TCP -> close -> new FakeTCP SYN`;
- a kernel TCP Reality listener and raw FakeTCP listener are the intended product same-port WBD path;
- a proprietary SYN-option tuple is the product demultiplexing identity.

ADR-0012 additionally supersedes the original ADR-0011 wording that said one entire WBD VPN session must keep one public 4-tuple from first SYN until user Disconnect. The correct scope is now one **Transport Lane/epoch**.
