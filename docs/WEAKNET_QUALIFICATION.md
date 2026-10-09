# 稳态修复与真实路径弱网资格（2026-09-21）

**当前执行入口：第10节（用户最新丢包容忍与单run单样本要求），第9节保留诊断证据。2026-09-23用户已解除性能主线HOLD，授权全新主线agent继续修复。历史HOLD不再阻塞此项工作。**

本文是 DEVELOPMENT_PLAN / ACCEPTANCE 引用的专项执行规范，不是第二套状态或交接。用户新增要求使 P4 稳态传输与 P5 性能资格重新打开；历史通过证据保留，但不能覆盖新增门槛。生产代码修复后 P6 必须按最终同 SHA 重新打包，P7 仍 NOT_RUN。

## 1. 低开销修复顺序

先检查最新代码确认问题，不照历史摘要盲改。一次原子变更，Actions 定向回归后再进入下一项。

1. runtimeowner tick 中重传选择和 gap-forgiveness ACK 不得互相覆盖；选中、尝试发送、成功发送、失败分别计数，发送失败不可悄悄释放已占 Seq 的修复债务。补同时到期测试。
2. 将 P2 已验证的 FIN/RST/半关闭语义接到稳态 handoff 后的实际入口。使用稳态最新 Seq/ACK，内部 detach 不是外层关闭；客户端、服务端、rotation、DORMANT、人工退出均需覆盖。有界关闭仅影响退出lane，不等待缺包阻塞其他业务。
3. 保留 no-HOL，补成熟的有界选择性确认、fresh优先修复预算和必要的 RTT/RTO 估计。只在已协商 SACK 时发送，SACK真实报告已接收范围，业务仍首次到达立即交付。保持有限gap forgiveness、4096有效记录边界和late first-arrival。不能为了累计ACK完整恢复严格等待。
4. 避免每ACK全量扫描/压缩数千记录，使用有界增量索引；补深洞、回绕、历史淘汰、稀疏ACK和停流清理。不要以增加锁、后台线程或每包统计抵消收益。
5. 建连到稳态的window/scale、MSS、实际TCP options、persona保持一致；SACK改变头长必须进入单一MTU预算。当前未实现的timestamps不伪造，不顺手增加新选项。padding仍默认off，禁用假业务、凑包等待和随机延迟。

性能保护：修改前后只比较新版本的具体修复，同输入/同配置/相同资源，修改前后分开独立Action run，每个run仅一个源码版本的一条样本；不再在同run顺序AB/BA。无损有效业务吞吐不得下降超过5%，单位有效MiB的进程CPU时间不得增加超过10%；超出需归因并修正，不靠跨VM平均掩盖。此为本轮工程目标，不是历史已达性能。不进行旧DTLS整体A/B。

### 1.1 高/低丢包恢复优先级

用户最新授权允许高丢包阶段业务保留相当比例的损失，不再追求强制近零丢包；第4节按第10节修订后的门槛执行。正确性、no-HOL、注入、低延迟和资源有界仍为硬门。

- 低丢包/无损：尽量保持 TCP-like，重点排查无证据 repair、重复 repair、过早 gap forgiveness、异常 duplicate ACK 与不必要的线上放大；但外观观察与业务正确性分栏，不能为了“看起来更TCP”回到累计ACK等洞。
- 高丢包/持续恢复压力：有效业务 goodput、唯一交付时延、连接连续性和资源有界优先。允许既有有限 gap forgiveness、repair metadata退役和 fresh 优先发挥作用；不得为了补齐抓包缺口阻塞 fresh、恢复 HOL、让 repair 持续挤压 fresh，或停流后制造修复风暴。
- FEC20:20是主冗余层但不是万能保证。每阶段分别报告 source/parity、reconstruction/recovered source、有限 repair、最终业务loss/goodput和queue age；FEC恢复次数、repair次数与最终业务收益存在交叉，禁止相加成“总救回字节”。
- 成本过高先查重复repair、FEC恢复后仍持续repair、parity过晚、fresh/repair公平性、产品queue与本机socket/interface drop；不得先扩大4096/FEC/socket buffer，也不得先提高重传强度。
- lane replacement 在高丢包下按连续性验收：单次candidate失败本身不判产品失败。旧lane仍可用时，失败candidate必须释放，旧lane继续承载业务，并按现有有界退避/生命周期重试；记录尝试次数、成功率、最终替换耗时、业务中断和candidate/retiring/physical峰值。
- 永久黑洞期间不要求成功换lane；网络恢复后必须在测试声明的既有deadline/backoff范围内恢复业务，且重试、并发candidate与资源占用有界。存在可用旧lane时，candidate失败不得主动切断旧业务。
- 不新增未经验证的“loss>=X%切换策略”产品开关。若确需改恢复策略，只能依据持续丢包、repair压力、queue age、FEC recovery、本机drop等实际信号，并证明有滞回/有界性，不发生来回抖动。

外观让步不取消硬正确性：同 Seq 修复密文一致、MTU/checksum正确、nonce不重用、用户/lease隔离、应用无重复/损坏、无跨记录/跨lane HOL、状态与队列有界仍是硬门。TCP抓包乱序、duplicate ACK、同密文repair以及有限缺口放弃单独解释，不直接等同业务失败。

## 2. 可借鉴老项目的明确范围

只读冻结来源 b5c848f 的 old/internal/faketcp/arq.go、repair_horizon.go、adaptive_pressure.go 及直接依赖/测试；FEC来源只看 MODULE_MAP 指定 fec/linkdata。禁止读取旧提示词恢复架构。

可借鉴：SACK记账与提前释放已收payload、有限shadow repair credit及fresh优先、RTT估计与RTO上下限/退避、adaptive gap forgiveness、元数据compact/退役、partial-FEC flush和source/parity公平性。先列旧参数值、单位、触发条件、依赖和新路径适用性，再提取最小闭包并登记REUSE_LEDGER。旧数值不是推荐常量；固定1s RTO/3s transport horizon和8ms FEC flush的实际调度精度都应以证据核对，不能把声明8ms误当实际8ms触发。

不默认迁入 strict sack-rack 模式，不恢复旧Controller/DTLS/回环UDP；不把FEC 3秒恢复期与transport repair期混为一项；不盲目扩大4096、FEC block上限或socket buffer。若性能不足，一次修改一个有证据的根因/参数，保留失败样本与新旧候选同口径结果，不开全参数矩阵。

