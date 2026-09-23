# 弱网生命周期移植验收

本文件是 DEVELOPMENT_PLAN/ACCEPTANCE 的专项测试细则，不是第二套状态入口。唯一进度在 STATUS.workstreams.WEAKNET_LIFECYCLE。原主线 AF_PACKET 容量诊断按用户要求 HOLD；生命周期验收完成也不自动解除 HOLD。

## 已实现的边界，待 Actions 证明

- 复用旧 receive-rate/RTT 自适应 pressure forgiveness：100ms 速率采样、EWMA 1/8，软阈值 ceil(rate×RTT + max(128,10% rate×RTT))，3584 emergency；软阈值还需 gap 至少 1 RTT，冷启动无 RTT 不臆造小窗口。只释放 TCP 外观元数据，业务 first-arrival 仍即时交付。3s 恢复 horizon 与已有发送重传预算继续有效。
- 单 lane 40B TLS-like health record（含 31B record 固定开销），每端默认 15s 一条；不进 LINK/FEC、不填充、不补充 repair credit、不入 repair 队列；它占用正常 TCP seq，丢失不会阻塞后续独立 record。
- 新 PN 且认证成功的数据/health 才刷新链路健康；重复 TCP payload、旧 health hint、单纯 ACK 均不能将旧空闲信息当新证据。
- business activity 与 health 分离。client 本地提交需求在发送/唤醒前计入；有效下行也计入。auto-idle 使用二次活动快照校验，丢失保活时保持“未知”，转恢复而非误休眠。client 是主动唤醒/休眠发起侧；server 的 auto-idle 不能仅凭周期 idle health 先关闭，必须等当前 authoritative lanes 收到 client 的有序 FIN 承诺后再跟随休眠。休眠前仍有一次有限最终 idle hint；idle hint/FIN 都是尽力收敛而非可靠关闭事务。
- 超过 dead-after 选择异常 lane，最多一个候选同时建连。候选沿用真实 TLS/admission、新 incarnation 和 generation fence；失败保留旧 authoritative lane，1～30s 有界退避，不退出 CLI。已成功 admission/promote 后仍沿用旧的 bounded retiring grace；不能将其描述为“任意 promotion 后故障可回滚旧 generation”。新 lane 再失活由下一轮恢复处理。
- 只增加可观测字段，不删除既有 weaknet 成本、AF_PACKET、输入有效性指标。吞吐瓶颈尚未修复，不能将本工作标成容量问题完成。

## 第一关：源码与核心正确性

确认精确 SOURCE_SHA、V2 两端一致、Linux/Windows 编译、全部 unit、race、repository gate、参数目录 gate。`next-lifecycle.yml` 提供定向入口；foundation 全集也是门槛。固定 wire vectors 仍验证独立 record 格式，另外验证 V1 明确拒绝。

重点补足/重复：health malformed/重复/乱序 PN、假 TCP ACK、重放、FEC 全档 health 无放大、health 不创建 flow、不领 repair credit；gap pressure 前 1 RTT 不退役、无 RTT 冷启动、极端乱序/Seq wrap、3s 到期、late systematic 首次仍交付、same Seq same ciphertext。normal/Game 1～4、并发 idle/new demand、纯下行、所有 lane 同时失活、Close 并发；race 必须实际执行。

## 第二关：真实进程生命周期矩阵

复用当前 netns/veth/netem、混合业务生成器、pcap 与逐方向统计。每个 job 独占 runner 跑一个场景；重复样本用独立 job，可并行扇出。禁止一个 runner 同时压多条隧道样本。默认每场景两个 seed，失败保存原始产物并单场景修复重跑。

| 场景 | 故障/操作 | 验收重点 |
| --- | --- | --- |
| L0 配置 | CLI 与 JSON 等价；CLI 显式覆盖 true/false/duration；错误与平台专有键；padding off/on × FEC off/20 | 实际生效参数有不含凭据的 receipt；padding budget/旁路行为可证，不只看参数解析成功。 |
| L1 空闲/稀疏 | 双端 idle=30s、keepalive=5s、dead=45s；无业务、零星请求、纯下行分别跑 | 无业务能休眠且新上行业务唤醒；health 不保持业务活跃；纯下行业务持续时不休眠；flow/lease 不漂移。 |
| L2 单向业务全丢 | 持续本地业务，按方向 100% 丢；保活也丢 | 不把 ongoing offered demand 判 idle；无假 lease 回收、无 tight retry；双端各测；最终恢复。 |
| L3 保活容错 | 用测试层注入只丢 health（FEC 前/加密前的明确 hook，不能凭包长猜），连续 1/2/3 次；正常业务继续 | 不误休眠、不误终止主进程；健康业务 record 能维持 liveness。没有该 hook 时明确此项 NOT_RUN，不用总丢包替代。 |
| L4 双边黑洞 | 原 4-tuple 双向永久丢，候选新 tuple 可通；再做所有 tuple 暂时黑洞 | 永久旧 tuple 必须由新 TLS/generation 恢复；所有 tuple 临时黑洞允许清障后旧 authoritative lane 自然恢复，promotion 不是硬门。两者都必须在预算内恢复持续 30s 业务、相同 TunnelID/lease/规则、候选/retiring 有界；发生 generation replacement 时旧 generation 迟到不能污染新状态。 |
| L5 候选失败 | SYN、TLS、admission、detach 各阶段分别丢，最后恢复 | 候选超时独立；旧 lane 可用时继续服务；不退出、不积累候选/goroutine/端口；验证 backoff。 |
| L6 休眠竞态 | idle 截止附近持续交替新业务，1/4 lane，各 100 次；新业务触发唤醒时网络仍黑洞 | 不丢业务活动证据，不每 tick 重试；失败后可再次唤醒；内存、lane、flow 有界，无 deadlock。 |
| L7 生产默认计时 | keepalive=15s、dead=90s，先稳定 30s、双边黑洞 120s、恢复 120s | 至少一组用真实默认时长；短时间缩放样本不能替代这项。 |

