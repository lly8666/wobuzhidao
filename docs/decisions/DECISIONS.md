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
