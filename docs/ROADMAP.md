# 唯一开发路线图

2026-10-06当前：0246526 Linux内层TUN9000/外层预算不变已过11正确性门（实际kernel+journal9000、invalidouter拒绝、37生命周期）及五独立严格性能/18RTT对/P6，同源Windows/ARM部署；Normal最低9.99926M/Game2.99955M，max p99增量11.21ms。M03短预检1437运行，原生收益未验；852最大UDP3late/p991404ms仍保留。38历史完整300s/16工况/27NOT_RUN跨源码不继承；每性能Action一条，先STATUS/latest_log。

2026-10-06当前：配套产品9211b24入口过滤已过core/race/真kernel/native12/服务化12/37生命周期、五独立性能18RTT对和P6；实机M03两次1415 PASS/1416 FAIL，最大UDP一missing一1.308s late，1352小包及时，rawsocketdrop0/过滤443实际生效。32完整native/12工况/31NOT_RUN跨源码不继承。先STATUS/latest_log验有界探针phase timing助手，再诊断回程碎片并D04；不将健康一条关闭M03、不扩缓存/FEC/4096/HOL。

历史3e3e094阶段入口（已由上段当前状态覆盖）：当时配套包已过Actions定向门并实机D01完整300s，长时间双向中断未再现，但下行8.90%字节损失、208探针超时仍FAIL。当时下一项S16/Windows收包诊断不作为本轮任务；最新下一步以STATUS为准。旧失败不继承为新SOURCE资格。

2026-10-05最新原生五分钟进展：固定6181db6未改产品；S01 Normal10与S02 Game4×3达到目标附近但C2S缺74/10包，不能写无损PASS；M01外层MTU1400大包至9000B无坏数据但有1次迟到。D01默认NRPT+10M出现约90秒双向中断、约30.12%业务loss和8/60 DNS失败，明确FAIL；整体P7仍PARTIAL。优先按STATUS.physical_5min复现D01并异常触发抓包定位最早边界，不直接归因DNS/VM或扩大buffer/FEC/4096。完整DNS互备、LAN/CN/IP/IPv6、其他配置/生命周期/弱网未跑。方案PHYSICAL_5MIN_ACCEPTANCE.md、日志devlog/20261005-021317-five-minute-native-capture-matrix.md、evidence/physical-5min-6181db6-20261005.json及压缩原始计数为当前证据入口；每性能Action只一条。原始pcap已删、退出owned清理通过，服务端保留active。

2026-10-05用户授权实机测试已推进：固定6181db6真实CLI/Npcap/Wintun→ARM WAN DNS/UDP/TCP/验证证书HTTPS、Normal1双向10M与Game4双向3M及退出清理通过对应门。首轮不完整异常和fresh Normal raw接收drop+125仍待定位；GUI实际操作、人为弱网、长测未验，P7仅PARTIAL，不能声明RELEASE_QUALIFIED。见STATUS.physical_native与最新日志。部署仍只上传包、解压配置和启动，不扩建安装管理/在线升级；下一任务按STATUS，不自动恢复Python安装器或旧性能重构。

> 当前决策（覆盖下文历史下一步）：2026-09-23用户最新决策：允许链路30%丢包时仍有至多30%业务包损失，优先处理性能、低延迟、无HOL与突发稳定性；不得主动丢业务凑指标。4096为可放弃的shadow-repair备份，不是fresh发送门。当前执行WEAKNET_QUALIFICATION第10节；历史近零损失门槛不再约束有损场景，无损满速、完整性、隔离和资源有界仍是硬门。 所有性能测试严格一个Action run一条样本，禁止同run A/B与matrix。

完成条件取 ACCEPTANCE；当前执行位置只看 STATUS.json。表中优先级和协议已决定，不再开选型阶段。

