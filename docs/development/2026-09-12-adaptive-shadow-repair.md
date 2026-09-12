# 2026-09-12：FakeTCP 修复资源隔离与动态压力候选

状态：代码实验候选。物理弱网、完整 DTLS/FEC 业务验收交给测试负责人；不代表发布合格。

## 开发依据和范围

本次从 `eb3de969b045fab1e88034345bfbf0f641cfd592` 开始。它已包含晚到首包放行；`agent/dynhorizon-d3b` 在获取仓库时也指向它，并没有已经完成动态窗口。另一个测试分支 `agent/validate-singlelane-20m-300ms-20loss-d3b` 的工作流仍明确把产品源码固定为 `d3b54dced255cc9becb3e501991f71a034e7438f`，不能把测试分支的最新提交当作被测产品版本。

本次明确授权为：修代码，准备多组候选，由其他人做弱网和物理测试。旧 handoff 中 9 月 5 日的部署任务不是本次任务。主开发线、冻结候选、测试主机和工作流均未改动。所有实验均保持现有 FakeTCP 线格式、ACK/SACK 编码、LINK/FEC 参数、DTLS 防重放设置和 Game 语义。

## 评估：方向正确，原先的正确性承诺过强

1. **资源隔离只能保证本层 admission。** FakeTCP 修复 payload 上限为 4096；wolfSSL 编译配置又独立使用 `WOLFSSL_DTLS_WINDOW_WORDS=128`，即 4096 条防重放记录。载体分片、DTLS record 和 FEC symbol 不是通用的一比一关系。放行迟到 FakeTCP 首包，不证明后面的 DTLS、FEC、Game 仍接受它；也不消除 socket/CPU/qdisc 的丢包。
2. **接收速率不是发送速率。** 有损路径只能观察已到达记录。B/C 使用的是接收端到达速率压力估计，不宣称准确测得发送端 BDP，也不依赖不稳定的 loss 估计反推发送速率。RTT 来自同一 association 的本地 Sender.SRTT，不能把带退避的 RTO 当 RTT；双向负载速率分别估计。
3. **加权预算不能套用不加权的修复公式。** 对独立、同概率丢失的理想模型，不加权重传期望为 `p/(1-p)`。按 1/2/4/8 封顶计费，虚拟支出期望变成 `p + 2p² + 4p³ + 8p⁴/(1-p)`；15% 丢失对应约 21.33% 虚拟支出，已经超过 20%。因此不能承诺 15% loss 下仍完整修复。1–2% 低损必须另做反证测试。
4. **只限制 active payload 不等于内存有界。** SACK tombstone 仍占序号索引；累计 ACK 卡住时，它们可持续增长。即使 live map 已限额，受保护的 bootstrap 条目也可能把稀疏 slice 的头钉住。本次同时约束 active、live metadata 和 sparse index。
5. **建链必须保持严格模式。** 原接收端 pressure forgiveness 没有检查 steady-state 模式。本次明确只在完成模式切换后跨洞。

FEC20:20 的理想二项分布计算假设完整编码块、独立丢失、足够缓存和足够恢复时间；它不是突发丢包、部分块 flush、实现期限及重放窗口条件下的端到端完整率保证。不能把某次外层重传推断为“真实业务一定已经被 FEC 恢复”。

