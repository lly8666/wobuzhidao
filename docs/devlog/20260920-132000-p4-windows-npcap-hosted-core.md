# 20260920-132000 P4 Windows Npcap hosted core

## 本轮目标和阶段

对应 P4 Windows platform underlay。开始基线为 `c02546d53ed6fb225566049ab467d98f2f70c854`，开始前 compare 对 `next/tlslike-dataplane` 为 identical / 0 ahead / 0 behind，最新 STATUS 仍指定 Npcap/physical Windows underlay 最小闭包。本轮只做 hosted 可编译、mock/fixture 可验收的 physical-path + Npcap adapter core；不恢复旧 Controller/Game child/DTLS/process orchestration，不提前进入 P5/P7。

## 修改与原因

- `internal/windowsclient/underlay.go`：新增 immutable PhysicalUnderlay。同一观察值同时生成现有 NetworkPlan 使用的 physical interface/next-hop 和 faketcp.NpcapConfig，保证 server /32 继续锁在 Wintun capture 之前观测到的物理路径。
- `internal/windowsclient/underlay_windows.go`：使用 Windows IP Helper 提取物理 IPv4 adapter、到 server 的 longest-prefix route、next-hop ARP/MAC 和 NPF device；允许显式排除已知非物理接口。没有迁移 route-rebind controller。
- `internal/faketcp/npcap.go`：新增 exact-flow BPF、Ethernet/VLAN ingress、packet lifetime、source-port/peer/MAC egress binding 和 generation/close gate。
- `internal/faketcp/npcap_windows.go`：Windows syscall adapter 使用 `pcap_open_live`、Ethernet DLT、`MODE_SENDTORX_CLEAR`、`pcap_compile`+`pcap_setfilter`、`pcap_next_ex`、`pcap_sendpacket`。Close 先拒绝新调用，再请求 breakloop 并等待活动调用退出，最后释放 handle/DLL。
- mock tests 覆盖 filter、send/receive ownership、source/destination fencing、packet lifetime、stale generation、close drain 和 route/Npcap 同源 physical path。
- `.github/workflows/next-foundation.yml` 在 Windows hosted job 额外执行 targeted Npcap/PhysicalUnderlay contracts，只输出 `WBD_WINDOWS_NPCAP_HOSTED_CORE_PASS physical=NOT_RUN`；不安装驱动，不冒充物理资格。

## 复用来源

固定 archive source SHA：`b5c848f4e9afdffd15d1bc451560edf4e9390a35`。

定向参考：
- `old/internal/windowsruntime/underlay_native_windows.go`
- `old/internal/windowsruntime/underlay_native_windows_test.go`
- `old/internal/windowsruntime/underlay_npcap_test.go`
- `old/internal/windowsruntime/underlay_path.go`
- `old/cmd/wbd-faketcp/main_windows.go`
- `old/cmd/wbd-faketcp/main_windows_test.go`
- `old/cmd/wbd-faketcp/main_windows_handshake_test.go`
- `old/cmd/wbd-faketcp/main_windows_ingress_sequence_test.go`
- `old/internal/releasecontract/windows_npcap_physical_contract_test.go`
- `old/.github/workflows/windows-npcap-runner-probe.yml`

只保留 active 单进程 physical I/O 所需语义；REUSE_LEDGER 按 concrete destination 分条登记。

## Actions证据

本日志随 candidate SOURCE_SHA 一起提交。提交前未运行任何本地 Go test/build/race/fuzz/network experiment；资格状态为 NOT_RUN。ref 推进后只以该 exact SOURCE_SHA 的 GitHub Actions 为产品资格。

Hosted Windows 即使 PASS 也只代表 core/adapter compile+mock contract；真实 `wpcap.dll` driver、Administrator、physical NIC capture/injection 仍为 NOT_RUN，不写成 PHYSICAL_PASS。

## 问题、排查与风险

active 侧目前还没有完整统一 Windows client runtime，因此本 atom 只建立后续 single-process runtime 可直接持有的 physical observation 与 generation-bound Npcap endpoint，不创建第二套 lane/controller。

Npcap `pcap_next_ex` 使用 50ms bounded read timeout，并同时请求 `pcap_breakloop`；Close 不在活动调用未返回时释放 handle。真实驱动是否在 GitHub-hosted Windows 存在不是本 atom 的 PASS 条件。

## 下一项原子任务

读取 exact SOURCE_SHA next-foundation 全部 jobs。全部通过则新增 evidence closure，把 last_tested_source_sha 指向产品 SHA，并进入 OpenWrt platform glue / TPROXY ownership；若失败只根据具体 job log 修这个 Npcap/underlay atom。