2026-10-04最新用户Linux部署、多客户端自动7天内存IP和Windows配套任务已交付：固定SOURCE6181db6、预发布linux-server-rc-20261004-6181db6，scope为core/native/部署/GUI/36生命周期/定向5205。当前下一原子任务见STATUS顶层；最新全70/严格18/1800s和P7尚未验，不能继承旧完整基线，也不因此回到无证据架构调参。

| 阶段 | 工作 | 退出条件 |
|---|---|---|
| P0 | 建新分支、隔离 old、章程/规范/交接、Actions 基础入口 | 仓库契约检查通过；明确产品尚未实现 |
| P1 | 建根 Go module；实现 tlsrecord keys/seal/open/parser/近期去重和固定向量 | Linux/Windows 基础测试、Linux race、fuzz；全部在 Actions |
| P2 | 重开：保留已通过的核心；补握手重传、FIN/RST/半关闭、bootstrap 窗口、TLS 外观与票据/移交边界 | 核心回归及普通内核 TCP 客户端经真实入口完成 TLS/HTTP 回落与连续抓包均通过 Actions；内层业务指纹不属 P2 门槛 |
| P3 | 保留已通过的 LINK/FEC/统一MTU，继续 session/owner；增加默认关闭的显式有界 record padding 能力 | 一次分片、全固定 FEC、完整性/no-HOL；padding 向量/零等待/MTU余量/重传一致性通过；不宣称抗识别 |
| P4 | Tunnel/Game/lifecycle 和平台入口；多业务复用现有 lane、可选 padding 配置与预算 | 保持1..4 lanes/10物理上限/rotation/DORMANT；默认off，不逐业务重建lane，不凑包或改竞速策略 |
| P5 | 新版本 fullstack 弱网/抓包/负载/长测；真实 HTTPS 内层业务外观验收 | 同 SHA 完整测试；长度/方向/突发/时序与成本分开报告，不做旧项目 A/B，不承诺不可识别 |
| P6 | 发布候选打包与全平台 hosted 收口 | 可下载同源码包、哈希、manifest；已知问题与能力缺失透明 |
| P7 | 最终物理 Windows/Npcap -> Linux ARM64/amd64 验收 | 用户安排物理机后实测；满足退出清理与业务路径才 RELEASE_QUALIFIED |

每阶段可分小任务，但不能越过未满足的功能依赖。P1 的 PASS 不表示 P2 握手或 P5 端到端通过。若审计发现已关闭阶段仍有入口/生命周期硬缺口，STATUS 必须回退到该阶段收口，不能以“已进入下一阶段”为由跳过。平台能力缺失须明确记 UNSUPPORTED，不改代码假装实现。

2026-10-04用户当前任务优先于历史P7待办：P4/P5补默认DNS双解析器互备、IPv6捕获后丢弃，以及profile定位的Game历史淘汰开销。先验默认/自定义/关闭DNS、UDP/TCP故障切换、IPv6无泄漏、owned清理与core/race；再独立Normal/Game5205、生命周期和新包。结果归属STATUS的精确SOURCE；旧完整矩阵/长测与真实Windows驱动仍单列，不能继承。

每个任务的默认输出：代码/文档、针对性测试、一次新的开发日志、STATUS 更新、必要的 REUSE_LEDGER 记录。优先完成一个可审阅范围，不跨阶段顺手改 FEC 参数或产品主旨。

2026-09-20 用户授权：P2 重开期间允许已在开发的 P3 独立模块继续，但 P2 仍是最早未关闭门。faketcp/realityfront 由 P2 收口；tlsrecord/pathmtu/datapath 由 P3 接手。共享文档/STATUS 更新前同步最新提交，只合并本任务范围；保留两条工作流证据。P2 真实网络资格只提取最小必要 I/O，不扩大成完整 P5；物理 Npcap 仍在 P7。

