# WBD NEXT 项目主旨（长期约束）

## 1. 产品目标

为 Windows、Linux/OpenWrt 与 Linux 服务端提供弱网可用的包/数据报隧道。外层尽量表现为正常 TCP/TLS，业务保持乱序首次到达立即交付。性能、延迟、状态有界优先；安全等级不是额外复杂化的目标，但基本账户隔离、地址归属、完整性及凭据保密不能取消。

新产品只有 TLS-like 数据面。保留旧源码是为了复用经验，不是为了兼容旧数据协议、CLI、进程或配置。新客户端/服务端配套升级，不兼容版本明确拒绝。

## 2. 不可退化的运输语义

- 一条 Transport Lane 从 SYN 到真实 TLS 建连、稳态数据始终使用同一 FakeTCP association、四元组和序列空间。
- 真实 TLS/Reality-like 建连、识别、认证和 fallback 复用成熟实现；只在受保护的应用消息内增加新版本参数。不另造假握手或第二个公开连接。
- 仅建立阶段允许有界有序 BootstrapStream。持续业务不得经过普通内核 TCP 字节流。
- 后到完整记录独立解密、独立交付，不等更早包、累计 ACK、FEC block 或另一 lane。分片仅等待自身数据报所需的碎片。
- FakeTCP 保留有限 shadow repair、4096 有效重传记录边界及当前自适应放弃逻辑；不得因追求全收齐回到严格 ACK 等待。
- 同一 TCP Seq 的修复重传必须重发相同 wire bytes。记录层不增加 ACK/NACK/RTO。
- FEC lane-local，systematic 不等填满 block 就交付/发送；P3 一次性提取并验证基线 live policy 的完整固定集合：off、20:4、20:8、20:10、20:12、20:16、20:20。保留当前 3 秒绝对恢复期限及迟到源包语义，不重新进行 FEC 参数选型，也不引入动态比例或第二套 FEC wire。
- 只有一个统一 MTU 来源；业务分片一次，不能逐层重复分片或隐藏截断。

## 3. 隧道、Game、多用户和生命周期

Account -> Installation -> Logical Tunnel。Tunnel 拥有稳定 TunnelID、服务端分配的 IPv4 lease、业务 SessionID/PacketID 和本机 TUN/路由/DNS 状态。lane 只拥有可替换的传输 incarnation。

正常模式 1 条逻辑 lane；Game/弱网模式 2..4 条。相同 PacketID 多 lane 竞速，首次有效到达交付，其余去重，无跨 lane HOL。FEC 不跨 lane。

最多 4 条权威逻辑 lane；复用最新生命周期中最多 6 个退休/过渡余量、最多 10 个物理 incarnation 的有界规则。不能把过渡余量当成 10 条逻辑 lane，也不能恢复早期“总共只有 5 个物理槽”的旧约束。第 5 条权威逻辑 lane、第 11 个并存物理 incarnation 被拒绝。

健康替换 A -> A+B -> B；候选失败保留 A。多 lane 逐条轮换，回调/任务/超时必须带 generation，旧任务不能复活已退役 lane。年龄、网络变更、失活、人工重连统一走这个生命周期。

payload idle 与 transport activity 分开，PING/PONG 不延长业务活跃时间。默认业务闲置 15 分钟可 DORMANT，0 表示不因闲置睡眠；lane 随机软年龄沿用 30..60 分钟并错开。

DORMANT 关闭传输但保留 Tunnel、lease、TUN、路由/DNS，真实业务到来唤醒。显式断开/退出则清理 WBD 自己创建的资源，不能影响其他网络规则。

## 4. 平台与网络边界

- Linux 服务端一个公开监听端口、多客户端、多 lane、一个共享 TUN、主路由与一套 WBD-owned NAT；不恢复每用户 netns/veth/double NAT 产品架构。
- 同账户不同 installation 获得不同地址；lane 更换尽量保持 tunnel lease。服务端校验业务 IPv4 source 等于该 tunnel lease。
- Windows 每 Tunnel 一个 Wintun，维护物理接口绑定、路由/DNS 和 IPv6 fail-closed/退出清理，不让 DHCP/APIPA 误占 tunnel lease。
- OpenWrt 复用 TPROXY/策略路由的业务入口语义；Linux/Windows 的直接连接、分流、DNS 及必要 bypass 不能在统一进程时丢失。
- 新运行时不启动 DTLS shim，也不以模块分层为由创建回环 UDP 转发链。真实业务 socket 可以存在。

## 5. 外观与性能的诚实边界

目标是握手自然、稳态 TLS record 格式正确、无明文私有 framing、重传一致、MTU/checksum/序列计算正确。普通合法 TCP SYN 必须能进入 ClientHello/fallback 路径，WBD 固定 SYN persona 不是服务端身份条件。网络真实丢包引起的重传/乱序提示不等于协议异常。

“真实 TLS / 浏览器风格 ClientHello”与“指定借用网站完整服务端握手指纹一致”是不同能力。前者可在 hosted Actions 验证；后者必须等平台 I/O、普通浏览器访问和真实抓包后再下结论。已识别 WBD 当前由本地 TLS server 握手，不能因为 SNI/证书正确就宣称服务端 ALPN、扩展、分段、会话恢复等已经与目标网站一致。

有限恢复的缺口放弃与严格 TCP 累计 ACK 语义存在差异。必须报告，不能承诺所有主动/双端观察都识别不出。不得为了隐藏差异牺牲无 HOL、有界状态或制造替代密文。

高效主要来自单进程直接调用、明确所有权、有界工作队列和低开销观测。架构和密码方案已定，不开展新旧性能对比或算法赛。只测试新版本是否正确、稳定并达到声明的运行负载。

## 6. 验收与协作

所有开发测试在 Actions。先完成 hosted 正确性、弱网、长测与打包；最终再安排物理 Windows/Npcap -> Linux 验收。NOT_RUN、UNSUPPORTED、PASS、FAIL 必须分开。源码精确 SHA 是证据归属，不继承旧分支成绩。

每个 agent 每轮留下详细日志、当前状态、下一任务和证据链接。新接手者能仅靠本分支根入口继续，不需要完整聊天史，不受 `old/` 指令影响。
