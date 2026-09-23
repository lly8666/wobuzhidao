# WBD NEXT 详细开发方案

此版本取代此前双后端、选型对比方案。决策已由用户授权确定：不兼容旧产品、不再做算法/架构选型或旧产品 A/B。工程测试只回答“新实现是否符合目标”，不是重新讨论采用哪条路线。

## 1. 最终形态

一个客户端运行进程、一个服务端运行进程；GUI 可以作为平台 UI 壳，不拥有第二套协议。公开网络入口仍然是 raw TCP-shaped FakeTCP，同 lane 真实 TLS 建连后切独立加密数据报。

```text
建立（复用）
FakeTCP SYN -> BootstrapStream -> 真实 TLS / Reality-like
           -> 账户、installation、lease admission -> 原有移交机制

发送（单进程直接调用）
业务/TUN -> Game 调度和业务 PacketID -> LINK 分片
         -> lane-local systematic FEC -> TLS-like record
         -> 当前有限恢复发送队列 -> raw TCP-like IO

接收（单进程直接调用）
raw IO -> 当前首次到达接收 -> record 独立 open
       -> FEC systematic 快路 / 恢复 -> LINK 单数据报重组
       -> Game 业务去重 -> 业务/TUN
```

图是职责顺序，不要求颠倒现有 Game envelope 与 LINK/FEC 具体 wire 封装。迁移时记录实际调用与字节关系，复用已验证字段和身份语义。

新根目录不保留 DTLS runtime、data_channel=both、兼容 CLI 或模块间回环 UDP。不同版本明确失败；不静默降级。

## 2. 复用与重写

执行 MODULE_MAP。不是从零发明 FakeTCP/FEC/Tunnel，也不是把整套旧程序复制回来再换一个 encrypt 函数。

旧分支完整保留；old 中快照通过 blob 对照保证没有偷偷改变。主动开发只在根目录新树发生。初次提取先列最小 import 闭包，保留许可与来源，登记 REUSE_LEDGER，删除不需要的旧数据面耦合。

根 Go module 沿用仓库 module path；初始 Go 工具链固定 1.23.12，与源项目已用工具链一致。按所提取代码逐项恢复精确依赖，不运行盲目全量依赖升级。`golang.org/x/crypto` 使用源快照固定 v0.38.0；uTLS 固定源快照 v1.6.5。需要升级时必须有具体兼容/缺陷证据与独立提交，而不是选型工作。

## 3. 建连与阶段移交

保留 SYN persona、同四元组/序列空间、识别、fallback、真实 TLS、账户认证、租约与 ticket。WBD 客户端继续使用当前 SYN persona，但服务端入口不得把该 persona 当身份：任何合法初始 SYN 都先建立同一 FakeTCP association，记录 peer MSS/WS/SACK；MSS 约束 bootstrap 分段，SYN-ACK 的 WS/SACK 只在对端提出时协商。WBD/普通访客的区分推迟到 ClientHello/受保护路径。只有受保护 admission message 做新版本扩展，以携带 WIRE_SPEC 参数。旧 V2 无需兼容；不修改 ClientHello 以宣告自定义数据协议。

不新增 READY/COMMIT/SYNC_ACK 迷你握手。复用可靠 bootstrap 和 stageTransition；允许建连阶段有界多分段在途，移交边界保留 ACK 屏障，不把逐 chunk stop-and-wait 当作不可改变的目标。补齐所有权界限：

1. Server 验证并读完最后一条请求后，记录客户端 bootstrap 结束序列位置，准备新数据接收；server 的最终 TLS 应答与 TCP ACK/修复仍可继续。
2. 最终应答前 exporter keys 与接收器就绪；晚到旧 bootstrap 按边界处理，不把提前新记录重新 Feed 到 TLS。
3. Client 完整读完应答、完成 exporter、安装 record 接收器之后才发送第一条新模式 LINK 数据。
4. Server 收到第一条合法新记录前不向 client 发送新模式业务，防止 TLS reader 预读新格式。
5. 首次 LINK 丢失沿用现有有限恢复、LINK 重试与候选绝对超时。无业务也必须通过现有建立/资格机制结束，不依赖一次性 SYNC 永不丢失。
6. 新 record 成功不是 lane ACTIVE 的充分条件；继续原 LINK attach、双向资格与 make-before-break gate。