2026-09-21 最新任务覆盖上述历史收口：P4重新打开稳态恢复/关闭/外观一致性，P5重新打开真实路径持续弱网性能，执行 [专项规范](WEAKNET_QUALIFICATION.md)。保留P2/P3与旧P4/P5/P6证据，不重做无关模块。Normal 10Mbps/方向、Game4 3Mbps/方向，FEC20:20，18份主测与目标负载长测/平台覆盖完成后，修复版重新P6打包，再进入P7。

2026-09-22 用户插入任务：P4/P5有界TLS启动填充，实施/关闭清单见TLS_STARTUP_PADDING.md。原主线任务暂时hold；本功能交新agent测试修复，完成后直接更新STATUS的小功能进度，不关闭P4/P5整阶段资格。


### 2026-09-23 插入的用户优先任务
WEAKNET_LIFECYCLE：功能 COMPLETE（最终源码0b206a0，36/36真实进程样本通过）；目标速率性能仍 FAIL_CAPACITY_LIMITED。2026-09-23 用户解除性能 HOLD，当前主线 PERFORMANCE_RECOVERY 按 WEAKNET_QUALIFICATION 第9节先修接收停顿，再审计线上放大，定向通过后跑严格矩阵与长测。P4/P5整体及容量缺口仍待完成。


## 2026-10-03 顺序资源优化定向收口

当前产品资格SOURCE_SHA ca8175d18acd7f7e3e1db5379a58d9f85417dd97。5个资源原子步骤+1个接入队列故障隔离修复，逐项Actions unit/build/race后分别独立Normal/Game5205；12条合格样本与逐阶段RTT/loss门PASS，最新36/36生命周期及aggregate PASS。见STATUS、最新日志及evidence/resource-optimization-5205.json，历史native接线失败/队列致退出/瞬态超时保留。优化实际触发且正确性、吞吐、延迟过门，不据跨VM单样本宣称固定CPU下降；SACK主导的5205下ACK收益未证实。当前不再继续微改，下一步固定最新SOURCE_SHA全18复验，随后有界长测框架与>=1800s目标速率Normal/Game独立run；P5/P6/P7未整体关闭。每性能Action一条样本，FEC deadline8ms、4096/3s和socket buffer不调大。

2026-10-04当前覆盖历史下一步：b1完整原门通过后逐秒审计暴露Normal换代约61%loss；P5重开。50有限retiring接收候选基础/race和Game短测通过，Normal短测FAIL并出现内部server ready queue overflow。先补retiring/ACK/tick诊断、按证据窄修；新1s门与内部queue0drop门通过后，最新整份源码完整资格/P6才能重闭。P7未跑，旧包仅复现。实时任务见STATUS，禁止根据以上旧ca资格继续发布。

2026-10-04最新收口覆盖上一段待办：SOURCE2b2bd9eb106d7c6fa83096cd88a59a0d0bfae8f8 core/race、严格18与完整78原始回执全部PASS，Normal/Game各1800s原门及逐秒/internal queue0门PASS，三目标P6hash/manifest/独立receipt已核验。固定本候选可交用户P7物理验收；不是RELEASE_QUALIFIED。参数功能与性能分开：70配置证明生效，主高负载资格FEC20:20/padding off；TLS启动填充完整on/稀疏/双入口弱网专项仍PARTIAL_ACTIONS_PASS。不要恢复已修的换代错误、继续微调缓存或按历史HOLD停工；当前SOURCE、证据及下一项以STATUS/PREDELIVERY_ACCEPTANCE为准。

2026-10-04最新用户优先任务已交付：WINDOWS_GUI.md 中文原生GUI、服务器切换、完整配置和单目录应用便携包。SOURCE9857bdb的206项GUI检查与基础/网络专项PASS，固定GitHub预发布标签windows-gui-rc-20261004-9857bdb；STATUS.windows_gui是结果入口。Wintun系统驱动安装已获用户接受；实际Npcap/Wintun/NIC/UAC与跨服务器业务留P7，不能冒充hosted完成。新源码完整性能/弱网矩阵未重跑。
