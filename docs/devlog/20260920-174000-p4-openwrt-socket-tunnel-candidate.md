# 20260920-174000 P4 OpenWrt socket -> TunnelOwner candidate

## Authority and scope

Base docs HEAD: `efef13244fba6a0ac46fa46fb90f4c01e6b2211c`.

Previous product gate remains SOURCE_SHA `5018dc2f417f195f8b1210e5f50208db7a8ba3ca` / Actions 35494105026: OpenWrt IPv4 kernel TPROXY ownership PASS. This candidate does not overwrite that tested SHA until its own Actions complete.

This atom implements transparent TCP/UDP application sockets directly on the existing leased TunnelOwner/Lane core. It does **not** build a unified final client/server executable and does not restore archived platform-proxy processes, localhost UDP WBD transport, DTLS shim, or old command topology.

## Archive extraction

Directed sources:
- `old/cmd/wbd-platform-proxy-openwrt/main_linux.go`
- `old/cmd/wbd-platform-proxy-server/main.go`
- `old/internal/platformproxy/frame.go`
- `old/internal/platformproxy/udp_client_flows.go`
- `old/internal/platformproxy/udp_relay.go`
- `old/internal/platformproxy/tcp_reliability.go`
- `old/internal/platformproxy/tcp_client.go`
- `old/internal/platformproxy/tcp_relay.go`
- `old/internal/platformproxy/udp_tproxy_linux.go`
- `old/internal/platformproxy/tcp_tproxy_linux.go`
- archived OpenWrt TCP/UDP/fullstack tests/workflow.

Only flow semantics and socket ownership were extracted.

## New platform service boundary

`internal/platformflow` defines a new internal `WBPF` v1 frame. It is deliberately not old `WBDP` compatibility.

Each frame is wrapped in one valid IPv4 service packet:
- protocol = 253;
- DF set;
- source = tunnel lease /32;
- destination = the same tunnel lease /32;
- exact IPv4 total length and header checksum;
- max packet remains `logicaltunnel.MaxLeasedIPv4PacketLen` (9000).

This shape keeps existing leased-client source validation authoritative. Server owner ingress still checks source==lease before the platform layer. The shared-TUN router recognizes protocol 253 before TUN egress; malformed reserved packets, missing handlers and stale binding tokens fail closed.

## Direct owner usage

`TunnelChannel.OpenFlow` always opens an existing `datapath.BusinessFlow`.

- Normal=1: frame packet -> `BusinessFlow.Outbound`.
- Game=2..4: frame packet -> `TunnelOwner.GameOutbound`; the BusinessFlow remains lifecycle/accounting ownership only.
- No path creates or replaces a lane.
- Flow close releases only its BusinessFlow.
- Wire output is handed to a caller-supplied concurrency-safe `WireSink`, which is the future unified runtime transport hook.

Hosted unit tests require Game fan-out through exactly the existing three lanes and require lane count to remain unchanged.

## UDP

Client mapping identity is only the intercepted client source endpoint. One source talking to different remote peers keeps one FlowID and one BusinessFlow (EIM).

Server uses one unconnected UDP socket per tunnel+FlowID. Responses carry the actual remote source, and the client mapping accepts an unseen same-family remote endpoint (EIF).

Both sides have mapping count and idle bounds. OpenWrt reverse delivery uses an IP_TRANSPARENT source socket keyed by remote peer with its own bounded cache.

## TCP

Application TCP is a byte stream above the lossy/no-HOL tunnel, so the extracted adapter keeps a separate bounded reliability layer:
- offset DATA;
- cumulative ACK;
- bounded in-flight and reorder bytes;
- duplicate idempotence/conflicting overlap rejection;
- finite RTO retransmits;
- FIN/half-close and CLOSE;
- bounded flow count and idle timeout.

This does not change FakeTCP repair, FEC, LINK or TLS-like record logic.

## Linux OpenWrt socket adapter

`internal/openwrtclient/socket_linux.go`:
- IPv4 IP_TRANSPARENT TCP listener;
- original TCP destination from accepted LocalAddr;
- IPv4 UDP IP_RECVORIGDSTADDR ancillary parsing;
- direct calls to `platformflow.Client`;
- bounded transparent source-socket cache for reverse UDP;
- deterministic Close and periodic Tick.

IPv6 remains NOT_IMPLEMENTED in this atom.

## Shared-TUN service handler

`internal/linuxserver/router.go` now supports a handler bound to the current `BindingToken`.

- service packet without a handler: reject;
- malformed protocol-253 packet: reject;
- stale token: reject;
- exact identity rebind clears the previous handler;
- ordinary leased IPv4 still goes to the TUN exactly as before.

## Root-netns contract

The existing OpenWrt namespace harness is extended rather than adding a parallel workflow.

`TestPrivilegedOpenWrtSocketTunnelAdapter` uses:
- real client/router/target namespaces;
- real active nft TPROXY plan/runtime;
- real IP_TRANSPARENT TCP/UDP capture;
- real leased client/server TunnelOwner + Lane instances;
- in-process WireSink to move the existing TLS-like WireRecords between those two owners;
- real server TCP/UDP target sockets in the target namespace.

Assertions:
1. target TCP service only accepts source `10.20.0.1`; successful client request proves server-side re-originated TCP;
2. one client UDP socket reaches `10.20.0.2:5355` and `10.20.0.4:5356` with the same observed server mapping port (EIM);
3. a never-contacted `10.20.0.5:7777` endpoint injects to that mapped port and arrives at the original client socket with its true source (EIF);
4. client/server authoritative lane count remains one;
5. platform service packets are consumed before shared TUN: `service_tun_leak=0`.

Required marker:

`WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED`

The existing TPROXY ownership marker remains required in the same job.

## Qualification boundary

This root-netns test is stronger than a pure mock because both application sides and TPROXY sockets are real kernel sockets. However, the carrier between the two in-process Lane owners is a WireSink callback, not final raw FakeTCP/Npcap platform I/O. Therefore this is platform/socket/TunnelOwner hosted qualification, not physical end-to-end or P7 evidence.

## Next

Commit the candidate and run full exact-SHA `next-foundation`. Any failure remains inside this atom. Only after all existing regressions and both OpenWrt markers pass should evidence closure advance to unified runtime transport wiring.
