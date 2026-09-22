# 有界 TLS 启动填充：实现与验收契约

2026-09-22 用户授权的 P4/P5 小功能。唯一进度仍是 STATUS.json；本文是功能规范，不能代替测试证据。主线既有弱网任务由用户暂停，本功能验收不能顺便恢复该任务或关闭整个 P4/P5。

## 行为与开关

Linux/OpenWrt、Windows client 与 Linux server 增加 `--tls-startup-padding`，默认 false。建议双端同时设置；单端只影响该端发送，不需要 wire 协商。runtimeentry 的 ClientConfig、TunnelClientConfig、ServerConfig 同样提供 TLSStartupPadding，LifecycleServerConfig 继承 ServerConfig。

复用现有真实建连、独立 ChaCha20-Poly1305 records、LINK/FEC/Game、MTU 和 tunnel padding allocator，不修改握手、wire layout、repair、SACK、4096 或 FEC 参数。普通旧 all-record padding API 保持原行为。

生产路径：业务包旁观识别 -> 原有一次 LINK 分片/FEC -> 在源 shard seal 前申请 padding -> 原有 FakeTCP 缓存最终密文。观察接收端已解密、重组、源地址校验且 Game 去重后的包，让服务端知道哪个反向流处于启动期。观察器不拥有待发/待交付队列；识别不完整仍立即发送原包。

支持 IPv4 非分片 TCP（按双向四元组，协议固定 TCP）和 platformflow v1 TCPData（按 lease + FlowID + 本地收发方向）。有 SYN 时以 ISN+1 定位前缀，支持 TCP 序号回绕内的短窗口；无 SYN 时只能保守识别首个看起来是 TLS 的 payload。platformflow 按绝对 stream offset 拼旁观副本。地址归属校验仍由既有 owner 完成。

识别检查首条 TLS record、ClientHello 类型与嵌套长度、版本、session ID、cipher/compression vectors 和扩展长度。不是固定 200–550 字节规则，不读 SNI/证书内容、不解密业务、不输出 payload。支持跨 TCP segment 的有限重排/重复；重叠字节冲突放弃识别。只接受一个 TLS record 内完整的 ClientHello；跨 TLS record、STARTTLS、QUIC、IP 分片、IPv6、超过前缀上限或入口丢了必需片段，均不承诺识别，原业务旁路。当前产品 lease 仍为 IPv4。

## 固定界限（每 endpoint 的 Logical Tunnel）

| 项目 | 首版固定值/语义 |
|---|---|
| 状态条数 | 256 个双向 flow，满了只跳过检测，不淘汰活跃业务 |
| 旁观前缀 | 每方向最多 4096 B；仅候选方向分配，已识别/拒绝/到期释放 |
| 内存 | 当前 prefix 以 byte+bool 标记，两个方向最坏约 4 MiB/tunnel，加少量 map/list 元数据；正常非 TLS 首包直接拒绝，不分配 prefix |
| 启动时间 | 从首次建立观察状态起绝对 2s，包含检测时间；检测/重复/反向流量不续期 |
| 防重复状态 | 从创建起绝对 30s；期内同 key 不重新启动；过期不能保证分辨极晚旧包与新流 |
| flow padding | 最多成功填充 12 records、2048 B，Game 所有副本/分片共同消耗；不是每 lane 独立预算 |
| 每 record | 均匀随机请求 1..256 B，仅 source records；headroom 或额度不够整个请求跳过 |
| tunnel padding | 累计最多 16 MiB，且不超过本端提交的逻辑业务包字节的 10%；两个条件同时满足 |

业务计数沿用既有 owner 的逻辑 IPv4 packet 字节口径（包含内层头），不能写成纯应用 payload。Game 副本、FEC parity、FakeTCP repair 不增加额度；内层应用自行重传形成新的业务提交仍按既有口径计数，但同一流不能因此刷新时间或 12 records/2048 B 限额。以上是每个发送 endpoint 的预算；双端合计每流最多 24 records/4096 B。累计额度不随时间补充、不随轮换或 DORMANT 重置，用尽就保持零填充直到新的 logical owner。

只对检测后、该方向 high-water 继续前进的数据申请；完全重复和低于 high-water 的乱序片段仍照常传输但不填充。首次检测由较低 offset 的迟到片段完成时，该片段也可能不填充，这是不等待的有意边界。FIN/RST/Close 终止观察，不改变实际关闭语义。

