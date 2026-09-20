# 20260920-124500 P4 Windows Wintun platform core

## 基线

Remote HEAD: 0d00261c6c2ba3cdd5d888062fed129a496b7f69。Linux privileged shared-TUN product SHA a2cf881445f0efb4dcca87663074d594894db6ff / Actions 35488614508 已完成6-job PASS；closure Actions 35488790743也全绿。

## 归档读取

Archive source SHA: b5c848f4e9afdffd15d1bc451560edf4e9390a35。

定向读取 old/internal/tunnel/tun_windows.go、old/scripts/windows_tun_route.ps1、old/scripts/windows_ipv6_killswitch.ps1、old/internal/windowsruntime route rebind/source-fence tests与release contracts。

保留的成熟语义：
- Wintun是单L3 adapter；receive处非IPv4 fail-closed。
- server-assigned IPv4 lease在WBD adapter上必须排他，DHCP disabled，不能留APIPA/旧地址影响Windows source selection。
- full IPv4 capture使用0.0.0.0/1 + 128.0.0.0/1，而不是覆盖underlay /32。
- server underlay /32必须先锁到pre-WBD物理ifindex/next-hop，防止capture route递归。
- direct prefixes走同一已观测物理path。
- DNS resolver /32先capture，通过WBD-owned NRPT namespace='.'规则改变普通DNS解析；不改adapter全局DNS。
- IPv6未实现时device-wide双向fail-closed；::/0在NetSecurity上不可用，因此用::/1和8000::/1精确覆盖。
- Cleanup只删除state/marker明确属于WBD的address/routes/NRPT/firewall rules。

明确不迁移：旧Controller/Game child、DTLS orchestration、CN bundle/split classifier、route-rebind lifecycle controller、Npcap raw IO。Npcap是下一独立atom。

## active internal/windowsclient

### Wintun

Windows-only tun_windows.go：WintunOpenAdapter/CreateAdapter、StartSession 4MiB ring、read wait + close event、Receive/Release、AllocateSend/Send、ring-full短暂yield、并发Close fencing。ReadPacket在copy前先做IPv4 family/IHL检查；非IPv4永不进入active core。!windows为明确unsupported stub。

### Router

一个Router绑定一个leased TunnelOwner和一条Wintun writer。
- TUN -> owner：strict IPv4 + source==lease；desired=1走NormalOutbound，2..4走GameOutbound。
- owner -> Wintun：strict IPv4 + destination==lease，防止cross-lease/IPv6误写adapter。
- 不创建BusinessFlow，不修改lane membership。

### NetworkPlan

输入：AdapterAlias、lease /32、server IPv4、已观测physical ifindex+next-hop、DNS IPv4、optional direct prefixes、state path。
输出：server /32 underlay route、physical direct routes、Wintun full-capture /1 routes + DNS /32 capture、NRPT namespace '.'、IPv6 /1 kill-switch ownership。

## PowerShell ownership script

scripts/windows_client_network.ps1 提供 Render/Apply/Cleanup。
Render只做纯参数/计划输出，不要求管理员，用于hosted Windows Actions语法与contract证据。
Apply要求管理员并在任何创建前保存state；stale state先执行精确inverse。underlay route先于capture；lease地址排他；DNS NRPT和IPv6 firewall rules使用exact WBD markers。Windows Firewall profile若被用户关闭则fail-closed，不擅自开启。
Cleanup按state删除capture/direct/underlay routes与owned address、NRPT rule，最后删exact WBD IPv6 rules；不flush系统route/firewall/DNS全局状态。

## Actions gate

Windows active-go-tests新增PowerShell Render step，断言PLAN/UNDERLAY/ADDRESS_EXCLUSIVE/DNS_NRPT/IPV6_FAIL_CLOSED/CLEANUP markers。真实wintun.dll、管理员route/firewall/Npcap仍不是本atom资格。

## Actions

状态：IMPLEMENTED / AWAITING_EXACT_SHA_ACTIONS。
没有运行本地go test/go build/race/fuzz或Windows网络实验；唯一资格来自提交后的exact SOURCE_SHA GitHub Actions。