方向边界保存于 session，不通过 TLS header 字节猜模式。使用正确 wrap 比较，纯 ACK 始终交 FakeTCP。读预取、残留 TLS writer、post-handshake ticket 和关闭行为必须专项测试；必要的 prepare/detach 是内部 API，不是第二个网络握手。

Transition queue 首版沿用最多 64 条并增加总字节上限 `64 * negotiated_record_wire_max`。每个候选只有一个绝对建连期限，覆盖 ClientHello 识别、TLS、受保护 admission、prepare、最终应答获得 FakeTCP ACK 与 detach；子阶段只能使用剩余预算，不能 TLS 成功后清空 deadline 重新计时。context 取消立即唤醒阻塞 I/O。超时/取消/失败必须关闭候选 association、解除 ACK wait 并清理 transition buffer；只有成功移交后才清除候选 deadline，不杀旧 ACTIVE lane。

### 外观能力边界

“真实 TLS + Firefox120 风格 ClientHello”不等于复刻指定网站的服务端握手。未识别访客 byte-exact replay 到 decoy；已识别 WBD 由本地 Go TLS server 完成握手。当前代码禁用 Session Tickets；P2 重新打开后须明确票据策略、ALPN/证书/服务端参数与实际差异。票据只能由真实 TLS 库生成，在明确的 bootstrap 所有权边界内处理；不强制每次两张，不单改开关、不固定睡眠等票据。支持票据不等于支持会话恢复，不引入 0-RTT。切换后禁止旧 TLS writer 续写 ticket/close_notify/KeyUpdate。不能为外观破坏 no-HOL。

### P2 重开收口（2026-09-20）

保留已修复的普通 SYN 与候选总超时，继续补：重复 SYN/一致 SYN-ACK 与有界握手重传；ACK 合法性；FIN/RST/尾部数据/半关闭和资源释放；bootstrap 有界发送窗口、peer window/zero-window 与真实接收容量。内部 detach 不等于网络关闭，不重写稳态有限恢复。普通回落必须能完成真实 TLS 证书验证、HTTP 响应和关闭。

P2 关闭需要 Actions 普通内核 TCP 客户端经真实网络入口的建连/回落及连续抓包。若缺平台 I/O，仅提取必要最小适配，不扩成完整 P5；没有此证据保持 P2 OPEN。物理 Windows/Npcap 仍属 P7。完整目标网站指纹一致性不以小样本或 serializer 测试宣称。内层业务指纹不属于 P2 关闭条件。

## 4. 加密记录与无 HOL

严格实现 WIRE_SPEC，唯一 profile。完整 PN 保护避免明文递增特征，也避免截断号依赖最高已收值。独立 nonce/open，无 expected receive number，无 record 重传。

记录解密成功即可交上层；近期重复集合没有按序等待或 old-PN 拒收边界。成熟 Game/FEC 的有界去重语义保留，不能新增更窄窗口错误拒收迟到首次包。

密钥随 lane incarnation 更换；不用自定义 KeyUpdate。进程重启不恢复旧 key/PN 会话，必须重新建 lane。

## 5. 单一 MTU 和一次分片

扩展 pathmtu，输入 operator connection MTU、本地实际限制、对端 MSS/记录上限、actual/reserved headers；输出 record/FEC/LINK/Game 预算。不要再加写死 1360、1400 的隐性 cap。

现有产品 connection MTU 576..9000 范围可保留，但若 enabled features 无法容纳最小合法包，配置阶段明确拒绝，不生成负容量。IPv6 外层若未实现不可假定 IPv4 开销适用。

TLS-like 基础开销固定 31 字节，默认 padding=0。P3 增加显式非零 padding 能力时，只用当前 record 的剩余预算；现有无填充 LINK/FEC 容量公式不缩小，不因填充增加分片。SOURCE 和最大 PARITY 均必须 fit。超大业务由 LINK 先分片；不拆加密 record，不调用 carrier 分片。

路径变小通过新 lane 的正确预算解决；不重新切割已分配 TCP Seq 的旧密文。配置 MTU 不是端到端 PMTU 探测，日志明确区分。

## 6. TCP-like 恢复

提取当前 default legacy 恢复行为：4096 有效记录、现有 metadata bound、fresh 优先与有限 repair credit、自适应 gap forgiveness、late first-arrival。SACK/RACK 既有算法可随核心代码存在，但新产品不增加模式选择 UI，也不再进行模式竞赛。