FEC 关块同批输出的 parity 和 timer flush parity 均不填充，full-MTU 直接零填充。Flow 与 tunnel 额度在 seal 前原子 reserve，失败释放；Game/并发共用同一 owner 锁。线上丢包后的 repair 复用原密文，不重新随机、不新建 PN。无凑包、假流量、随机睡眠、额外分片或新的外层连接。

## 观测及诚实边界

TunnelOwner.Stats().Padding 增加 TLSStartupOnly、StartupTracked、StartupDetected、StartupCapacitySkips、StartupRejected、StartupBudgetSkips，沿用 RecordRequests/PaddedRecords/PaddingBytes/BudgetSkips/HeadroomSkips。必须保存默认配置及上述固定界限到测试 receipt；服务端容量估算要乘实际 tunnel 数，不能只报一个 detector 的内存。

此功能仅有限扰动启动阶段长度，不承诺消除 TLS-in-TLS、方向/节奏/突发字节指纹，不提高已有 TLS-like 外观等级。有限预算允许零填充，不能为提高“命中率”突破 MTU、延迟或预算；真实稀疏流是否有可见长度变化与实际成本留给 P5 测量。

## 新 agent 的验收和关闭条件

1. 从 STATUS 的 TLS_STARTUP_PADDING 工作流接手，精确记录代码 SHA/harness SHA。主线旧任务保持暂停。所有编译、测试、race、fuzz、netem 和负载都在 Actions；本地只阅读/编辑/Git。
2. `.github/workflows/next-tls-startup-padding.yml` 先跑 Windows/Linux core/build、Linux race 与 parser fuzz；foundation 与既有回归也必须检查。本功能已有 parser、真实 Go TLS ClientHello + production platformflow serializer、双向 owner、no-HOL、各 FEC 档位、Game4 预算、并发 reservation rollback 测试。它们是 core 资格，不能冒充正式进程端到端。
3. 补真实独立 client/server 二进制网络路径专项：单稀疏 HTTPS 首连接/复用 lane 后续连接、非 TLS TCP/UDP 对照，TUN 与 platformflow/TPROXY 都覆盖。每个负载样本一个独立 Action job/runner，不在同 VM 并跑高压样本。开关 off/on 同源配对，FEC off/20:20，Normal=1/Game=4；确认双端实际开关与计数，不能仅改 harness 的 owner。
4. 对重点组合做 300ms 单向、5%->20%->5% 和 5%->30%->5%、120s（30/60/30）及无损回归；只为此功能验证不退化，不擅自恢复主线完整 18 场弱网资格。至少两独立重复，保存注入负载、pcap、分阶段 RTT/goodput、skipped/send failure、CPU/每线程、softirq/steal、队列、socket drops、FEC/repair/padding 分项。原弱网规范的门槛不降低；runner 饱和单列 CAPACITY_LIMITED，不据此改协议或算 PASS。
5. 抓包/计数验收：padding off 保持原路径且计数零；非 TLS 无填充；开启后至少真实可识别且有 headroom/预算的 HTTPS 样本观察到 >0 padding（不能用“代码走过”替代）；业务内容不变，record/IP 不超统一 MTU，无新增分片/记录/业务连接；同 Seq repair wire bytes 一致；永久丢一个早包仍交付后到完整业务包。不要要求所有随机请求都成功。
6. 负向：短包、错长、4KiB/超大 CH、TCP 重排/分段/重复/回绕/重叠冲突、畸形平台帧、同 FlowID 不同 lease、流重用、FIN/RST、prefix/flow 表饱和、停流到期、Game 去重、源地址伪造、轮换/DORMANT/wake、seal失败、并发预算。资源达到固定上限必须旁路且业务不断，停流后 tick 释放观察前缀及过期表项。
7. 如发现缺陷，定向修实现和测试，保留原失败证据；不为了测试全收齐恢复 HOL，不调整 FEC/4096/recovery、不全局扩大缓存、不把默认改 on。最新修复 SHA 重跑受影响专项与基础回归。
8. 只有上述所需证据成立，直接把 STATUS.workstreams.TLS_STARTUP_PADDING 标为 COMPLETE、加入 completed 摘要，更新 ACCEPTANCE、本文结论和详细 devlog，记录 exact SHA/run/artifact/未验证限制。主线 P4/P5 总阶段及用户暂停状态保持原样；未跑的项目写 NOT_RUN，不能提前关闭。
