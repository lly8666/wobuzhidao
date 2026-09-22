# Actions 验收契约

## 环境与证据

开发期所有构建和测试只在 GitHub Actions；本地不运行 go test、编译、fuzz、性能或网络试验。最终物理资格在 P7，不能提前成为开发依赖。

每次运行保存产品 SOURCE_SHA、harness SHA、依赖/toolchain、runner 架构与 CPU/内存、完整配置、种子、原始 stdout/stderr、分析器结果。收到新结果先核实源 SHA，不能只看 workflow 全绿或其他 agent 摘要。

当前只有 `next-foundation.yml`：仓库契约和条件启用的根 Go 基础测试。其成功只表示已实现检查成功，不表示下列未来网络资格已实现。新阶段必须创建相应可执行工作流，不能将占位输出当测试。

## P1 基础

Go unit：Linux、Windows；race：Linux；定向 fuzz：Linux。固定 keys 与期望 wire bytes、方向分离、PN边界、深度乱序、永久早期缺包、重复/历史淘汰、短包/错长/tag位翻转、多记录、nonce唯一、重传完全一致。

不得仅调用 Encode 再 Decode 自证 wire 正确，必须有独立固定向量。缓存有界、未知旧 PN 可交付、失败不更新状态。root go module 存在后没有实际 Go 包必须失败，不能绿灯占位。

## P2 建连和外观

真实 TLS/persona/fallback/认证、同 lane一个SYN lineage、无第二公开连接；服务端不得用 WBD 固定 MSS/WS/SACK 组合做入口身份门槛，普通合法 TCP SYN（含不同 MSS、窗口缩放、SACK 组合及无选项情形）必须能完成三次握手并到达 ClientHello 分类，身份只在后续 TLS/受保护路径判断。对端 MSS 必须约束 bootstrap 发包，WS/SACK 只按合法 SYN 协商。最后bootstrap丢包/重传/ACK丢失、首条新记录丢失/提前到达、reader预读、退出候选、transition队列上限。新版本拒绝不支持版本，不降级。

候选建连必须有一个绝对期限覆盖 ClientHello 识别 -> TLS -> admission -> prepare/final reply ACK -> detach；各子阶段不得重新获得完整 timeout。TLS 完成后沉默、认证只发送一部分、context 取消、最终应答无法获得 FakeTCP ACK 都必须在该期限内释放候选连接、等待者和 transition buffer；成功 detach 后才清除候选 deadline。

抓包格式门槛：无损且无capture loss时TLS记录连续可解析；重传相同Seq下payload完全相同；无明文私有外层头；MTU/checksum/options/MSS正确，无意外IP分片。真实丢包的乱序、SACK、Dup ACK提示单独解释；有限gap forgiveness造成的标准TCP差异单独计数，不要求消灭。

外观结论必须分层：serializer/unit 只能证明格式。回落的真实目标 TLS 与 WBD 本地 TLS server 是两条路径，不能混称目标网站一致。P2 重开后至少验收：重复 SYN/一致 ISN、SYN-ACK 丢失/有界重传、重复或带数据 ACK、非法 ACK 不释放状态；FIN占序列号/带尾部数据/重复乱序/重传、合法 RST、半关闭与资源释放；bootstrap 多 chunk 有界在途、peer window/zero-window、窗口容量一致、候选总期限；票据启用/禁用的明确策略及与认证合并/拆分/丢包/切换的边界，切换后无旧 TLS writer。不能只改 SessionTicketsDisabled 开关。

P2 关闭必须有 Actions 普通内核 TCP TLS 客户端经真实网络入口访问受控 decoy、验证证书、完成 HTTP 响应及正常关闭的证据，以及同流连续 pcap/capture-loss 记录。受控本地 CA 可用于测试但客户端必须信任并实际验证，不以 InsecureSkipVerify 冒充验证。缺平台 I/O 则最小提取；未跑记 NOT_RUN，不用内存 peer 或 serializer 替代。无损下无额外内核 RST、非法 Seq/ACK、错误重传内容或意外分片。普通互通仍不证明指定网站完整指纹一致。物理 Npcap 留在 P7。

