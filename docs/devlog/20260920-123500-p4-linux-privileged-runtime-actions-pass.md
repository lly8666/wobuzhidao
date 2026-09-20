# 20260920-123500 P4 Linux privileged shared-TUN runtime Actions闭环

## 最终资格

SOURCE_SHA: a2cf881445f0efb4dcca87663074d594894db6ff
Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35488614508
Result: completed / success.

PASS jobs:
- repository-contract
- active-go-tests windows-2022
- active-go-tests ubuntu-24.04 including race/fuzz/reference
- p2-kernel-fallback
- p4-linux-shared-tun-privileged (iptables)
- p4-linux-shared-tun-privileged (nft)

Privileged evidence:
- iptables: WBD_P4_LINUX_SHARED_TUN_PASS backend=iptables active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged; TestPrivilegedSharedTUNRuntime PASS 0.08s.
- nft: WBD_P4_LINUX_SHARED_TUN_PASS backend=nft active_tunnels=2 cleanup=owned-only dns=host-dns-unchanged; TestPrivilegedSharedTUNRuntime PASS 0.12s.

Each backend ran as root in an isolated network namespace and actually:
- verified /dev/net/tun after modprobe tun
- created one nonpersistent wbdg0 IFF_TUN|IFF_NO_PI device
- applied MTU/up and 10.66.0.0/16 route
- saved/set/restored ip_forward and wbdg0 rp_filter
- installed exactly WBD-owned forward/NAT markers
- registered two Logical Tunnel leases and exercised Normal/Game shared-TUN router boundaries
- closed runtime and verified TUN/route/WBD firewall state disappeared
- preserved a pre-existing forward policy DROP

P2 regression remained PASS: 1.24s, 29 packets captured, 58 received by filter, 0 dropped.

Artifacts:
- foundation 10597644172
- tlsrecord-reference 10598128874
- p2-kernel-fallback 10597659274
- p4 linux iptables 10598437116
- p4 linux nft 10597902454

## Failure history retained

45b17953... / Actions 35488432157: workflow YAML parse FAIL before jobs due heredoc indentation; CI-only fix.
5d7a05de... / Actions 35488488384: all base jobs PASS, both privileged backends reached real runtime but test over-required literal interface name in an already dev-filtered `ip route show`; test-only assertion fix.

## Qualification boundary

This is a real GitHub-hosted privileged Linux TUN/netfilter gate, not merely mock/core evidence. It is still not P7 physical-machine soak/performance/fault qualification.

## Next atom

Windows client Wintun platform core:
- Wintun L3 adapter primitive with IPv4-only fail-closed reads and clean shutdown
- stable lease /32 exclusive address ownership
- WBD-owned route state with underlay server /32 pinned to physical path to prevent recursive capture
- capture/direct route ownership and exact cleanup
- NRPT DNS ownership/cleanup
- device-wide IPv6 fail-closed while IPv6 proxy is not implemented
- direct integration with existing TunnelOwner Normal/Game boundaries, no legacy subprocess Controller/Game child orchestration
- Npcap physical underlay remains a separate following atom.
