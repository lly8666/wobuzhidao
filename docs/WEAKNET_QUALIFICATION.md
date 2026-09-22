# 稳态修复与真实路径弱网资格（2026-09-21）

本文是 DEVELOPMENT_PLAN / ACCEPTANCE 引用的专项执行规范，不是第二套状态或交接。用户新增要求使 P4 稳态传输与 P5 性能资格重新打开；历史通过证据保留，但不能覆盖新增门槛。生产代码修复后 P6 必须按最终同 SHA 重新打包，P7 仍 NOT_RUN。

## 1. 低开销修复顺序

先检查最新代码确认问题，不照历史摘要盲改。一次原子变更，Actions 定向回归后再进入下一项。

1. runtimeowner tick 中重传选择和 gap-forgiveness ACK 不得互相覆盖；选中、尝试发送、成功发送、失败分别计数，发送失败不可悄悄释放已占 Seq 的修复债务。补同时到期测试。
2. 将 P2 已验证的 FIN/RST/半关闭语义接到稳态 handoff 后的实际入口。使用稳态最新 Seq/ACK，内部 detach 不是外层关闭；客户端、服务端、rotation、DORMANT、人工退出均需覆盖。有界关闭仅影响退出lane，不等待缺包阻塞其他业务。
3. 保留 no-HOL，补成熟的有界选择性确认、fresh优先修复预算和必要的 RTT/RTO 估计。只在已协商 SACK 时发送，SACK真实报告已接收范围，业务仍首次到达立即交付。保持有限gap forgiveness、4096有效记录边界和late first-arrival。不能为了累计ACK完整恢复严格等待。
4. 避免每ACK全量扫描/压缩数千记录，使用有界增量索引；补深洞、回绕、历史淘汰、稀疏ACK和停流清理。不要以增加锁、后台线程或每包统计抵消收益。
5. 建连到稳态的window/scale、MSS、实际TCP options、persona保持一致；SACK改变头长必须进入单一MTU预算。当前未实现的timestamps不伪造，不顺手增加新选项。padding仍默认off，禁用假业务、凑包等待和随机延迟。

性能保护：修改前后只比较新版本的具体修复，同输入/同配置/相同资源，必要时同runner顺序AB/BA，每次仅一条负载。无损有效业务吞吐不得下降超过5%，单位有效MiB的进程CPU时间不得增加超过10%；超出需归因并修正，不靠跨VM平均掩盖。此为本轮工程目标，不是历史已达性能。不进行旧DTLS整体A/B。

### 1.1 高/低丢包恢复优先级

这些原则修正的是**外观与轮换验收口径**，不放宽第4节的业务损失、goodput、时延、post5或资源目标。

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

每配置三个场景：无损300ms单向控制、5%->20%->5%、5%->30%->5%。每场景120秒有效注入，30/60/30秒阶段；额外warmup/建连不计入，结束后固定10秒drain仅收尾计数，不延长goodput时间窗。每场景3个独立job/VM与固定不同seed，共18份主测样本。一个job一个配置/seed/场景；不同job可并行。方向独立损伤；四lane共享同一瓶颈qdisc、对包独立抽样，不能人为保证四副本中必有一份幸存。额外共享链路突发/黑洞专项见下节。

主测用双向持续UDP序号+长度+内容校验负载，经正式业务入口和正式client/server，不能用每3秒一次HTTPS请求代替10/3Mbps注入。固定等包数64/256/1200字节循环（包含测试序号和时间字段），按实际字节精确pacing，保存发生器配置；与旧混合模式不一致时明确标注，不称完全同口径。同步低速RTT探针另计开销、不计主业务输入。HTTPS/TCP保持独立真实业务专项，不用其重传掩盖UDP丢包。

## 4. 严格验收目标

以下是新增目标，不是由FEC数学保证的所有网络条件承诺。原始数据和分析器独立复算，逐方向/逐阶段/每次重复都报告；不能用全程均值覆盖高丢包阶段失败。

- 注入：每阶段actual send payload在目标99%..101%；发送失败0；skipped slots <=0.01%；p99调度迟到<=10ms；后段不持续积压。达不到标INVALID_INPUT，原结果保留并定位发生器/背压/环境，不能叫协议PASS。无效项仍继续检查数据损坏等硬错误。
- UDP有效唯一payload：无损损失0、goodput>=目标99%；20%阶段损失<=0.1%、goodput>=目标99%；30%阶段损失<=1%、goodput>=目标98%。损失按发送时间阶段分组，10秒drain后仍未收到才计最终丢失，同时报告每阶段墙钟实际交付速率及迟到量，避免迟到掩盖堵塞。
- 正确性：坏payload/错误用户或lane交付/应用重复/nonce重用/同Seq不同密文/意外IP分片均0；MTU/checksum与generation/source fencing保持正确。丢A交B与跨lane无HOL单独定向证明，不以低平均RTT代替。TCP乱序、duplicate ACK、同Seq同密文有限repair、decoder已见重复和有限gap forgiveness进入非门控transport-hygiene账本；它们若造成业务损失/时延/资源超门仍由对应硬门失败，但不因“外观不够严格TCP”单独把CORRECTNESS判FAIL。
- 时延：同场景无损300ms控制作为基线；损伤场景低速探针的p95 RTT增量<=200ms，p99增量<=500ms，探针超时独立计数（超时不得从分位数报告中隐去），探针损失<=1%。报告UDP单程p50/p95/p99，跨时钟测量说明同步方式，不能假设物理双机时钟相同。
- post5：降回5%后10秒内进入连续3个1秒窗口goodput>=目标98%、该窗口发送包最终损失<=0.1%；同时队列年龄回到初始5%稳定段p95+200ms以内。固定时间定义，不能沿用稀疏请求“第三次成功”口径。
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

平台：Linux真实root网络/raw/TUN与OpenWrt风格TPROXY在Actions尽量实跑；iptables/nft支持的后端分别资格。Windows Actions执行真实可用模块，无Wintun/Npcap驱动/管理员能力时明确UNSUPPORTED，mock/compile不能写physical PASS。ARM64交叉编译不能写运行通过；可用原生runner则原生验证，仿真标明。OpenWrt IPv6未实现时明确失败/不支持和旁路风险，不以IPv4通过承诺IPv6已代理。硬件相关能力仍留P7。

## 8. 交付与关闭

每轮详细devlog、唯一STATUS、实际源码/构建/harness/分析器SHA、完整配置/seed/runner、原始日志/pcap/计数/分析脚本与产物hash。同一最终SHA必须通过受影响核心Windows/Linux回归与Linux race、Game4间歇问题闭环、真实路径主测和新增模块矩阵，再标增强P4/P5 CLOSED；旧P6包历史有效但不覆盖修复版，重新打包后才交P7。

文档本身不能把新增目标标PASS。对失败先给根因证据、最小修复和回归，不无限试参数；主旨约束不因性能不够而降低。