内层 TLS 的长度/方向/突发/往返特征属于 P3/P4/P5，不是 P2 关闭条件。外层 ticket 或握手 PASS 不代表内层特征消失，不增加假 HTTP/随机睡眠/凑包等待。

## P3 数据面

no-HOL：永久丢A，50ms后发B，B在A未恢复时交付；多洞连续超过4096/8192条记录仍前进且状态有界。大包A缺片不阻塞完整B；FEC某block缺失不阻塞其他source。停流后定时退役仍执行。

FEC 在本次 P3 提取中一次性覆盖 live policy 全集合：off、20:4、20:8、20:10、20:12、20:16、20:20；不得只用20:20结果代表其他挡位。每个 fixed profile 都要验证 systematic source 首到立即交付、partial block parity 数量、parity budget 内恢复、某 block 永久缺失不阻塞其他 source、3 秒绝对期限且停流 timer 可退役、迟到 systematic first-arrival、重复 shard 幂等和 active conflicting duplicate/header/profile mismatch 拒绝。所有数据有序号/内容校验，bad_payload、header/shard mismatch、wrong lane、ownership损坏必须为零。MTU覆盖576/1280/1400/1500/1600/9000中的有效组合，无效组合明确拒绝；超大输入后同业务peer合法包仍可用。

### P3 可选填充资格

P3 新增 padding 能力专项门槛：保留 Seal 默认零填充向量；显式非零填充具独立向量和尾零payload/非法padding/边界/tag破坏测试；open 去填充后 FEC/LINK 字节完全一致。FEC off及全部固定挡位、满尺寸 source/parity、MTU576..9000有效组合均不超 peer MSS/record/packet 限制；不足余量策略跳过，显式非法请求拒绝，不多生成一个分片或record。预算不足不等待，丢A仍交B，同Seq重传完全一致；接口和生产启用状态分开报告。相关统计无每包日志。

## P4 产品（生命周期与可选填充）

Normal=1，Game=2/3/4；第5权威logical lane拒绝；物理incarnation最多10，第11拒绝；退休占用不阻塞合法替换。多installation地址不同，lane更换lease不随意变化，source anti-spoof保留。

A->A+B->B，候选失败保留A，逐lane轮换，generation fencing，DORMANT/wake，keepalive不刷新payload idle，手动断开/退出确定清理。高丢包下单次candidate失败不是硬失败：旧lane可用时必须保持业务、清理candidate并按既有有界退避重试；黑洞内不要求换lane成功，网络恢复后要求有界恢复并记录attempt/success/failure、最终替换耗时、业务中断与candidate/retiring/physical峰值。

Linux共享TUN/单host NAT/DNS/UDP/TCP、Windows路由/分流/lease/IPv6清理、OpenWrt策略路由。真实数据端到端输出，不只看READY。

多个真实业务 flow 复用既有 lane，不每flow重建外层；不改变 Game 竞速/PacketID 语义、不增加 lane 数、不等其他业务混流。可选 padding 默认 off；若实现配置，验证每包与累计额外字节预算、tunnel/server总上限、无额度立即零填充。padding/keepalive 不刷新 payload idle，空闲无假流量，DORMANT/wake 和轮换行为不退化。

## P5 弱网、负载和长测（仅新版本）

网络：300ms单向；120秒；30/60/30秒；5%->20%->5%和5%->30%->5%。保留原业务大中小包混合和双向负载，harness从old定向提取后删除旧源码下载与DTLS选择。

每个Action job同时只跑一条负载，允许不同job扇出。每场景至少两次独立运行。5/10/15/20Mbps起步；Mbps明确每方向或总和，同时报告PPS和线上实际字节。再做资源允许的40Mbps aggregate-inner目标；50/100Mbps只是容量探索，不把达不到定为协议错误，也不恢复旧分支40Mbps成绩。

额外覆盖无损低延迟、单向/双向、小包PPS、FEC off、多lane。记录 offered/accepted/actual send、send failure、skipped slots、send lag；不能因一个可忽略skip就判全无效，也不能忽略真实注入不足。注入有效性必须单独出结果。

