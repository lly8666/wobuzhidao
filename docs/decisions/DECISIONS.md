# 已决定事项

2026-09-19 / D001：同仓库新分支，旧DTLS分支独立保留。old只是代码素材，不具开发指令权威。

2026-09-19 / D002：单进程、单TLS-like数据面，不兼容DTLS/旧CLI；建立流程复用真实TLS与成熟FakeTCP机制。

2026-09-19 / D003：record固定ChaCha20-Poly1305+完整64位PN保护；不做算法竞赛、不做旧项目A/B。固定格式见WIRE_SPEC。

2026-09-19 / D004：复用有限恢复、ownership、FEC/Game/lease/lifecycle的已验证行为。新架构不能删产品主旨或把数据交付改回按序。

2026-09-19 / D005：开发测试只在Actions，最终物理资格晚于hosted稳定；每轮详细日志+唯一STATUS+SHA证据。

未来改动必须说明具体缺陷/用户新要求、受影响约束及验收。没有新证据，不重新讨论这些决定。

2026-09-20 / D006（用户要求）：重开 P2 收口 TCP 生命周期、bootstrap 窗口/握手恢复、TLS 外观与票据边界，真实网络普通客户端回落资格纳入 P2；保留已通过的普通 SYN/总期限及 P3 成果。P3 独立工作继续，STATUS 记录并行工作流，不能覆盖他人源码或交接。

2026-09-20 / D007（用户要求）：内层 TLS 流量特征由 P3/P4/P5 承接，不阻塞 P2。P3 加有界显式 record padding 能力、默认 off/0，只使用 MTU 剩余空间；P4 复用既有 lane 与无等待预算策略；P5 真实 HTTPS 流量、抓包与开销验收。取代 V1“发送端永远只能 padding=0”的限制，但不改变当前生产默认、wire layout、密码、FEC 或 no-HOL。接口实现不等于抗识别通过，不加随机延迟、假业务或强制混流。

依据：[USENIX Security 2024 封装 TLS 握手指纹研究](https://www.usenix.org/system/files/usenixsecurity24-xue-fingerprinting.pdf)、[RFC 8446 E.3](https://www.rfc-editor.org/rfc/rfc8446.html#appendix-E.3)。研究表明长度/方向/时序风险及简单 padding 的局限，未测试 WBD，不可直接套用其识别率。


2026-09-20 / D008：P2 recognized WBD本地TLS固定为Go TLS 1.3；不协商ALPN，因为该路径不承载HTTP/2或HTTP/1.1应用协议。启用crypto/tls真实TLS 1.3 NewSessionTicket生成，但服务器通过UnwrapSession明确拒绝恢复，WBD客户端不配置session cache，因此每条lane仍完整握手、重新admission、重新生成exporter/context/nonce，不启用0-RTT。Go 1.23.12源码 `crypto/tls/handshake_server_tls13.go` 在server Finished之后、Handshake返回前调用 `sendSessionTickets`，标准TCP路径至多自动发送一张ticket；因此顺序固定为TLS握手内ticket -> protected admission -> PrepareTransition -> final reply ACK -> Detach，切换后不保留旧TLS writer。普通访客fallback不套用此策略，原始ClientHello仍交真实decoy决定ALPN/证书/ServerHello。

2026-09-21 / D008（用户要求）：重开P4稳态低开销修复与P5真实路径高负载弱网资格。固定主测Normal每方向10Mbps、Game4每方向3Mbps、FEC20:20，保存原业务与复制/FEC/repair分层计数。严格目标及Actions真实网络/模块矩阵见WEAKNET_QUALIFICATION。允许证据驱动定向借鉴旧恢复参数，不允许旧项目全架构A/B、扩大缓存或恢复HOL。runner容量不足单独报告且不算PASS；最终修复SHA重新回归和P6打包，P7不提前。

2026-09-22 / D009（用户要求）：允许默认关闭的内层TLS启动选择性填充，作为“不读取内层TLS”的窄范围例外。旁观有界结构前缀、不等待原包，复用现有record padding和tunnel预算；不加假业务/延时/额外分片，不动建连/FEC/recovery。实现及Actions关闭门槛见TLS_STARTUP_PADDING.md。旧主线由用户暂停；全新agent测试修复通过后直接标小功能完成。


## 2026-09-23：lifecycle health 与 admission V2
用户明确要求移植旧弱网/keepalive/黑洞恢复/idle保护。决策：保留稳定逻辑owner与真实TLS建连；有限认证health独立于FEC/LINK，业务与健康双时钟。为拒绝不支持health的旧端，升级TLS内admission版本2，不静默猜测peer能力。保活缺失只触发有限候选重试，不作为idle证据；不因候选失败退出程序。receiver pressure退役复用旧rate/RTT原理；sender credit/RTO/horizon及4096不改。生产默认idle仍off。配置只有CLI同名JSON一套，清单随代码校验。依据和未验证项见本轮开发日志及LIFECYCLE_ACCEPTANCE；没有性能通过声明。


## 2026-09-23：有损容忍与低成本shadow repair

2026-09-23用户最新决策：允许链路30%丢包时仍有至多30%业务包损失，优先处理性能、低延迟、无HOL与突发稳定性；不得主动丢业务凑指标。4096为可放弃的shadow-repair备份，不是fresh发送门。当前执行WEAKNET_QUALIFICATION第10节；历史近零损失门槛不再约束有损场景，无损满速、完整性、隔离和资源有界仍是硬门。

选择保留4096与3秒期限，优先消除满窗淘汰全量扫描，以有界索引和增量清理实现便宜的备份失效；不扩大socket/缓存掩盖开销，不退回旧DTLS。业务修复查无备份可跳过，握手/FIN/生命周期继续专门保护。实施细则和loss-tolerant-v1验收见WEAKNET_QUALIFICATION第10节。用户明确要求所有性能测试每Action run一条，因此历史同run A/B控制取消，使用独立run重复并报告runner差异。当前仅方案更新，产品和新版测试入口均待实现验证。


2026-09-23补充：shadow repair采用双方独立的有界退役，不增加ABANDON控制消息。接收端已有no-HOL，但缺口元数据需索引化并有不可被连续新包重置的截止点。发送缓存失效不等于接收证据，接收forgiveness不等于真实可靠ACK；保持既有部分可靠展示语义、迟到交付及FIN保护，实施/联测见专项10.4。
