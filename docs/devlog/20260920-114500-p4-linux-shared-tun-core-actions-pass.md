# 20260920-114500 P4 Linux shared-TUN hosted core Actions闭环

## 最终资格

SOURCE_SHA: 3d1c3831fb7726971ee8d52a076800a07af36d5f
GitHub Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35487091018
顶层结果：completed / success。

## Actions jobs

- repository-contract: PASS
- Windows 2022 active packages/unit/build: PASS
- Ubuntu 24.04 active packages/unit/build: PASS
- Linux race: PASS
- directed parser fuzz: PASS
- independent tlsrecord reference generator: PASS
- P2 kernel fallback + continuous pcap: PASS

P2 regression:
- TestKernelTLSFallbackVerifiedHTTPAndNormalClose PASS, 1.24s
- 29 packets captured
- 58 packets received by filter
- 0 packets dropped by kernel
- existing pcap analyzer PASS

Artifacts:
- foundation: 10597846826
- tlsrecord-reference: 10597442965
- p2-kernel-fallback: 10598275716

## 资格边界

本次只能称为 hosted core/adapter PASS，不能称为真实 Linux shared-TUN/netfilter platform PASS。

Actions 已验证：
- internal/linuxserver registry/router 的多 lease destination demux
- desired=1 NormalOutbound 与 desired=2/3/4 GameOutbound dispatch
- stable TunnelID+/32 exact runtime rebind 与 stale BindingToken fencing
- owner->TUN source==lease defense-in-depth
- malformed/no-route/short-write fail-closed
- Linux TUN source文件与 !linux unsupported stub 的跨平台编译边界
- NetworkPlan 的 shared-TUN route、sysctl save/restore intent、WBD-owned FORWARD/NAT marker、host DNS unchanged
- Linux race 下 registry/token state 无 data race

Actions 没有执行：
- root 打开 /dev/net/tun
- ip link / ip route 实际 apply
- net.ipv4.ip_forward / rp_filter 实际保存恢复
- nft/iptables 实际 apply/cleanup
- two lease through real host forwarding/NAT

因此真实 privileged platform gate 仍是下一原子任务。

## 产品语义保持

- 一个 root-namespace shared TUN 承载整个 lease pool。
- 一个 host NAT/conntrack domain。
- 不恢复 per-user netns/veth/double NAT。
- 不恢复 old rawipbackend localhost UDP metadata bridge。
- TUN return packet按 destination lease 查正确 TunnelOwner。
- shared TUN egress仍要求 source==lease；authoritative fence更早位于 server owner ingress。
- platform return path不会创建BusinessFlow/lane。
- DNS server-side不改host resolver，DNS packet只是普通inner IPv4。

## 下一原子任务

实现 Linux privileged runtime executor，把 NetworkPlan materialize 为受控 apply/cleanup：
- create/open one shared TUN
- set MTU/up and lease-pool route
- snapshot/set/restore ip_forward + tun rp_filter
- materialize exactly WBD-owned FORWARD/NAT markers
- cleanup只移除WBD-owned state
- 在 hosted runner facilities 允许时做 root smoke，并严格区分设施缺失与产品失败。

Windows Wintun/Npcap、OpenWrt、P5/P7 均继续后置。
