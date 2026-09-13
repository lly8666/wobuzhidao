# 综合审查与实验交接

## 当前判断

优先级：先查真实 LINK 尺寸错误与实验存活性，再验证分片损失放大。暂不修改 FEC wire format、窗口容量、horizon 或全局 MTU 默认值。

其他 agent 的分片机制是合理假说，但“唯一根因，证据闭合”的判断过强。最新样本混入了 LINK 退出；引用的 generation 数据也与对应 artifact 不一致。

## 重新读取的证据

来源：[run 34728895268](https://github.com/lly8666/wobuzhidao/actions/runs/34728895268)，artifact ID 10308477027。分析读取该 artifact 原始日志最后一次周期快照，没有把别的 run 混入。

| 最新快照字段 | link-1.log | link-server.log |
| --- | ---: | ---: |
| rx_horizon_settled_blocks | 2667 | 2731 |
| rx_horizon_under_required | 203 | 236 |
| 不足恢复数量比例 | 7.61% | 8.64% |
| rx_horizon_missing_required | 452 | 577 |
| 不足组平均缺口 | 2.23 | 2.44 |
| rx_horizon_no_final_metadata | 0 | 0 |
| reconstruct_success | 2524 | 2550 |

对话中 54.8%/57.1%、平均缺 8.39/8.78 的数字不能用于描述这份 artifact；来源需要另行核对。周期快照不是双方同步终态，不能用收发累计数差直接精确定位丢包。

最新 load-result：58317 发出，51411 返回，6906 丢失；包损失 11.842%，字节损失 19.159%。
但同时存在：

- link-1.log：`WBD_LINK_PROXY_FAIL fec: packet too large`。
- game-client.log：`lane_fail=1 dormant_drop=1854`，另有发送到 LINK 的 connection refused。
- host-pressure.json：客户端 UDP NoPorts 增量 1071；RcvbufErrors/InErrors 和 softnet dropped/time_squeeze 为 0。

因此这个结果包含链路失败影响，不能作为干净稳态曲线。1854 dormant drop 是具体证据，不应从 6906 中简单扣掉就宣称其余全是某种机制。
平均 CPU 约 2.15/4 核不能排除单线程瓶颈；零 RcvbufErrors 只排除已采样的该类错误，不等于排除了所有本地交付问题。

## 对分片假说的评价

实际代码中 FakeTCP 输入来自 DTLS，carrier MTU=1500 时 IPv4/TCP payload budget=1460；超过 1460 才产生多片，每片再扣 20 字节 fragmentation header。
FEC 1456 字节位于 DTLS 之前，必须计算或实测 DTLS 输出，不能直接与 1460 比较后宣布单片。也不能遗漏 DTLS 自己的 record/datagram 边界。

若某个完整恢复单元确实依赖两片独立、无重传的 carrier，其存活率是 (1-p)^2。29.29% 是该简化模型 40 shard 平均剩 20 的交点，不是有限长 RS 的硬阈值或根因证明。
本实验有混合包长、systematic 立即发送、parity 按组内最大长度填充、部分组、可选 repair 和双向 echo。不同类型 shard 的 loss unit 数可能不同。
最新端间粗略计数的 systematic 存活约 65.6%～67.5%，parity 约 60.8%～62.2%，并不等于所有 shard 都只剩 49%。这些未同步计数只能提示方向。

零 would-decode 计数支持“在该观察范围内，单纯多等不能修好这些组”，不能证明所有容量或状态生命周期问题都已永久排除。RS reconstruct 全成功也只说明被实际调用的那些重建成功。

## 这版代码

基于 d32992c902eeda03273671733a0509b211e5a987：

1. 保持错误类别和退出行为，补充 Encode/Decode 方向、实际长度、MTU、block/shard ID、声明长度和期待长度。原来的 `fec: packet too large` 同时代表超限和长度不匹配，仅凭旧日志不能确定原因。
2. CarrierFragmenter 增加成功处理的输入数据报、字节数、被拆分数据报和生成 frame 数；客户端及 server mux teardown 输出 `WBD_CARRIER_FRAGMENT_STATS`。只计碎片生成决策，不把它冒充实际 raw 发送数或 FEC symbol 数；控制/握手也包含在内。
3. 新增 artifact 审查脚本，检查 FAIL、lane_fail、dormant_drop、缺失分片计数，输出 `loss-unit-audit.json`。旧样本被正确标记为非干净稳态。
4. 单场景工作流固定旧 helper SHA cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d，产品使用执行提交的 exact SHA；保留 logic64+horizon64、32ms、5Mbps、45s、realistic mix、300ms/方向、FEC20:20。horizon/observer overlay 保存为 artifact，不能把仅 checkout SHA 称为完整构建内容。

本轮是产品诊断能力与实验有效性修正，还没有证实并修复造成尺寸错误的根因。拒绝直接吞掉错误、扩大上限或改变分片格式来掩盖它。

## 其他 agent 执行顺序

每次只跑一个 Action、一个 job、一个配置，上一条完成后再发下一条。不使用 matrix。

修改 `.github/loss-unit-case.json`，从本交接提交建 `run/fec-loss-unit-<case>-<id>` 分支，提交该文件再推送。工作流只在该分支前缀及该文件变更时触发；默认配置未变时可修改 JSON 格式产生变更。实验提交勿带 skip ci。若 workflow 已在默认分支注册，也可 dispatch 指定实验分支。

| 顺序 | loss_pct | carrier_mtu | 目的 |
| --- | ---: | ---: | --- |
| 1 | 30 | 1500 | 复现失败并获取尺寸上下文和真实分片量 |
| 2 | 30 | 1600 | 同业务尺寸、同算法，只改变实验 carrier 空间 |
| 3 | 20 | 1500 | 健康侧基线 |
| 4 | 20 | 1600 | 检查对照是否引入其他变化 |

1600 只用于同机 veth 因果实验，绝不宣称互联网路径支持它，也不作为产品修复发布。若仍有分片，这个控制组未成功隔离变量，应先核对实际尺寸，再调整实验。

第一条若报 Encode 超限，查上游 datagram 来源、完整 framing 和峰值尺寸；若报 Decode 声明/实际不一致，查传输边界、截断和 framing。不要因字符串中有“too large”就扩大 MTU。
四条要同时核对存活、实际分片比例、业务损失、各尺寸业务分布、FEC 不足组、资源错误；建议关键 30% 对照各另跑一次，避免 runner 差异。
若 1600 消除分片且稳定显著改善，才进入可兼容的 source 分块/重组设计，并建立经过真实 DTLS 的 encoded shard 单 carrier 回归。仅用假造固定 DTLS overhead 的单测不能替代该回归。
产品修复完成后才运行 constant20、constant30、5→30→5 的三条独立验收；暂不沿用旧样本承诺“数量级改善”。

## 验证状态

本地 Linux Go 1.23.12：internal/faketcp、internal/linkdata、internal/fec、cmd/wbd-faketcp、cmd/wbd-faketcp-mux 通过；另外在副本应用原 helper 的 horizon64、origin、shard-flow overlay 后，FEC/LINK 测试通过。
不启动 Actions 或远程负载，由其他 agent 执行。发布框架的提交带 `[skip ci]`，避免意外启动仓库普通 push CI。
