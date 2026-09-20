# 20260920-184000 P4 OpenWrt socket -> TunnelOwner Actions闭环

## Final qualification

SOURCE_SHA: a8438872f948849ab378238a69dc1a1e28c86ac6
GitHub Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35496415805
Result: completed / success, 7/7 jobs PASS.

## Jobs

- repository-contract: PASS
- active-go-tests Windows 2022: PASS
- active-go-tests Ubuntu 24.04: PASS, including race, directed tlsrecord fuzz and independent reference vectors
- p2-kernel-fallback: PASS
- p4-linux-shared-tun-privileged iptables: PASS
- p4-linux-shared-tun-privileged nft: PASS
- p4-openwrt-tproxy-privileged: PASS

## OpenWrt real root-netns evidence

Existing kernel-ownership marker:
WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED

New socket/TunnelOwner marker:
WBD_P4_OPENWRT_SOCKET_TUNNEL_PASS tcp=1 udp=1 eim=1 eif=1 tunnelowner=1 lanes=1 service_tun_leak=0 ipv6=NOT_IMPLEMENTED

TestPrivilegedOpenWrtTPROXYRuntime PASS in 0.21s. TestPrivilegedOpenWrtSocketTunnelAdapter PASS in 0.14s.

The socket contract used real client/router/target namespaces, active nft TPROXY, IP_TRANSPARENT TCP/UDP sockets, leased client/server TunnelOwner+Lane objects and real target application sockets. TCP target only accepts the server-side router source, so success proves re-originated target I/O. One client UDP socket used two destinations with one observed server mapping port (EIM), then accepted an unsolicited packet from a never-contacted third endpoint with its true source (EIF). Authoritative lane count stayed one and reserved platform service traffic produced zero shared-TUN writes.

## Regression evidence

Windows Wintun Render emitted WBD_WINDOWS_CLIENT_PLAN, UNDERLAY 198.51.100.10/32, ADDRESS_EXCLUSIVE 10.66.0.7/32, DNS_NRPT namespace=., IPV6_FAIL_CLOSED and CLEANUP state_owned_only=1. Windows Npcap hosted marker: WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN.

P2 kernel fallback: TestKernelTLSFallbackVerifiedHTTPAndNormalClose PASS 1.04s; 28 packets captured; 56 packets received by filter; 0 packets dropped by kernel; analyzer result PASS.

Linux shared-TUN markers:
- WBD_P4_LINUX_SHARED_TUN_PASS backend=iptables active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged
- WBD_P4_LINUX_SHARED_TUN_PASS backend=nft active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged

## Artifacts

- foundation: 10601140638, sha256:e5120e3c16c094f168f3ad4499d1b7fdad2b351562f5a92bf963229538ecf3e4
- tlsrecord-reference: 10600661014, sha256:ed0d3fc654d666fc625c3c321b9487b64159f8d4c7cacddd8ec074ecdc3020a0
- p2-kernel-fallback: 10601495214, sha256:f584fefa363885e6989c12fa0de8f9685c31444d5c330acf1375911921f40e75
- shared-TUN iptables: 10601410284, sha256:fe6fb1c0044e412afa71f8ad5b4c36f833206eb95bf2243023ba272f5bb5f6fb
- shared-TUN nft: 10600735686, sha256:2fcb762451b4320055b45fdbf3de6f1b2a3176dff08a1ba835c8c38873500a41
- OpenWrt TPROXY/socket: 10601295373, sha256:3fcd71df9a88da59c704b5e75f56c81057b0d9f3bb993462841441addc743b7b

## Failure history retained

64b54310215898fbc9a2a559bbd7104acea9dcda / run 35496266975 remains recorded as a harness-only FAIL: all non-OpenWrt jobs passed and the OpenWrt test binary compiled, but the namespace shell script had an unclosed quoted test regex and had not applied target .4/.5 aliases, so no root-netns tests executed. The follow-up a8438872... changed only the harness/docs and produced the final PASS.

## Qualification boundary

The application endpoints, TPROXY sockets and TunnelOwner/Lane objects are real. The carrier between the in-process client/server Lane owners is still a WireSink callback; this is hosted platform/socket/TunnelOwner qualification, not final raw FakeTCP/Npcap transport or P7 physical evidence. OpenWrt IPv6 remains NOT_IMPLEMENTED.

## Next atomic task

Wire the unified client/server runtime to the real transport: admission/handoff -> leased TunnelOwner -> transport WireSink/receive dispatch -> platform endpoints, with one-process accept/attach/shutdown lifecycle. Do not restore localhost UDP, DTLS shim, platform-proxy subprocesses or old Controller topology. Keep P5 load/weak-network work out of this atom.
