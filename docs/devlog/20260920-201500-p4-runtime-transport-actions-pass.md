# P4 unified runtime transport Actions PASS

Date: 2026-09-20  
Qualified SOURCE_SHA: `ef269081e45bc516ae41943e56a01a8032e4cf8a`  
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35498412990  
Result: completed / success, 7 of 7 jobs PASS

## Qualified product boundary

This closes the hosted P4 runtime-transport atom started after the OpenWrt socket/TunnelOwner closure.

`internal/runtimeowner` now owns the post-admission transport seam for one leased `TunnelOwner`:

- existing TLS-like `WireRecord.Wire` bytes are emitted on the lane's existing TCP-shaped four-tuple and sequence space through `faketcp.SegmentEmitter`;
- inbound steady payload is dispatched directly to `TunnelOwner.InboundPayload` or `GameInboundPayload`, then to the platform packet sink;
- FakeTCP sequence gaps do not impose business HOL: later independent TLS-like records are delivered on first arrival while cumulative ACK/repair state continues below them;
- repair metadata is bounded at 4096 records; default retry is 1s with a 3s absolute first-send horizon, after which optional repair is abandoned rather than blocking fresh traffic;
- exact retransmit reuses sequence and payload bytes;
- initial attach, same-ID replacement promote/fail/retire, DORMANT cleanup and generation fencing remain owned by the existing `TunnelOwner`;
- client protected-admission results now have the same explicit `LaneConfig` handoff as server admission;
- after detach, `ServerAssociation` no longer rejects a larger steady ACK as an invalid bootstrap ACK; runtime transport owns that post-bootstrap sequence range;
- `openwrtclient.SocketAdapter.DeliverFromOwner` provides the public reverse in-process service path.

No localhost UDP WBD bridge, DTLS shim, platform-proxy subprocess, or old Controller topology was restored.

## Pre-fix run retained

SOURCE_SHA `12dfe1c15fca20210f47fd3cd557eb8eff6779d3`, run `35497343943`, did not qualify. repository-contract, P2, OpenWrt and both shared-TUN privileged jobs passed, but Windows/Linux active-go-tests stopped compiling the new test helper:

```
runtime_test.go:112:23: 40000 (untyped int constant) overflows uint8
runtime_test.go:113:23: 440 (untyped int constant) overflows uint8
```

The follow-up changed only the test helper to perform port arithmetic in `uint16`; product runtime code was unchanged.

## Exact-SHA evidence

### Active Go / runtimeowner

Ubuntu 24.04:

- full unit/build PASS;
- `internal/runtimeowner` unit PASS: 0.010s;
- `go test -race ./... -count=1` PASS;
- `internal/runtimeowner` under race PASS: 1.038s;
- directed `tlsrecord` fuzz ran for 15s, 63,415 executions, PASS.

Windows Server 2022:

- full unit/build PASS;
- `internal/runtimeowner` PASS: 0.024s;
- existing Wintun render contract PASS, including `WBD_WINDOWS_CLIENT_PLAN schema=wbd-windows-client-state/v1 adapter=WBD lease=10.66.0.7/32`;
- Npcap hosted marker remains `WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN`.

### P2 kernel fallback regression

`TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS in 1.24s.

Continuous pcap:

- 29 packets captured;
- 58 packets received by filter;
- 0 packets dropped by kernel;
- analyzer result PASS.

### Linux shared-TUN privileged regression

Both real root/netfilter backends PASS:

```
WBD_P4_LINUX_SHARED_TUN_PASS backend=iptables active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged
WBD_P4_LINUX_SHARED_TUN_PASS backend=nft active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged
```

### OpenWrt privileged regression

Both existing root-netns contracts continue to PASS:

```
WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED
WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED
```

The TPROXY runtime test passed in 0.22s; the socket/TunnelOwner test passed in 0.13s.

## Artifacts

- foundation: `10601182487`, `sha256:fab9863f94e74e7e3fff2b6c4eb3af8244750885803fe62ebef066493d5e5f9e`
- tlsrecord-reference: `10601483400`, `sha256:7a1b53dc650c1ffd6cc83b7b8d70d2a9759b2908ab316d89497c9b01e5815361`
- P2 kernel fallback: `10600969017`, `sha256:2e954b3d9e5a05c3fbe78db17cf4a462267bdfd10920ceb667bb2948ced3602e`
- shared-TUN iptables: `10601428401`, `sha256:b5331d5a42dba77d2191f032a56143527c84682095472892694048105f6ba96d`
- shared-TUN nft: `10601203685`, `sha256:be20467f72911f02c7042276621c3e827091948f49fcd6374b89303d2eb8943b`
- OpenWrt privileged: `10600588872`, `sha256:83450087a523f9611b8e7f5dd64aac1b73347f241988d2be20a4da6e6e95f991`

## Qualification interpretation

`docs/STATUS.json:last_tested_source_sha` advances to `ef269081e45bc516ae41943e56a01a8032e4cf8a`.

This is a hosted transport-owner qualification, not a physical Windows/Npcap end-to-end qualification. `SegmentEmitter` is deliberately the seam for the already-extracted Linux raw and Windows Npcap packet adapters; the final runnable client/server entry and endpoint loops are the next P4 atom. Physical status remains `NOT_RUN`.

P5 load/weak-network/HTTPS appearance work remains out of scope.