输出 goodput、app loss/duplicate、RTT p50/p95/p99、post5恢复、CPU分核/host busy、内存/GC、各队列峰值和驻留、实际drop位置、FEC/repair库存、wire amplification。无旧版本对照、无cipher赛，不做参数矩阵选型。

30分钟以上新版本soak覆盖持续丢包、轮换、停流退役和内存平台期。若host达到资源上限，报告 CAPACITY_LIMITED 并继续定位，不把样本抹掉或一概归为runner差。

硬失败：任何内容损坏、错误隧道交付、nonce重用（不同新记录）、同Seq重传密文变化、MTU/checksum错误、lane-wide等待缺失前包、状态/队列无界、超时不释放、存在健康旧lane却因candidate失败主动中断业务、资源清理破坏。一般性能和有损场景loss仍按增强门槛报告，结合FEC能力与输入校验归因。TCP抓包乱序、duplicate ACK、同密文有限repair、有限gap forgiveness和单次lane candidate失败只作transport/lifecycle解释，不因外观本身直接判业务失败；但若引起loss/goodput/latency/continuity/resource超门，仍如实FAIL/CAPACITY_LIMITED。

### P5 流量外观专项

P5 流量外观专项：受控真实 HTTPS 覆盖新外层首连接、已建lane后续连接、稀疏单连接/自然并发、不同证书链、完整/恢复握手、FEC off/实际挡位及无损/既定弱网。普通 UDP 混合包测试不能替代。分层记录 outer packet/record 长度、方向、间隔、突发字节与往返节奏；标注 capture loss 和丢包位置，分开 FEC/repair/padding 放大及 CPU/延迟/线上开销。正常 HTTPS 是参考，不要求每业务都像浏览网页、不做旧 DTLS A/B。保留可复现原始数据/分析口径；不得只凭固定200..550字节规则或小样本“检测失败”宣称不可识别。若报告分类结果，按独立会话/站点分组并报告误报漏报，不能同会话切片泄漏到训练与评估。当前生产默认0，任何启用决策需另行记录，不能把代码能力等同于抵抗流量分析通过。

## P6/P7 状态与发布

逐个区分 IMPLEMENTED、ACTIONS_PASS、PHYSICAL_PASS、RELEASE_QUALIFIED。build包含源SHA与文件哈希，客户端/服务端同源码。Windows hosted若不具备真实Npcap驱动/TUN能力则明确UNSUPPORTED，提供可测适配路径但不替代物理证明。

P7只在主流程已稳定后由用户安排，最终核对真实链路外观、业务DNS/UDP/TCP、Game/rotation/DORMANT和退出网络恢复。不提前调用物理机、不因为未测物理停止P1..P6。

## 2026-09-21 增强验收（当前发布前必需）

[WEAKNET_QUALIFICATION.md](WEAKNET_QUALIFICATION.md) 是本契约组成部分，其第3至8节定义持续负载、18份独立样本、真实网络、严格逐方向/逐阶段目标、runner诊断、模块矩阵与关闭证据；第1.1节定义高/低丢包的解释优先级。新增门槛优先于旧P5的低频HTTPS和无损15秒load资格；不追溯删除旧证据，也不沿用旧CLOSED代表新增通过。第1.1节只降低“严格TCP外观/单次rotation成功”的门控地位，不降低业务loss/goodput/latency/continuity/resource和基本正确性门槛。相关核心回归有红灯须定位修复，不以仅打包job绿替代同SHA完整回归。

## TLS启动填充小功能（待 exact-SHA Actions）

实现及完整关闭门槛见 [TLS_STARTUP_PADDING](TLS_STARTUP_PADDING.md)。默认off、无等待/no-HOL、全FEC source/parity区分、Game共享预算、生命周期/失败rollback/资源上限必须覆盖；真实二进制TUN与平台转发专项必须证明启用和实际padding，不得用core/serializer替代。当前仅已编写测试与workflow，验收NOT_RUN，不能宣称消除了TLS-in-TLS。通过后只关闭此小功能，原主线暂停不变。
