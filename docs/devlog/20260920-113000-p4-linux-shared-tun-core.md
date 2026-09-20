# 20260920-113000 P4 Linux shared-TUN platform core

## 原子任务

开始时确认 next/tlslike-dataplane HEAD 仍为 5335dcff6b4f0a4d39da7511be6ab3c1e6e8e674，STATUS.next_task 仍要求 Linux server shared-TUN / NAT / DNS platform core。

本轮只做 Actions 可测试的 platform core、Linux TUN primitive 与 ownership plan；不会把 hosted unit test 冒充真实 root TUN/netfilter 资格。

## 归档读取

Archive source SHA: b5c848f4e9afdffd15d1bc451560edf4e9390a35

- old/cmd/wbd-ip-gateway-server/main_linux.go：确认 per-session netns/TUN/veth/double-NAT 是已淘汰 prototype，不迁移。
- old/cmd/wbd-ip-gateway-shared/main_linux.go + tests：保留 one shared root TUN、lease /32 registry、source fence、destination demux、exact tunnel+lease rebind。
- old/internal/rawipbackend/meta.go：只用于理解旧进程间 metadata；active 不迁移该 bridge。
- old/internal/tunnel/tun_linux.go：提取最小 /dev/net/tun IFF_TUN|IFF_NO_PI primitive。
- old/scripts/linux_shared_tun_firewall.sh：保留 WBD-owned FORWARD/NAT 与 ip_forward restore ownership。
- old linux server manager/settings 与 shared-TUN development logs：确认最终架构是 one root TUN + one host NAT，不是 per-session netns。

## active TunnelOwner platform send

新增 TunnelOwner.NormalOutbound(packet, now)：只允许 desired=1，直接使用 authoritative lane1，不创建 BusinessFlow；RoleClient leased owner 仍在 Lane 前执行 source==lease；沿用 tunnel padding budget，最后仍做 generation fence。Game desired=2/3/4 继续使用已资格化的 GameOutbound。

## internal/linuxserver SharedTUNRouter

active registry 直接绑定 TunnelOwner，不再有 old localhost UDP backend peer。注册要求 owner 有 valid stable Lease，lease /32 在 shared pool 内，TunnelID 与 lease 唯一。exact same TunnelID+/32 可 rebind 到新 runtime owner，产生新 BindingToken；old token 立即 stale。

TUN -> tunnel owner: strict parse one IPv4 packet，destination /32 查 byLease；desired=1 调 NormalOutbound，desired=2/3/4 调 GameOutbound；没有 live lease fail-closed。

Tunnel owner -> shared TUN: 必须携带当前 binding token；stale/rebound token fail-closed；每个 decoded inner packet 再次要求 source==lease；owned copy 写 TUN；short write/error fail-closed。authoritative source anti-spoof 仍在 leased server TunnelOwner ingress，这里是 platform defense-in-depth。

## Linux TUN primitive

internal/linuxserver/tun_linux.go 仅实现 /dev/net/tun + IFF_TUN|IFF_NO_PI + ReadPacket/WritePacket/Close。tun_other.go 在非 Linux 明确返回 ErrTUNUnsupported。当前 Actions 不要求 root 创建真实 TUN。

## NetworkPlan ownership

BuildNetworkPlan 是纯结构化 plan，不执行 shell：
- ip link set <tun> mtu <mtu> up
- ip route replace <lease-pool> dev <tun>
- cleanup 只删除这条 WBD-owned route
- save/enable/restore net.ipv4.ip_forward
- save/set/restore net.ipv4.conf.<tun>.rp_filter=0
- exactly three firewall semantics with markers: wbd-shared-tun-out, wbd-shared-tun-in (ESTABLISHED,RELATED), wbd-shared-tun-nat (host MASQUERADE)

DNS boundary：server shared-TUN runtime 不修改 host DNS/resolv.conf/systemd-resolved。client DNS selection 属于后续 client platform；DNS query 在 server 端只是普通 inner IPv4，经相同 forwarding/NAT。

## 专项测试

- internal/linuxserver/router_test.go：two leases demux、Normal/Game、unknown destination、duplicate lease/TunnelID、exact rebind/stale token、source spoof、malformed IPv4、short write。
- internal/linuxserver/network_plan_test.go：route/setup/teardown、sysctl restore intent、three WBD markers、DNS unchanged、nft forward spec、invalid config fail-closed。
- internal/datapath/platform_outbound_test.go：Normal owner send 不创建 BusinessFlow；leased client spoof 在 Lane 前拒绝，随后首个合法 record 仍 PN=0。

## 明确未做

- 不恢复 per-user netns/veth/double NAT。
- 不恢复 old rawipbackend localhost UDP metadata/service bridge。
- 不恢复旧 gateway 独立 CLI/process。
- 不实现 Linux runtime executor/systemd manager。
- 不实际执行 nft/iptables 或 root shared TUN。
- 不接 Windows/Wintun/Npcap 或 OpenWrt TPROXY。
- 不把 hosted core/adapter PASS 称作物理平台 PASS。

## Actions

当前状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。
按仓库规则没有运行本地 go test/go build/race/fuzz/network experiment；唯一运行期资格是提交后的 exact SOURCE_SHA GitHub Actions。