弱网恢复的验收优先级按网络压力分层，但**不是**新增固定丢包率模式开关。低丢包/无损时尽量维持 TCP-like：避免无证据 fast repair、重复修复和不必要 gap forgiveness，减少多余 duplicate ACK/repair 成本。高丢包或持续修复压力下则以有效业务 goodput、交付时延、连接连续性和状态/资源有界为先；允许既有有限 gap forgiveness、旧修复状态退役与 fresh 优先发挥作用，不为抓包更像严格 TCP 等待永久缺口、阻塞 fresh 或制造 repair storm。是否需要策略修改必须由持续丢包、repair 压力、queue age、FEC recovery 和本机 drop 的时间线证据驱动，并防止状态反复切换；不得先造一个按固定丢包率切换的新模式。

seal 后的 bytes 不可变。只有准备提交的记录才分配 TCP Seq；已分配序列空间的首次发送失败必须由明确恢复/失败路径处理，不能静默跳号。

短时乱序仍可 SACK/repair，长期不可恢复债务有界退出。不能为了严格 smoke 全收齐而关闭退役。测试确认 B 到达时不等待 A，并覆盖超过 4096/8192 条记录的永久洞。

外层 checksum、flags、options、MSS、seq wrap 和重传区间逐项正确。有限 ACK 放弃与标准 TCP 的差异公开记录；不要假装能同时满足无限可靠确认和有限恢复。

## 7. FEC 与业务层

复用 live LINK 路径实际允许的完整固定集合，并在同一个 P3 原子任务一次性提取/验收：off、20:4、20:8、20:10、20:12、20:16、20:20。固定 K=20、TailRS 和既有 FEC v1 56-byte wire；不得因为归档里存在 profile-v2 试验代码就引入第二套 wire。所有 fixed profile 保留 source first-arrival、partial-block 只发送有用 parity、compact/retired 行为和绝对 3 秒恢复期限；不新增自动调 FEC、动态比例或参数 sweep。

FEC 不能跨 lane 或阻止其他 block 的 source。每个固定挡位都必须独立测试 systematic 快路、其 parity budget 内恢复、超过 budget 不伪恢复、重复 shard、header/profile mismatch 与跨 block no-HOL。过期扫描只处理到期项，空闲时定时器也能退役，任何进展/重复包都不能刷新绝对 3 秒期限。

Game 1..4 逻辑 lane、物理退休余量、PacketID、replacement、DORMANT、idle/age 等按章程完整复用。统一进程不能删成“临时单 lane UDP 转发器”然后宣称产品完成。

高丢包下 lane replacement 采用 make-before-break 的业务连续性口径：单次候选建连失败不是产品失败；只要旧 lane 仍可用，就必须清理失败候选、保留旧 lane 业务并按既有有界退避/生命周期重试。永久黑洞期间不要求候选成功，网络恢复后必须在已有 deadline/backoff/physical-incarnation 上限内恢复业务并最终完成可行替换。专项证据分别记录 candidate attempts/successes/failures、首次到最终成功耗时、旧 lane 是否持续可用、业务中断窗口、并发 candidate/retiring/physical 峰值；不得因候选失败先切断健康旧 lane。

## 8. 执行模型和公平性

每 lane 或固定分组一个 owner 管理可变热状态。并行来自不同 owner 和有界 FEC worker pool；禁止一个全局锁包住加密、重建和 socket IO。

raw IO 循环快速接收/分发，不在 IO 回调中执行长时间矩阵重建。重建结果带 lane generation、blockID 与 payload ownership；晚到任务不能复活已退出 session。

首版 owner 每轮最多处理 32 个 ready 事件或约 1ms 后让出调度（先达到者），分别覆盖 RX、TX、到期清理，不持续 drain 一个方向。此值是确定的初始实现参数，不安排参数 sweep；若实际测试证明饥饿或高 CPU，按缺陷定位修正并记日志。

只批处理已就绪事件，不使用凑批等待 timer。新数据发送仍受真实带宽、工作队列和当前 repair budget 约束，无 HOL 不等于无限缓存或不做节奏控制。

初版维持当前 source/parity 排序和 repair credit。统一统计各类字节、年龄与丢弃，不新加第二套拥塞算法。不得让 fresh 持续输入饿死已承诺的 parity；发现此类缺陷按具体证据修复。