## 3. 主测流量口径和场景

Mbps为十进制；业务速率按加密/协议头/FEC/Game复制之前的应用payload计算。每方向独立计数，不把echo响应当第二份独立注入，不把Game副本计入goodput。

| 配置 | 每方向业务输入 | 双向业务合计 | 固定条件 |
|---|---|---|---|
| Normal 1 lane | C2S 10Mbps、S2C 10Mbps | 20Mbps | FEC20:20、padding off |
| Game 4 lanes | C2S 3Mbps、S2C 3Mbps | 6Mbps | 每lane FEC20:20、同PacketID四lane竞速，非四等分 |

四lane在满块情况下仅复制与FEC的payload量级可达约48Mbps双向，另有头/ACK/repair，不能按6Mbps评估runner压力；单lane对应约40Mbps加额外开销。实际partial块与包长改变放大率，必须实测。MTU主测固定合法1400并保存所有派生预算。

每配置三个场景：无损300ms单向控制、5%->20%->5%、5%->30%->5%。每场景120秒有效注入，30/60/30秒阶段；额外warmup/建连不计入，结束后固定10秒drain仅收尾计数，不延长goodput时间窗。每场景3个独立Action run与固定不同seed，共18个性能run。一个workflow run只运行一个配置、一个源码版本、一个seed、一个场景的一条负载；不得在同run中matrix扇出或顺序跑多条。独立run可并发，不能共享负载runner；汇总另用只读产物的run。方向独立损伤；四lane共享同一瓶颈qdisc、对包独立抽样，不能人为保证四副本中必有一份幸存。额外共享链路突发/黑洞专项见下节。

主测用双向持续UDP序号+长度+内容校验负载，经正式业务入口和正式client/server，不能用每3秒一次HTTPS请求代替10/3Mbps注入。固定等包数64/256/1200字节循环（包含测试序号和时间字段），按实际字节精确pacing，保存发生器配置；与旧混合模式不一致时明确标注，不称完全同口径。同步低速RTT探针另计开销、不计主业务输入。HTTPS/TCP保持独立真实业务专项，不用其重传掩盖UDP丢包。

## 4. 严格验收目标

以下是新增目标，不是由FEC数学保证的所有网络条件承诺。原始数据和分析器独立复算，逐方向/逐阶段/每次重复都报告；不能用全程均值覆盖高丢包阶段失败。

- 注入：每阶段actual send payload在目标99%..101%；发送失败0；skipped slots <=0.01%；p99调度迟到<=10ms；后段不持续积压。达不到标INVALID_INPUT，原结果保留并定位发生器/背压/环境，不能叫协议PASS。无效项仍继续检查数据损坏等硬错误。
- UDP有效唯一payload：无损损失0、goodput>=目标99%；有损阶段允许业务包损失<=该阶段配置丢包比例p（5%/20%/30%），墙钟有效字节goodput>=目标×(1-p)×99%。这是工程验收目标，不是FEC数学保证；包损失和字节goodput分开检查。损失按发送时间阶段分组，10秒drain后仍未收到才计最终丢失，同时报告每阶段墙钟实际交付速率及迟到量，避免迟到掩盖堵塞。
- 正确性：坏payload/错误用户或lane交付/应用重复/nonce重用/同Seq不同密文/意外IP分片均0；MTU/checksum与generation/source fencing保持正确。丢A交B与跨lane无HOL单独定向证明，不以低平均RTT代替。TCP乱序、duplicate ACK、同Seq同密文有限repair、decoder已见重复和有限gap forgiveness进入非门控transport-hygiene账本；它们若造成业务损失/时延/资源超门仍由对应硬门失败，但不因“外观不够严格TCP”单独把CORRECTNESS判FAIL。
- 时延：同场景无损300ms控制作为基线；损伤场景低速探针的p95 RTT增量<=200ms，p99增量<=500ms，探针超时独立计数（超时不得从分位数报告中隐去），不再要求有损阶段探针损失<=1%；分别报告请求单向损失、响应单向损失和往返超时，不能把双向探针失败率直接当单向业务损失率。无损探针丢失仍为失败；有损时不得过滤超时以伪造低时延，若成功探针不足以形成可信分位数则时延资格未通过。报告UDP单程p50/p95/p99，跨时钟测量说明同步方式，不能假设物理双机时钟相同。
- post5：降回5%后10秒内进入连续3个1秒窗口goodput>=目标×95%×99%、该窗口发送包最终损失<=5%；同时队列年龄回到初始5%稳定段p95+200ms以内。固定时间定义，不能沿用稀疏请求“第三次成功”口径。
- 资源：所有队列有条数/字节/年龄界，重建/修复/退役在停流后仍执行；drain后按各状态既定deadline回收，不要求有TTL的去重元数据立即归零。严重持续host/socket/非预期qdisc drop时目标未通过，不默认为协议正确或仅runner问题。高丢包下额外给出fresh/repair/FEC时间线，检查FEC已恢复后冗余repair、parity迟到、repair挤压fresh、停流repair风暴与恢复阶段queue不退；没有时间线和字节证据不改策略。
- 验收标识分开：CORRECTNESS、INPUT_VALIDITY、PERFORMANCE、ENVIRONMENT、CAPTURE。PERFORMANCE可为PASS/FAIL/CAPACITY_LIMITED；CAPACITY_LIMITED不是PASS，不能关闭性能门。3次均满足才关闭该场景；异质样本保留并补诊断，不择优重跑。

## 5. 接近真实部署的Actions路径

Linux root runner内用隔离netns/veth搭建：业务发生器/客户端入口 -> 独立正式client进程 -> underlay损伤router namespace -> 独立正式server进程 -> 目标业务/HTTPS服务。可用测试netns/veth，不恢复产品每用户netns架构。正式二进制必须经过真实raw socket、内核路由/firewall和TUN或TPROXY；若Linux客户端只支持OpenWrt模式就使用该正式入口，不虚构Linux TUN客户端。

两方向netem各作用一次，配置300ms单向和正确阶段loss；主测不叠加隐藏带宽cap，记录link速率/MTU、队列limit和超限drop，避免netem队列自身制造非预期损失。以qdisc计数、分段序号与入/出双点抓包确认损伤；完整pcap与capture dropped统计保留，适当snaplen/离线分析减少测量开销，不能用序列化Segment伪装网卡抓包。测试harness不解析修改业务包来绕过产品路径。