RTT/重传背景参考：[RFC 6298](https://www.rfc-editor.org/rfc/rfc6298.html)、[RFC 8985](https://www.rfc-editor.org/rfc/rfc8985.html)。本候选是有意的部分可靠 datagram carrier 策略，不声称完整实现标准 TCP/RACK-TLP，也不把放弃累计 ACK 洞描述成标准可靠 TCP。

## 三个可比较版本

| 分支 | 改动 | 对照目的 |
| --- | --- | --- |
| `agent/shadow-a-bounded` | 晚到首包修复 + 有限 token bucket + sender 资源有界 + bootstrap 禁止跨洞；接收端仍用固定 4096 压力阈值 | 验证基础隔离改动 |
| `agent/shadow-b-adaptive` | A + 动态接收压力阈值 + 1 SRTT 洞龄 + 3584 紧急阈值 | 首选动态候选，但必须与 A 对照 |
| `agent/shadow-c-conservative` | B，仅把普通压力洞龄改为 2 SRTT；3584 紧急阈值不变 | 观察低损/强乱序时是否需要更长修复机会 |

C 在高包率下可能与 B 接近，因为紧急阈值会先触发；它不是“任何情况下多等一个 RTT”。精确提交号由交付目录 `CANDIDATES.json` 记录。客户端和服务端必须使用同一个候选 SHA。

### A：额度和内存

- bucket 容量及初始额度 128 KiB；每 5 字节非 bootstrap fresh 得到 1 字节额度，保留除法余数，满桶丢弃新增额度，空闲不增发额度。
- 第 1/2/3/后续重传分别耗费 1/2/4/8 倍 payload 字节。bootstrap 不赚取也不消耗此额度。
- 对任意测量区间，虚拟 repair spend 至多为区间开始可用额度加新 fresh 额度，因此有 `spend <= 128 KiB + fresh/5`（包含分数额度的向下取整口径）。这是允许有限突发的约束，**不是每个 1 秒区间都 <=20%**。
- 计费发生在入队和重传调度处。`ShadowRetransmitBytes` 是调度字节，不是抓包测得的成功上线路字节；发送失败或排队损失要单独核对。byte 口径是 FakeTCP payload，不包括 IP/TCP header、ACK 或 bootstrap。
- active payload <=4096；live 序号 metadata <=8192；稀疏索引达到 16384 且足够稀疏时压缩。先释放旧 optional repair，fresh 继续进入。受保护 bootstrap 不退休。
- 极端超大 batch（单批 >4096）和非法的全 bootstrap 占满状态仍可能报容量错误；正常 post-bootstrap carrier 批次不属于这些情形。不能把它宣传成任意 API 输入必定成功。
- 截掉很老的 SACK tombstone 也会牺牲从该旧起点遍历 merged SACK 的能力，这是有限元数据的明确取舍；累计 ACK 推进后正常工作。

### B/C：接收端策略

- 100 ms 到达计数窗口，EWMA 更新权重 1/8；已识别的 duplicate 和 ACK-only 不计数。它测的是此前未见过的到达，无法无协议扩展地区分原始发送与第一次成功到达的重传。
- `soft = min(ceil(rate × SRTT + max(128, rate × SRTT × 10%)), 3584)`。
- 未取得 RTT 或速率样本时，保守使用 3584，不伪造一个很小的 BDP。
- 普通压力需同时达到 soft 和洞龄；推进到下一洞后重新计时，不能继承前一洞年龄。
- >=3584 取消洞龄条件，减少旧 repair debt；4096 sender hard limit 继续独立保护 fresh。
- 只在收到新的、可计入的记录时评估压力；空闲时不额外发定时跨洞 ACK。
- 已放弃的 ACK 洞不重建。未知晚到首包继续交给 carrier/DTLS；最近的精确 duplicate 仍抑制，超过有界历史的歧义交给上层重放机制。

## 交给测试负责人的测量要求

先复测原始 d3b 基线，再对照 eb3de 与 A/B/C，避免把晚到首包修复的收益算到动态窗口上。每次保存产品 SHA、辅助脚本 SHA、完整命令、client/server 版本、实际包长分布、两个方向 tc 计数和日志。

核心负载：单 lane、20 Mbps inner、每个方向单程 300 ms（约 600 ms RTT）、20% 独立随机 loss、FEC20:20、无带宽 cap。保持 bootstrap 的损伤施加时机一致。至少再覆盖：

- 1%/2% loss，以及 0% loss + 乱序，验证不会过早放弃正常修复；
- 512/1000/1360 B 内层包、发送速率上升/下降、空闲后突发；
- 单向大量数据 + 极少反向数据、ACK loss、burst loss；
- FEC off 对照，此时不能沿用 FEC20:20 的业务丢失目标；
- 序号回绕、lane 更换和 bootstrap 基本功能回归。

同时看完整率与截止时间完整率（例如 1 s/2 s）、goodput、p50/p95/p99/max echo RTT、FEC residual、DTLS/reassembly/Game 丢弃原因。理论 FEC 数字只作参考，不应凭空用它宣布绿灯。

发送端新增统计：`FreshAdmitted`、`FreshAdmittedBytes`、`FreshBlockedByRepair`、`RepairEvicted`、`RepairEvictedBytes`、`RepairMetadataEvicted`、`RepairCreditBytes`、`ShadowRetransmitBytes`。已有 `RepairBudgetSpent` 为加权虚拟支出。不存在凭空填写的 `FreshDroppedByRepair=0` 计数；无 shed 性质由 admission 返回值和 payload 回归直接证明，上线路交付仍需抓包。

B/C 接收端新增：`PressureSoftLimit`、`PressureRate`（records/s）、`PressureSRTTMillis`、`SoftForgivenGaps`、`EmergencyForgivenGaps`。结合已有 `LateBelowACK`、`BelowNextDrops`、`ForgivenGaps`、`PeakBufferedOO`。

客户端退出时输出 `WBD_FAKETCP_STATS` JSON；服务端每个 association 清理时输出 `WBD_FAKETCP_MUX_SHADOW_STATS` JSON。服务端新增前缀故意区别旧测试脚本的文本 stats，解析器要取正确行；强杀进程可能拿不到清理统计。不要再给此源码套改变 receiver 或 mux 算法的旧 patch 脚本。

已有 single-lane workflow 内硬编码 SOURCE_SHA=d3b，直接运行并不能测试这些候选。测试人员必须显式选择候选，并把 helper 和产品源码版本分别记录，不能只改产物标签。

## 本地验证边界

新增回归覆盖：连续两片大 fresh batch 满窗口、SACK tombstone 长期卡洞、bootstrap 钉住稀疏索引、低损历史不能无限攒 repair、分数额度和 bootstrap 排除、严格建链、20 Mbps/600 ms 的确定性到达序列、洞龄和换洞重置、冷启动紧急阈值、序号回绕、去重、空闲后速率恢复。

本地验证记录和各候选 SHA 随交付目录提供。Linux/Windows 的纯 Go 检查不能证明真实 Npcap、DTLS/FEC 弱网和物理链路指标。
