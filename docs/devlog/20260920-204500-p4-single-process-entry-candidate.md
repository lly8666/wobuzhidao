# P4 single-process client/server entry + endpoint loop candidate

## 目标

完成 STATUS.next_task 的最小可运行单进程入口闭包：让同一 FakeTCP association 从 SYN/bootstrap/真实 TLS/受保护 admission 显式移交到已资格的 runtimeowner，并把 steady SegmentEmitter/receive dispatch 绑定到现有 Linux raw 与 Windows Npcap endpoint；平台侧复用 Linux shared-TUN、Windows Wintun、OpenWrt TPROXY SocketAdapter。仍不进入 P5，不恢复 localhost UDP、DTLS shim、platform-proxy 子进程或旧 Controller topology。

## 归档来源与复用边界

- 读取 `old/cmd/wbd-faketcp/main.go` 中客户端 SYN/SYN-ACK/final-ACK、BootstrapStream 与同 association raw-loop 的行为形状。
- 只把客户端 bootstrap association 缺口重写为 `internal/faketcp/client_association.go`，并在 `internal/runtimeentry/runtime.go` 复用“一个 endpoint loop 从 bootstrap 延续到 steady”的所有权原则。
- 不迁入归档 localhost UDP carrier/target-UDP、CarrierFragmenter、DTLS、旧 steady Receiver/SACK/RACK controller、旧命令参数兼容或多进程拓扑。

## 修改

- 新增 `faketcp.ClientAssociation`：WBD SYN presentation、SYN 重传、peer MSS/WS/SACK 记录、有界 BootstrapStream、ACK/FIN、显式 `Detach` 返回 send/receive sequence handoff。
- `ServerAssociationTable` 增加统一 retransmit tick 与 close，供单进程 server owner 驱动。
- `runtimeowner.HandleServerSegmentQualified` 在保留原 API 的同时返回“合法 detached steady record 已处理”信号；server 用它保证首条客户端 steady record 前不主动发新模式业务。
- 新增 `internal/runtimeentry`：
  - client：endpoint read/tick loop -> ClientAssociation -> realityfront.EstablishClient -> runtimeowner.AttachClientAdmission；
  - server：单监听端口 ServerAssociationTable -> HandleServerAssociation -> lease lookup -> shared-TUN register -> platformflow.Server -> runtimeowner.AttachServerAdmission；
  - detach 与 runtime attach 之间的 steady segment 有界排队；
  - shared-TUN egress 在资格前直接拒绝，避免提前消耗 record PN。
- 新增命令入口：
  - Linux `cmd/wbd-server`：shared-TUN Runtime + raw IPv4 FakeTCP endpoint；
  - Linux/OpenWrt `cmd/wbd-client`：TPROXY Runtime + SocketAdapter + raw IPv4 endpoint；
  - Windows `cmd/wbd-client`：PhysicalUnderlay/Npcap + Wintun + 现有 `scripts/windows_client_network.ps1` Apply/Cleanup。
- 当前命令入口是最小 Normal 单 lane + 静态 lease 配置闭包；不把 Game 2..4/rotation/replacement 的命令级编排冒充已完成。

## 测试候选

- `internal/faketcp/client_association_test.go`：客户端 SYN、peer profile、bootstrap 双向、ACK、detach sequence handoff。
- `internal/runtimeentry/runtime_test.go`：内存 segment endpoint 上执行真实 TLS + protected admission，同 association attach runtimeowner，client->server lease IPv4 写入 shared-TUN，再 server->client 反向交付。
- 既有 workflow 还必须继续覆盖 Windows Wintun/Npcap hosted contract、Linux raw P2 kernel fallback、Linux shared-TUN privileged iptables/nft、OpenWrt privileged TPROXY + SocketAdapter、Ubuntu race/fuzz/reference。

## Actions

- 当前：NOT_RUN。本日志对应的候选将在分支 fast-forward 后以精确 SOURCE_SHA 运行 `.github/workflows/next-foundation.yml`。
- 上一个已资格产品 SHA 仍为 `ef269081e45bc516ae41943e56a01a8032e4cf8a` / Actions 35498412990；本候选成功前不得覆盖 `last_tested_source_sha`。

## 风险 / 未验证

- Windows/Npcap 真实 driver + physical NIC 仍为 P7 `NOT_RUN`；本 atom 只能取得 hosted compile/mock。
- OpenWrt IPv6 仍 `NOT_IMPLEMENTED`。
- 当前命令级入口固定 Normal 单 lane；Game 2..4 与 make-before-break rotation/replacement 的核心 owner API 已资格，但最终可执行编排需在本候选通过后做 P4 closure audit。
- steady recovery 仍是当前 runtimeowner 的 cumulative-ACK + 4096 metadata + 1s RTO/3s horizon，不声称恢复归档完整 SACK/RACK/adaptive-pressure。

## 下一步

对本候选精确 SHA 跑全量 next-foundation；只修本 atom 的失败直到 7 jobs 全 PASS，然后更新证据与是否满足 P4 关闭条件。