memorySegmentPair与内存故障注入保留为core回归，不替代以上端到端性能资格。bootstrap-under-loss另建测试，主测从完成资格的lane开始计时，二者分开报告。

先真实无损闭环和链路校准，再18个主测；最后每配置一次>=30分钟目标速率长测（持续5%loss、周期20%阶段、至少一次rotation），不是低频请求soak。安排独立生命周期低速测试覆盖DORMANT/wake和退出，不要求持续满载时自然休眠。

## 6. runner瓶颈必须重点报告

每秒采样：CPU型号/核数/cgroup quota；每核user/system/softirq/steal、进程及线程CPU、run queue/PSI、context switch；RSS/heap/GC；发生器send lag/PPS；socket实际SO_RCVBUF与skmem/drop、/proc/net/snmp、接口及qdisc drop/backlog；capture drop；每lane fresh/FEC source/parity/repair/ACK/padding线上字节和队列年龄。采样与抓包自身开销用短控制确认，不在热路径逐包打印日志。

整机CPU未100%不能排除单核/线程/锁/softirq瓶颈。报告事件时间顺序和最早掉包位置，区分预期netem loss、qdisc limit、socket overflow、产品队列和发生器不足。固定队列不能只给包数不报实际bytes/age。

容量怀疑时保留原目标FAIL/CAPACITY_LIMITED，另做同拓扑旁路校准及同配置减半速率诊断；必要时两档速率验证平台拐点。旁路不算产品PASS，降速不替代10/3Mbps目标。同runner服务/发生器竞争和共享四lane瓶颈必须写入结论。若更大runner不可用，明确需要什么CPU/链路资源，不假称已达标，也不自行采购/转物理机。

## 7. 模块/配置覆盖矩阵（有界，不做全笛卡尔积）

主测只固定20:20。另用低成本独立功能job覆盖：FEC off/4/8/10/12/16/20全部挡位；Normal1/Game2/3/4；实际MTU576/1280/1400/1500及1600/9000可用veth路径、不同peer MSS/record limit；padding off/on与预算耗尽；DNS/UDP/TCP/HTTPS；IPv4 lease隔离/伪源拒绝；多用户；候选失败、A->A+B->B、旧generation包；DORMANT/wake；FIN/RST/半关闭；无业务退役；进程异常退出/重启和WBD-owned网络清理。选成对交叉加高风险组合，保存覆盖表，不以默认配置PASS代表全部。

损伤专项：仅上行/仅下行loss、ACK loss、乱序/重复、对所有四lane共同100ms/500ms短黑洞、单lane故障、rotation；描述精确seed/时间，不套用主测独立随机loss数值门槛，业务正确性/连续性/有界性仍为硬门并报告恢复时间。rotation不得把“每次candidate必须一次成功”设为门：分别统计attempt/success/failure、失败清理、旧lane持续可用、最终替换耗时、业务中断和并发资源峰值；黑洞内允许持续失败，恢复后必须有界恢复。HTTPS证书验证、完整/恢复握手/稀疏/并发继续保留。四lane下loss相关性、竞速去重成本单独报告。

平台：Linux真实root网络/raw/TUN与OpenWrt风格TPROXY在Actions尽量实跑；iptables/nft支持的后端分别资格。Windows Actions执行真实可用模块，无Wintun/Npcap驱动/管理员能力时明确UNSUPPORTED，mock/compile不能写physical PASS。ARM64交叉编译不能写运行通过。IPv6隧道传输不支持，2026-10-04新增的默认黑洞/drop须独立验证无出口与清理，不能把IPv4通过写成IPv6已代理。硬件相关能力仍留P7。

## 8. 交付与关闭

每轮详细devlog、唯一STATUS、实际源码/构建/harness/分析器SHA、完整配置/seed/runner、原始日志/pcap/计数/分析脚本与产物hash。同一最终SHA必须通过受影响核心Windows/Linux回归与Linux race、Game4间歇问题闭环、真实路径主测和新增模块矩阵，再标增强P4/P5 CLOSED；旧P6包历史有效但不覆盖修复版，重新打包后才交P7。

文档本身不能把新增目标标PASS。对失败先给根因证据、最小修复和回归，不无限试参数；主旨约束不因性能不够而降低。

## 9. 当前主线：接收容量与流量放大修复（2026-09-23）

生命周期功能保持COMPLETE；性能仍FAIL_CAPACITY_LIMITED。本节是原性能任务的继续，不是重做项目或恢复旧DTLS。开始前核对最新远端，禁止回退到84c466f覆盖后续修复。

### 9.1 基线事实与证据

资格源码为 `0b206a07f91513133a80a147656b637c286ce3e2`，休眠竞态产品修复为 `65ff2ef27dd763cba2f7293e6ef6274bca6632c3`，本次文档起点c3a10a2。

