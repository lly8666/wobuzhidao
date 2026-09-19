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

外观结论必须分层：hosted serializer/unit 只能证明 TCP/IP/TLS 格式与本地 persona，不得宣称“已与指定借用网站握手指纹一致”。未识别访客的 byte-exact ClientHello replay + decoy splice 与已识别 WBD 的本地 Go TLS server 是两条不同路径。指定网站的服务端参数、ALPN、握手长度/分段、会话恢复等相似性只有在平台 I/O 接入后的真实普通浏览器访问和连续抓包中才能验收；禁用 Session Tickets 的阶段切换安全性优先，不得为外观擅自重新开启。

## P3 数据面

no-HOL：永久丢A，50ms后发B，B在A未恢复时交付；多洞连续超过4096/8192条记录仍前进且状态有界。大包A缺片不阻塞完整B；FEC某block缺失不阻塞其他source。停流后定时退役仍执行。

FEC off/20:20首批，之后源快照已实现的其他固定档位。所有数据有序号/内容校验，bad_payload、header/shard mismatch、wrong lane、ownership损坏必须为零。MTU覆盖576/1280/1400/1500/1600/9000中的有效组合，无效组合明确拒绝；超大输入后同业务peer合法包仍可用。

## P4 产品

Normal=1，Game=2/3/4；第5权威logical lane拒绝；物理incarnation最多10，第11拒绝；退休占用不阻塞合法替换。多installation地址不同，lane更换lease不随意变化，source anti-spoof保留。

A->A+B->B，候选失败保留A，逐lane轮换，generation fencing，DORMANT/wake，keepalive不刷新payload idle，手动断开/退出确定清理。

Linux共享TUN/单host NAT/DNS/UDP/TCP、Windows路由/分流/lease/IPv6清理、OpenWrt策略路由。真实数据端到端输出，不只看READY。

## P5 弱网、负载和长测（仅新版本）

网络：300ms单向；120秒；30/60/30秒；5%->20%->5%和5%->30%->5%。保留原业务大中小包混合和双向负载，harness从old定向提取后删除旧源码下载与DTLS选择。

每个Action job同时只跑一条负载，允许不同job扇出。每场景至少两次独立运行。5/10/15/20Mbps起步；Mbps明确每方向或总和，同时报告PPS和线上实际字节。再做资源允许的40Mbps aggregate-inner目标；50/100Mbps只是容量探索，不把达不到定为协议错误，也不恢复旧分支40Mbps成绩。

额外覆盖无损低延迟、单向/双向、小包PPS、FEC off、多lane。记录 offered/accepted/actual send、send failure、skipped slots、send lag；不能因一个可忽略skip就判全无效，也不能忽略真实注入不足。注入有效性必须单独出结果。

输出 goodput、app loss/duplicate、RTT p50/p95/p99、post5恢复、CPU分核/host busy、内存/GC、各队列峰值和驻留、实际drop位置、FEC/repair库存、wire amplification。无旧版本对照、无cipher赛，不做参数矩阵选型。

30分钟以上新版本soak覆盖持续丢包、轮换、停流退役和内存平台期。若host达到资源上限，报告 CAPACITY_LIMITED 并继续定位，不把样本抹掉或一概归为runner差。

硬失败：任何内容损坏、错误隧道交付、nonce重用（不同新记录）、lane-wide等待缺失前包、状态无界、超时不释放、意外业务中断/资源清理破坏。一般性能和有损场景loss先报告，结合FEC能力与输入校验归因，不硬要求有限恢复随机丢包下100%收到。

## P6/P7 状态

逐个区分 IMPLEMENTED、ACTIONS_PASS、PHYSICAL_PASS、RELEASE_QUALIFIED。build包含源SHA与文件哈希，客户端/服务端同源码。Windows hosted若不具备真实Npcap驱动/TUN能力则明确UNSUPPORTED，提供可测适配路径但不替代物理证明。

P7只在主流程已稳定后由用户安排，最终核对真实链路外观、业务DNS/UDP/TCP、Game/rotation/DORMANT和退出网络恢复。不提前调用物理机、不因为未测物理停止P1..P6。
