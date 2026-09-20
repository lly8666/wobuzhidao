# 20260920-153000 P4 OpenWrt TPROXY ownership candidate

## 基线与范围

开始 HEAD: `b86373c7e8643abb00da62edbcb2fa772fac6761`。该 docs closure 的 next-foundation run 35493500498 已 completed/success，且最新 STATUS.next_task 明确要求 OpenWrt TPROXY/策略路由 ownership。

本 atom 只做 IPv4 kernel ingress ownership：pure plan + in-process Linux executor + real root-netns Actions harness。不会在同一个提交里恢复或重写旧 platform proxy，也不会把透明 TCP/UDP socket 业务适配和 TunnelOwner 一起塞进来。

## archive 定向读取

固定 archive source SHA: `b5c848f4e9afdffd15d1bc451560edf4e9390a35`。

读取：
- `old/scripts/openwrt_tproxy.sh`
- `old/scripts/openwrt_tcp_tproxy_netns.sh`
- `old/scripts/openwrt_udp_tproxy_netns.sh`
- `old/scripts/openwrt_udp_hairpin_netns.sh`
- `old/scripts/openwrt_fullstack_one_shot.sh`
- `old/.github/workflows/openwrt-tcp-tproxy.yml`
- `old/.github/workflows/openwrt-udp-tproxy.yml`
- `old/.github/workflows/openwrt-fullstack-one-shot.yml`

保留的语义只有：WBD-owned TPROXY table、fwmark policy routing、local route、already-marked bypass、server underlay source/destination bypass、local-destination bypass、TCP/UDP capture、precise cleanup，以及 root netns 可执行资格。

删除/不迁移：旧 `wbd-platform-proxy-openwrt` / server、127.0.0.1 UDP bridge、DTLS shim、LINK subprocess stack、full-cone/hairpin mapping implementation、geo mode/cn sets、旧 CLI/process lifecycle。

## active 修改

- `internal/openwrtclient/network_plan.go`：固定 IPv4 plan。专用 `inet wbd_tproxy` prerouting chain；bypass 顺序在 capture 前；TCP/UDP 共享一个 transparent listen port；一个 mark、一个 policy table、一个 priority。
- `internal/openwrtclient/runtime_linux.go`：Apply 前检测 state conflict；先 local route -> fwmark rule -> nft capture。Close 先删 nft capture，再删 exact rule/route。绝不 flush 全局 ruleset/table。
- `runtime_other.go`：非 Linux 显式 unsupported，保证 Windows 全仓编译边界明确。
- `runtime_privileged_linux_test.go`：在 router namespace 用真实 `IP_TRANSPARENT` TCP/UDP socket 接收 TPROXY 流量。TCP 还验证 accepted socket 的 LocalAddr 保留原目标 `10.20.0.2:8080`。
- `scripts/openwrt_tproxy_netns.sh`：只创建 client/router/target 三 namespace 与 veth/route，随后在 router namespace 运行 active Go test binary。
- `next-foundation.yml`：新增 `p4-openwrt-tproxy-privileged`，仍保留所有既有 regression jobs。

## 资格断言

privileged gate 必须同时证明：
1. TCP remote destination 被真实 TPROXY 到 IP_TRANSPARENT listener；
2. UDP datagram 被真实 TPROXY 到 IP_TRANSPARENT listener；
3. 配置的 server underlay IPv4 对 TCP/UDP 都绕过 capture；
4. active runtime 存在时第二实例因 state conflict fail-closed；
5. foreign nft table 和 foreign ip rule 在 cleanup 后仍存在；
6. WBD nft table、fwmark rule、local route 全部清除；
7. cleanup 后此前被 capture 的目标恢复普通 TCP/UDP routing。

marker:
`WBD_P4_OPENWRT_TPROXY_PASS tcp=1 udp=1 underlay_bypass=1 cleanup=owned-only ipv6=NOT_IMPLEMENTED`

## 诚实边界

本 atom 不证明业务已经经过 TLS-like TunnelOwner。它只证明 OpenWrt 透明入口的内核 ownership 正确，避免为了“接通”而复活旧 localhost UDP/DTLS/process topology。

IPv6 TPROXY 当前 NOT_IMPLEMENTED；该事实写进 marker/STATUS，不能将 IPv4 PASS 扩大成 IPv6 资格。

## 下一步

提交 candidate 后只认 exact SOURCE_SHA GitHub Actions。若新增 privileged gate 和所有既有回归全绿，再做 evidence closure；下一 atom 才实现 transparent TCP/UDP socket ingress 直接适配已有 TunnelOwner 的单进程业务边界。