所有新队列必须同时有条数/字节上限和满时处理；不得阻塞整个 tunnel 等一个 lane。源代码已有上限先继承，新队列参数写入 `internal/datapath/limits.go` 并在 manifest 输出，禁止散落魔数。所有总量还受 server 预算限制。

## 9. Buffer 与观测

定义明确类别：mutable builder、immutable wire、borrowed decode view、owned application packet。函数签名/注释表明是否转移所有权。复制无法安全省掉时保留，不追求零复制口号。

重传 buffer 不能在首次发送后马上归池；FEC 返回的借用内存不能在锁释放/异步投递后被其他包覆盖。这是此前 ownership 修复的核心，必须覆盖 race 与压力测试。

每包只增量计数，不扫描所有 FEC block/tombstone、不 JSON 输出、不打印 payload/key/nonce。低频快照在锁外构建。指标包括记录错误、队列年龄、repair 开销、FEC 库存、各层 drop、generation discard、duplicate、late、运行时间和内存。

## 10. 平台与产品接口

新 CLI 固定为 client/server 单 TLS-like 模式；配置保留账户、SNI/route、MTU、FEC、lanes、idle/age、分流、DNS、物理接口与必要端口。无 data_channel 模式选择。

Linux 用共享 TUN/单 host NAT、多客户端 lease 和源地址校验。Windows 保留 Wintun/Npcap/物理接口绑定、exclusive lease、route/DNS、IPv6 fail-closed 和退出恢复。OpenWrt 保留 TPROXY/policy routing 入口；不要求搬回旧全进程架构。

真实运行中所有业务都经过新统一核心。测试适配器可以绕开 TUN/Npcap 跑协议内核，但结果必须标为 core/adapter，不可当物理网卡端到端证据。

## 11. 开发顺序、测试和发布

按 ROADMAP 的 P1..P7 依赖推进；具体验收见 ACCEPTANCE。不做 DTLS 对比、不做密码算法选型微基准。可测新版本 CPU/内存/PPS，是为了确认目标和定位缺陷。

每个阶段先交功能和 Actions。源码/依赖/runner/toolchain/config/随机种子都写入 receipt。关键候选发同 SHA Windows/Linux 包；hosted 稳定后交物理机，未测物理就保持 NOT_RUN，不能阻塞前面的开发。

“代码写完”“自动测试通过”“物理测试通过”“可以发布”是四个不同状态。任何文档和日志不得混用。

## 12. 不需要做的事

不兼容旧数据协议/CLI；不维护 both；不重新设计身份/lease；不引入第三套分片；不照搬 QUIC 或普通 TLS 可靠流；不为99%外观生成随机业务或延迟凑包；不把旧40Mbps历史成绩当新性能证据；不因 CI 环境限制提前要求物理机或归咎 runner。

## 13. 内层业务流量特征：P3/P4/P5 职责

风险来自长度、方向、突发字节数和往返节奏的组合，不是某个固定 ClientHello 大小。外层加密算法、FEC 或票据不能证明风险消失。只做低成本能力与证据收口，不开展反检测算法赛。

P3：保留零填充默认及既有向量，增加显式、受长度校验的加密内 padding 接口，接入单一 MTU。位置为 FEC 输出之后、seal 之前；open 去 padding 后原样交 FEC。padding 不参与 FEC，不影响 original_lengths，不生成额外 record；满尺寸 source/parity 可零填充直接发。策略层只能即时决定填多少，额度不足立即零填充，不能等令牌或其他包。协议容量之外还必须有每包和累计额外字节上限；生产策略尚未启用，不能仅有 API 就宣称降低了识别率。记录请求/实际 padding bytes 和预算不足跳过次数；默认不读取内层 TLS；用户授权的有界结构识别例外见第15节，不逐包输出日志。FakeTCP 缓存最终密文，重传不重新 padding/seal。

P4：多个真实业务会话复用现有 Tunnel/lane，不将每个 HTTPS flow 绑定新外层连接。保留 Normal=1、Game=2..4、现有竞速/去重、轮换与 DORMANT，不能把多 lane 竞速改成跨包条带化来“混流”。业务稀疏不等待别的流，不为了外观保活延迟休眠。若暴露可选填充配置，默认 off；策略 owner 同时执行每包上限与累计 padding/有效负载比例预算，无额度立即跳过。不得在各 lane 独立额度之外绕过 tunnel/server 总预算；不把多 lane/FEC/repair 放大算作有效业务来获取填充额度。