恢复成功按“恢复后持续 30s 可交付且 bounded state 收敛”，不要求高丢包下首次换 lane 成功。给出首次怀疑/每次 admission/成功/业务恢复时间；恢复预算与 dead-after、候选绝对 timeout、backoff 相符，超预算必须 FAIL，不能只无限等到成功。

L6 的黑洞阶段只验证“业务活动证据不会被当成 idle、失败后仍可再次唤醒、重试有界”，不要求 UDP payload 在故障期间可交付。清障后旧端口可能出现故障期间已进入本地队列的迟到 UDP；这些必须单独计数且不得超过该阶段实际发送数，不能混入严格竞态结论。严格的 100 次 cutoff race 使用独立端口/seed，要求 clear-path 100/100 unique、corrupt=0、unexpected=0；不规定 100 个业务事件必须对应固定数量的 Dormant→Wake transition，只要求从明确的双端 DORMANT 前置状态至少成功唤醒一次，且之后不得出现 clear-path wake failure、重试风暴或资源泄漏。

## 第三关：弱网质量与额外开销

沿用正式目标：Normal 单 lane FEC20:20，10Mbps **每方向**；Game 四 lane FEC20:20，3Mbps **逻辑业务每方向**。均使用当前大中小混包，不用大包替换。300ms 单向、30/60/30s 的 5%→20%/30%→5%，各两个固定 seed。另补 FEC off 稀疏 TLS 链路 + padding off/on、FEC20:4/10 小矩阵验证配置连通与无 HOL；不能拿低档实验替代目标负载。

分开报告：实际 offered/unique goodput、按包/按字节 loss、排队延迟/RTT p95/p99、post5 排空与恢复、fresh/FEC/Game复制/repair/health/padding/握手 outer bytes。health 理论 fresh record 预算为每端每 active lane 40B/interval，额外 TCP/IP/链路/ACK 和建连流量独立统计；休眠最终提示每次每 lane 至多一条。既有 1/5 repair credit 是内部 wire accounting，不可冒充应用层总开销上限。

所有 host/网络边界同时记录：产品各进程 CPU、全机 busy/steal、softirq、GC/alloc/goroutine、AF_PACKET `ss -0`、UDP/raw-IP `ss`、qdisc/interface/capture drop、生成器 send lag/skipped、逐阶段注入有效性。性能差必须找最早异常边界；不能仅以总 CPU 没满、socket_drop=0 或相关性证明 runner 无瓶颈/有瓶颈。原 a045a28 诊断成果继续保留；禁止扩大 4096、FEC/全局 socket buffer 或恢复严格 ACK/HOL 来美化指标。

## 完成与回写规则

新 agent 可直接修复本次功能发现的问题，不需反复请示。每轮更新正式日志、参数目录/规范、STATUS，固定源码 SHA 重跑相关失败场景及受影响回归。完成时列 exact SHA、run/job/artifact、真实 PASS/FAIL/NOT_RUN、限制与资源证据。

仅当上述核心及生命周期必测项通过，才将 WEAKNET_LIFECYCLE 从 IMPLEMENTED_PENDING_ACTIONS 改 COMPLETE，并在 DEVELOPMENT_PLAN/ACCEPTANCE 回填证据；仍有未测项就保持未完成并精确列缺口。吞吐门槛未达时独立标 PERFORMANCE_FAIL，不把 correctness 通过包装成整体弱网资格。原主线 HOLD 保留，恢复开发需用户另行安排。


2026-09-23 核心证据：SOURCE_SHA `84c466f81860c3e87aac3b571a9bce419018aabc`；[next-lifecycle](https://github.com/lly8666/wobuzhidao/actions/runs/35788463576) 和 [foundation](https://github.com/lly8666/wobuzhidao/actions/runs/35788463670) PASS（编译、unit、race，定向race重复3次；foundation parser fuzz）；另 padding、steady-targeted、harness-preflight、realpath-calibration PASS。**完整生命周期真实故障矩阵/严格10M与3M目标弱网仍 NOT_RUN**，不得据此关闭专项或原容量缺口。详见最新开发日志及STATUS。