- [功能run 35803458184](https://github.com/lly8666/wobuzhidao/actions/runs/35803458184)：36/36样本及aggregate PASS，artifact10727500614。保留客户端主导休眠、服务端等待全部当前权威lane的PeerFIN、黑洞恢复、候选失败退避、100/100截止点交付、稳定lease等已验证语义。
- [性能run 35803458166](https://github.com/lly8666/wobuzhidao/actions/runs/35803458166)：18份样本，aggregate job107000004406/artifact10727035867。CORRECTNESS/CAPTURE各18/18通过，输入17/18通过（Normal/5205/seed101 S2C skipped_slots=37）；ENVIRONMENT全部失败，PERFORMANCE全部CAPACITY_LIMITED。不能将输入无效样本当有效性能证据，也不能将环境失败解释成产品已达标。
- 无人工丢包样本起始阶段：Normal目标10Mbps，C2S仅0.338～0.814Mbps、S2C4.197～9.588Mbps；Game4目标逻辑3Mbps，C2S0.467～0.500Mbps、S2C约3Mbps。异常不依赖20%/30%损伤才出现。
- 无损样本server AF_PACKET socket累计drops：Normal三seed为1,388,390/700,414/835,210；Game为1,451,308/1,451,704/1,454,157。代表样本内存水位约1.002倍接收缓冲上限。已定位丢包边界，尚未区分读包停顿、同步处理、锁/CPU/调度、放大流量和runner竞争的贡献。
- Normal/lossless/seed101原始job106998829314、artifact10726539321：C2S outer IP bytes/app raw input约5.07892倍，FEC parity481,014,962B、repair outer0B、health320B/120s。它是代表样本，不是全组均值或理论常量。Game health每方向1280B/120s；health数字未含IP/TCP头与ACK。不能据此断言全部补丁CPU开销为零。
- 历史同拓扑旁路约50Mbps业务/方向通过只能证明旁路能力，不能代替正式产品路径，也不排除当前runner差异。复用原始证据，不反复全量重跑同一旁路当作进展。

### 9.2 第一原子任务：定位并修复服务端接收停顿

实际路径：`internal/faketcp/raw_linux.go:ReadSegment` → `internal/runtimeentry/lifecycle.go:Run` 的容量1 readCh → HandleServerSegmentQualified → record/FEC/LINK/Game处理 → SharedTUNRouter/平台service/TUN交付。同步链值得检查，但不得预先断定channel=1就是根因。

补低开销证据：reader相邻成功读取间隔、readCh交接阻塞时间、handler耗时、owner锁等待/持锁、decode/reconstruction/retire耗时、下游写入阻塞、队列条数/字节/最老年龄。用有界直方图或抽样、低频快照；需要时只对短样本启用CPU/block/mutex profile，测量开销单列。和每核CPU/softirq/steal/GC、AF_PACKET r/rb/d、pps、外层流量按同一时间轴对齐，禁止逐包日志。

先做一个Normal10M无人工丢包独占runner定向样本。证据支持后一次修改一个原因，再用Game4逻辑3M验证方向/多lane影响。可以针对性拆开接收和下游处理、改进有界调度、减少热路径工作，但必须说明包所有权、每association状态串行化、generation、关闭/drain、队列条数/字节/年龄及溢出策略。禁止每包起goroutine、无限队列、静默丢包，或靠扩容暂时拖延overflow。bootstrap保持必要有序，稳态不能重新等TCP缺口。

### 9.3 第二原子任务：审计线上放大账本

逐方向/阶段核对app payload → platform envelope → LINK片数 → FEC source/parity → TLS-like record → TCP/IP/ACK。记录原始包长分布、MTU派生值、fragment数量、每块有效source字节/最长shard/尾部补齐、full/partial block比例、实际flush时刻、source/parity长度分布、Game额外副本、repair、health、padding和握手。

区分线上抓包与encoder计数、尝试与实际发出、输入与成功交付；禁止嵌套计数重复相加。FEC20:20是分片数量比例，不保证混合包长下总外层字节恰好两倍，5.08倍也不自动证明bug。先验证窗口/分母，解释每项差额，再查重复编码、错误长度、意外分片、flush/调度偏差或可避免复制。只修有证据的实现浪费，不通过降低FEC档、减少Game副本、全改大包、降低业务注入或攒包等待冒充优化。涉及既定wire/算法边界时明确证据和影响，不顺手重构。

### 9.4 验证顺序与关闭条件

1. 最小修改先跑相关core/race和定向真实路径样本；无损目标仍崩时，不重复整个18份大矩阵。
2. 改善后用各自独立Action run比较当前新架构修复前后，输入/配置一致，每run只有一条样本，覆盖Normal/Game；分别重复并报告runner差异，不能声称同runner配对。沿用第1节吞吐/CPU保护门槛，另报总CPU与每有效MiB CPU，防止丢包更多让CPU看似下降；不做旧DTLS A/B。
3. 两模式无损目标过关后，最终源码SHA跑第3～4节原18份主测，各场景3seed、逐方向/阶段执行第10节修订后的门槛。输入失败样本修好发生器/环境后重跑，原FAIL保留，分析器显式版本化本次用户授权的门槛变更；禁止因失败再次私下降标。
4. 保留Linux/Windows核心、race、no-HOL/MTU/同Seq密文、padding/配置及受影响L1/L4/L5/L6/L7回归；改生命周期/队列所有权时重跑完整36样本。参数变更同步PARAMETERS.md/json。
5. 稳定后做原规范要求的两模式各>=30分钟目标负载长测。未跑写NOT_RUN，不提前关闭P5或发布。功能COMPLETE与性能状态独立，满足最终资格才关闭性能主线；物理P7不在本轮冒充通过。

若runner仍限制，指出具体核/线程/队列、pps、CPU/steal竞争、容量拐点与所需资源，保留目标FAIL/CAPACITY_LIMITED；可以降速诊断，不得降速验收。每轮devlog记录事实/假设、改动、前后成本、精确源码/分析器SHA、run/job/artifact及下一步；两轮没有新证据就缩小诊断，不无限扫描参数。

## 10. 当前开发决策：允许损失，禁止恢复工作拖垮业务

2026-09-23用户明确：链路30%丢包时可以接受项目仍有30%业务丢包，优先保持处理性能、无HOL、低延迟和突发后的稳定性。本节覆盖历史近零损失门槛及同run A/B要求；第4节已同步修订。不是主动丢掉30%业务的配额，不允许以人为丢包降低CPU，也不能把本机overflow解释为预期netem损失。FEC与Game照常尽力恢复，不保证每组成功，不降低原10Mbps/逻辑3Mbps注入目标。应用层可靠协议可继续补救，普通UDP不保证自行恢复。

### 10.1 产品实现定型

- 4096是有限shadow-repair备份额度，不是新业务等待ACK的发送窗口。先保留4096及现有3秒repair horizon；本轮不扩容、不新增调参档位。满窗、过期、查无备份均可放弃未来业务record修复；已经发出的包没有因此被撤回。不能把Abandoned当业务丢失。
- 以有界槽位/队列、Seq索引、增量ACK/SACK实现直接查找与淘汰，正常插入、查找、满窗淘汰应摊销O(1)，不能为了寻找最佳淘汰对象逐新包扫4096项。保留现有有用的增量索引，不要求整套重写。可直接淘汰最旧可淘汰业务备份；保留已退休优先时必须用独立索引。清理按到期顺序有预算推进，重传扫描有界，停流后也回收。
- 首要审计点：runtimeowner/recovery.go的evictRepairForFreshLocked先遍历全部pending寻找retired/sacked，再淘汰旧record。无损高RTT、无SACK且窗口满时可能反复白扫全窗。路径存在不等于已证明本轮雪崩根因，需计数scan steps/eviction、锁等待和耗时验证。
- fresh业务优先；缺少备份只跳过本次修复，不等待、不报成业务发送失败、不触发重连。ACK继续反映接收侧既有策略，sender不能因淘汰就假装收到了peer ACK。保留有限repair credit、RTT/RTO和gap forgiveness，不新增强可靠模式或无证据loss阈值切换。
- 备份同时有条数、字节和寿命边界。所有槽位复用须核对Seq/记录身份/generation；正在Emit的不可变密文引用不能被覆盖或回收复用。保护FIN、握手和关闭控制，不把业务record的任意退役规则套到bootstrap/PeerFIN生命周期。全槽位受保护时也必须有明确有界处理，不引入稳态ACK等待；实现必须用竞态测试证明。
- no-HOL继续覆盖record首次交付、systematic FEC、跨lane和迟到首次到达；分片只等待自身完整性。保持同Seq同密文、nonce、MTU、鉴权/隔离、所有FEC档位、padding配置及黑洞恢复。

### 10.2 必须证明“满了也不崩”

增加定向core测试：无损高RTT但备份持续超限；长时间ACK缺失但fresh持续；全部无SACK时连续淘汰；过期/不存在的补包查询；Seq回绕/槽位复用；修复Emit与ACK/SACK/Close并发；停流清理；FIN保护。用操作计数证明每次淘汰无全窗扫描，不用脆弱墙钟单测当性能证据。

Actions先单条Normal10M无损，再单条Game4逻辑3M无损；随后按独立run验证5→20→5、5→30→5、短突发和共享黑洞。随机loss门槛见第4节；100%黑洞不要求期间吞吐或零损失，恢复后不得等待积压重传队列才能发新数据，连接重建仍按现有有界退避期限验收。报告成功交付延迟、超时/损失、最大无交付间隔、恢复耗时，不能靠只统计幸存小样本隐藏卡死。

增加fresh发送失败/等待、备份淘汰、查无备份、真实重传、scan steps、队列年龄、CPU/输入MiB与CPU/有效MiB、本机drop、FEC恢复和线上放大账本。窗口长期满本身不判失败；满窗导致CPU突增、fresh停顿、无HOL退化、延迟或本机drop失控才是缺陷。输入、正确性、抓包、资源门槛不放宽。历史FAIL按原规范保留；新规范命名loss-tolerant-v1，不能追溯改绿旧样本，也不能只改validator宣称产品优化。

### 10.3 每个Action run只允许一条性能测试

适用于吞吐、容量、弱网、校准、微基准及soak。一个run只能有一个SOURCE_SHA、一个配置、一个seed、一个场景的一份测量负载；场景内预先规定的30/60/30损伤阶段属于同一条。禁止同run矩阵扇出、顺序多案例、A/B或B/A；重复、前后版本、另一模式和校准分别新建run。允许必要构建/准备/清理/上传job，禁止夹带其他性能负载。汇总run只读已有产物，不启动测量。

新agent先改测试入口为显式单样本workflow_dispatch，并加输入/执行记录校验及意外第二样本拒绝；现有next-performance-ab-game等多样本入口停用或改造，不能直接沿用。普通unit/race的多用例不受此限制，但应独立于性能测量运行。改前后只能作独立runner重复比较，报告异质性，不能为了同runner控制重新塞两条进一个run。

正式 final18 的 artifact-only 汇总必须同时验证两种模式 × 三场景 × seeds 101/202/303，全18条、同一产品SOURCE_SHA、Normal每向10Mbps/1 lane、Game每向3Mbps/4 lanes。每条（包括lossless基线）五分类均PASS，缺组、缺重复、重复身份、混源码、错误配置均不得通过。逐seed逐模式配对独立lossless基线，pre/stress/post全部满足原p95增量200ms/p99增量500ms；缺失、非有限或没有成功探针的分位数不能作为有效基线。正式汇总使用 `--source-sha SHA --seeds 101,202,303 --require-run-receipts`，每个summary旁保存GitHub API原始 `run-receipt.json`，验证唯一run ID、workflow_dispatch、attempt=1、completed/success。诊断canary与正式新矩阵用不同冻结ref，历史失败不得被重试或新样本覆盖。控制器只分发和读取产物，不运行性能负载。

### 10.4 发送端放弃与接收端缺口退役必须一起完成

当前已有first-arrival no-HOL和forgiveGapLocked：接收端不会为外层缺包扣住后续完整业务，但仍保留ACK/SACK缺口元数据；压力阈值或repair horizon可放弃缺口。当前forgiveGapLocked每次遍历received map选候选，包括tick未到期时的无效扫描；发送端淘汰优化不能遗漏这一侧。两侧扫描对实测CPU的贡献均待验证。

设计采用“双方独立、有界、允许不同时结束修复”，不增加逐包ABANDON/NACK、新record类型或可靠控制事务。接收端没有sender缓存状态，不能声称知道某包永远不会到。业务交付、接收覆盖记账、缺口放弃游标分别理解：有限forgiveness推进的是TCP-like展示进度，不是证明缺失字节真的收到，也不能伪造SACK已收范围。

1. 后续有效完整record立即进入原有解密/FEC/LINK交付，和ACK缺口寿命无关。LINK分片仍只等待自身重组；未知丢失记录可能影响的FEC/LINK状态继续按各自原期限清理，不能因跳Seq删除无关block或datagram。
2. 有后续接收证据才建立缺口。用最早有效后继的首次观察时间确定绝对截止点，最长沿用现有3秒repair horizon；后续包、重复包、重复ACK不得延期。同一阻塞区间拆分/游标推进时，剩余缺口继承可证明的最早后继证据年龄，禁止每跳一个洞再等完整3秒。此为接收侧本地期限，不声称与发送时钟严格同步。
3. 低压力保留既有修复机会，压力下沿用现有soft/emergency依据提前放弃。到期或压力退役时只跳到已观测覆盖范围，增量吞并连续覆盖；不得跳到猜测的sender sendNext，尤其不能凭空生成FIN。接收端完全没看见后续数据的尾部丢包不可推断，通过既有keepalive/dead-after/恢复逻辑处理。
4. 用有界的有序覆盖索引和到期索引代替每tick/每包全map扫描；可采用有界小顶堆等O(log N)结构，不为强求O(1)制造复杂协议。合并相邻范围但保留期限需要的最早证据；稀疏最坏情况也要界定节点数、字节和清理预算。到期前能直接判断无需扫描；每次最多处理固定预算，未完成工作继续排程而非等待新业务。硬容量到来时必须先同步腾出必要元数据，不能让预算失效造成越界。
5. 进度变化后发出更新ACK，使对端增量回收；若ACK丢失，后续正常ACK继续携带当前进度，停流则sender自身到期释放。不要新增可靠放弃通知。首个缺口、必要SACK变化、FIN/控制等ACK语义保留；如合并冗余ACK，另作有界定向修改，不能为攒ACK延迟业务或形成ACK风暴。本轮优先索引/期限修复，不顺手重做ACK协议。
6. 被forgive的Seq后来首次到达，仍经原鉴权/去重路径尽力交付，不能仅因seq<recvNext丢弃；历史重复由现有有界record/Game去重继续处理，保持其既有保证，不宣称无限历史exactly-once。FIN不能按普通业务范围合并丢失控制语义；保留有序PeerFIN发布及已验证服务端休眠跟随规则。

新增联合定向测试：sender先淘汰、receiver有后继仍立即交付；receiver先forgive后sender迟到repair；连续深洞的期限不重置；缺口在截止前补齐；ACK丢失与停流双端最终回收；纯尾部丢失不伪造ACK/FIN；稀疏/回绕/压力淘汰；FIN与Close/Emit并发。记录gap count/oldest age、timer wakeups、scan steps、forgive原因、ACK量、fresh延迟及锁耗时。验证发送/接收两侧资源有界且持续损失不制造CPU或控制包正反馈，全部性能测量仍一个Action run一条。


### 10.5 受控损伤与真实WAN的验收边界（2026-10-07）

本节澄清结果解释，不修改 loss-tolerant-v1 分析器或历史 PASS/FAIL，不设置新的“原生少于0.1%即可通过”等门槛。当前资格及精确源码以 STATUS.current_qualification 为准。

| 环境 | 质量判定 | 必须同时报告 |
|---|---|---|
| Actions受控无损 | 业务与探针损失0，目标速率及原资源/完整性门全部满足 | 输入实效、socket/NIC drop、延迟与资源 |
| Actions受控5205/5305 | 现有逐阶段业务packet loss≤配置损伤p；delay-aligned wall goodput≥目标×(1−p/100)×99%；配对独立同seed无损p95增量≤200ms/p99≤500ms | pre/stress/post各方向、实际注入、恢复时间、队列及所有探针超时；不是要求FEC补齐每包 |
| 原生未人为损伤WAN | 继续保留原始零损失参考门及其FAIL；不能将“未加netem”当已知0%底层loss，也不能据少量loss直接断言协议错误或整版不可用 | 功能/完整性硬门、实测性能质量、本机receive pressure、线路因素及未定位部分分别记录；在归因或预先定义新资格前保持PARTIAL |

共同硬门仍包括源码/配套版本和实际配置一致、输入有效、payload/record完整性、账户与lease隔离、no-HOL、有界资源与owned退出清理。本机socket/driver/queue overflow单列接收压力，即使FEC救回全部业务也不能删除告警。输入无效也不能掩盖同时发现的完整性错误。吞吐接近目标不抵消p99、探针超时或长停顿。

FEC20:20完整块的理论范围是40个合法shard中至少20个及时可用即可恢复源数据。实际还有partial flush、大小组、有限恢复槽和绝对3秒期限；“平均30% loss”不保证每块都丢30%，突发/相关丢包或parity晚于退役时可能不可恢复。本机入口drop也可能叠加。ExpiredMissingSources、重构次数、raw drop与最终业务missing具有不同单位和交叉路径，不能一一抵扣或相加。无法恢复的洞仍有界退役，由内层业务自行恢复；不得因此加外层无限重传、全局等待或盲目扩大FEC/shadow/buffer。

RTT分位数是**已返回探针的条件分位数**。每阶段必须列sent/received/timeout及连续超时、最大无交付间隔。若超过1%的探针未返回，不能用幸存探针p99描述全部请求的尾部；未返回项只知道超过观察期限，不任意填成一个精确RTT。当前aggregate的probe_valid仅验证有限分位数及0<received≤sent，弱网未设最小coverage门；因此历史配对PASS不等于无超时或强尾延迟保证。当前d6八条定向样本各阶段30/60/30全回，未受此问题影响；120s每阶段仅30/60点仍是有限样本，不能冒充长期精确p99。后续扩展长时探针/覆盖约束需事先定义新测试规范、独立Actions验证，不能追溯改绿旧结果。

原生ARM观察到packet socket有效接收预算212992B（208KiB），当前Actions为1048576B（1MiB）；raw_linux.go未显式设置SO_RCVBUF，平台默认值和调度不同。共享ARM两核心、Windows vmxnet3和hosted runner属于不同容量环境。hosted drop0不证明原生drop0，较小预算也不证明扩大缓存能解决处理瓶颈；先记录最早拥塞位置、read-service/队列驻留和压力时间线。晚高峰WAN是可能因素，须与本机drop分别保留；跨时间/runner的CPU与p99差值是观测，不作为单项优化的固定收益。


### 10.6 各FEC档位的能力口径与检测标准（2026-10-07用户补充）

用户当前优先级：持续业务性能、低延迟、无HOL、突发后继续前进、状态有界。FEC只尽力恢复；不为低档位强制零丢包增加修复强度、等待、缓存或动态改档。本节是后续profile专项的设计标准，当前正式20:20 loss-tolerant-v1门和所有历史结果不改。理论依据为[RFC5510的RS擦除恢复性质](https://www.rfc-editor.org/rfc/rfc5510.html)，项目实际行为以本分支源码为准；本项目并未宣称实现RFC5510 wire格式。

**先看每个实际编码块，不能把名义档位当全局丢包阈值。** 完整20:R块最多弥补R个缺失shard，即该块20+R个shard中丢失不超过R且所需合法shard在有效恢复期间到达。R/(20+R)是该块的数量上限，不是“这条线路低于此百分比永远零loss”。随机有限块有尾部概率；相关/连续丢包更不能按平均比例推断。

项目partial flush的实际规则是k=DataCount、r=min(k,R)，未发的20−k个源槽是已知零，不计网络丢包。因此20:4的k=3 partial块实际是3+3，不是3+0.6或20+4；名义低档位在稀疏负载下冗余比例可能更高。20:20按256/512/SourceMTU三个lane-local大小组分别flush，低档位仍单组。按配置总包数乘R/20推算冗余成本或恢复极限会失真。记录实际k/r、source/parity包数和字节、flush/partial大小分布，header声明的profile也不等于实际发出的parity数。

下表是**完整块、每个shard独立以p概率丢失、所有幸存shard及时到达、无限恢复机会、不计外层repair/Game**时的源shard残余损失期望，作为解释参考，绝不是业务包丢失硬门或生产承诺。单位均为百分比；很小值用“<0.000001”而非假装严格0。

| 档位 | 完整块最多丢shard | 完整块擦除比例上限 | p=5%残余源loss | p=20%残余源loss | p=30%残余源loss |
|---|---:|---:|---:|---:|---:|
| off | 0 | 0% | 5% | 20% | 30% |
| 20:4 | 4/24 | 16.67% | 0.129073% | 14.069372% | 28.384685% |
| 20:8 | 8/28 | 28.57% | 0.000184% | 3.111194% | 17.661156% |
| 20:10 | 10/30 | 33.33% | 0.000004% | 0.985270% | 10.920122% |
| 20:12 | 12/32 | 37.50% | <0.000001% | 0.254355% | 5.772424% |
| 20:16 | 16/36 | 44.44% | <0.000001% | 0.010496% | 1.076467% |
| 20:20 | 20/40 | 50.00% | <0.000001% | 0.000267% | 0.130108% |

推导：指定源shard最终缺失当且仅当自身丢失且其余k+r−1个shard至少再丢r个，故理想残余q=p×Pr[Binomial(k+r−1,p)≥r]。整块超过擦除预算的概率Pr[Binomial(k+r,p)>r]不是源loss百分比，不能混用。实际partial要逐k加权；例如k=r=1、p=30%时q=p²=9%，明显不同于完整20:20的0.130108%。计算全精度和假设保存在evidence/fec-theory-reference-20261007.json，该文件是解析参考，非测试PASS。

**检测分三栏，恢复率不能替代性能/无HOL。**

1. 正确性硬门：所有档位均无bad payload/重复交付/错误lease/nonce或wire错误；仍systematic首次到达立即交付。编码器/decoder定向可控测试内，final metadata有效、恢复资源在测试声明范围且擦除≤实际r必须逐字节恢复；超过r不要求补齐，但已到systematic和后续其他完整datagram必须继续交付。刻意超恢复槽/超3秒的live case按既定退役策略验证，不能把数学足够但已按原策略退役误当codec bug，也不能借此掩盖性能压力。
2. 性能与延迟硬门：off及每档位各自同配置无损独立基线仍业务/probe loss0、目标输入速率保持、goodput≥99%、input有效；有损持续输入不能降速凑绿。正式20:20沿用原loss≤p/goodput≥目标×(1−p)×99%、配对RTT200/500ms、post5恢复原门。其他档位新专项事前声明基于实际shard/业务fragment的参考有效交付预算；不得机械照20:20近零loss，也不得把源shard理论q直接套到多片业务包。先有合法oracle再关闭恢复质量资格，缺oracle记NOT_EVALUATED，不能补一个任意“q+2%”宽限。所有profile保留有限修复/有限FEC/no-HOL、本机drop/queue age、CPU/inputMiB与CPU/deliveredMiB、线上放大和停流收敛门；弱档位超过恢复能力时允许缺失，但不允许fresh停发、长停顿、无限恢复库存或queuebloat。RTT继续覆盖全部探针超时，幸存p99不足以证明无卡顿。
3. 恢复效率和归因：用测试端有界离线事件账本按lane/generation/BlockID/ShardIndex、真实k/r、source/parity发送与首次有效接收时间分析。离线oracle分别列“理想及时可恢复”“原3秒/pressure-retire策略范围内可恢复”“实际恢复”“业务datagram全部fragment满足期限”，不在产品热路径加逐包日志、跨lane同步或新控制协议。未发parity、网络drop、raw/driver overflow、deadline/pressure退役、最后业务missing分开；完全未到接收端的block需发送端账本，不能从decoder统计消失掉。超过能力项不能判codec FAIL；范围内可恢复却未恢复、收到的systematic丢失、未解释的额外交付缺失则定位/FAIL，不一概叫线路差。

Game的同一业务PacketID在多个lane首次有效到达才算成功，各lane单独FEC，统计逻辑业务一次。只有独立丢失且时限/复制路径独立时才能乘各lane失败概率；共享队列、共同黑洞、相关WAN损伤下不能套q^4或保证至少一lane活。业务多片在off且独立丢失时是1−(1−p)^m；FEC块内恢复相关、跨块分片和Game情况下按实际映射算oracle，不直接套该式。字节goodput按每包真实长度，不将packet loss百分比当byte loss。

**实施次序与成本边界。** 当前next-strict-weaknet固定20:20，尚无其他profile的5305资格，现有lower-profile unit/native连通结果不能填补。先在测试框架增加显式fec_parity并贯穿check-config/实际生效、summary identity/receipts/aggregator；保留正式18的20:20入口默认与原判定不变，另建命名清楚的profile专项schema。编解码器定向门可以unit/race批量用例；真实性能每Action只能一个档位/场景/seed，配对无损与重复分别独立run。优先off、20:4超预算20%和20:10接近预算30%的代表场景，各两seed；复用当前20:20的受控5305作为不同scope参考，不伪称新profile源码已测。再补20:8/12/16边界，不开展挡位选型赛或自动调参。oracle重放/理论计算是只读分析，测量期间的探针/观测负担单列，诊断样本不替代默认off性能样本。没有产品新缺陷证据先补检测口径，不因此重构FEC、4096或提高冗余。

2026-10-07用户追加快速筛查：本轮只新增profile-screen-v1测试入口，固定产品d6，6profiles（off/4/8/10/12/16）×Normal/Game×lossless/5205/5305共36独立Actions；不是参数选型赛，也不替代正式20:20 final18或精确有限策略oracle。原分类失败全部保留，任何与理论比较都区分source与业务分片、partial、Game与policy；看持续性能/条件延迟+超时/资源和定向noHOL后再冻结实现，只有明确异常才继续改代码。screen原始pcap分析后hash回执并删除，不上传原始payload。


### 10.7 其他FEC档位快速筛查收口（2026-10-07）

产品固定d6cb6cee4c241aac8dd2f542a876edc57bf3d7db，harness49eba9ba91985bbf771601788fd6d4348d950ef9；off/4/8/10/12/16 × Normal10M/Game4逻辑3M × lossless/5205/5305，共36独立120s Actions，每run一条。12无损样本均业务/probe零loss并达目标；36样本input/capture/integrity/environment、实际两端profile通过，socketdrop0、无完整1秒零业务桶。32样本分析PASS、4原性能reference FAIL；workflow31success/5failure，其中off无损多一个批量路径断言误判，原失败不改。144配对RTT只有133PASS/11FAIL，不能称全批资格通过。证据[evidence/fec-profile-screen-d6-49eba9b-20261007.json](evidence/fec-profile-screen-d6-49eba9b-20261007.json)及压缩完整方向/阶段/原错误/探针覆盖回执。

Normal压力阶段goodput（上/下Mbps）：

| 档位 | 20%实测 | 30%实测 | 30%完整块+同块两片参考 |
|---|---:|---:|---:|
| off | 6.795 / 8.316 | 5.380 / 7.186 | 两方向分片不同；不套统一30%业务loss |
| 20:4 | 7.738 / 7.770 | 5.671 / 5.662 | 5.616 |
| 20:8 | 9.520 / 9.521 | 7.338 / 7.331 | 7.339 |
| 20:10 | 9.854 / 9.850 | 8.415 / 8.410 | 8.382 |
| 20:12 | 9.973 / 9.975 | 9.166 / 9.184 | 9.159 |
| 20:16 | 10.000 / 9.999 | 9.871 / 9.878 | 9.848 |

这是恢复曲线的解释参考，不是新硬门。MTU1400、方向record limits1250/1300、固定TLS-like31B/FEC56B：FEC-on LINK预算1163/1213，最大原始IPv4UDP包1228B需要两片。等包数64/256/1200业务不能把source loss直接当业务packet或byte loss。完整块内m个关键源片的业务失败概率为Pr[Bin(n,p)>r]−(1−p)^m Pr[Bin(n−m,p)>r]；m=2同块、m=1其余两种包、按320/1200字节加权得到上表。跨块/partial/repair改变数值，完整实际映射未重放，精确恢复质量oracle仍NOT_EVALUATED。off C2S LINK1219而S2C1269使1228B大包仅上行分片，方向差异并非自动说明丢数据bug。

**决定：冻结现有FEC实现，性能资格保留例外。** 不修改codec/wire/各档位、partial/size class/3s退役、不继续扩大FEC/shadow/buffer；恢复吞吐未见需要新修复的反常曲线。Game4在30%下off约2.83M、20:4约2.87M、20:8约2.98M、20:10/12/16接近3M；它包含4份业务复制，不能与Normal当同等线上成本或宣称共享黑洞也有q^4收益。

Normal off的5205/5305与20:4/8/12的5305保留配对p95/p99超门：条件尾延迟约2.17–3.04s，含600ms基线。弱档位缺包可解释，晚到延迟仍是真实缺陷/能力边界，不因恢复理论吻合改绿。5条尾部诊断均fresh blocked/emit失败0，off/4/8业务每10ms桶均非零，说明多秒迟到并未把整条业务卡住；20:12却有270/290ms无业务桶，要单列调度/队列排查，不断言完全无停顿。late-first-arrival和repair计数支持有限补包迟到假设，无record→probe映射，因果未关闭。所有未返回探针必须保留，低档位幸存p99不是所有请求p99。

Normal无损线上IP字节/业务分别off约1.32–1.37、20:4约2.22–2.25、8约2.91–2.96、10约3.26–3.33、12约3.61–3.68、16约4.30–4.40。低档位仍单size组，padding-off不等于parity无补齐成本；不把高档位成本说成配置无效。Game4复制和头部进一步放大。跨独立runner CPU不能当不同档位固定CPU收益；本轮资源门通过/drop0，但不代表原生Windows→ARM容量或全局最优。

off无损原run37574266150数据面/分析PASS，额外receipt要求send_multi>0导致workflow FAIL。FEC-off即时逐包发送可能合法不形成multi，不能为了触发测试而攒包增加等待。仅显式profile_screen=true/fec=off/parity0的send允许single并标MULTI_NOT_EXERCISED；receive原要求、所有fallback/空流量拒绝、正式20/on原门保留，必须在Actions验证修正及独立off基线，原run不改绿。后续原子任务只调查transport迟到/调度，有新证据才改；不重开FEC选型赛、不把screen当full70/final18/长测/P7完成。

收口补充：测试harness48d9cf2的preflight [37575761975](https://github.com/lly8666/wobuzhidao/actions/runs/37575761975) PASS（7profile+4raw检查），exactd6产品独立off/Normal/lossless/seed1500 [37575781199](https://github.com/lly8666/wobuzhidao/actions/runs/37575781199) PASS。双向各阶段约10M、全业务/probe零loss、p99最高601.246ms/socketdrop0；server仍send_multi0、fallback0并如实标single-only未触发multi。仅修harness断言，未改变即时发送或FEC策略，旧run37574266150不改绿。37独立performance Actions含36原screen+1复验，11配对RTT失败仍原样保留。FEC实现冻结，所有profile全项性能不标COMPLETE；单位trace oracle/full70/final18/1800s/原生剩余不继承。


## 2026-10-09 新增FEC开关成本/恢复实验（PLANNED_NOT_RUN）

用户最新授权本项一个Action单job顺序跑不同业务/off与20:20；本项覆盖旧单样本限制，禁止并行负载，原正式资格规则和门槛保持。详见[FEC_POLICY_EXPERIMENT](FEC_POLICY_EXPERIMENT.md)、[夹具功能与使用](REALPATH_TEST_FIXTURE_GUIDE.md)、[接手模板](templates/FEC_POLICY_AGENT_PROMPT.md)。实际夹具当前300s/300ms/不含1%等约束必须先适配并验真，不直接声称已有serial支持。实验分支独立，产品恢复政策/默认不改，首批结果与CPU收益均NOT_RUN，现有STATUS和历史失败保留。