P5：增加受控真实 HTTPS 客户端/服务器，覆盖首个与复用 lane 上的后续连接、稀疏单连接/自然并发、不同证书链、完整/恢复握手、FEC off/实际启用挡位、无损/既定弱网。采集 outer packet 与 TLS-like record 长度、方向、时间间隔、突发字节数；分开 capture loss、网络丢包、FEC、repair、padding。记录业务延迟/CPU/PPS/线上字节与 padding 成本。生产默认零填充；可选能力只验证正确性和成本，启用为生产策略需单独证据与决策。正常 HTTPS 仅作外观参考，不是 DTLS 旧项目 A/B。样本按站点/会话分组，不用同一会话切片同时作训练和评估；如果报告分类结果必须报告样本来源、误报/漏报及范围，禁止从少量样本推导不可识别。

阶段协调：P2 是当前未关闭的前置验收门；用户授权 P3 的独立工作继续。P2 owner 修改 faketcp/realityfront，P3 owner 修改 tlsrecord/pathmtu/datapath 接口，各自只合并本任务提交；修改共享 wire/STATUS 前先同步远端。唯一 STATUS 同时记录两条工作流，不回滚 P3 成果。P2 未关闭不宣称后续整体验收完成。

## 14. 稳态低开销收口与真实路径弱网性能（2026-09-21）

按 [专项执行规范](WEAKNET_QUALIFICATION.md) 重新打开P4稳态传输与P5增强验收。先修复repair/ACK发送记账、稳态FIN/RST和参数连续性，再补有界SACK/修复预算及增量索引。只定向借鉴旧arq/repair_horizon/adaptive_pressure的已验证行为，不恢复旧拓扑或严格等待。

主测Normal单lane每方向10Mbps、Game四lane每方向3Mbps，FEC20:20/padding off；300ms单向，120秒，无损/5->20->5/5->30->5各3次，共18样本。真实二进制独立进程+内核raw/TUN或TPROXY+netem路径；memorySegmentPair资格不能替代。遵循专项注入、损失、时延、恢复、资源和平台覆盖门槛，CAPACITY_LIMITED不算PASS。最终同SHA全套相关回归及重打P6包，物理资格仍P7。

该资格的分析顺序固定为：先业务唯一交付/goodput/latency/continuity，再看 queue/resource/local drop，再拆 FEC/repair 成本，最后解释 TCP-like 外观。FEC20:20承担主冗余但不是“必然全救回”的数学替代品；FEC recovered、transport repair、最终业务恢复是交叉维度，分别报告而不相加成虚假收益。TCP抓包中的乱序、重复 ACK、同密文有限重传和有限 gap forgiveness 单独列为 transport-hygiene 观察；只要没有同 Seq 不同密文、nonce 重用、错误用户/lane交付、MTU/checksum错误、应用重复/损坏或 HOL，它们本身不直接改写业务正确性结论。

## 15. TLS启动选择性填充（2026-09-22 用户授权）

按 [实现与验收契约](TLS_STARTUP_PADDING.md) 增加默认关闭的 --tls-startup-padding。只在业务owner旁观有界ClientHello前缀，绝对启动窗口内即时申请既有record padding，不等待、不改MTU/FEC/repair、不新建连接；Game共享flow/tunnel预算，parity不填充。这是第13节“不读取内层TLS”的限定例外，只做结构识别，不解密或输出内层内容。原主线任务暂停；新agent完成功能专项后直接更新唯一STATUS，不关闭整个P4/P5。


## 用户追加：弱网生命周期移植与参数统一（2026-09-23）
状态：**生命周期移植功能专项 COMPLETE；目标速率整体吞吐资格 FAIL_CAPACITY_LIMITED。** 2026-09-23 用户已恢复 AF_PACKET/上行容量主线，按 WEAKNET_QUALIFICATION 第9节实施；不关闭整个 P4/P5，也不恢复旧严格 ACK/HOL、4096 以上缓存或全局 buffer 扩容。

实现沿用旧 adaptive_pressure 的 rate/RTT 有界退役原则与 idle activity 二次检查，在新单进程所有权内提供独立认证 health、missing≠idle、客户端有界重连、失败不终止、业务唤醒退避，并修复真实 Actions 暴露的 unilateral-server-idle 竞态：server 不再仅凭周期 idle health 抢先休眠，而是等待当前 authoritative lane 收到 client 有序 FIN 承诺后跟随 DORMANT。wire/profile、FEC、repair horizon、4096、lease/generation fence 与 TLS startup padding 语义均未扩大。

