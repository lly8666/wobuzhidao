# 20260920-151000 P4 Windows Npcap hosted core Actions闭环

## 最终资格

SOURCE_SHA: 1ec7d667564a8e0f26a3afb811cb89eb484ea5dd  
GitHub Actions: https://github.com/lly8666/wobuzhidao/actions/runs/35493372121  
Result: completed / success, attempt 1.

## Jobs

- repository-contract: PASS
- active-go-tests (windows-2022): PASS
- active-go-tests (ubuntu-24.04): PASS
- p2-kernel-fallback: PASS
- p4-linux-shared-tun-privileged (iptables): PASS
- p4-linux-shared-tun-privileged (nft): PASS

Windows 2022 先执行整仓 `go test ./...` + `go build ./...`，证明真实 `npcap_windows.go` syscall adapter 和 `underlay_windows.go` IP Helper implementation 可在固定 Go 1.23.12 工具链编译。随后既有 Wintun route render contract PASS，并运行新增 targeted contract：

`go test ./internal/faketcp ./internal/windowsclient -count=1 -run='Npcap|PhysicalUnderlay'`

日志输出：

`WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN`

这条 marker 明确把 hosted adapter/core 资格和真实物理 Npcap 资格分开。

## Qualified semantics

- Windows IP Helper 观察 server IPv4 的 physical route，按 longest-prefix、route metric、interface index 选路，排除 loopback/APIPA 和显式 excluded interface。
- SendARP 获取 next-hop MAC；adapter GUID 转 canonical `\Device\NPF_{...}`。
- 同一 immutable PhysicalUnderlay 同时提供现有 Wintun NetworkPlan 的 server /32 physical path 与 FakeTCP NpcapConfig，避免 broad capture 把 public underlay 递归导入 Wintun。
- Npcap config 绑定 source/peer IPv4、source/peer port、source MAC、next-hop MAC、packet persona、lane generation。
- open 时要求 Ethernet DLT、`MODE_SENDTORX_CLEAR`，并用 `pcap_compile` + `pcap_setfilter` 安装 exact inbound four-tuple/no-fragment BPF。
- capture 后仍做用户态 exact-flow/fragment fence；只把 owned IPv4 packet copy 交现有 FakeTCP parser，Npcap capture buffer lifetime 不泄漏。
- send 前再次验证 Segment 未逃离绑定四元组，再构造 Ethernet frame 并 `pcap_sendpacket`。
- 每次 Read/Write 带 generation；stale generation fail-closed。
- Close 先 fence 新调用，调用 breakloop，等待 active calls 退出，再释放 pcap handle/DLL，避免 use-after-close。
- old Controller/Game child/DTLS/process orchestration 没有迁移。

## 回归证据

Ubuntu 24.04 unit/build/race、15s tlsrecord directed fuzz、independent reference generator 全 PASS。

P2 kernel fallback：`TestKernelTLSFallbackVerifiedHTTPAndNormalClose` PASS 1.24s；连续 pcap 28 packets captured / 56 received by filter / 0 packets dropped by kernel；既有 analyzer result=PASS。

Linux privileged shared-TUN iptables/nft 两个 backend 均 PASS。

Artifacts:
- foundation: 10599402708, digest sha256:a74e946890b02957e40b673745c6d6c45324b716bb6397884b26bec07d1f714d
- tlsrecord-reference: 10599921615, digest sha256:304ca5a19a6ad3c8969b4e081f7ba8525f60686cd8bd5bbcb4a811a45dd0c3a0
- p2-kernel-fallback: 10599339989, digest sha256:58df049995355ea1f82389af48602c9fc0a814793aa62b8e83604106b0089ceb
- p4-linux-shared-tun-iptables: 10600395602, digest sha256:fd773728e61cb3212e0c83e5748afe4b89b8fef74301bdc01bf61f8820d34300
- p4-linux-shared-tun-nft: 10600220880, digest sha256:8b159f2a6b77ec2fac798d133c652a53b486bc5a5e1eef22f2e96c7946d11b3b

## Qualification boundary

真实 `wpcap.dll`/Npcap driver、Administrator、physical Windows NIC capture/injection 和真实 Wintun Apply/Cleanup 没有在 GitHub-hosted runner 执行。STATUS.physical_status 保持 NOT_RUN；本轮不是 PHYSICAL_PASS，也不提前调用用户物理机。

## 下一项原子任务

OpenWrt platform glue / TPROXY ownership：定向从 archive 提取 WBD-owned TPROXY、策略路由、TCP/UDP、server-underlay bypass、防递归与 owned-only cleanup；先做 active plan + Linux root-netns privileged Actions，不恢复旧 localhost UDP/DTLS/process topology。
