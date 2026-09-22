# 唯一开发路线图

完成条件取 ACCEPTANCE；当前执行位置只看 STATUS.json。表中优先级和协议已决定，不再开选型阶段。

| 阶段 | 工作 | 退出条件 |
|---|---|---|
| P0 | 建新分支、隔离 old、章程/规范/交接、Actions 基础入口 | 仓库契约检查通过；明确产品尚未实现 |
| P1 | 建根 Go module；实现 tlsrecord keys/seal/open/parser/近期去重和固定向量 | Linux/Windows 基础测试、Linux race、fuzz；全部在 Actions |
| P2 | 重开：保留已通过的核心；补握手重传、FIN/RST/半关闭、bootstrap 窗口、TLS 外观与票据/移交边界 | 核心回归及普通内核 TCP 客户端经真实入口完成 TLS/HTTP 回落与连续抓包均通过 Actions；内层业务指纹不属 P2 门槛 |
| P3 | 保留已通过的 LINK/FEC/统一MTU，继续 session/owner；增加默认关闭的显式有界 record padding 能力 | 一次分片、全固定 FEC、完整性/no-HOL；padding 向量/零等待/MTU余量/重传一致性通过；不宣称抗识别 |
| P4 | Tunnel/Game/lifecycle 和平台入口；多业务复用现有 lane、可选 padding 配置与预算 | 保持1..4 lanes/10物理上限/rotation/DORMANT；默认off，不逐业务重建lane，不凑包或改竞速策略 |
| P5 | 新版本 fullstack 弱网/抓包/负载/长测；真实 HTTPS 内层业务外观验收 | 同 SHA 完整测试；长度/方向/突发/时序与成本分开报告，不做旧项目 A/B，不承诺不可识别 |
| P6 | 发布候选打包与全平台 hosted 收口 | 可下载同源码包、哈希、manifest；已知问题与能力缺失透明 |
| P7 | 最终物理 Windows/Npcap -> Linux ARM64/amd64 验收 | 用户安排物理机后实测；满足退出清理与业务路径才 RELEASE_QUALIFIED |

每阶段可分小任务，但不能越过未满足的功能依赖。P1 的 PASS 不表示 P2 握手或 P5 端到端通过。若审计发现已关闭阶段仍有入口/生命周期硬缺口，STATUS 必须回退到该阶段收口，不能以“已进入下一阶段”为由跳过。平台能力缺失须明确记 UNSUPPORTED，不改代码假装实现。

每个任务的默认输出：代码/文档、针对性测试、一次新的开发日志、STATUS 更新、必要的 REUSE_LEDGER 记录。优先完成一个可审阅范围，不跨阶段顺手改 FEC 参数或产品主旨。

2026-09-20 用户授权：P2 重开期间允许已在开发的 P3 独立模块继续，但 P2 仍是最早未关闭门。faketcp/realityfront 由 P2 收口；tlsrecord/pathmtu/datapath 由 P3 接手。共享文档/STATUS 更新前同步最新提交，只合并本任务范围；保留两条工作流证据。P2 真实网络资格只提取最小必要 I/O，不扩大成完整 P5；物理 Npcap 仍在 P7。

2026-09-21 最新任务覆盖上述历史收口：P4重新打开稳态恢复/关闭/外观一致性，P5重新打开真实路径持续弱网性能，执行 [专项规范](WEAKNET_QUALIFICATION.md)。保留P2/P3与旧P4/P5/P6证据，不重做无关模块。Normal 10Mbps/方向、Game4 3Mbps/方向，FEC20:20，18份主测与目标负载长测/平台覆盖完成后，修复版重新P6打包，再进入P7。

2026-09-22 用户插入任务：P4/P5有界TLS启动填充，实施/关闭清单见TLS_STARTUP_PADDING.md。原主线任务暂时hold；本功能交新agent测试修复，完成后直接更新STATUS的小功能进度，不关闭P4/P5整阶段资格。