最终功能资格 SOURCE_SHA `0b206a07f91513133a80a147656b637c286ce3e2`：
- `next-lifecycle` run 35803458197 / job 106998829058 PASS；
- `next-foundation` run 35803458187 PASS，repository、Linux/Windows active Go、P2 fallback、Linux shared-TUN iptables/nft、OpenWrt privileged jobs 全绿；
- `next-p4-steady-targeted` run 35803458203 六个 job 全 PASS；
- `next-lifecycle-fullstack` run 35803458184：36/36 独占 runner sample + aggregate job 107001464744 全 PASS，aggregate artifact 10727500614，覆盖 L0 配置实际生效、L1 idle/稀疏/纯下行、L2 单向业务全丢、L3 pre-seal health 1/2/3 连丢、L4 永久旧四元组与临时全四元组黑洞、L5 SYN/TLS/admission/detach 候选失败与退避、L6 1/4 lane cutoff race/黑洞失败后再唤醒/部分多lane、L7 真实默认 15s keepalive/90s dead-after。两 seed 均 PASS，稳定 lease、generation 隔离、candidate/retiring/physical/resource bound 与恢复后持续业务均在 validator 门内。

正式弱网目标也在同一 SOURCE_SHA 重跑：`next-strict-weaknet` run 35803458166 / aggregate job 107000004406 / aggregate artifact 10727035867，18/18 样本均有原始 artifact。CORRECTNESS=PASS 18/18、CAPTURE=PASS 18/18、INPUT_VALIDITY=PASS 17/18（Normal/5205 seed101 因 S2C skipped_slots=37 单独 FAIL），ENVIRONMENT=FAIL 18/18、PERFORMANCE=CAPACITY_LIMITED 18/18。最早异常已经出现在 lossless 控制段：server AF_PACKET `ss_packet` drop Normal 三 seed 约 1,388,390 / 700,414 / 835,210，Game 三 seed约 1,451,308 / 1,451,704 / 1,454,157，并伴随部分 UDP drop；server packet receive buffer 在代表样本达到约 1.002×配置上限。Normal lossless C2S pre goodput 仅约 0.338–0.814 Mbps/10 Mbps，Game lossless C2S约 0.467–0.500 Mbps/3 Mbps，而 Game S2C仍约3 Mbps。异常早于20%/30%注入，不能笼统归因“高丢包”或只写“VM慢”。

同一 strict 账本分开记录成本：Normal 每方向 health=8 records=320B TLS-like wire、padding=0、Game复制=0、reconnect flow=0；Game 每方向 health=32 records=1280B、padding=0、reconnect flow=0。18样本中 Normal FEC parity约 C2S 260–481MB / S2C 195–472MB，repair outer约 C2S 0–12.4KB / S2C 0–243KB；Game FEC parity约 C2S 414–446MB / S2C 462–601MB，Game replication extra约 C2S 117–126MB / S2C 125–163MB，repair outer约 C2S 3.28–4.32MB / S2C 0.002–4.58MB。初始握手 outer 也单独入账；本批没有 reconnect。上述成本受容量塌陷影响，仅作该失败环境的实际账本，不包装成合格性能比。

参数没有新增或改默认值，`docs/PARAMETERS.md/json` 不需要修改；最终 core 的 parameter catalog gate PASS。完整证据与中间修复链见 STATUS 和最新 devlog。原诊断成果保留；用户现已明确恢复性能开发。当前执行顺序、证据、修复边界与最终验收见 WEAKNET_QUALIFICATION 第9节，STATUS.PERFORMANCE_RECOVERY 为进度入口。

2026-09-23性能恢复实施进度：观测 SHA `7ac4f2236c1fa0efbdb8032e7613bbef1a510cf5` 的 `next-performance-recovery` run 35812290504 已完成 core/race 与独占 Normal10 lossless。样本仍为 CAPACITY_LIMITED，但时间线显示 server handler 与容量1 readCh 反压几乎等时，且 handler 内已细分的 transport lock/owner/FEC-LINK/downstream 只能解释少部分耗时；当前最小修复把 `HandleServerSegmentQualified` 每包前后两次完整 `TransportStats`（会扫描 pending/received）改为 O(1) authenticated-record 计数读取。该修改不调整队列、buffer、FEC、Game、注入、生命周期或 wire；必须等同 runner 修复后样本/A-B 证据，不能提前标性能通过。

