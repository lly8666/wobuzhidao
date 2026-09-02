# 2026-08-30 Windows Raw-IP Gateway Development Log

> **SUPERSEDED PRODUCT DIRECTION — historical evidence only.**
>
> The `Selected model: one Linux network namespace per Windows LiveID` section below records the correctness path chosen before ADR-0012. It is no longer the final product architecture. ADR-0012 now requires a server-assigned unique Logical Tunnel address lease, one shared Linux TUN, one WBD-owned host NAT, and lease-based downlink demux. The existing netns/veth/double-NAT implementation/tests may remain as historical/reference evidence, but agents must not continue expanding it as mainline product work.
>
> The VRF/conntrack-zone prototype remains rejected. Do not revive it.
>
> Current execution contract: `docs/development/2026-08-30-architecture-pivot-tunnel-multipath.md`.

This log is append-only engineering context for the post-single-flow upper-data-plane repair. It complements `SINGLE_FLOW_V23_DEVLOG.md` and exists so an interrupted chat cannot erase why a design was selected or rejected.

## Physical blocker being repaired

The physical Windows 11 -> Ubuntu ARM64 test proved the one-public-flow transport through Reality-like bootstrap, FakeTCP, DTLS 1.3, LINK, Wintun creation and Windows route application. The first deterministic failure is sustained L3 payload: DNS-style UDP, generic UDP and TCP time out after `connect_pass`.

The source-level root cause is an adapter mismatch above LINK. Windows Wintun emits M6A raw-IP `WBDP` envelopes (`internal/dataplane`), while the Linux service at 127.0.0.1:49000 is `wbd-platform-proxy-server`, which expects the unrelated 44-byte L4 `internal/platformproxy` envelope. LINK correctly preserves payload bytes and therefore exposed this wrong backend composition.

FakeTCP recovery/FEC, DTLS and LINK wire semantics are not part of this repair.

## Backend classifier

`wbd-link-server-mux` now defers its service dial until the first decoded application datagram. It recognizes a valid M6A raw-IP frame with `dataplane.UnmarshalIP`; otherwise it requires a valid `platformproxy.Unmarshal`. The selected backend is pinned for the LiveID lifetime.

This preserves the existing OpenWrt L4 platform proxy while allowing Windows L3 sessions to use a dedicated raw-IP gateway.

## Isolation requirement

Every current Windows client uses inner `10.66.0.2/30`. Multiple same-account LiveIDs must be concurrent, and two clients may also create identical inner TCP/UDP tuples. Therefore a single root-namespace TUN is invalid: Linux routing/conntrack could not identify which LiveID owns an identical packet.

The server-side Windows adapter must provide a distinct kernel network/conntrack domain per active LiveID while keeping the outer FakeTCP/DTLS/LINK path unchanged.

## Rejected first prototype: VRF + conntrack zone

An initial implementation used one VRF, TUN and conntrack zone per service peer, with a unique RFC2544 transit /30. This had an attractive property: only CAP_NET_ADMIN was needed and identical inner tuples could be separated by conntrack zone.

It was rejected before product integration for a return-path/NAT reason.

VRFs share the host network namespace and therefore share the same conntrack table. A packet can be assigned a per-session zone when it first enters a session TUN, but the product still needs public egress NAT on the host. On the public return path, before conntrack lookup, the physical packet does not contain a trustworthy LiveID/zone selector. A test network that happens to route the private transit prefix can conceal this problem and produce a false green result. Relying on stacked SNAT inside one conntrack namespace is not an acceptable release design.

The VRF prototype is therefore experimental history only and must not be wired into the Linux manager or release bundle.

## Selected model: one Linux network namespace per Windows LiveID

> **Historical selection only; superseded by ADR-0012. Do not implement new product work from this section.**

The raw-IP backend will create a real Linux network namespace per active Windows service peer/LiveID.

For each session:

1. create a TUN device for the Windows inner L3 link;
2. move the TUN into that session namespace while the gateway retains the TUN file descriptor;
3. create one root<->namespace veth /30 from a WBD-owned transit prefix;
4. assign `10.66.0.1/30` to the namespace-side TUN; the remote Windows client remains `10.66.0.2/30`;
5. enable forwarding in the namespace;
6. perform inner NAT in the namespace from `10.66.0.0/30` to the namespace veth address;
7. perform ordinary WBD-owned host NAT from the unique transit prefix to the physical egress path;
8. on return, host conntrack reverses outer NAT, routing selects the unique session veth, then that namespace's independent conntrack reverses inner NAT and the reply reaches the correct TUN;
9. read the reply from the retained TUN fd, wrap it as M6A and return it to the same LINK service peer.

Because every namespace owns a separate conntrack table, two sessions can safely use the same inner `10.66.0.2`, the same TCP source port and the same Internet destination simultaneously. The kernel, not WBD, continues to implement inner TCP semantics.

This model needs namespace-management privilege. Product hardening should isolate that privilege to the raw-IP gateway/service rather than broaden unrelated public transport children. Correctness is required before capability minimization.

## Qualification requirement

> **Historical netns qualification contract only.** ADR-0012 replaces the final product gate with distinct server-assigned tunnel IPs, shared TUN, one host NAT, anti-spoof validation, dynamic lane lifecycle, idle/wake and make-before-break replacement. The scenarios below may still be reused as behavioral stress cases where useful.

The privileged CI must not merely ping a transit address. It must:

- launch two real `wbd-tun` client endpoints;
- give both client TUNs `10.66.0.2/30`;
- create two simultaneous sessions through the raw-IP gateway;
- run DNS-style UDP and generic UDP for both clients;
- run simultaneous TCP where both clients bind `10.66.0.2:40000` and connect to the same target/port;
- place the target in an Internet namespace that sees the host-side NAT identity, not the WBD private transit address, so missing outer NAT cannot fake success;
- exercise both iptables and nft backends where supported;
- prove deterministic namespace/session cleanup;
- only after this direct adapter test passes, insert it behind the real single-flow FakeTCP -> DTLS -> LINK stack and repeat the probes.

No physical package is release-ready until the current ADR-0012 deterministic tests are green and the final real Windows probes pass.
