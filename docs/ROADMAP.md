# 唯一开发路线图

完成条件取 ACCEPTANCE；当前执行位置只看 STATUS.json。表中优先级和协议已决定，不再开选型阶段。

| 阶段 | 工作 | 退出条件 |
|---|---|---|
| P0 | 建新分支、隔离 old、章程/规范/交接、Actions 基础入口 | 仓库契约检查通过；明确产品尚未实现 |
| P1 | 建根 Go module；实现 tlsrecord keys/seal/open/parser/近期去重和固定向量 | Linux/Windows 基础测试、Linux race、fuzz；全部在 Actions |
| P2 | 按 MODULE_MAP 提取 FakeTCP + bootstrap + auth；接入真实 exporter 和阶段移交 | 真实握手、fallback、同流、切换竞态、record no-HOL 核心资格通过 |
| P3 | 提取 LINK/FEC/MTU 和必要 session 核心，单进程串接 | 业务一次分片、FEC off/20:20、完整性和有限恢复压力通过 |
| P4 | 提取 Tunnel/Game/lifecycle；集成 Windows/Linux/OpenWrt 入口 | 多用户、1..4 lanes、10物理上限、rotation、DORMANT、分流/清理 |
| P5 | 新版本 fullstack Actions 弱网/抓包/负载/长测 | 同 SHA 的新版本完整测试，不做旧项目 A/B |
| P6 | 发布候选打包与全平台 hosted 收口 | 可下载同源码包、哈希、manifest；已知问题与能力缺失透明 |
| P7 | 最终物理 Windows/Npcap -> Linux ARM64/amd64 验收 | 用户安排物理机后实测；满足退出清理与业务路径才 RELEASE_QUALIFIED |

每阶段可分小任务，但不能越过未满足的功能依赖。P1 的 PASS 不表示 P2 握手或 P5 端到端通过。平台能力缺失须明确记 UNSUPPORTED，不改代码假装实现。

每个任务的默认输出：代码/文档、针对性测试、一次新的开发日志、STATUS 更新、必要的 REUSE_LEDGER 记录。优先完成一个可审阅范围，不跨阶段顺手改 FEC 参数或产品主旨。