O(1) qualification 修复在最终验证 SHA `7550844af73ca07481a932d17d6793ccb23ae05a` / run 35815797772 使同seed Normal10的 server reads 从61162增至269830、handler均值从1.899ms降至0.359ms、C2S pre goodput从0.335Mbps升至1.794Mbps、S2C从4.146Mbps升至6.000Mbps；AF_PACKET drops仍有631423，故资格继续 CAPACITY_LIMITED。随着更多包真正进入进程，server TotalAlloc约25.7GB、NumGC=2721。当前第二个最小修复针对 `RawIPv4Endpoint.ReadSegment` 每调用新建约64KiB receive scratch 的确定分配：scratch改为endpoint级复用，匹配包仍复制到精确大小owned backing并从该owned backing解析，保证 `Segment.Payload` 不引用可复用scratch。官方Linux client/server均丢弃ReadSegment第二raw返回值，因此不额外复制；该修改不改变socket buffer、readCh容量、FEC/Game、wire、生命周期或注入负载。

raw receive最终候选 `a924b7c9853e1d280cb7a7062dc46ccd30039b99` 在 `next-performance-recovery` run 35816817908 attempt2 完成完整core/race与独占Normal10 lossless：所有classification PASS，双向10Mbps业务0%丢失，client/server AF_PACKET drops均0；server handler均值约30.3us，进程CPU约76.56s/120s。Normal线上/原始业务约C2S 5.0913x、S2C 5.2231x，但transport retransmitted=0、padding=0；S2C FEC source/parity实测195022224/501074802B。按第9节，当前不直接启动18样本主矩阵，而是先在独立runner做修复前7550844a与a924b7c9同runner顺序A/B及B/A，再跑Game4四lane、每方向合计3Mbps逻辑业务量的lossless定向样本，并输出完整方向账本。只有这些继续通过后才进入原严格矩阵与目标负载长测；不提前关闭性能专项。


### 2026-09-23 容量稳定性诊断：Normal单lane 4096-BDP边缘

Game4逻辑3Mbps lossless 已在 run 35818718764 / job 107045770392 全门PASS，方向账本确认无repair/padding/reconnect放大；但Normal10在同runner A/B与B/A中，fixed a924虽稳定显著优于755，仍在后60秒阶段出现AF_PACKET drop与goodput塌陷。进一步单独rerun AB（attempt2 job 107048157380）排除本workflow三个性能job并发：fixed pre C2S/S2C约9.55/9.86Mbps，stress/post仍降到约2.85/6.31与2.88/6.22Mbps，server/client AF_PACKET drops约439529/65734。因此暂不进入18份主矩阵。

成功的a924 standalone Normal10有 FreshSent=789726、SRTT=601.3ms：约6581 records/s × 0.601s = 3957条平均record-BDP，距4096硬边界仅139条；实测PeakOutstanding=4075，仅余21条，且成功样本Abandoned=0。代码在pending满4096时会优先清理retired/SACK metadata，否则淘汰一个仍未ACK的repair-owned record并计Abandoned/RepairEvicted。该边界与runner差异是当前待证假设，绝不通过扩大4096解决。

下一原子仅增加qualification evidence workflow：固定a924/Normal10/seed631，采集AF_PACKET首次drop、Outstanding/PeakOutstanding/Abandoned/RepairEvicted时序、每核busy/softirq/steal、cgroup throttle、CPU PSI、线程CPU、GC/alloc和server handler/readCh区间长尾，并上传可直接读取的compact artifact。先判定pressure与drop的因果顺序，再做单原因最小修复；不扫描参数、不降FEC/速率、不扩大buffer/4096。

2026-09-23 capacity diagnostic基础设施修正：首次新增workflow的push（SOURCE_SHA `9a509e96587c68424fe7d172f9b799f26c5ad487`，run 35820349990）在Actions调度前直接FAIL，Jobs API返回0 job、0 artifact；静态审计发现compact timeline的Python heredoc未保持在YAML `run: |` block缩进内。该失败不属于产品运行期，也不改变a924 standalone PASS、seed631同runner CAPACITY_LIMITED或Game4 PASS的事实。当前只修workflow缩进并重新触发同一evidence-only Normal10/seed631诊断；产品代码、4096/FEC/Game/速率/buffer/lifecycle均不动。
